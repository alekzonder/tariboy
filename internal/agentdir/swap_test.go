package agentdir

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMarker(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMarker(t *testing.T, dir, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, "marker"))
	if err != nil || string(got) != want {
		t.Fatalf("%s marker=%q err=%v, want %q", dir, got, err, want)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still exists (err=%v)", path, err)
	}
}

func TestSwapImageLeavesCrashMarkerAtImageBackup(t *testing.T) {
	t.Parallel()
	l := New(t.TempDir(), "worker")
	writeMarker(t, l.ImageDir(), "old")
	stage := filepath.Join(l.Root, ".image-stage-x")
	writeMarker(t, stage, "new")

	// No commit or rollback: a crash right after the swap must leave the
	// backup exactly where recovery, and data from older daemons, expects it.
	if _, _, err := SwapImage(l, stage); err != nil {
		t.Fatal(err)
	}
	assertMarker(t, filepath.Join(l.Root, ".image-backup"), "old")
	assertMarker(t, l.ImageDir(), "new")
}
