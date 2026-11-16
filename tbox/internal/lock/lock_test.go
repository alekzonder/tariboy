package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/paths"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), Filename)
	f := File{Version: 1, Entries: map[string]Entry{
		Key("git", "https://h/r", "skills/a", "v1"): {Kind: "git", URL: "https://h/r", Path: "skills/a", Ref: "v1", Commit: "c1", SHA256: "s1"},
		Key("url", "https://h/a.tgz", "", ""):       {Kind: "url", URL: "https://h/a.tgz", SHA256: "s2"},
	}}
	if err := f.Write(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(data), "}\n") {
		t.Fatalf("no trailing newline: %q", data)
	}
	if strings.Index(string(data), "git|") > strings.Index(string(data), "url|") {
		t.Fatalf("keys not sorted: %s", data)
	}
	got, err := Read(path)
	if err != nil || len(got.Entries) != 2 || got.Entries[Key("url", "https://h/a.tgz", "", "")].SHA256 != "s2" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestReadMissingIsEmpty(t *testing.T) {
	got, err := Read(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || got.Version != 1 || got.Entries == nil || len(got.Entries) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestReadRejectsBadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), Filename)
	for _, body := range []string{"{", `{"version":9,"entries":{}}`} {
		os.WriteFile(path, []byte(body), 0o644)
		if _, err := Read(path); err == nil {
			t.Fatalf("%q accepted", body)
		}
	}
}

func TestPathFor(t *testing.T) {
	home := paths.Home(t.TempDir())
	writable := t.TempDir()
	if got := PathFor(writable, home); got != filepath.Join(writable, Filename) {
		t.Fatalf("writable: %s", got)
	}
	if os.Geteuid() != 0 {
		ro := t.TempDir()
		os.Chmod(ro, 0o555)
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		got := PathFor(ro, home)
		if filepath.Dir(got) != filepath.Join(string(home), "locks") || filepath.Ext(got) != ".json" {
			t.Fatalf("read-only: %s", got)
		}
	}
	inStore := filepath.Join(home.StoresDir(), "s", "img")
	os.MkdirAll(inStore, 0o755)
	got := PathFor(inStore, home)
	if filepath.Dir(got) != filepath.Join(string(home), "locks") {
		t.Fatalf("store: %s", got)
	}
	if got != PathFor(inStore, home) {
		t.Fatal("not stable")
	}
}
