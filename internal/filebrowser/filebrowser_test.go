package filebrowser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// codeOf returns the *Error code, or "" if err is not a *filebrowser.Error.
func codeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestResolveWithinRoot(t *testing.T) {
	root := t.TempDir()
	got, err := Resolve(root, "a/b.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(root, "a", "b.txt"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveEmptyRelIsRoot(t *testing.T) {
	root := t.TempDir()
	got, err := Resolve(root, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Clean(root) {
		t.Fatalf("got %q, want root %q", got, root)
	}
}

// TestResolveRejectsTraversal proves lexical ../ escapes are refused, including
// deep and mixed forms, and that a leading absolute path cannot escape either.
func TestResolveRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"..",
		"../",
		"../secret",
		"../../etc/passwd",
		"a/../../b",
		"a/b/../../../c",
		"./../../x",
	}
	for _, rel := range cases {
		if _, err := Resolve(root, rel); codeOf(err) != "bad_path" {
			t.Errorf("Resolve(%q) = %v, want bad_path", rel, err)
		}
	}
}

// TestResolveAbsoluteInputStaysJailed proves an absolute-looking rel is treated
// as relative to root rather than honored as an absolute filesystem path.
func TestResolveAbsoluteInputStaysJailed(t *testing.T) {
	root := t.TempDir()
	got, err := Resolve(root, "/etc/passwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(root, "etc", "passwd"); got != want {
		t.Fatalf("absolute input escaped: got %q, want %q", got, want)
	}
}

// TestResolveRejectsSymlinkEscape proves a symlink inside the jail that points
// outside it is refused — both when reading through it and when targeting a file
// beneath it (a path about to be created under the symlinked dir).
func TestResolveRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A symlink INSIDE root pointing at a directory OUTSIDE root.
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	// Resolving the symlink itself escapes.
	if _, err := Resolve(root, "escape"); codeOf(err) != "bad_path" {
		t.Errorf("Resolve(escape) = %v, want bad_path", err)
	}
	// Resolving a not-yet-created file THROUGH the symlink escapes too.
	if _, err := Resolve(root, "escape/secret.txt"); codeOf(err) != "bad_path" {
		t.Errorf("Resolve(escape/secret.txt) = %v, want bad_path", err)
	}
	if _, err := Resolve(root, "escape/new.txt"); codeOf(err) != "bad_path" {
		t.Errorf("Resolve(escape/new.txt) = %v, want bad_path", err)
	}
}

// TestResolveAllowsInternalSymlink proves a symlink whose target stays inside
// the jail is permitted (the jail confines by real location, not by presence of
// a symlink).
func TestResolveAllowsInternalSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "link/inside.txt"); err != nil {
		t.Errorf("internal symlink wrongly rejected: %v", err)
	}
}

func TestListSortedDirsFirst(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "b.txt", "x")
	mustWrite(t, root, "a.txt", "x")
	if err := os.Mkdir(filepath.Join(root, "zdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := List(root, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	want := []string{"zdir", "a.txt", "b.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", names, want)
	}
	if !entries[0].IsDir {
		t.Errorf("first entry should be a dir")
	}
}

func TestListMissingDir(t *testing.T) {
	root := t.TempDir()
	if _, err := List(root, "nope"); codeOf(err) != "not_found" {
		t.Errorf("got %v, want not_found", err)
	}
}

func TestListFileIsNotDir(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "file.txt", "x")
	// Listing a regular file makes os.ReadDir return an ENOTDIR syscall error,
	// which must map to the typed not_dir envelope rather than a raw 500.
	if _, err := List(root, "file.txt"); codeOf(err) != "not_dir" {
		t.Errorf("got %v, want not_dir", err)
	}
}

func TestReadTextBinaryTooLarge(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "hello.txt", "hello world")
	c, err := Read(root, "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "text" || c.Data != "hello world" {
		t.Fatalf("text read wrong: %+v", c)
	}

	if err := os.WriteFile(filepath.Join(root, "bin"), []byte{0x00, 0x01, 0x02}, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = Read(root, "bin")
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "binary" || c.Data != "" {
		t.Fatalf("binary read wrong: %+v", c)
	}

	big := make([]byte, MaxReadBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(root, "big"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = Read(root, "big")
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "too_large" || c.Data != "" {
		t.Fatalf("too_large read wrong: %+v", c)
	}
	if c.Size != int64(len(big)) {
		t.Fatalf("too_large size = %d, want %d", c.Size, len(big))
	}
}

func TestReadDirIsError(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, "d"); codeOf(err) != "is_dir" {
		t.Errorf("got %v, want is_dir", err)
	}
}

func TestReadMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := Read(root, "nope"); codeOf(err) != "not_found" {
		t.Errorf("got %v, want not_found", err)
	}
}

func TestWriteCreatesParents(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, "a/b/c.txt", []byte("data")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "a", "b", "c.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "data" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteRejectsTraversalAndRoot(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, "../evil.txt", []byte("x")); codeOf(err) != "bad_path" {
		t.Errorf("traversal write = %v, want bad_path", err)
	}
	if err := Write(root, "", []byte("x")); codeOf(err) != "bad_path" {
		t.Errorf("root write = %v, want bad_path", err)
	}
}

func TestCreateFileAndDir(t *testing.T) {
	root := t.TempDir()
	if err := Create(root, "new.txt", "file"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if err := Create(root, "sub", "dir"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "sub"))
	if err != nil || !info.IsDir() {
		t.Fatalf("dir not created: %v", err)
	}
}

func TestCreateExisting(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "dup", "x")
	if err := Create(root, "dup", "file"); codeOf(err) != "exists" {
		t.Errorf("got %v, want exists", err)
	}
}

func TestCreateBadType(t *testing.T) {
	root := t.TempDir()
	if err := Create(root, "x", "symlink"); codeOf(err) != "bad_type" {
		t.Errorf("got %v, want bad_type", err)
	}
}

func TestRename(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "hi")
	if err := Rename(root, "a.txt", "sub/b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("source still present")
	}
	got, err := os.ReadFile(filepath.Join(root, "sub", "b.txt"))
	if err != nil || string(got) != "hi" {
		t.Fatalf("dest wrong: %v %q", err, got)
	}
}

func TestRenameEscapeRejected(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "hi")
	if err := Rename(root, "a.txt", "../b.txt"); codeOf(err) != "bad_path" {
		t.Errorf("rename escape = %v, want bad_path", err)
	}
	if err := Rename(root, "../x", "b.txt"); codeOf(err) != "bad_path" {
		t.Errorf("rename from escape = %v, want bad_path", err)
	}
}

func TestRenameMissingSource(t *testing.T) {
	root := t.TempDir()
	if err := Rename(root, "nope", "b.txt"); codeOf(err) != "not_found" {
		t.Errorf("got %v, want not_found", err)
	}
}

func TestDeleteFileAndDir(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	if err := Delete(root, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("file not deleted")
	}
	if err := os.MkdirAll(filepath.Join(root, "d", "e"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Delete(root, "d"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "d")); !os.IsNotExist(err) {
		t.Errorf("dir not deleted recursively")
	}
}

func TestDeleteRootRejected(t *testing.T) {
	root := t.TempDir()
	if err := Delete(root, ""); codeOf(err) != "bad_path" {
		t.Errorf("delete root = %v, want bad_path", err)
	}
}

func TestDeleteMissing(t *testing.T) {
	root := t.TempDir()
	if err := Delete(root, "nope"); codeOf(err) != "not_found" {
		t.Errorf("got %v, want not_found", err)
	}
}

func mustWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
