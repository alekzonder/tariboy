package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runIn(t *testing.T, home, cwd string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	env := Env{
		Args:   args,
		Getenv: func(k string) string { return map[string]string{"TBOX_HOME": home}[k] },
		Stdout: &out,
		Stderr: &errb,
		Cwd:    cwd,
		Now:    func() time.Time { return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC) },
	}
	code := Main(env)
	return code, out.String(), errb.String()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func demoSource(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "demo")
	writeFile(t, filepath.Join(dir, "Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.0.0\nskills:\n  - dir: ./skills/hello\nprompts:\n  - file: ./p.md\n")
	writeFile(t, filepath.Join(dir, "skills/hello/SKILL.md"), "---\nname: hello\ndescription: Says hello\n---\nbody\n")
	writeFile(t, filepath.Join(dir, "p.md"), "prompt\n")
	old := time.Now().Add(-time.Hour)
	_ = filepath.WalkDir(dir, func(p string, _ os.DirEntry, _ error) error { return os.Chtimes(p, old, old) })
	return dir
}

func TestBuildImagesShowRm(t *testing.T) {
	home, src := t.TempDir(), demoSource(t)

	code, out, errs := runIn(t, home, "/", "build", "--path", src)
	if code != 0 || errs != "" {
		t.Fatalf("code %d %q", code, errs)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "built demo:1.0.0 ") || !strings.HasPrefix(lines[1], "built demo:latest ") {
		t.Fatalf("build output %q", out)
	}
	digest := strings.Fields(lines[0])[2]

	code, out, _ = runIn(t, home, src, "build")
	if code != 0 || out != "up to date demo "+digest+"\n" {
		t.Fatalf("second build %d %q", code, out)
	}

	writeFile(t, filepath.Join(src, "skills/hello/SKILL.md"), "---\nname: hello\ndescription: Says hello\n---\nchanged\n")
	code, out, _ = runIn(t, home, "/", "build", "--path", src)
	if code != 0 || !strings.Contains(out, "moved latest to new content\n") || !strings.Contains(out, "moved 1.0.0 to new content\n") {
		t.Fatalf("rebuild %d %q", code, out)
	}

	code, out, _ = runIn(t, home, "/", "images")
	if code != 0 || !strings.Contains(out, "NAME") || !strings.Contains(out, "demo") || !strings.Contains(out, "1.0.0") {
		t.Fatalf("images %d %q", code, out)
	}

	code, out, _ = runIn(t, home, "/", "image", "show", "demo")
	if code != 0 || !strings.Contains(out, "hello: Says hello") || !strings.Contains(out, "./p.md") {
		t.Fatalf("show %d %q", code, out)
	}

	for _, ref := range []string{"demo", "demo:1.0.0"} {
		if code, _, errs = runIn(t, home, "/", "image", "rm", ref); code != 0 {
			t.Fatalf("rm %s: %d %q", ref, code, errs)
		}
	}
	// The older archive is still in refs/ but untagged, so it does not list.
	code, out, _ = runIn(t, home, "/", "images")
	if code != 0 || strings.Contains(out, "demo") {
		t.Fatalf("images after rm %d %q", code, out)
	}
	if code, _, errs = runIn(t, home, "/", "image", "rm", "demo"); code != 1 || !strings.Contains(errs, "not found") {
		t.Fatalf("second rm %d %q", code, errs)
	}
}

func TestBuildErrors(t *testing.T) {
	home := t.TempDir()
	if code, _, _ := runIn(t, home, "/", "build", "--path", "/nonexistent-tbox-dir"); code != 1 {
		t.Fatalf("missing dir code %d", code)
	}
	if code, _, _ := runIn(t, home, "/", "build", "--bogus"); code != 2 {
		t.Fatalf("bad flag code %d", code)
	}
	if code, _, errs := runIn(t, home, "/", "build", "--path", demoSource(t), "--name", "My Image"); code != 1 || !strings.Contains(errs, "invalid image name") {
		t.Fatalf("bad name %d %q", code, errs)
	}
}
