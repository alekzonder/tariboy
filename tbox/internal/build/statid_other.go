//go:build !linux && !darwin

package build

import "io/fs"

// statID has no inode and change time here, so the stat cache is never
// trusted and every file is hashed.
func statID(fs.FileInfo) (uint64, int64, bool) { return 0, 0, false }
