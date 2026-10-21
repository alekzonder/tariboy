package refstore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	idA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestCheckName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", "/a", `a\b`, "a\x00b"} {
		if err := CheckName(bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("CheckName(%q) = %v, want ErrInvalidName", bad, err)
		}
	}
	for _, good := range []string{"demo", "1.2.3", "latest", ".hidden", "a..b", "v1.0.0+build.1"} {
		if err := CheckName(good); err != nil {
			t.Errorf("CheckName(%q) = %v, want nil", good, err)
		}
	}
}

func TestLayoutPaths(t *testing.T) {
	s := Store{Dir: "/base"}
	if got := s.NameDir("demo"); got != filepath.Join("/base", "demo") {
		t.Fatalf("NameDir = %q", got)
	}
	if got := s.RefsDir("demo"); got != filepath.Join("/base", "demo", "refs") {
		t.Fatalf("RefsDir = %q", got)
	}
	if got := s.TagsDir("demo"); got != filepath.Join("/base", "demo", "tags") {
		t.Fatalf("TagsDir = %q", got)
	}
	if got := s.TagPath("demo", "latest"); got != filepath.Join("/base", "demo", "tags", "latest") {
		t.Fatalf("TagPath = %q", got)
	}
}

func TestWriteTagStoresOneHexLineAtomically(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.WriteTag("demo", "latest", idA); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(s.Dir, "demo", "tags", "latest"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != idA+"\n" {
		t.Fatalf("tag body = %q", body)
	}
	info, err := os.Stat(filepath.Join(s.Dir, "demo", "tags", "latest"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("tag mode = %#o", info.Mode().Perm())
	}
	if err := s.WriteTag("demo", "latest", idB); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(s.Dir, "demo", "tags"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "latest" {
		t.Fatalf("tags dir holds %v; a temp file was left behind", entries)
	}
	if got, err := s.Resolve("demo", "latest"); err != nil || got != idB {
		t.Fatalf("Resolve = %q, %v", got, err)
	}
}

func TestWriteTagRejectsBadInput(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.WriteTag("demo", "latest", "nothex"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("bad id: %v", err)
	}
	if err := s.WriteTag("..", "latest", idA); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("bad name: %v", err)
	}
	if err := s.WriteTag("demo", "a/b", idA); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("bad tag: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("rejected writes created the name directory: %v", err)
	}
}

func TestWriteTagFailureLeavesOldPointer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s := Store{Dir: t.TempDir()}
	if err := s.WriteTag("demo", "latest", idA); err != nil {
		t.Fatal(err)
	}
	tags := s.TagsDir("demo")
	if err := os.Chmod(tags, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(tags, 0o700) })
	if err := s.WriteTag("demo", "latest", idB); err == nil {
		t.Fatal("write into a read-only tags directory succeeded")
	}
	if got, err := s.Resolve("demo", "latest"); err != nil || got != idA {
		t.Fatalf("Resolve = %q, %v; want the old pointer", got, err)
	}
}

func TestResolve(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if _, err := s.Resolve("demo", "latest"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tag: %v", err)
	}
	if _, err := s.Resolve("..", "latest"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("bad name: %v", err)
	}
	if err := os.MkdirAll(s.TagsDir("demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.TagPath("demo", "broken"), []byte("  "+idA+"  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Resolve("demo", "broken"); err != nil || got != idA {
		t.Fatalf("surrounding space: %q, %v", got, err)
	}
	if err := os.WriteFile(s.TagPath("demo", "broken"), []byte("garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := s.Resolve("demo", "broken")
	var pointer *PointerError
	if !errors.Is(err, ErrInvalidPointer) || !errors.As(err, &pointer) || pointer.Value != "garbage" || pointer.Name != "demo" || pointer.Tag != "broken" {
		t.Fatalf("garbage pointer: %#v", err)
	}
}

func TestRemoveTag(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.WriteTag("demo", "latest", idA); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTag("demo", "latest"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve("demo", "latest"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed tag still resolves: %v", err)
	}
	if err := s.RemoveTag("demo", "latest"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestTagsListsSortedRegularVisibleFiles(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if got, err := s.Tags("demo"); err != nil || got != nil {
		t.Fatalf("missing name: %v, %v", got, err)
	}
	for _, tag := range []string{"latest", "1.0.0", "0.9.0"} {
		if err := s.WriteTag("demo", tag, idA); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.TagsDir("demo"), ".tmp-123"), []byte(idB), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.TagsDir("demo"), "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := s.Tags("demo")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"0.9.0", "1.0.0", "latest"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tags = %v, want %v", got, want)
	}
	if _, err := s.Tags("a/b"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("bad name: %v", err)
	}
}

func TestListReturnsSortedNameDirectories(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "missing")}
	if got, err := s.List(); err != nil || got != nil {
		t.Fatalf("missing dir: %v, %v", got, err)
	}
	s.Dir = t.TempDir()
	for _, name := range []string{"zeta", "alpha", ".hidden"} {
		if err := s.WriteTag(name, "latest", idA); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "stray-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".hidden", "alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
}

func TestPruneEmptyKeepsNonEmptyDirectories(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.WriteTag("demo", "latest", idA); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.RefsDir("demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	s.PruneEmpty("demo")
	if _, err := os.Stat(s.TagPath("demo", "latest")); err != nil {
		t.Fatalf("prune removed a live tag: %v", err)
	}
	if _, err := os.Stat(s.RefsDir("demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty refs dir survived: %v", err)
	}
	if err := s.RemoveTag("demo", "latest"); err != nil {
		t.Fatal(err)
	}
	s.PruneEmpty("demo")
	if _, err := os.Stat(s.NameDir("demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty name dir survived: %v", err)
	}
}

func TestPointerErrorText(t *testing.T) {
	err := &PointerError{Name: "demo", Tag: "latest", Value: "x"}
	if !strings.Contains(err.Error(), `"x"`) {
		t.Fatalf("text = %q", err.Error())
	}
}
