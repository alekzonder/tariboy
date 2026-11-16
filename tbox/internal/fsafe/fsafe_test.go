package fsafe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileNoFollow(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFileNoFollow(file, 10)
	if err != nil || string(got) != "hello" {
		t.Fatalf("read = %q, %v", got, err)
	}
	if _, err := ReadFileNoFollow(file, 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("over limit error = %v", err)
	}
	if _, err := ReadFileNoFollow(link, 10); err == nil {
		t.Fatal("symlink must be rejected")
	}
	if _, err := ReadFileNoFollow(dir, 10); err == nil {
		t.Fatal("directory must be rejected")
	}
}

func TestWithin(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	canonRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, target string
		ok           bool
	}{
		{"inside existing", filepath.Join(root, "sub"), true},
		{"inside missing", filepath.Join(root, "sub", "new", "file"), true},
		{"root itself", root, true},
		{"dotdot", filepath.Join(root, "..", "x"), false},
		{"sibling prefix", root + "-other", false},
		{"dangling symlink", filepath.Join(root, "dangling"), false},
		{"symlink escape", filepath.Join(root, "escape", "file"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Within(root, tc.target)
			if tc.ok {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.HasPrefix(got, canonRoot) {
					t.Fatalf("result %q not under %q", got, canonRoot)
				}
			} else if err == nil {
				t.Fatalf("expected error, got %q", got)
			}
		})
	}
}
