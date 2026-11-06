package shim

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestRunTmuxSupervisorRejectsEmptyArgv(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status")
	err := RunTmuxSupervisor("supervised", statusPath, nil)
	if err == nil || err.Error() != "tmux supervisor requires a harness command" {
		t.Fatalf("error = %v", err)
	}
}

func TestRunTmuxSupervisorStartFailureWritesNoStatus(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status")
	err := RunTmuxSupervisor("supervised", statusPath, []string{filepath.Join(t.TempDir(), "missing")})
	if err == nil || !strings.HasPrefix(err.Error(), "start harness: ") || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(statusPath); !os.IsNotExist(statErr) {
		t.Fatalf("status written: %v", statErr)
	}
}

func TestRunTmuxSupervisorReportsUnreadableTerminal(t *testing.T) {
	closed, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = closed
	t.Cleanup(func() { os.Stdin = oldStdin })
	err = RunTmuxSupervisor("supervised", filepath.Join(t.TempDir(), "status"), []string{"/bin/true"})
	if err == nil || !strings.HasPrefix(err.Error(), "inspect harness terminal: ") || !errors.Is(err, syscall.EBADF) {
		t.Fatalf("error = %v", err)
	}
}

// supervisorSignals records the group signals of one supervisor run.
type supervisorSignals struct {
	mu   sync.Mutex
	sent []syscall.Signal
}

// add records signal and returns how many times it has now been sent.
func (s *supervisorSignals) add(signal syscall.Signal) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, signal)
	count := 0
	for _, sent := range s.sent {
		if sent == signal {
			count++
		}
	}
	return count
}

func (s *supervisorSignals) list() []syscall.Signal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]syscall.Signal(nil), s.sent...)
}

// TestRunTmuxSupervisorFailureStages pins the error text of each stage and the
// group signals sent on the way, including the deferred final kill.
func TestRunTmuxSupervisorFailureStages(t *testing.T) {
	shortenTmuxSupervisorWaits(t, 50*time.Millisecond)
	testErr := errors.New("test failure")
	const term, kill = syscall.SIGTERM, syscall.SIGKILL
	const never = syscall.Signal(-1)
	for _, tc := range []struct {
		name string
		hup  bool
		// exitOn is the signal after which the fake observer reports exit: zero
		// reports once the harness is ready and never reports nothing.
		exitOn      syscall.Signal
		observerErr error
		killFails   func(signal syscall.Signal, count int) bool
		reap        func(int, *syscall.WaitStatus, int, *syscall.Rusage) (int, error)
		wantErr     string
		wantSignals []syscall.Signal
	}{
		{
			name: "observer fails", observerErr: testErr,
			wantErr: "observe harness exit: test failure", wantSignals: []syscall.Signal{kill},
		},
		{
			name: "group clean up fails", killFails: func(syscall.Signal, int) bool { return true },
			wantErr:     "clean up harness group: test failure\nfinal harness cleanup: test failure",
			wantSignals: []syscall.Signal{kill, kill},
		},
		{
			name: "SIGTERM fails", hup: true, exitOn: never,
			killFails: func(signal syscall.Signal, _ int) bool { return signal == term },
			wantErr:   "terminate harness: test failure", wantSignals: []syscall.Signal{term, kill},
		},
		{
			name: "observer fails after SIGTERM", hup: true, exitOn: term, observerErr: testErr,
			wantErr: "observe harness exit after SIGTERM: test failure", wantSignals: []syscall.Signal{term, kill},
		},
		{
			name: "SIGKILL escalation fails", hup: true, exitOn: never,
			killFails: func(signal syscall.Signal, count int) bool { return signal == kill && count == 1 },
			wantErr:   "kill harness: test failure", wantSignals: []syscall.Signal{term, kill, kill},
		},
		{
			name: "observer fails after SIGKILL", hup: true, exitOn: kill, observerErr: testErr,
			wantErr: "observe harness exit after SIGKILL: test failure", wantSignals: []syscall.Signal{term, kill, kill},
		},
		{
			name: "SIGKILL deadline", hup: true, exitOn: never,
			wantErr: "observe harness exit after SIGKILL: deadline exceeded", wantSignals: []syscall.Signal{term, kill, kill},
		},
		{
			name:    "reap fails",
			reap:    func(int, *syscall.WaitStatus, int, *syscall.Rusage) (int, error) { return -1, testErr },
			wantErr: "reap harness: test failure", wantSignals: []syscall.Signal{kill, kill},
		},
		{
			name:    "reap finds no child",
			reap:    func(int, *syscall.WaitStatus, int, *syscall.Rusage) (int, error) { return 0, nil },
			wantErr: "reap harness: wait returned without child", wantSignals: []syscall.Signal{kill, kill},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldWait, oldKill, oldReap := waitTmuxChildExit, killTmuxGroup, reapTmuxChild
			restore := func() { waitTmuxChildExit, killTmuxGroup, reapTmuxChild = oldWait, oldKill, oldReap }
			defer restore()
			signals := &supervisorSignals{}
			release := make(chan struct{})
			var once sync.Once
			open := func() { once.Do(func() { close(release) }) }
			defer open()
			observerDone := make(chan struct{})
			waitTmuxChildExit = func(int) error {
				defer close(observerDone)
				<-release
				return tc.observerErr
			}
			killTmuxGroup = func(target int, signal syscall.Signal) error {
				count := signals.add(signal)
				if tc.killFails != nil && tc.killFails(signal, count) {
					return testErr
				}
				if tc.exitOn == signal {
					open()
				}
				return syscall.Kill(target, signal)
			}
			if tc.reap != nil {
				reapTmuxChild = tc.reap
			}

			dir := t.TempDir()
			pidPath := filepath.Join(dir, "harness.pid")
			statusPath := filepath.Join(dir, "status")
			done := make(chan error, 1)
			go func() {
				done <- RunTmuxSupervisor("supervised", statusPath, []string{"/bin/sh", "-c", `trap '' TERM; printf '%s' "$$" > "$1"; while :; do /bin/sleep 1; done`, "harness", pidPath})
			}()
			pid := waitForPIDFile(pidPath, 2*time.Second)
			if pid == 0 {
				t.Fatal("harness did not become ready")
			}
			if tc.exitOn == 0 {
				open()
			}
			if tc.hup {
				if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("supervisor did not return")
			}
			// A stage that gives up leaves the observer goroutine running; it
			// reads the hook variables, so it must end before they are restored.
			open()
			<-observerDone
			restore()
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			var status syscall.WaitStatus
			_, _ = syscall.Wait4(pid, &status, 0, nil)

			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if got := signals.list(); !reflect.DeepEqual(got, tc.wantSignals) {
				t.Fatalf("signals = %v, want %v", got, tc.wantSignals)
			}
			if _, statErr := os.Stat(statusPath); !os.IsNotExist(statErr) {
				t.Fatalf("status written after failure: %v", statErr)
			}
		})
	}
}

// TestRunTmuxSupervisorRetriesInterruptedReap pins the EINTR retry and that a
// reaped harness gets no final group kill even when writing the status fails:
// its process group id may already be reused.
func TestRunTmuxSupervisorRetriesInterruptedReap(t *testing.T) {
	oldKill, oldReap := killTmuxGroup, reapTmuxChild
	defer func() { killTmuxGroup, reapTmuxChild = oldKill, oldReap }()
	signals := &supervisorSignals{}
	killTmuxGroup = func(target int, signal syscall.Signal) error {
		signals.add(signal)
		return syscall.Kill(target, signal)
	}
	calls := 0
	reapTmuxChild = func(pid int, status *syscall.WaitStatus, options int, usage *syscall.Rusage) (int, error) {
		calls++
		if calls == 1 {
			return -1, syscall.EINTR
		}
		return syscall.Wait4(pid, status, options, usage)
	}
	statusPath := filepath.Join(t.TempDir(), "missing-dir", "status")
	err := RunTmuxSupervisor("supervised", statusPath, []string{"/bin/sh", "-c", "exit 9"})
	if !errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "final harness cleanup") {
		t.Fatalf("error = %v, want the status write failure alone", err)
	}
	if calls != 2 {
		t.Fatalf("reap calls = %d, want 2", calls)
	}
	if got := signals.list(); !reflect.DeepEqual(got, []syscall.Signal{syscall.SIGKILL}) {
		t.Fatalf("signals = %v, want a single SIGKILL before the reap", got)
	}
}
