package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/plugincaps"
)

func TestBuildResolvesInstalledExternalPlugin(t *testing.T) {
	resolver := func(name string) (plugincaps.ResolvedPlugin, error) {
		if name == "issue-provider" {
			return plugincaps.ResolvedPlugin{Installed: true, HasPrompt: true, Prompt: "## External issue guidance"}, nil
		}
		return plugincaps.ResolvedPlugin{}, nil
	}
	st := &Store{Dir: t.TempDir()}
	imageSpec := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "issue-provider"}},
		Dir:           t.TempDir(),
	}
	ref := Ref{Name: "external", Tag: "latest"}
	manifest, err := Build(imageSpec, ref, st, fixedClock(), WithExternalPlugins(resolver))
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Plugins[len(manifest.Plugins)-1].Name; got != "issue-provider" {
		t.Fatalf("last plugin = %q", got)
	}
	if prompt := readMember(t, st, ref, "PROMPT.md"); !strings.Contains(prompt, "## External issue guidance") {
		t.Fatalf("external prompt missing:\n%s", prompt)
	}

	missing := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "missing-provider"}},
		Dir:           t.TempDir(),
	}
	_, err = Build(missing, Ref{Name: "missing", Tag: "latest"}, st, fixedClock(), WithExternalPlugins(resolver))
	if err == nil || !strings.Contains(err.Error(), "unknown plugin") {
		t.Fatalf("missing external plugin error = %v", err)
	}
}

func fixedClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC) }
}

func promptFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildSingleImage(t *testing.T) {
	src := t.TempDir()
	task := promptFile(t, src, "task.md", "DO THE TASK")
	st := &Store{Dir: t.TempDir()}
	im := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "context"}},
		Harness:       imagefile.Harness{Type: "claude"},
		Prompts:       []imagefile.Prompt{{Filepath: task}},
		Dir:           src,
	}
	man, err := Build(im, Ref{Name: "app", Tag: "latest"}, st, fixedClock())
	if err != nil {
		t.Fatal(err)
	}
	if man.BuiltAt != "2026-07-05T12:00:00Z" || man.Digest == "" {
		t.Fatalf("manifest header: %+v", man)
	}
	// full resolved set present, CORE first, then context
	var names []string
	for _, p := range man.Plugins {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "whoami,loop,messages,context" {
		t.Fatalf("plugins = %v", names)
	}
	if !st.Exists(Ref{Name: "app", Tag: "latest"}) {
		t.Fatal("archive not written")
	}
	got, err := st.Inspect(Ref{Name: "app", Tag: "latest"})
	if err != nil || got.Digest != man.Digest {
		t.Fatalf("inspect digest = %q want %q err=%v", got.Digest, man.Digest, err)
	}
	prompt, tail := readMember(t, st, Ref{Name: "app", Tag: "latest"}, "PROMPT.md"),
		readMember(t, st, Ref{Name: "app", Tag: "latest"}, "PROMPT_TAIL.md")
	for _, want := range []string{"# Who you are", "## Context", "DO THE TASK"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("PROMPT.md missing %q", want)
		}
	}
	if !strings.Contains(tail, "i-am-done") {
		t.Fatalf("tail missing i-am-done: %q", tail)
	}
	if strings.Contains(prompt, "i-am-done") {
		t.Fatal("tail leaked into PROMPT.md (must be separate)")
	}
}

func TestBuildFromChain(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	baseSrc := t.TempDir()
	base := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "context"}},
		Harness:       imagefile.Harness{Type: "claude"},
		Prompts:       []imagefile.Prompt{{Filepath: promptFile(t, baseSrc, "base.md", "BASE BODY")}},
		Dir:           baseSrc,
	}
	if _, err := Build(base, Ref{Name: "base", Tag: "latest"}, st, fixedClock()); err != nil {
		t.Fatal(err)
	}
	childSrc := t.TempDir()
	child := &imagefile.Imagefile{
		SchemaVersion: 1,
		From:          "base:latest",
		Plugins:       []imagefile.Plugin{{Name: "status"}},
		Prompts:       []imagefile.Prompt{{Filepath: promptFile(t, childSrc, "child.md", "CHILD BODY")}},
		Dir:           childSrc,
	}
	man, err := Build(child, Ref{Name: "child", Tag: "latest"}, st, fixedClock())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range man.Plugins {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "whoami,loop,messages,context,status" {
		t.Fatalf("plugins = %v", names)
	}
	if want := []string{"base:latest"}; len(man.Parents) != 1 || man.Parents[0] != want[0] {
		t.Fatalf("parents = %v", man.Parents)
	}
	prompt := readMember(t, st, Ref{Name: "child", Tag: "latest"}, "PROMPT.md")
	// SYSTEM recomputed for the full set, not duplicated
	if n := strings.Count(prompt, "## Context"); n != 1 {
		t.Fatalf("## Context appears %d times, want 1", n)
	}
	if !strings.Contains(prompt, "## Status") {
		t.Fatal("status system fragment missing")
	}
	// body inherited from base and extended
	for _, want := range []string{"BASE BODY", "CHILD BODY"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("PROMPT.md missing %q", want)
		}
	}
}

func TestBuildHarnessInheritsInteractiveWhenChildOmitsHarness(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	base := &imagefile.Imagefile{
		SchemaVersion: 1,
		Harness: imagefile.Harness{
			Type:           "codex",
			Model:          "gpt-5",
			Effort:         "high",
			Interactive:    true,
			InteractiveSet: true,
		},
		Dir: t.TempDir(),
	}
	if _, err := Build(base, Ref{Name: "base", Tag: "latest"}, st, fixedClock()); err != nil {
		t.Fatal(err)
	}

	child := &imagefile.Imagefile{
		SchemaVersion: 1,
		From:          "base:latest",
		Dir:           t.TempDir(),
	}
	man, err := Build(child, Ref{Name: "child", Tag: "latest"}, st, fixedClock())
	if err != nil {
		t.Fatal(err)
	}
	if man.Harness.Type != "codex" || man.Harness.Model != "gpt-5" ||
		man.Harness.Effort != "high" || !man.Harness.Interactive {
		t.Fatalf("inherited harness = %+v", man.Harness)
	}
}

func TestBuildHarnessExplicitFalseOverridesInteractiveParent(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	base := &imagefile.Imagefile{
		SchemaVersion: 1,
		Harness: imagefile.Harness{
			Type:           "codex",
			Interactive:    true,
			InteractiveSet: true,
		},
		Dir: t.TempDir(),
	}
	if _, err := Build(base, Ref{Name: "base", Tag: "latest"}, st, fixedClock()); err != nil {
		t.Fatal(err)
	}

	child := &imagefile.Imagefile{
		SchemaVersion: 1,
		From:          "base:latest",
		Harness: imagefile.Harness{
			Type:           "claude",
			Model:          "sonnet",
			Effort:         "medium",
			Interactive:    false,
			InteractiveSet: true,
		},
		Dir: t.TempDir(),
	}
	man, err := Build(child, Ref{Name: "child", Tag: "latest"}, st, fixedClock())
	if err != nil {
		t.Fatal(err)
	}
	if man.Harness.Type != "claude" || man.Harness.Model != "sonnet" ||
		man.Harness.Effort != "medium" || man.Harness.Interactive {
		t.Fatalf("overridden harness = %+v", man.Harness)
	}
}

func TestBuildHarnessDefaultsToClaudeNonInteractive(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	man, err := Build(&imagefile.Imagefile{SchemaVersion: 1, Dir: t.TempDir()},
		Ref{Name: "plain", Tag: "latest"}, st, fixedClock())
	if err != nil {
		t.Fatal(err)
	}
	if man.Harness.Type != "claude" || man.Harness.Interactive {
		t.Fatalf("default harness = %+v", man.Harness)
	}
}

func TestBuildOverride(t *testing.T) {
	src := t.TempDir()
	st := &Store{Dir: t.TempDir()}
	over := promptFile(t, src, "ctx.md", "CUSTOM CONTEXT FRAGMENT")
	im := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "context"}},
		Prompts:       []imagefile.Prompt{{Name: "system:context", Filepath: over}},
		Dir:           src,
	}
	if _, err := Build(im, Ref{Name: "o", Tag: "latest"}, st, fixedClock()); err != nil {
		t.Fatal(err)
	}
	prompt := readMember(t, st, Ref{Name: "o", Tag: "latest"}, "PROMPT.md")
	if !strings.Contains(prompt, "CUSTOM CONTEXT FRAGMENT") || strings.Contains(prompt, "durable working memory") {
		t.Fatalf("override not applied: %q", prompt)
	}
}

func TestBuildErrors(t *testing.T) {
	src := t.TempDir()
	st := &Store{Dir: t.TempDir()}
	// unknown plugin
	if _, err := Build(&imagefile.Imagefile{SchemaVersion: 1, Plugins: []imagefile.Plugin{{Name: "nope"}}, Dir: src},
		Ref{Name: "a", Tag: "latest"}, st, fixedClock()); err == nil {
		t.Fatal("unknown plugin accepted")
	}
	// missing parent
	if _, err := Build(&imagefile.Imagefile{SchemaVersion: 1, From: "ghost:latest", Dir: src},
		Ref{Name: "b", Tag: "latest"}, st, fixedClock()); err == nil || !strings.Contains(err.Error(), "not built") {
		t.Fatalf("missing parent err = %v", err)
	}
	// override for a plugin not in the set
	bad := promptFile(t, src, "x.md", "x")
	if _, err := Build(&imagefile.Imagefile{SchemaVersion: 1,
		Prompts: []imagefile.Prompt{{Name: "system:status", Filepath: bad}}, Dir: src},
		Ref{Name: "c", Tag: "latest"}, st, fixedClock()); err == nil {
		t.Fatal("override for absent plugin accepted")
	}
}

func TestBuildMutableRetainsPinnedGeneration(t *testing.T) {
	source := t.TempDir()
	prompt := promptFile(t, source, "prompt.md", "first generation")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "reviewer", Tag: "latest"}
	imagefile := &imagefile.Imagefile{SchemaVersion: 1, Dir: source, Prompts: []imagefile.Prompt{{Filepath: prompt}}}

	first, err := Build(imagefile, ref, store, fixedClock(), WithMutableRef())
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsMutable(ref) {
		t.Fatal("mutable build did not mark ref")
	}
	if err := os.WriteFile(prompt, []byte("second generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := Build(imagefile, ref, store, fixedClock(), WithMutableRef())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatal("mutable rebuild did not change digest")
	}
	if pinned, err := store.InspectPinned(ref, first.Digest); err != nil || pinned.Digest != first.Digest {
		t.Fatalf("inspect pinned generation = %#v, %v", pinned, err)
	}
	dest := t.TempDir()
	if err := store.UnpackPinned(ref, first.Digest, dest); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(dest, "BODY.md")); err != nil || string(body) != "first generation" {
		t.Fatalf("unpacked pinned body = %q, %v", body, err)
	}
}

// readMember is a test helper reading one file out of an image tarball.
func readMember(t *testing.T, s *Store, ref Ref, name string) string {
	t.Helper()
	b, err := readFileFromTar(s.tarPath(ref), name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}
