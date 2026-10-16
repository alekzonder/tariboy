// Package logrotate keeps a log file under a size limit by renaming it to
// numbered copies: path.1 is the newest, path.<keep> the oldest.
package logrotate

import (
	"fmt"
	"os"
	"sync"
)

// File is an append-only log file that rotates itself when a write would take
// it past max bytes. It is safe for concurrent use.
type File struct {
	mu   sync.Mutex
	path string
	max  int64
	keep int
	f    *os.File
	size int64
}

func Open(path string, max int64, keep int) (*File, error) {
	l := &File{path: path, max: max, keep: keep}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *File) open() error {
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.size = f, st.Size()
	return nil
}

func (l *File) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size > 0 && l.size+int64(len(p)) > l.max {
		l.f.Close()
		if err := Rotate(l.path, l.keep); err != nil {
			return 0, err
		}
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

func (l *File) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}

// Rotate shifts path.N to path.N+1, drops the copy past keep, and renames path
// to path.1. A missing file is not an error.
func Rotate(path string, keep int) error {
	if keep < 1 {
		return os.Remove(path)
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, keep))
	for i := keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1))
	}
	if err := os.Rename(path, path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RotateIfLarger rotates path when it is larger than max bytes. Launchers call
// it before they open the file for a new daemon's stdout and stderr.
func RotateIfLarger(path string, max int64, keep int) error {
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Size() <= max {
		return nil
	}
	return Rotate(path, keep)
}
