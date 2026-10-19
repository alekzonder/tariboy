package agentdir

import (
	"os"
	"path/filepath"
)

// ImageBackupName is the directory an in-flight image swap parks the previous
// image/ under. Crash recovery treats its presence as the swap marker, so the
// name must stay stable across daemon versions.
const ImageBackupName = ".image-backup"

// SwapDir moves dir aside to backup and, when stage is non-empty, renames stage
// into dir. A stale backup is discarded first; a missing dir means there is
// nothing to back up. If stage cannot be moved into place the backup is
// restored and the error returned. After a successful swap, rollback discards
// the new dir and restores the backup, and commit discards the backup. An empty
// stage only moves dir aside so the caller can rebuild it in place.
func SwapDir(dir, backup, stage string) (commit, rollback func(), err error) {
	_ = os.RemoveAll(backup)
	hadOld := false
	if err := os.Rename(dir, backup); err == nil {
		hadOld = true
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	restore := func() {
		if hadOld {
			_ = os.Rename(backup, dir)
		}
	}
	if stage != "" {
		if err := os.Rename(stage, dir); err != nil {
			restore()
			return nil, nil, err
		}
	}
	commit = func() { _ = os.RemoveAll(backup) }
	rollback = func() {
		_ = os.RemoveAll(dir)
		restore()
	}
	return commit, rollback, nil
}

// SwapImage swaps stage into l's image/ through the <root>/.image-backup crash
// marker; see SwapDir.
func SwapImage(l Layout, stage string) (commit, rollback func(), err error) {
	return SwapDir(l.ImageDir(), filepath.Join(l.Root, ImageBackupName), stage)
}
