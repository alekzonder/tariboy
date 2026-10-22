package loop

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"syscall"
)

// logCleanup reports a failed best-effort cleanup step (removing a temp file,
// closing a handle on an error path, signalling a process group that may have
// exited). Outcomes that only mean "already gone" are expected and stay quiet:
// fs.ErrNotExist, ESRCH and os.ErrClosed.
func logCleanup(log *slog.Logger, msg string, err error, args ...any) {
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrClosed) {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	log.Warn(msg, append(args, "err", err)...)
}
