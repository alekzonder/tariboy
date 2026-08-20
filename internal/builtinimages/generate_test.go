package builtinimages

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
)

func TestGenerateBuildsCanonicalBasicBundle(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	out := filepath.Join(t.TempDir(), "generated")
	if err := Generate(filepath.Join(root, "internal", "builtinimages", "source"), out, "9.8.7"); err != nil {
		t.Fatal(err)
	}

	versionBytes, err := os.ReadFile(filepath.Join(out, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(versionBytes)); got != "9.8.7" {
		t.Fatalf("bundle version = %q, want 9.8.7", got)
	}

	storeDir := t.TempDir()
	refDir := filepath.Join(storeDir, "basic")
	if err := os.MkdirAll(refDir, 0o700); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(out, "basic.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refDir, "latest.tar.gz"), archive, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &image.Store{Dir: storeDir}
	manifest, err := store.Inspect(image.Ref{Name: "basic", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	plugins := map[string]bool{}
	for _, plugin := range manifest.Plugins {
		plugins[plugin.Name] = true
	}
	for _, required := range []string{"tasks", "current-task", "workdir"} {
		if !plugins[required] {
			t.Errorf("basic image missing %q: %#v", required, manifest.Plugins)
		}
	}
	for _, excluded := range []string{"messenger-provider", "issue-provider", "review-provider"} {
		if plugins[excluded] {
			t.Errorf("basic image unexpectedly includes %q", excluded)
		}
	}
	prompt, err := store.RenderPrompt(image.Ref{Name: "basic", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "tasks ready --claim") {
		t.Fatalf("basic prompt missing native Tasks guidance:\n%s", prompt)
	}
	template, err := store.ReadTemplate(image.Ref{Name: "basic", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	foundWorkdirSequence := false
	for i := 0; i+2 < len(template.Entries); i++ {
		if template.Entries[i].Source == "$CURRENT_VERSION_STORE/skills/workdir/prompt.md" &&
			template.Entries[i+1].Kind == "runtime" && template.Entries[i+1].Runtime == "workdir" &&
			template.Entries[i+2].Source == "$CURRENT_VERSION_STORE/skills/scripts/prompt.md" {
			foundWorkdirSequence = true
			break
		}
	}
	if !foundWorkdirSequence {
		t.Fatalf("basic template missing workdir static/runtime entries immediately before scripts: %#v", template.Entries)
	}
}
