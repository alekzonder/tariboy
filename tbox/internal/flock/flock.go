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
// free; the kernel drops it when the process exits. When the holder removed
// the file before releasing it, Lock locks the file now at path instead, so
// a holder may remove its lock file.
func Lock(path string) (func(), error) {
	for {
		f, err := lockOnce(path)
		if err != nil {
			return nil, err
		}
		if same(f, path) {
			return func() { f.Close() }, nil
		}
		f.Close()
	}
}

func lockOnce(path string) (*os.File, error) {
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
	return f, nil
}

// same reports whether the locked file f is still the file at path.
func same(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	now, err := os.Stat(path)
	return err == nil && os.SameFile(held, now)
}
