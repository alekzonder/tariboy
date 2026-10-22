package shim

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// busySocketPath returns a socket path that cannot be bound: a non-empty
// directory sits there, so neither the stale-socket removal nor the bind can
// take it over.
func busySocketPath(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "busy.sock")
	if err := os.MkdirAll(sock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sock, "occupant"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return sock
}

func TestRunTmuxBusySocketFailsBeforeNewSession(t *testing.T) {
	old := execCommand
	t.Cleanup(func() { execCommand = old })
	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "new-session":
			return exec.Command("/bin/sh", "-c", "printf '@42'")
		case "has-session", "list-sessions":
			return exec.Command("/bin/false")
		default:
			return exec.Command("true")
		}
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := runTmux(context.Background(), Options{
		IterationDir: dir, Agent: "manager", IterationID: "manager-bind-1",
		TmuxSession: "manager", HarnessArgv: []string{"claude"}, ShimSock: busySocketPath(t),
	}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "bind shim rpc socket") {
		t.Fatalf("runTmux() error = %v, want bind failure", err)
	}
	for _, call := range calls {
		if call[0] == "new-session" || call[0] == "new-window" {
			t.Fatalf("runTmux ran tmux %s after the RPC bind failed: %#v", call[0], calls)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(statErr) {
		t.Fatalf("result.json written after bind failure: %v", statErr)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "logs", "shim.log"))
	if !strings.Contains(string(log), "ERROR bind shim rpc socket") {
		t.Fatalf("shim.log missing bind error line:\n%s", log)
	}
}

func TestRunProcessBusySocketFailsBeforeHarnessStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "harness-ran")
	err := runWith(Options{
		IterationDir: dir, Agent: "smoke", IterationID: "smoke-bind-1", HardTimeoutS: 10,
		HarnessArgv: []string{"/bin/sh", "-c", `: > "$1"`, "harness", marker},
		ShimSock:    busySocketPath(t),
	}, time.Second, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "bind shim rpc socket") {
		t.Fatalf("runWith() error = %v, want bind failure", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("harness started after the RPC bind failed: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(statErr) {
		t.Fatalf("result.json written after bind failure: %v", statErr)
	}
}

// The socket answers before the harness exists. A Kill then must report that
// nothing was stopped, so callers retry instead of trusting a no-op success.
func TestProcShimKillBeforeHarnessStartReportsNotStarted(t *testing.T) {
	if err := (&procShim{}).Kill(); !errors.Is(err, errHarnessNotStarted) {
		t.Fatalf("Kill() = %v, want errHarnessNotStarted", err)
	}
}

func TestTmuxShimKillBeforeHarnessWindowReportsNotStarted(t *testing.T) {
	failOnTmux(t)
	if err := (&tmuxShim{session: "managed"}).Kill(); !errors.Is(err, errHarnessNotStarted) {
		t.Fatalf("Kill() = %v, want errHarnessNotStarted", err)
	}
}
