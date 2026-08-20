package fsbrowser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// codeOf returns the *Error code, or "" if err is not a *fsbrowser.Error.
func codeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func mustMkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestRootFromEnv proves TARIBOY_FS_ROOT wins over $HOME, and $HOME is the
// fallback when the env var is unset.
func TestRootFromEnv(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("TARIBOY_FS_ROOT", custom)
	got, err := Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if got != filepath.Clean(custom) {
		t.Fatalf("Root = %q, want %q", got, custom)
	}

	home := t.TempDir()
	t.Setenv("TARIBOY_FS_ROOT", "")
	t.Setenv("HOME", home)
	got, err = Root()
	if err != nil {
		t.Fatalf("Root (home): %v", err)
	}
	if got != filepath.Clean(home) {
		t.Fatalf("Root fallback = %q, want $HOME %q", got, home)
	}
}

// TestListRootAndResolution proves empty / relative / absolute-inside-root / ~
// all resolve to the same directory, that parent is "" at the root and the
// absolute parent below it.
func TestListRootAndResolution(t *testing.T) {
	root := t.TempDir()
	sub := mustMkdir(t, root, "project")

	// Root itself: parent is "" and the child dir shows up.
	l, err := List(root, "")
	if err != nil {
		t.Fatalf("List(root): %v", err)
	}
	if l.Path != filepath.Clean(root) {
		t.Fatalf("Path = %q, want %q", l.Path, root)
	}
	if l.Parent != "" {
		t.Fatalf("Parent at root = %q, want empty", l.Parent)
	}
	if len(l.Entries) != 1 || l.Entries[0].Name != "project" || !l.Entries[0].Dir {
		t.Fatalf("entries = %+v, want [project dir]", l.Entries)
	}

	// Every spelling of "the project dir" resolves to the same target with the
	// root as its parent (relative, ~-prefixed, and the real absolute path — a
	// bare "/project" is a different, out-of-root absolute path and is refused
	// separately in TestRejectsLexicalEscape).
	for _, path := range []string{"project", "~/project", sub} {
		l, err := List(root, path)
		if err != nil {
			t.Fatalf("List(%q): %v", path, err)
		}
		if l.Path != filepath.Clean(sub) {
			t.Fatalf("List(%q).Path = %q, want %q", path, l.Path, sub)
		}
		if l.Parent != filepath.Clean(root) {
			t.Fatalf("List(%q).Parent = %q, want %q", path, l.Parent, root)
		}
	}
}

// TestWithinFilesystemRoot proves within() treats a root of "/" as containing
// its children (the TARIBOY_FS_ROOT=/ edge, where root+sep used to become
// "//" and reject everything below it) without weakening containment for an
// ordinary root — a prefix-sibling and an out-of-root path are still refused.
func TestWithinFilesystemRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("'/' is not the filesystem root on windows")
	}
	sep := string(os.PathSeparator)
	fsRoot := sep // "/"

	// Root itself and every child are contained when root is "/".
	if !within(fsRoot, fsRoot) {
		t.Errorf("within(%q, %q) = false, want true (root itself)", fsRoot, fsRoot)
	}
	for _, child := range []string{"/etc", "/home/x", "/a"} {
		if !within(fsRoot, child) {
			t.Errorf("within(%q, %q) = false, want true (child of /)", fsRoot, child)
		}
	}

	// Containment is not weakened for an ordinary root: a prefix-sibling and an
	// unrelated absolute path are both refused (guards the naive HasPrefix bug).
	root := "/srv/root"
	for _, escape := range []string{"/srv/root-evil", "/etc", "/srv"} {
		if within(root, escape) {
			t.Errorf("within(%q, %q) = true, want false (escape)", root, escape)
		}
	}
}

// TestDirectoriesOnly proves files are filtered out and only directories (incl.
// dotfile dirs) are returned, sorted case-insensitively.
func TestDirectoriesOnly(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, root, "Beta")
	mustMkdir(t, root, "alpha")
	mustMkdir(t, root, ".config") // dotfile dir must be included
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "zzz.bin"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	l, err := List(root, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, e := range l.Entries {
		if !e.Dir {
			t.Fatalf("non-dir entry leaked: %+v", e)
		}
		names = append(names, e.Name)
	}
	want := []string{".config", "alpha", "Beta"} // case-insensitive sort
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
}

// TestRejectsLexicalEscape proves .. traversal and out-of-root absolute paths
// are refused with bad_path.
func TestRejectsLexicalEscape(t *testing.T) {
	parent := t.TempDir()
	root := mustMkdir(t, parent, "root")
	// A real sibling dir outside the root, reachable only by escaping.
	mustMkdir(t, parent, "outside")

	cases := []string{
		"..",
		"../outside",
		"../../etc",
		"a/../../outside",
		filepath.Join(parent, "outside"), // absolute path outside root
		"/etc",
	}
	for _, path := range cases {
		if _, err := List(root, path); codeOf(err) != "bad_path" {
			t.Errorf("List(%q) = %v, want bad_path", path, err)
		}
	}
}

// TestRejectsSymlinkEscape proves a symlink inside the root that points outside
// it is refused with bad_path — both the link itself and a path beneath it.
func TestRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	mustMkdir(t, outside, "secret")

	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	if _, err := List(root, "escape"); codeOf(err) != "bad_path" {
		t.Errorf("List(escape) = %v, want bad_path", err)
	}
	if _, err := List(root, "escape/secret"); codeOf(err) != "bad_path" {
		t.Errorf("List(escape/secret) = %v, want bad_path", err)
	}
}

// TestErrorCodes proves all three typed error codes surface: bad_path (escape),
// not_found (missing dir) and not_dir (path is a file).
func TestErrorCodes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "afile"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := List(root, "/etc"); codeOf(err) != "bad_path" {
		t.Errorf("escape: got %v, want bad_path", err)
	}
	if _, err := List(root, "nope/missing"); codeOf(err) != "not_found" {
		t.Errorf("missing: got %v, want not_found", err)
	}
	if _, err := List(root, "afile"); codeOf(err) != "not_dir" {
		t.Errorf("file: got %v, want not_dir", err)
	}
}
