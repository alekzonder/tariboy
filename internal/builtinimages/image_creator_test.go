package builtinimages

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

// Catch missing capabilities, unpackaged entrypoints and broken sibling paths.
// The default is offline; the full-source gate requires restored upstream skills.
func TestImageCreatorBuild(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	source := filepath.Join(root, "store", "images", "tariboy-image-creator")
	parsed, err := imagefile.ParseV2(source)
	if err != nil {
		t.Fatal(err)
	}
	lockBytes, err := os.ReadFile(filepath.Join(source, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Skills map[string]struct{ Source, SourceType, SkillPath, ComputedHash string }
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatal(err)
	}
	full := os.Getenv("TARIBOY_TEST_IMAGE_CREATOR_FULL") == "1"
	selected := parsed.Skills[:0]
	for _, skill := range parsed.Skills {
		if strings.HasPrefix(skill.Dir, "./.agents/skills/") {
			name := filepath.Base(skill.Dir)
			entry, ok := lock.Skills[name]
			if !ok || entry.Source != "obra/superpowers" || entry.SourceType != "github" || entry.SkillPath != "skills/"+name+"/SKILL.md" || len(entry.ComputedHash) != 64 {
				t.Fatalf("upstream skill %q lacks restorable lock entry: %+v", name, entry)
			}
			if !full {
				continue
			}
		}
		selected = append(selected, skill)
	}
	parsed.Skills = selected
	assets := filepath.Join(root, "store")
	store := &image.Store{Dir: t.TempDir()}
	ref := image.Ref{Name: "tariboy-image-creator", Tag: "0.1.0"}
	if _, err := image.BuildV2(parsed, imagefile.ResolveRoots{CurrentVersionStore: assets, Store: assets, CurrentStoreVersion: "test"}, ref, store, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	manifest, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	plugins := map[string]bool{}
	for _, plugin := range manifest.Plugins {
		plugins[plugin.Name] = true
	}
	for _, name := range []string{"whoami", "loop", "messages", "context", "status", "workdir", "scripts", "goal", "tasks", "image-creator"} {
		if !plugins[name] {
			t.Errorf("missing capability %q", name)
		}
	}
	skills := map[string]bool{}
	for _, skill := range manifest.Skills {
		skills[skill.Name] = true
	}
	required := []string{"tariboy-image-authoring", "tariboy-image-evals", "tariboy-image-delivery", "github-pr-workflow", "tasks", "scripts", "image-creator", "whoami", "loop", "messages", "context", "status", "workdir", "goal"}
	if full {
		required = append(required, "writing-skills", "using-superpowers", "brainstorming", "using-git-worktrees", "verification-before-completion")
	}
	for _, name := range required {
		if !skills[name] {
			t.Errorf("missing packaged skill %q", name)
		}
	}
	template, err := store.ReadTemplate(ref)
	if err != nil {
		t.Fatal(err)
	}
	runtimes := map[string]bool{}
	for _, entry := range template.Entries {
		if entry.Kind == "runtime" {
			runtimes[entry.Runtime] = true
		}
	}
	for _, name := range []string{"identity", "one-shot", "messages", "goal", "context", "workdir", "user-prompt"} {
		if !runtimes[name] {
			t.Errorf("missing runtime %q", name)
		}
	}
	if len(template.Entries) == 0 || template.Entries[len(template.Entries)-1].Source != "$CURRENT_VERSION_STORE/prompts/iteration-finish.md" {
		t.Fatal("missing final iteration finish asset")
	}
	archive, err := os.Open(filepath.Join(store.Dir, ref.Name, ref.Tag+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	gz, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatal("PR monitor utility absent from runnable archive")
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "skills/github-pr-workflow/scripts/github-pr.py" {
			if header.Mode&0o100 == 0 || header.Size == 0 {
				t.Fatalf("PR monitor utility is not runnable: %+v", header)
			}
			break
		}
	}
}
