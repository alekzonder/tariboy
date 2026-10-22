package image

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// failUnpackAfter makes the next unpacks fail once n members are written.
func failUnpackAfter(t *testing.T, n int) {
	t.Helper()
	unpackFailAfter = n
	t.Cleanup(func() { unpackFailAfter = 0 })
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestUnpackFailureLeavesNoDestDir(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	seed(t, st, "app")
	parent := t.TempDir()
	dest := filepath.Join(parent, "image")
	failUnpackAfter(t, 1)

	err := st.Unpack(Ref{Name: "app", Tag: "latest"}, dest)
	if !errors.Is(err, errUnpackInjected) {
		t.Fatalf("Unpack error = %v, want the injected failure", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("interrupted unpack left %s (stat err=%v)", dest, err)
	}
	if names := dirNames(t, parent); len(names) != 0 {
		t.Fatalf("interrupted unpack left %v next to the destination", names)
	}
}

func TestUnpackFailureLeavesExistingDestDirUnchanged(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	seed(t, st, "app")
	parent := t.TempDir()
	dest := filepath.Join(parent, "image")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	failUnpackAfter(t, 1)

	if err := st.Unpack(Ref{Name: "app", Tag: "latest"}, dest); !errors.Is(err, errUnpackInjected) {
		t.Fatalf("Unpack error = %v, want the injected failure", err)
	}
	if names := dirNames(t, dest); len(names) != 1 || names[0] != "old.txt" {
		t.Fatalf("existing destination now holds %v, want only old.txt", names)
	}
	if body, err := os.ReadFile(filepath.Join(dest, "old.txt")); err != nil || string(body) != "old" {
		t.Fatalf("old.txt = %q, %v", body, err)
	}
	if names := dirNames(t, parent); len(names) != 1 || names[0] != "image" {
		t.Fatalf("interrupted unpack left %v next to the destination", names)
	}
}

func TestUnpackReplacesExistingDestDir(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	seed(t, st, "app")
	parent := t.TempDir()
	dest := filepath.Join(parent, "image")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "stale.txt"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := st.Unpack(Ref{Name: "app", Tag: "latest"}, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file survived the unpack (stat err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		t.Fatalf("unpacked manifest: %v", err)
	}
	if names := dirNames(t, parent); len(names) != 1 || names[0] != "image" {
		t.Fatalf("unpack left %v next to the destination", names)
	}
}

// TestUnpackPinnedWritesIntoTheGivenDir keeps UnpackPinned writing straight
// into the staging dir its caller made, with no second staging level.
func TestUnpackPinnedWritesIntoTheGivenDir(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	seed(t, st, "app")
	ref := Ref{Name: "app", Tag: "latest"}
	id, err := st.Resolve(ref)
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	stage, err := os.MkdirTemp(parent, ".image-stage-")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UnpackPinned(ref, id, stage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stage, "manifest.json")); err != nil {
		t.Fatalf("unpacked manifest: %v", err)
	}
	if names := dirNames(t, parent); len(names) != 1 || names[0] != filepath.Base(stage) {
		t.Fatalf("UnpackPinned left %v next to its staging dir", names)
	}
}
