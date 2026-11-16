package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/source"
)

func localResolver(t *testing.T) {
	t.Helper()
	prev := newResolver
	newResolver = func(home paths.Home) *source.Resolver {
		return &source.Resolver{Home: home, Git: gitx.Git{AllowLocal: true}}
	}
	t.Cleanup(func() { newResolver = prev })
}

func dirStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills/a/SKILL.md"), "---\nname: a\ndescription: Skill a\n---\nbody\n")
	writeFile(t, filepath.Join(dir, "prompts/p.md"), "prompt p\n")
	return dir
}

func TestImageNewBuildsFromDirStore(t *testing.T) {
	localRegistry(t)
	home, store := t.TempDir(), dirStore(t)
	if code, _, errs := runIn(t, home, "/", "store", "add", "s", store); code != 0 {
		t.Fatalf("store add: %d %s", code, errs)
	}
	code, out, errs := runIn(t, home, "/", "image", "new", "demo", "--skill", "s/a", "--prompt", "s/p")
	if code != 0 || errs != "" {
		t.Fatalf("new: %d %q %q", code, out, errs)
	}
	src := filepath.Join(home, "sources", "demo")
	if !strings.Contains(out, "created "+src+"\n") || !strings.Contains(out, "built demo:0.1.0 ") {
		t.Fatalf("output %q", out)
	}
	code, out, _ = runIn(t, home, "/", "image", "show", "demo")
	if code != 0 || !strings.Contains(out, "a: Skill a") {
		t.Fatalf("show %d %q", code, out)
	}
	if code, out, _ = runIn(t, home, "/", "image", "new", "demo", "--skill", "s/a"); code != 1 || out != "" {
		t.Fatalf("existing source: %d %q", code, out)
	}
	if code, out, errs = runIn(t, home, "/", "image", "new", "demo", "--skill", "s/a", "--force", "--version", "0.2.0"); code != 0 || !strings.Contains(out, "built demo:0.2.0 ") {
		t.Fatalf("force: %d %q %q", code, out, errs)
	}
}

func TestImageNewNoBuild(t *testing.T) {
	localRegistry(t)
	home, store := t.TempDir(), dirStore(t)
	runIn(t, home, "/", "store", "add", "s", store)
	code, out, errs := runIn(t, home, "/", "image", "new", "demo", "--skill", "s/a", "--no-build")
	if code != 0 || errs != "" || strings.Contains(out, "built") || !strings.HasPrefix(out, "created ") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(home, "sources", "demo", "Tariboyfile.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := runIn(t, home, "/", "images"); !strings.Contains(out, "no images") {
		t.Fatalf("image built: %q", out)
	}
}

func TestImageNewSourceOutlivesGitStore(t *testing.T) {
	localRegistry(t)
	localResolver(t)
	repo := storeRepo(t)
	home := t.TempDir()
	if code, _, errs := runIn(t, home, "/", "store", "add", "g", "file://"+repo); code != 0 {
		t.Fatalf("store add: %s", errs)
	}
	code, out, errs := runIn(t, home, "/", "image", "new", "demo", "--skill", "g/hello", "--prompt", "g/p")
	if code != 0 {
		t.Fatalf("new: %d %q %q", code, out, errs)
	}
	if code, _, errs := runIn(t, home, "/", "store", "remove", "g"); code != 0 {
		t.Fatalf("remove: %s", errs)
	}
	code, out, errs = runIn(t, home, "/", "build", "--path", filepath.Join(home, "sources", "demo"))
	if code != 0 || !strings.HasPrefix(out, "up to date demo ") {
		t.Fatalf("rebuild: %d %q %q", code, out, errs)
	}
}

func TestImageNewUsage(t *testing.T) {
	localRegistry(t)
	home := t.TempDir()
	for _, args := range [][]string{
		{"image", "new"},
		{"image", "new", "a", "b"},
		{"image", "new", "demo", "--skill", "nostore"},
	} {
		code, out, errs := runIn(t, home, "/", args...)
		if code != 2 || out != "" || errs == "" {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	if code, _, errs := runIn(t, home, "/", "image", "new", "demo", "--skill", "s/zzz"); code != 1 || !strings.Contains(errs, "store not found") {
		t.Errorf("unknown store: %d %q", code, errs)
	}
}
