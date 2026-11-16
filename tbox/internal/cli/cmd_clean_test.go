package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	writeFile(t, filepath.Join(codexHome, "tbox-img.config.toml"), "x")
	writeFile(t, filepath.Join(codexHome, "tbox-other.config.toml"), "x")
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
	writeFile(t, filepath.Join(codexHome, "tbox-img.config.toml"), "x")
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
