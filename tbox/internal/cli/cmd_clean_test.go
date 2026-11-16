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

// legacyImage builds demo and renames its image directory to name, the way an
// image built before the image-name rule was tightened sits on disk.
func legacyImage(t *testing.T, home, name string) {
	t.Helper()
	if code, _, errs := runIn(t, home, "/", "build", "--path", demoSource(t)); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	if err := os.Rename(filepath.Join(home, "images", "demo"), filepath.Join(home, "images", name)); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyImageNamesCanBeListedAndRemoved(t *testing.T) {
	home := t.TempDir()
	legacyImage(t, home, "_foo")
	if code, out, errs := runIn(t, home, "/", "images"); code != 0 || !strings.Contains(out, "_foo") {
		t.Fatalf("images: %d %q %q", code, out, errs)
	}
	if code, out, errs := runIn(t, home, "/", "image", "show", "_foo"); code != 0 || !strings.Contains(out, "ref: _foo:latest") {
		t.Fatalf("show: %d %q %q", code, out, errs)
	}
	if code, _, errs := cleanCmd(t, home, t.TempDir(), "_foo"); code != 0 {
		t.Fatalf("clean _foo: %d %q", code, errs)
	}
	for _, tag := range []string{"1.0.0", "latest"} {
		if code, _, errs := runIn(t, home, "/", "image", "rm", "_foo:"+tag); code != 0 {
			t.Fatalf("rm _foo:%s: %d %q", tag, code, errs)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "images", "_foo")); !os.IsNotExist(err) {
		t.Fatalf("image directory left: %v", err)
	}

	legacyImage(t, home, "-x")
	if code, _, errs := cleanCmd(t, home, t.TempDir(), "-x"); code != 2 || !strings.Contains(errs, cleanUsage) {
		t.Fatalf("clean -x: %d %q", code, errs)
	}
	for _, tag := range []string{"1.0.0", "latest"} {
		if code, _, errs := runIn(t, home, "/", "image", "rm", "--", "-x:"+tag); code != 0 {
			t.Fatalf("rm -- -x:%s: %d %q", tag, code, errs)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "images", "-x")); !os.IsNotExist(err) {
		t.Fatalf("image directory left: %v", err)
	}
	if code, _, errs := runIn(t, home, "/", "build", "--path", demoSource(t), "--name", "_new"); code != 1 || errs == "" {
		t.Fatalf("build with a legacy name: %d %q", code, errs)
	}
}

func TestDashDashEndsOptionsForImageCommands(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	if code, _, errs := runIn(t, home, "/", "build", "--path", demoSource(t)); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	if code, out, errs := runIn(t, home, "/", "image", "show", "--", "demo"); code != 0 || !strings.Contains(out, "ref: demo:latest") {
		t.Fatalf("show --: %d %q %q", code, out, errs)
	}
	// After -- an option is a positional argument: a second REF or FILE.
	if code, _, errs := runIn(t, home, work, "image", "export", "--", "demo", "-o", "x.tar.gz"); code != 2 {
		t.Fatalf("export -- REF -o: %d %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(work, "x.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("export wrote after --: %v", err)
	}
	if code, _, errs := runIn(t, home, work, "image", "export", "-o", "x.tar.gz", "--", "demo"); code != 0 {
		t.Fatalf("export -o FILE -- REF: %d %q", code, errs)
	}
	if code, _, errs := runIn(t, t.TempDir(), work, "image", "import", "--", "x.tar.gz", "--force"); code != 2 {
		t.Fatalf("import -- FILE --force: %d %q", code, errs)
	}
	if code, _, errs := runIn(t, t.TempDir(), work, "image", "import", "--force", "--", "x.tar.gz"); code != 0 {
		t.Fatalf("import --force -- FILE: %d %q", code, errs)
	}
	if code, out, errs := cleanCmd(t, home, t.TempDir(), "--", "demo"); code != 0 || !strings.Contains(out, "of demo") {
		t.Fatalf("clean -- demo: %d %q %q", code, out, errs)
	}
	if code, _, errs := runIn(t, home, "/", "image", "rm", "--", "demo:1.0.0"); code != 0 {
		t.Fatalf("rm --: %d %q", code, errs)
	}
}
