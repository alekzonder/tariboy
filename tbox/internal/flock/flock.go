// Package flock serializes work between tbox processes with advisory file
// locks.
package flock

import (
	"os"
	"path/filepath"
	"syscall"
)

// Lock takes an exclusive lock on the file at path, creating the file and its
// parent directories, and returns its release. It blocks until the lock is
// free; the kernel drops it when the process exits.
func Lock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
