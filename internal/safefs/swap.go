package safefs

import (
	"os"

	"github.com/alekzonder/tariboy/internal/tidy"
)

// SwapDir moves dir aside to backup and, when stage is non-empty, renames stage
// into dir. A stale backup is discarded first; a missing dir means there is
// nothing to back up. If stage cannot be moved into place the backup is
// restored and the error returned. After a successful swap, rollback discards
// the new dir and restores the backup, and commit discards the backup. An empty
// stage only moves dir aside so the caller can rebuild it in place. dir, backup
// and stage must be siblings on one filesystem so every move is one rename.
func SwapDir(dir, backup, stage string) (commit, rollback func(), err error) {
	tidy.RemoveAll(backup)
	hadOld := false
	if err := os.Rename(dir, backup); err == nil {
		hadOld = true
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	restore := func() {
		if hadOld {
			tidy.Log("best effort: os.Rename", os.Rename(backup, dir))
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
		tidy.RemoveAll(dir)
		restore()
	}
	return commit, rollback, nil
}
