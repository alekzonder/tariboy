package build

import (
	"io/fs"
	"syscall"
)

// statID returns the inode number and the change time of info.
func statID(info fs.FileInfo) (ino uint64, ctimeNS int64, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return st.Ino, st.Ctimespec.Nano(), true
}
