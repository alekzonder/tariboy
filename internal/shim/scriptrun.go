package shim

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// ScriptRunMode is the hidden tariboy-shim mode that owns one script run:
// tariboy-shim __script-run -- ARGV.
const ScriptRunMode = "__script-run"

// scriptStateFD is the descriptor of the run's state file. The daemon locks
// the file before it starts the wrapper; the lock stays with the wrapper until
// it exits, so a free lock means the wrapper is gone.
const scriptStateFD = 3

// ScriptState is what the wrapper records when its script ends. The format is
// read by later daemon versions too: add fields, never rename them.
type ScriptState struct {
	ExitCode   *int   `json:"exit_code,omitempty"`
	Signal     *int   `json:"signal,omitempty"`
	StartError string `json:"start_error,omitempty"`
}

// Finished reports whether the state holds a result.
func (s ScriptState) Finished() bool {
	return s.ExitCode != nil || s.Signal != nil || s.StartError != ""
}

// RunScript runs argv to its end and writes its result to the state file on
// fd 3. It outlives the daemon that started it, so a later daemon can read the
// exit code it cannot wait for.
func RunScript(argv []string) error {
	if len(argv) == 0 {
		return errors.New("script run requires a command")
	}
	state := os.NewFile(scriptStateFD, "script-state")
	if state == nil {
		return errors.New("script state descriptor is missing")
	}
	defer state.Close()
	// The script must not hold the lock: a leftover child would keep the run
	// alive after its end.
	unix.CloseOnExec(scriptStateFD)
	// A TERM to the run's group ends the script, never this recorder. Notify,
	// unlike Ignore, does not pass the disposition to the script.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(signals)

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	var result ScriptState
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			result.StartError = err.Error()
		} else if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			sig := int(status.Signal())
			result.Signal = &sig
		} else {
			code := exitErr.ExitCode()
			result.ExitCode = &code
		}
	} else {
		code := 0
		result.ExitCode = &code
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err := state.Write(raw); err != nil {
		return fmt.Errorf("write script state: %w", err)
	}
	return state.Sync()
}
