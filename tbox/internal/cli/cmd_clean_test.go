package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cleanCmd(t *testing.T, home, codexHome string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	vars := map[string]string{"TBOX_HOME": home, "CODEX_HOME": codexHome}
	env := Env{
		Args:   append([]string{"clean"}, args...),
		Getenv: func(k string) string { return vars[k] },
		Stdout: &out,
		Stderr: &errb,
		Cwd:    "/",
	}
	code := Main(env)
	return code, out.String(), errb.String()
}

func runManifest(t *testing.T, home, digest, name string) string {
	t.Helper()
	dir := filepath.Join(home, "run", digest)
	writeFile(t, filepath.Join(dir, "codex", "image", "manifest.json"), `{"schema_version":2,"name":"`+name+`","skills":[]}`)
	return dir
}

func TestCleanOneImage(t *testing.T) {
	home, codexHome := t.TempDir(), t.TempDir()
	mine := runManifest(t, home, "d1", "img")
	theirs := runManifest(t, home, "d2", "other")
	writeFile(t, filepath.Join(codexHome, "config.toml"), "a = 1\n")
	writeFile(t, filepath.Join(codexHome, "tbox-img.config.toml"), "# Written by tbox for image img.\n")
	writeFile(t, filepath.Join(codexHome, "tbox-other.config.toml"), "# Written by tbox for image other.\n")
	writeFile(t, filepath.Join(codexHome, "plugins", "cache", "tbox", "img", ".tbox-digest"), "{}")
	code, out, errs := cleanCmd(t, home, codexHome, "img")
	if code != 0 || errs != "" || !strings.Contains(out, "img") {
		t.Fatalf("clean %d %q %q", code, out, errs)
	}
	for _, gone := range []string{mine, filepath.Join(codexHome, "tbox-img.config.toml"), filepath.Join(codexHome, "plugins", "cache", "tbox", "img")} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s kept: %v", gone, err)
		}
	}
	for _, kept := range []string{theirs, filepath.Join(codexHome, "tbox-other.config.toml"), filepath.Join(codexHome, "config.toml")} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s removed: %v", kept, err)
		}
	}
}

func TestCleanEverything(t *testing.T) {
	home, codexHome := t.TempDir(), t.TempDir()
	runManifest(t, home, "d1", "img")
	writeFile(t, filepath.Join(codexHome, "config.toml"), "a = 1\n")
	writeFile(t, filepath.Join(codexHome, "work.config.toml"), "b = 2\n")
	writeFile(t, filepath.Join(codexHome, "tbox-img.config.toml"), "# Written by tbox for image img.\n")
	code, _, errs := cleanCmd(t, home, codexHome)
	if code != 0 || errs != "" {
		t.Fatalf("clean %d %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(home, "run")); !os.IsNotExist(err) {
		t.Fatalf("run kept: %v", err)
	}
	entries, _ := os.ReadDir(codexHome)
	if len(entries) != 2 {
		t.Fatalf("codex home %v", entries)
	}
}

func TestCleanUsage(t *testing.T) {
	home := t.TempDir()
	if code, _, _ := cleanCmd(t, home, t.TempDir(), "a", "b"); code != 2 {
		t.Fatalf("two args code %d", code)
	}
	if code, _, errs := cleanCmd(t, home, t.TempDir(), "Bad/Name"); code != 1 || errs == "" {
		t.Fatalf("bad name %d %q", code, errs)
	}
}

func TestCleanPrintsNotesForKeptFiles(t *testing.T) {
	home, codexHome := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(codexHome, "tbox-img.config.toml"), "mine = 1\n")
	writeFile(t, filepath.Join(home, "run", "d9", "codex", "junk"), "x")
	code, _, errs := cleanCmd(t, home, codexHome, "img")
	if code != 0 || !strings.Contains(errs, filepath.Join(codexHome, "tbox-img.config.toml")) || !strings.Contains(errs, filepath.Join(home, "run", "d9")) {
		t.Fatalf("%d %q", code, errs)
	}
}

func TestProbeOutputHasTimeout(t *testing.T) {
	prev := probeTimeout
	probeTimeout = 100 * time.Millisecond
	t.Cleanup(func() { probeTimeout = prev })
	host := harnessHost(&Env{Environ: func() []string { return []string{"PATH=/usr/bin:/bin"} }})
	start := time.Now()
	if _, err := host.Output(context.Background(), nil, "sleep", "5"); err == nil {
		t.Fatal("no error from a probe that outlives the timeout")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("probe took %v", d)
	}
}

// buildTwice builds the demo source, changes it and builds it again, so the
// first archive is left without a tag. It returns the image directory.
func buildTwice(t *testing.T, home string) string {
	t.Helper()
	src := demoSource(t)
	if code, _, errs := runIn(t, home, "/", "build", "--path", src); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	writeFile(t, filepath.Join(src, "p.md"), "changed prompt\n")
	if code, _, errs := runIn(t, home, "/", "build", "--path", src); code != 0 {
		t.Fatalf("rebuild: %s", errs)
	}
	dir := filepath.Join(home, "images", "demo")
	if refs, _ := os.ReadDir(filepath.Join(dir, "refs")); len(refs) != 2 {
		t.Fatalf("refs %v", refs)
	}
	return dir
}

func TestCleanRemovesUntaggedArchives(t *testing.T) {
	for _, args := range [][]string{{"demo"}, nil} {
		home := t.TempDir()
		dir := buildTwice(t, home)
		code, out, errs := cleanCmd(t, home, t.TempDir(), args...)
		if code != 0 || errs != "" || !strings.Contains(out, "removed 1 untagged image archive") {
			t.Fatalf("clean %v: %d %q %q", args, code, out, errs)
		}
		if refs, _ := os.ReadDir(filepath.Join(dir, "refs")); len(refs) != 1 {
			t.Fatalf("clean %v: refs %v", args, refs)
		}
		if code, _, errs := runIn(t, home, "/", "image", "show", "demo:latest"); code != 0 {
			t.Fatalf("clean %v: tagged image lost: %s", args, errs)
		}
	}
}
