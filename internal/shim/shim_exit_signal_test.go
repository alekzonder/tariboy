package shim

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// failOnTmux makes any tmux call fail the test: the waits below must end
// without consulting the has-session fallback.
func failOnTmux(t *testing.T) {
	t.Helper()
	old := execCommand
	t.Cleanup(func() { execCommand = old })
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Errorf("unexpected command %s %v", name, args)
		return exec.Command("/bin/false")
	}
}

// waitEnd runs waitTmuxEnd in a goroutine so a regression fails instead of
// hanging the package.
func waitEnd(t *testing.T, ctx context.Context, exited <-chan struct{}, fallback <-chan time.Time) tmuxWaitEnd {
	t.Helper()
	got := make(chan tmuxWaitEnd, 1)
	go func() { got <- waitTmuxEnd(ctx, "managed", exited, fallback) }()
	select {
	case end := <-got:
		return end
	case <-time.After(5 * time.Second):
		t.Fatal("waitTmuxEnd did not return")
		return ""
	}
}

func TestWaitTmuxEndReturnsOnSupervisorSignalWithoutTicker(t *testing.T) {
	failOnTmux(t)
	exited := make(chan struct{})
	close(exited)
	if end := waitEnd(t, context.Background(), exited, nil); end != tmuxEndHarnessExited {
		t.Fatalf("end = %q, want %q", end, tmuxEndHarnessExited)
	}
}

func TestWaitTmuxEndReturnsOnContextCancel(t *testing.T) {
	failOnTmux(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if end := waitEnd(t, ctx, make(chan struct{}), nil); end != tmuxEndCanceled {
		t.Fatalf("end = %q, want %q", end, tmuxEndCanceled)
	}
}

func TestWaitTmuxEndFallbackTickEndsOnlyWhenSessionIsGone(t *testing.T) {
	old := execCommand
	t.Cleanup(func() { execCommand = old })
	checks := 0
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "tmux" || len(args) == 0 || args[0] != "has-session" {
			t.Errorf("unexpected command %s %v", name, args)
		}
		checks++
		if checks == 1 {
			return exec.Command("true") // still alive on the first tick
		}
		return exec.Command("/bin/false")
	}
	ticks := make(chan time.Time, 2)
	ticks <- time.Time{}
	ticks <- time.Time{}
	if end := waitEnd(t, context.Background(), make(chan struct{}), ticks); end != tmuxEndSessionGone {
		t.Fatalf("end = %q, want %q", end, tmuxEndSessionGone)
	}
	if checks != 2 {
		t.Fatalf("has-session checks = %d, want 2", checks)
	}
}

func serveShortSocket(t *testing.T, h Handler) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "shimx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go Serve(l, h)
	return sock
}

func TestHarnessExitedRPCClosesWaitChannelOnce(t *testing.T) {
	s := &tmuxShim{session: "managed", harnessExited: make(chan struct{})}
	sock := serveShortSocket(t, s)
	for i := 0; i < 2; i++ {
		if err := Dial(sock).NotifyHarnessExited(); err != nil {
			t.Fatalf("report %d: %v", i, err)
		}
	}
	select {
	case <-s.harnessExited:
	default:
		t.Fatal("harness exit report did not close the wait channel")
	}
}

func TestHarnessExitedRPCUnsupportedByProcessShim(t *testing.T) {
	sock := serveShortSocket(t, &fakeHandler{})
	if err := Dial(sock).NotifyHarnessExited(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("report error = %v, want unsupported", err)
	}
}

func TestRunTmuxSupervisorReportsExitAfterStatusAndHidesSocket(t *testing.T) {
	s := &tmuxShim{session: "managed", harnessExited: make(chan struct{})}
	sock := serveShortSocket(t, s)
	t.Setenv(tmuxSupervisorSockEnv, sock)
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status")
	envPath := filepath.Join(dir, "harness-env")
	harness := []string{"/bin/sh", "-c", `printf '%s' "${` + tmuxSupervisorSockEnv + `-unset}" > "$1"; exit 3`, "harness", envPath}
	if err := RunTmuxSupervisor("managed", statusPath, harness); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.harnessExited:
	default:
		t.Fatal("supervisor did not report harness exit")
	}
	if status, err := readTmuxExitStatus(statusPath); err != nil || status != 3 {
		t.Fatalf("status = %d, %v; want 3", status, err)
	}
	if env, err := os.ReadFile(envPath); err != nil || string(env) != "unset" {
		t.Fatalf("harness saw %s = %q, %v; want it removed", tmuxSupervisorSockEnv, env, err)
	}
}

func TestRunTmuxSupervisorIgnoresUnreachableExitSocket(t *testing.T) {
	t.Setenv(tmuxSupervisorSockEnv, filepath.Join(t.TempDir(), "absent.sock"))
	statusPath := filepath.Join(t.TempDir(), "status")
	if err := RunTmuxSupervisor("managed", statusPath, []string{"/bin/sh", "-c", "exit 0"}); err != nil {
		t.Fatalf("supervisor error = %v; the exit report is best-effort", err)
	}
	if status, err := readTmuxExitStatus(statusPath); err != nil || status != 0 {
		t.Fatalf("status = %d, %v; want 0", status, err)
	}
}

func TestRunTmuxEndsOnSupervisorReportWithoutFallbackTick(t *testing.T) {
	old := execCommand
	t.Cleanup(func() { execCommand = old })
	var mu sync.Mutex
	alive := true
	var supervisor *exec.Cmd
	execCommand = func(name string, args ...string) *exec.Cmd {
		mu.Lock()
		defer mu.Unlock()
		switch args[0] {
		case "new-session":
			return exec.Command("/bin/sh", "-c", "printf @42")
		case "new-window":
			// Run the pane command with the window environment, as tmux would.
			if len(args) != 6 || args[1] != "-e" {
				t.Errorf("new-window args = %#v", args)
				return exec.Command("/bin/false")
			}
			supervisor = exec.Command("/bin/sh", "-c", args[5])
			supervisor.Env = append(os.Environ(), args[2])
			if err := supervisor.Start(); err != nil {
				t.Errorf("start supervisor: %v", err)
			}
			return exec.Command("true")
		case "has-session":
			t.Errorf("fallback has-session ran; the supervisor report must end the wait")
			return exec.Command("/bin/false")
		case "list-sessions":
			if alive {
				return exec.Command("/bin/sh", "-c", "printf managed")
			}
			return exec.Command("/bin/sh", "-c", "printf 'no server running on x' >&2; exit 1")
		case "kill-session":
			alive = false
			return exec.Command("true")
		default:
			return exec.Command("true")
		}
	}

	dir, err := os.MkdirTemp("", "shimrun")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- runTmux(context.Background(), Options{
			IterationDir: dir, Agent: "managed", IterationID: "managed-1",
			TmuxSession: "managed", HarnessArgv: []string{"/bin/sh", "-c", "exit 7"},
		}, time.Hour)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runTmux did not end after the supervisor report")
	}
	mu.Lock()
	sup := supervisor
	stillAlive := alive
	mu.Unlock()
	if sup != nil {
		if err := sup.Wait(); err != nil {
			t.Fatalf("supervisor: %v", err)
		}
	}
	if stillAlive {
		t.Fatal("runTmux returned without removing the session")
	}
	data, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil || !strings.Contains(string(data), `"exit_code": 7`) {
		t.Fatalf("result.json = %s, %v; want exit_code 7", data, err)
	}
}
