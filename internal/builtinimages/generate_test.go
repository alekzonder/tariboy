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
	skills := map[string]bool{}
	for _, skill := range manifest.Skills {
		skills[skill.Name] = true
	}
	for _, required := range []string{"messages", "scripts", "tasks"} {
		if !skills[required] {
			t.Errorf("basic image missing packaged skill %q: %#v", required, manifest.Skills)
		}
	}
	for _, required := range []string{"whoami", "loop", "messages", "context", "status", "workdir", "scripts", "current-task", "tasks"} {
		if !skills[required] {
			t.Errorf("basic image capability %q is not packaged as a skill: %#v", required, manifest.Skills)
		}
	}
	template, err := store.ReadTemplate(image.Ref{Name: "basic", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	runtimes := map[string]bool{}
	for _, entry := range template.Entries {
		if entry.Kind == "runtime" {
			runtimes[entry.Runtime] = true
		}
		if strings.HasSuffix(entry.Source, "/prompt.md") {
			t.Errorf("basic template retains migrated prompt fragment %q", entry.Source)
		}
	}
	for _, required := range []string{"identity", "messages", "context", "workdir", "user-prompt", "one-shot"} {
		if !runtimes[required] {
			t.Errorf("basic template missing runtime %q: %#v", required, template.Entries)
		}
	}
	if got := template.Entries[len(template.Entries)-1].Source; got != "$CURRENT_VERSION_STORE/prompts/iteration-finish.md" {
		t.Fatalf("basic final prompt = %q, want mandatory iteration finish prompt", got)
	}
}

func TestDeveloperInstructionsDistinguishQuestionForms(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	instructions, err := os.ReadFile(filepath.Join(root, "store", "images", "tariboy-developer", "instructions.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Join(strings.Fields(string(instructions)), " ")
	for _, want := range []string{
		"The flexible and workflow forms are mutually exclusive.",
		"For a flexible task without a work packet, ask with `tasks ask <TASK-KEY> user:<login>|agent:<name> <TEXT>`.",
		"requires neither an assignment ID nor revisions",
		"A plain comment is not a substitute",
		"For a workflow-managed task with a work packet, use only its assignment-scoped `tasks ask` form",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("developer instructions missing %q:\n%s", want, body)
		}
	}
}
