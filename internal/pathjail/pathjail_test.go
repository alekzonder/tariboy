package pathjail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithin(t *testing.T) {
	cases := []struct {
		root, target string
		want         bool
	}{
		{"/a", "/a", true},
		{"/a", "/a/b", true},
		{"/a", "/ab", false},
		{"/a", "/", false},
		{"/", "/", true},
		{"/", "/etc", true},
	}
	for _, c := range cases {
		if got := Within(c.root, c.target); got != c.want {
			t.Errorf("Within(%q,%q)=%v want %v", c.root, c.target, got, c.want)
		}
	}
}

func TestResolveExistingSymlinkEscape(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveExisting(filepath.Join(root, "link", "new", "file"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(outside, "new", "file")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if Within(root, got) {
		t.Fatalf("resolved path %q must be outside root %q", got, root)
	}
}

func TestResolveExistingMissingTail(t *testing.T) {
	base, _ := filepath.EvalSymlinks(t.TempDir())
	got, err := ResolveExisting(filepath.Join(base, "a", "b"))
	if err != nil || got != filepath.Join(base, "a", "b") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestIsNotDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := os.ReadDir(f)
	if !IsNotDir(err) {
		t.Fatalf("IsNotDir(%v)=false", err)
	}
	if IsNotDir(os.ErrNotExist) {
		t.Fatal("ErrNotExist is not ENOTDIR")
	}
}

func TestErrorConstructors(t *testing.T) {
	if e := BadPath("x"); e.Code != "bad_path" || e.Error() != "bad_path: x" {
		t.Fatalf("%v", e)
	}
	if NotFound("x").Code != "not_found" || NotDir("x").Code != "not_dir" {
		t.Fatal("codes")
	}
}

func TestResolveExistingFilesystemRoot(t *testing.T) {
	got, err := ResolveExisting("/")
	if err != nil || got != "/" {
		t.Fatalf("got %q, %v", got, err)
	}
	if !Within(got, "/tmp") {
		t.Fatal("children of / must be within /")
	}
}
