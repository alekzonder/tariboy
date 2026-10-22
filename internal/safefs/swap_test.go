package safefs

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

func TestSwapDirCommitReplacesOldAndDropsBackup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, backup, stage := filepath.Join(root, "image"), filepath.Join(root, ".backup"), filepath.Join(root, ".stage")
	writeMarker(t, dir, "old")
	writeMarker(t, stage, "new")
	writeMarker(t, backup, "stale")

	commit, _, err := SwapDir(dir, backup, stage)
	if err != nil {
		t.Fatal(err)
	}
	assertMarker(t, dir, "new")
	assertMarker(t, backup, "old")
	assertAbsent(t, stage)
	commit()
	assertMarker(t, dir, "new")
	assertAbsent(t, backup)
}

func TestSwapDirRollbackRestoresOld(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, backup, stage := filepath.Join(root, "image"), filepath.Join(root, ".backup"), filepath.Join(root, ".stage")
	writeMarker(t, dir, "old")
	writeMarker(t, stage, "new")

	_, rollback, err := SwapDir(dir, backup, stage)
	if err != nil {
		t.Fatal(err)
	}
	rollback()
	assertMarker(t, dir, "old")
	assertAbsent(t, backup)
}

func TestSwapDirMissingOldDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, backup, stage := filepath.Join(root, "image"), filepath.Join(root, ".backup"), filepath.Join(root, ".stage")
	writeMarker(t, stage, "new")

	commit, rollback, err := SwapDir(dir, backup, stage)
	if err != nil {
		t.Fatal(err)
	}
	assertMarker(t, dir, "new")
	assertAbsent(t, backup)
	rollback()
	assertAbsent(t, dir)
	assertAbsent(t, backup)
	commit()
	assertAbsent(t, backup)
}

func TestSwapDirWithoutStageMovesOldAside(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, backup := filepath.Join(root, "bin"), filepath.Join(root, ".bin-backup")
	writeMarker(t, dir, "old")

	_, rollback, err := SwapDir(dir, backup, "")
	if err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, dir)
	assertMarker(t, backup, "old")
	writeMarker(t, dir, "rebuilt")
	rollback()
	assertMarker(t, dir, "old")
	assertAbsent(t, backup)
}

func TestSwapDirStageFailureRestoresOld(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, backup, stage := filepath.Join(root, "image"), filepath.Join(root, ".backup"), filepath.Join(root, ".missing-stage")
	writeMarker(t, dir, "old")

	if _, _, err := SwapDir(dir, backup, stage); err == nil {
		t.Fatal("swap from a missing stage succeeded")
	}
	assertMarker(t, dir, "old")
	assertAbsent(t, backup)
}
