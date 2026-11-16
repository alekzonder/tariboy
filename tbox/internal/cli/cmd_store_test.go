package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
)

func localRegistry(t *testing.T) {
	t.Helper()
	prev := newRegistry
	newRegistry = func(home paths.Home, cwd string) stores.Registry {
		return stores.Registry{Home: home, Cwd: cwd, Git: gitx.Git{AllowLocal: true}}
	}
	t.Cleanup(func() { newRegistry = prev })
}

func storeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "skills/hello/SKILL.md"), "---\nname: hello\ndescription: Says hello\n---\nbody\n")
	writeFile(t, filepath.Join(repo, "skills/bad/SKILL.md"), "nothing\n")
	writeFile(t, filepath.Join(repo, "images/demo/Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.0.0\n")
	writeFile(t, filepath.Join(repo, "prompts/p.md"), "p\n")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "one"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func TestStoreCommands(t *testing.T) {
	localRegistry(t)
	repo := storeRepo(t)
	home := t.TempDir()
	url := "file://" + repo

	code, out, errs := runIn(t, home, "/", "store", "list")
	if code != 0 || out != "no stores\n" || errs != "" {
		t.Fatalf("empty list %d %q %q", code, out, errs)
	}
	code, out, errs = runIn(t, home, "/", "store", "add", "main", url, "--ref", "main")
	if code != 0 || out != "added main git\n" || errs != "" {
		t.Fatalf("add %d %q %q", code, out, errs)
	}
	dir := t.TempDir()
	if code, out, _ = runIn(t, home, dir, "store", "add", "local", "."); code != 0 || out != "added local dir\n" {
		t.Fatalf("add dir %d %q", code, out)
	}

	code, out, _ = runIn(t, home, "/", "store", "list")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if code != 0 || len(lines) != 2 {
		t.Fatalf("list %d %q", code, out)
	}
	f := strings.Split(lines[0], "\t")
	if len(f) != 5 || f[0] != "main" || f[1] != "git" || f[2] != url || f[3] != "main" || len(f[4]) != 12 {
		t.Fatalf("git row %q", lines[0])
	}
	if want := "local\tdir\t" + dir + "\t\t"; lines[1] != want {
		t.Fatalf("dir row %q want %q", lines[1], want)
	}

	code, out, errs = runIn(t, home, "/", "store", "show", "main")
	if code != 0 || errs != "" {
		t.Fatalf("show %d %q", code, errs)
	}
	for _, want := range []string{"name: main\n", "kind: git\n", "source: " + url + "\n", "ref: main\n", "commit: ", "images:\n  demo\t1.0.0\tschema 3\n", "skills:\n", "  hello\tSays hello\n", "  bad\terror: ", "prompts:\n  p\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output lacks %q:\n%s", want, out)
		}
	}

	code, out, _ = runIn(t, home, "/", "store", "refresh", "main")
	if code != 0 || !strings.HasPrefix(out, "refreshed main ") {
		t.Fatalf("refresh %d %q", code, out)
	}
	code, out, _ = runIn(t, home, "/", "store", "ref", "main", "")
	if code != 0 || !strings.HasPrefix(out, "ref main default branch ") {
		t.Fatalf("ref %d %q", code, out)
	}
	if code, _, errs = runIn(t, home, "/", "store", "ref", "main", "nope"); code != 1 || errs == "" {
		t.Fatalf("bad ref %d %q", code, errs)
	}
	if code, out, _ = runIn(t, home, "/", "store", "remove", "main"); code != 0 || out != "removed main\n" {
		t.Fatalf("remove %d %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, "stores", "main")); err == nil {
		t.Fatal("clone left behind")
	}
}

func TestStoreErrors(t *testing.T) {
	localRegistry(t)
	home := t.TempDir()
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"store"}, 2},
		{[]string{"store", "bogus"}, 2},
		{[]string{"store", "add", "x"}, 2},
		{[]string{"store", "list", "extra"}, 2},
		{[]string{"store", "show", "nope"}, 1},
		{[]string{"store", "refresh", "nope"}, 1},
		{[]string{"store", "remove", "nope"}, 1},
		{[]string{"store", "ref", "nope", "x"}, 1},
		{[]string{"store", "add", "Bad Name", t.TempDir()}, 1},
		{[]string{"store", "add", "x", filepath.Join(t.TempDir(), "gone")}, 1},
	}
	for _, c := range cases {
		code, out, errs := runIn(t, home, "/", c.args...)
		if code != c.code || (c.code == 1 && (out != "" || !strings.HasPrefix(errs, "tbox: "))) || errs == "" {
			t.Errorf("%v -> %d %q %q", c.args, code, out, errs)
		}
	}
}

func TestStoreShowSanitizesUntrustedText(t *testing.T) {
	localRegistry(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills/hello/SKILL.md"), "---\nname: hello\ndescription: \"ok\\e[31m red\\nforged\\tline\"\n---\nbody\n")
	writeFile(t, filepath.Join(dir, "skills/evil\nforged/SKILL.md"), "x")
	writeFile(t, filepath.Join(dir, "prompts/p\x1b[2Jq.md"), "p")
	writeFile(t, filepath.Join(dir, "images/v/Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.0.0\n")
	home := t.TempDir()
	if code, _, errs := runIn(t, home, "/", "store", "add", "s", dir); code != 0 {
		t.Fatalf("add %d %q", code, errs)
	}
	code, out, _ := runIn(t, home, "/", "store", "show", "s")
	if code != 0 || strings.ContainsRune(out, 0x1b) {
		t.Fatalf("show %d %q", code, out)
	}
	// name, kind, source, ref, commit, 3 headers, 1 image, 2 skills, 1 prompt
	if n := strings.Count(out, "\n"); n != 12 {
		t.Fatalf("%d lines:\n%s", n, out)
	}
}
