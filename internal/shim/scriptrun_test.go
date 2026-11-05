package shim

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/testutil"
	"golang.org/x/sys/unix"
)

// startScriptRun starts the hidden script-run mode the way the daemon does:
// a new process group, the locked state file as fd 3, the log as stdout and
// stderr. The test drops its own copy of the state file after start.
func startScriptRun(t *testing.T, command string) (cmd *exec.Cmd, statePath, logPath string) {
	t.Helper()
	dir := t.TempDir()
	statePath, logPath = filepath.Join(dir, "run.state"), filepath.Join(dir, "run.log")
	state, err := os.OpenFile(statePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(state.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(os.Args[0], ScriptRunMode, "--", "/bin/sh", "-c", command)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.ExtraFiles = []*os.File{state}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	state.Close()
	logFile.Close()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	return cmd, statePath, logPath
}

func readScriptState(t *testing.T, path string) ScriptState {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state ScriptState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("state %q: %v", raw, err)
	}
	return state
}

func TestRunScriptRecordsExitCode(t *testing.T) {
	t.Parallel()
	for _, code := range []int{0, 7} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			t.Parallel()
			cmd, statePath, logPath := startScriptRun(t, "echo out; echo err >&2; exit "+strconv.Itoa(code))
			if err := cmd.Wait(); err != nil {
				t.Fatalf("wrapper: %v", err)
			}
			state := readScriptState(t, statePath)
			if state.ExitCode == nil || *state.ExitCode != code || state.Signal != nil || state.StartError != "" {
				t.Fatalf("state = %+v, want exit code %d", state, code)
			}
			if out, _ := os.ReadFile(logPath); string(out) != "out\nerr\n" {
				t.Fatalf("log = %q", out)
			}
		})
	}
}

func TestRunScriptRecordsSignal(t *testing.T) {
	t.Parallel()
	cmd, statePath, _ := startScriptRun(t, "kill -KILL $$")
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wrapper: %v", err)
	}
	state := readScriptState(t, statePath)
	if state.Signal == nil || *state.Signal != int(syscall.SIGKILL) || state.ExitCode != nil {
		t.Fatalf("state = %+v, want signal %d", state, syscall.SIGKILL)
	}
}

func TestRunScriptRecordsStartError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run.state")
	state, err := os.OpenFile(statePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], ScriptRunMode, "--", filepath.Join(dir, "missing"))
	cmd.ExtraFiles = []*os.File{state}
	if err := cmd.Run(); err != nil {
		t.Fatalf("wrapper: %v", err)
	}
	state.Close()
	if got := readScriptState(t, statePath); got.StartError == "" || got.ExitCode != nil || got.Signal != nil {
		t.Fatalf("state = %+v, want a start error", got)
	}
}

// The wrapper survives a TERM to its group, so the script's own end is still
// recorded; while the script runs the wrapper holds the state file lock.
func TestRunScriptSurvivesGroupSIGTERM(t *testing.T) {
	t.Parallel()
	cmd, statePath, logPath := startScriptRun(t, "echo ready; sleep 30")
	if !testutil.WaitFor(5*time.Second, func() bool {
		out, _ := os.ReadFile(logPath)
		return strings.Contains(string(out), "ready")
	}) {
		t.Fatal("script did not start")
	}
	probe, err := os.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	err = unix.Flock(int(probe.Fd()), unix.LOCK_SH|unix.LOCK_NB)
	probe.Close()
	if !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("state lock while the script runs: err=%v, want EWOULDBLOCK", err)
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wrapper: %v", err)
	}
	state := readScriptState(t, statePath)
	if state.Signal == nil || *state.Signal != int(syscall.SIGTERM) {
		t.Fatalf("state = %+v, want signal %d", state, syscall.SIGTERM)
	}
}

func TestRunScriptDoesNotLeakStateFD(t *testing.T) {
	t.Parallel()
	cmd, statePath, _ := startScriptRun(t, `[ -e /proc/$$/fd/3 ] && exit 9; exit 0`)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wrapper: %v", err)
	}
	if state := readScriptState(t, statePath); state.ExitCode == nil || *state.ExitCode != 0 {
		t.Fatalf("state = %+v: the script inherited the state file descriptor", state)
	}
}
