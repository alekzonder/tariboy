package agentdir

import (
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/safefs"
)

// ImageBackupName is the directory an in-flight image swap parks the previous
// image/ under. Crash recovery treats its presence as the swap marker, so the
// name must stay stable across daemon versions.
const ImageBackupName = ".image-backup"

// SwapDir is safefs.SwapDir, which image.Store.Unpack shares; see there.
func SwapDir(dir, backup, stage string) (commit, rollback func(), err error) {
	return safefs.SwapDir(dir, backup, stage)
}

// SwapImage swaps stage into l's image/ through the <root>/.image-backup crash
// marker; see SwapDir.
func SwapImage(l Layout, stage string) (commit, rollback func(), err error) {
	return SwapDir(l.ImageDir(), filepath.Join(l.Root, ImageBackupName), stage)
}
