// Package tidy logs the errors of best-effort cleanup (removing a temporary
// file, closing a descriptor, signalling a process group that may have exited)
// instead of discarding them. Errors that mean "already gone" are ignored.
package tidy

import (
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"syscall"
)

// Log warns about err unless it is nil or an expected already-gone error
// (missing file, no such process, closed file or listener, finished tx).
func Log(msg string, err error, args ...any) {
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) ||
		errors.Is(err, os.ErrClosed) || errors.Is(err, net.ErrClosed) || errors.Is(err, sql.ErrTxDone) {
		return
	}
	slog.Warn(msg, append(args, "err", err)...)
}

// Remove deletes path and logs an unexpected failure.
func Remove(path string) { Log("cleanup: remove", os.Remove(path), "path", path) }

// RemoveAll deletes path recursively and logs an unexpected failure.
func RemoveAll(path string) { Log("cleanup: remove all", os.RemoveAll(path), "path", path) }

// Close closes c and logs an unexpected failure.
func Close(c io.Closer) { Log("cleanup: close", c.Close()) }
