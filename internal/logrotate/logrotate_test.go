package logrotate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFileRotatesAtLimitAndKeepsNCopies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	f, err := Open(path, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"1111111\n", "2222222\n", "3333333\n", "4444444\n"} {
		if _, err := f.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	if got := read(t, path); got != "4444444\n" {
		t.Fatalf("current = %q", got)
	}
	if got := read(t, path+".1"); got != "3333333\n" {
		t.Fatalf(".1 = %q", got)
	}
	if got := read(t, path+".2"); got != "2222222\n" {
		t.Fatalf(".2 = %q", got)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf(".3 exists: %v", err)
	}
}

func TestOpenAppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	os.WriteFile(path, []byte("old\n"), 0o644)
	f, err := Open(path, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("new\n"))
	f.Close()
	if got := read(t, path); got != "old\nnew\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRotateIfLarger(t *testing.T) {
	dir := t.TempDir()
	small, big := filepath.Join(dir, "small.log"), filepath.Join(dir, "big.log")
	os.WriteFile(small, []byte("abc"), 0o644)
	os.WriteFile(big, []byte(strings.Repeat("x", 20)), 0o644)
	if err := RotateIfLarger(small, 10, 1); err != nil {
		t.Fatal(err)
	}
	if err := RotateIfLarger(big, 10, 1); err != nil {
		t.Fatal(err)
	}
	if err := RotateIfLarger(filepath.Join(dir, "missing.log"), 10, 1); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if read(t, small) != "abc" {
		t.Fatal("small file rotated")
	}
	if _, err := os.Stat(big); !os.IsNotExist(err) {
		t.Fatalf("big file still present: %v", err)
	}
	if len(read(t, big+".1")) != 20 {
		t.Fatal("big.1 missing")
	}
}
