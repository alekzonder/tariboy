package loop

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/scriptnotify"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// scriptHandoff holds what two daemons in a row share: the database, the
// agents directory and the runtime directory.
type scriptHandoff struct {
	cfg ManagerConfig
	raw *store.Store
	st  *script.Store
}

func newScriptHandoff(t *testing.T) *scriptHandoff {
	t.Helper()
	m, as, _, raw := newManager(t, &fakeRunner{})
	st := script.NewStore(raw, time.Now)
	cfg := m.cfg
	cfg.Scripts, cfg.Mail = st, newTestMail(raw, "worker")
	if err := as.Create(agent.Agent{Name: "worker", ImageRef: "basic:latest", Cwd: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	return &scriptHandoff{cfg: cfg, raw: raw, st: st}
}

// start runs one daemon's manager; handoff tells it how its stop ends.
func (h *scriptHandoff) start(t *testing.T, handoff bool, timeout time.Duration) *Manager {
	t.Helper()
	cfg := h.cfg
	cfg.ScriptHandoff = func() bool { return handoff }
	cfg.ScriptTimeout = timeout
	m := NewManager(cfg)
	if err := m.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	return m
}

// runUntilLogged starts a one-shot script and waits until its log holds want.
func (h *scriptHandoff) runUntilLogged(t *testing.T, m *Manager, command, want string) script.Run {
	t.Helper()
	_, run, err := m.RunOnce("worker", script.CreateOnce{Name: "long", Description: "test", Command: command})
	if err != nil {
		t.Fatal(err)
	}
	run = awaitScriptRun(t, h.st, "worker", run.ID, func(r script.Run) bool { return r.Status == script.RunRunning && r.PID != nil })
	if !testutil.WaitFor(5*time.Second, func() bool {
		out, _ := os.ReadFile(run.LogPath)
		return strings.Contains(string(out), want)
	}) {
		out, _ := os.ReadFile(run.LogPath)
		t.Fatalf("script log %q does not contain %q", out, want)
	}
	return run
}

// results publishes the outbox and returns the agent's script.result letters.
func (h *scriptHandoff) results(t *testing.T) []map[string]any {
	t.Helper()
	publisher := scriptnotify.New(h.raw.DB, h.cfg.Mail, time.Now, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	letters, err := inboxOf(h.cfg.Mail, "worker")
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, letter := range letters {
		if letter.Kind == "script.result" {
			out = append(out, letter.Data)
		}
	}
	return out
}

func groupAlive(pid int) bool { return syscall.Kill(-pid, 0) == nil }

func TestScriptRunSurvivesHandoffWithExitCode(t *testing.T) {
	t.Parallel()
	for _, code := range []int{0, 7} {
		t.Run("exit_"+strconv.Itoa(code), func(t *testing.T) {
			t.Parallel()
			h := newScriptHandoff(t)
			release := filepath.Join(t.TempDir(), "release")
			first := h.start(t, true, 0)
			run := h.runUntilLogged(t, first,
				`echo before; while [ ! -e "`+release+`" ]; do sleep 0.05; done; echo after; exit `+strconv.Itoa(code), "before")

			first.Shutdown()
			if got, err := h.st.GetRun("worker", run.ID); err != nil || got.Status != script.RunRunning {
				t.Fatalf("run after handoff stop = %#v err=%v, want running", got, err)
			}
			if !groupAlive(*run.PID) {
				t.Fatal("handoff stop ended the script")
			}

			h.start(t, true, 0)
			if err := os.WriteFile(release, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			want := script.RunSucceeded
			if code != 0 {
				want = script.RunFailed
			}
			got := awaitScriptRun(t, h.st, "worker", run.ID, func(r script.Run) bool { return r.Status == want })
			if got.ExitCode == nil || *got.ExitCode != code {
				t.Fatalf("run = %#v, want exit code %d", got, code)
			}
			if out, _ := os.ReadFile(run.LogPath); !strings.Contains(string(out), "before\n") || !strings.Contains(string(out), "after\n") {
				t.Fatalf("log = %q, want output from before and after the handoff", out)
			}
			results := h.results(t)
			if len(results) != 1 || results[0]["run_id"] != run.ID || results[0]["status"] != want || results[0]["exit_code"] != float64(code) {
				t.Fatalf("script.result letters = %#v, want one %s with exit code %d", results, want, code)
			}
		})
	}
}

func TestScriptRunLostAtRestartIsInterrupted(t *testing.T) {
	t.Parallel()
	h := newScriptHandoff(t)
	first := h.start(t, true, 0)
	run := h.runUntilLogged(t, first, "echo before; sleep 30", "before")
	first.Shutdown()
	if err := syscall.Kill(-*run.PID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	statePath := strings.TrimSuffix(run.LogPath, ".log") + ".state"
	if !testutil.WaitFor(5*time.Second, func() bool {
		alive, _, _ := probeScriptState(statePath)
		return !alive
	}) {
		t.Fatal("killed wrapper still holds the state lock")
	}

	started := time.Now()
	h.start(t, true, 0)
	awaitScriptRun(t, h.st, "worker", run.ID, func(r script.Run) bool { return r.Status == script.RunInterrupted })
	results := h.results(t)
	if elapsed := time.Since(started); elapsed >= time.Minute {
		t.Fatalf("interrupted result took %v after the restart", elapsed)
	}
	if len(results) != 1 || results[0]["run_id"] != run.ID || results[0]["status"] != script.RunInterrupted {
		t.Fatalf("script.result letters = %#v, want one interrupted", results)
	}
}

func TestScriptRunWithReusedPIDIsNotAdopted(t *testing.T) {
	t.Parallel()
	for _, withState := range []bool{true, false} {
		t.Run("state_file_"+strconv.FormatBool(withState), func(t *testing.T) {
			t.Parallel()
			h := newScriptHandoff(t)
			stranger := exec.Command("sleep", "30")
			if err := stranger.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan struct{})
			go func() { _ = stranger.Wait(); close(exited) }()
			t.Cleanup(func() { _ = stranger.Process.Kill(); <-exited })

			_, run, err := h.st.CreateOnce("worker", script.CreateOnce{Name: "old", Description: "test", Command: "sleep 30"})
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(h.cfg.AgentsDir, "worker", "scripts")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			logPath, err := filepath.Abs(filepath.Join(dir, run.ID+".log"))
			if err != nil {
				t.Fatal(err)
			}
			if claimed, err := h.st.ClaimRun("worker", run.ID, store.FormatTime(time.Now()), logPath); err != nil || !claimed {
				t.Fatalf("claim=%v err=%v", claimed, err)
			}
			if set, err := h.st.SetRunPID("worker", run.ID, stranger.Process.Pid); err != nil || !set {
				t.Fatalf("pid set=%v err=%v", set, err)
			}
			if withState {
				state, err := createScriptState(filepath.Join(dir, run.ID+".state"))
				if err != nil {
					t.Fatal(err)
				}
				state.Close()
			}

			h.start(t, true, 0)
			got, err := h.st.GetRun("worker", run.ID)
			if err != nil || got.Status != script.RunInterrupted {
				t.Fatalf("run with a reused pid = %#v err=%v, want interrupted", got, err)
			}
			select {
			case <-exited:
				t.Fatal("the process that reused the pid was signalled")
			case <-time.After(300 * time.Millisecond):
			}
		})
	}
}

func TestAdoptedScriptRunCancel(t *testing.T) {
	oldGrace := scriptCancelGrace
	scriptCancelGrace = 200 * time.Millisecond
	t.Cleanup(func() { scriptCancelGrace = oldGrace })

	t.Run("after_handoff", func(t *testing.T) {
		h := newScriptHandoff(t)
		first := h.start(t, true, 0)
		run := h.runUntilLogged(t, first, "echo before; sleep 30", "before")
		first.Shutdown()

		second := h.start(t, true, 0)
		if err := second.CancelScriptTarget("worker", run.ID); err != nil {
			t.Fatal(err)
		}
		awaitScriptRun(t, h.st, "worker", run.ID, func(r script.Run) bool { return r.Status == script.RunCancelled })
		if !testutil.WaitFor(3*time.Second, func() bool { return !groupAlive(*run.PID) }) {
			t.Fatal("cancelled script group is still alive")
		}
	})

	t.Run("cancelling_before_handoff", func(t *testing.T) {
		h := newScriptHandoff(t)
		first := h.start(t, true, 0)
		run := h.runUntilLogged(t, first, "trap '' TERM; echo started; while :; do sleep 1; done", "started")
		if err := first.CancelScriptTarget("worker", run.ID); err != nil {
			t.Fatal(err)
		}
		first.Shutdown()
		time.Sleep(2 * scriptCancelGrace)
		if got, err := h.st.GetRun("worker", run.ID); err != nil || got.Status != script.RunRunning || !got.CancelRequested {
			t.Fatalf("run after handoff stop = %#v err=%v, want cancelling", got, err)
		}
		if !groupAlive(*run.PID) {
			t.Fatal("the stopped daemon still killed the script")
		}

		h.start(t, true, 0)
		awaitScriptRun(t, h.st, "worker", run.ID, func(r script.Run) bool { return r.Status == script.RunCancelled })
		if !testutil.WaitFor(3*time.Second, func() bool { return !groupAlive(*run.PID) }) {
			t.Fatal("cancelled script group is still alive")
		}
	})
}

func TestAdoptedScriptRunTimeout(t *testing.T) {
	t.Parallel()
	h := newScriptHandoff(t)
	first := h.start(t, true, 0)
	run := h.runUntilLogged(t, first, "echo before; sleep 30", "before")
	first.Shutdown()

	h.start(t, true, time.Second)
	got := awaitScriptRun(t, h.st, "worker", run.ID, func(r script.Run) bool { return r.Status == script.RunTimedOut })
	if got.ExitCode != nil {
		t.Fatalf("timed out run = %#v, want no exit code", got)
	}
	if !testutil.WaitFor(3*time.Second, func() bool { return !groupAlive(*run.PID) }) {
		t.Fatal("timed out script group is still alive")
	}
}

func TestPlainStopInterruptsScriptRun(t *testing.T) {
	t.Parallel()
	h := newScriptHandoff(t)
	first := h.start(t, false, 0)
	run := h.runUntilLogged(t, first, "echo before; sleep 30", "before")
	first.Shutdown()
	if got, err := h.st.GetRun("worker", run.ID); err != nil || got.Status != script.RunInterrupted {
		t.Fatalf("run after plain stop = %#v err=%v, want interrupted", got, err)
	}
	if !testutil.WaitFor(3*time.Second, func() bool {
		return errors.Is(syscall.Kill(-*run.PID, 0), syscall.ESRCH)
	}) {
		t.Fatal("plain stop left the script group alive")
	}
}
