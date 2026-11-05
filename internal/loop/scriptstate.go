package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alekzonder/tariboy/internal/shim"
	"golang.org/x/sys/unix"
)

// maxScriptStateBytes bounds what a probe reads: the wrapper writes one short
// JSON object.
const maxScriptStateBytes = 4096

// createScriptState creates a run's state file and takes its exclusive lock.
// The caller passes the file to the script-run wrapper, which keeps the lock
// until it exits, and then closes its own copy.
func createScriptState(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock script state: %w", err)
	}
	return f, nil
}

// probeScriptState tells whether a run's wrapper still lives. A held lock
// means it does; a free lock with a result means the run ended with it; a
// free lock without one, or no file, means the process is lost. The pid takes
// no part, so a reused pid never looks like a live run.
func probeScriptState(path string) (alive bool, state *shim.ScriptState, err error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("open script state: %w", err)
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, nil, err
	}
	if !info.Mode().IsRegular() {
		return false, nil, errors.New("script state is not a regular file")
	}
	if err := unix.Flock(fd, unix.LOCK_SH|unix.LOCK_NB); errors.Is(err, unix.EWOULDBLOCK) {
		return true, nil, nil
	} else if err != nil {
		return false, nil, fmt.Errorf("lock script state: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxScriptStateBytes))
	if err != nil {
		return false, nil, err
	}
	var result shim.ScriptState
	if json.Unmarshal(raw, &result) != nil || !result.Finished() {
		return false, nil, nil
	}
	return false, &result, nil
}
