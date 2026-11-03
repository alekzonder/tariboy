package loop

import (
	"errors"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/proctree"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/testutil"
)

const probeTestTimeout = 200 * time.Millisecond

type pidShim struct {
	stubShim
	pid int
}

func (s pidShim) Status() shim.StatusResult { return shim.StatusResult{Running: true, PID: s.pid} }

// fakeProcs is a process table seam that counts reads.
type fakeProcs struct {
	procs []proctree.Proc
	delay time.Duration
	err   error
	reads atomic.Int32
}

func (f *fakeProcs) read([]int) ([]proctree.Proc, error) {
	f.reads.Add(1)
	time.Sleep(f.delay)
	return f.procs, f.err
}

type stallFixture struct {
	m     *Manager
	as    *agent.Store
	now   time.Time
	procs *fakeProcs
}

func newStallFixture(t *testing.T) *stallFixture {
	t.Helper()
	m, as, _, _ := newManager(t, &fakeRunner{})
	f := &stallFixture{m: m, as: as, now: time.Date(2026, 9, 10, 2, 10, 0, 0, time.UTC), procs: &fakeProcs{}}
	m.cfg.Clock = func() time.Time { return f.now }
	m.cfg.ReadProcs = f.procs.read
	m.cfg.StallProbeTimeout = probeTestTimeout
	return f
}

// stalled creates an agent whose adopted iteration made no AI-proxy request
// for longer than its stall timeout, served by a shim reporting harness pid.
func (f *stallFixture) stalled(t *testing.T, name string, pid int) agent.Agent {
	t.Helper()
	a := agent.Agent{Name: name, ImageRef: "basic:latest", Enabled: true, HarnessType: "stub", AIStallTimeoutS: 300}
	if err := f.as.Create(a); err != nil {
		t.Fatal(err)
	}
	f.iteration(t, name, name+"-1", pid)
	return a
}

func (f *stallFixture) iteration(t *testing.T, name, id string, pid int) {
	t.Helper()
	if err := f.as.CreateIteration(agent.Iteration{ID: id, Agent: name, Status: "running",
		StartedAt: store.FormatTime(f.now.Add(-6 * time.Minute))}); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(testutil.ShortDir(t), "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = shim.Serve(ln, pidShim{pid: pid}) }()
	f.m.runtimes.mu.Lock()
	f.m.runtimes.adopting[name] = agentdir.LiveIteration{Agent: name, ID: id, ShimSock: sock}
	f.m.runtimes.mu.Unlock()
}

func harnessChild(harness, pid int, comm string, args ...string) proctree.Proc {
	return proctree.Proc{PID: pid, PPID: harness, Comm: comm, Args: args}
}

func assertStatus(t *testing.T, got registry.LiveStatus, state, reason string) {
	t.Helper()
	if got.State != state || got.Reason != reason {
		t.Fatalf("status = %+v; want state %q reason %q", got, state, reason)
	}
}

func TestStalledIterationWaitingForAgentCommandIsRunning(t *testing.T) {
	f := newStallFixture(t)
	a := f.stalled(t, "worker", 500)
	f.procs.procs = []proctree.Proc{harnessChild(500, 501, "bash", "/bin/bash", "-c", "make check")}
	assertStatus(t, f.m.LiveStatusOf(a), "running", StateReasonAgentCommand)
	if got := f.m.LiveStateOf(a); got != "running" {
		t.Fatalf("LiveStateOf = %q; want running", got)
	}
}

func TestStalledIterationWithoutAgentCommandIsError(t *testing.T) {
	f := newStallFixture(t)
	a := f.stalled(t, "worker", 500)
	assertStatus(t, f.m.LiveStatusOf(a), "error", "")
}

func TestStalledIterationWithOnlyExcludedChildIsError(t *testing.T) {
	f := newStallFixture(t)
	a := f.stalled(t, "worker", 500)
	f.procs.procs = []proctree.Proc{harnessChild(500, 501, "node", "node", "mcp.js")}
	assertStatus(t, f.m.LiveStatusOf(a), "error", "")
}

// The probe excludes the hook commands the runner reported for the iteration.
func TestStalledIterationWithOnlyHookShellIsError(t *testing.T) {
	f := newStallFixture(t)
	a := f.stalled(t, "worker", 500)
	hook := "'/b/tmail' instant --format claude-stop"
	f.procs.procs = []proctree.Proc{harnessChild(500, 501, "sh", "/bin/sh", "-c", hook)}
	f.m.noteHookCommands("worker", "worker-1", []string{hook})
	assertStatus(t, f.m.LiveStatusOf(a), "error", "")
}

func TestRunnerReportsHookCommandsToTheManager(t *testing.T) {
	f := newStallFixture(t)
	f.m.cfg.RunnerFactory = nil
	r, ok := f.m.runnerFor(agent.Agent{Name: "worker"}).(*ShimRunner)
	if !ok || r.cfg.HookCommands == nil {
		t.Fatal("runner does not report hook commands to the manager")
	}
}

func TestStallProbeFailuresKeepError(t *testing.T) {
	t.Run("pid 0", func(t *testing.T) {
		f := newStallFixture(t)
		a := f.stalled(t, "worker", 0)
		f.procs.procs = []proctree.Proc{harnessChild(0, 501, "bash", "bash", "-c", "x")}
		assertStatus(t, f.m.LiveStatusOf(a), "error", "")
	})
	t.Run("shim unreachable", func(t *testing.T) {
		f := newStallFixture(t)
		a := f.stalled(t, "worker", 500)
		f.procs.procs = []proctree.Proc{harnessChild(500, 501, "bash", "bash", "-c", "x")}
		f.m.cfg.ConnectShim = func(string) (*shim.Client, error) { return nil, errors.New("no shim") }
		assertStatus(t, f.m.LiveStatusOf(a), "error", "")
	})
	t.Run("table error", func(t *testing.T) {
		f := newStallFixture(t)
		a := f.stalled(t, "worker", 500)
		f.procs.err = errors.New("no /proc")
		assertStatus(t, f.m.LiveStatusOf(a), "error", "")
	})
	t.Run("table timeout", func(t *testing.T) {
		f := newStallFixture(t)
		a := f.stalled(t, "worker", 500)
		f.procs.procs = []proctree.Proc{harnessChild(500, 501, "bash", "bash", "-c", "x")}
		f.procs.delay = 2 * probeTestTimeout
		assertStatus(t, f.m.LiveStatusOf(a), "error", "")
	})
}

func TestStallProbeIsCachedPerIteration(t *testing.T) {
	f := newStallFixture(t)
	a := f.stalled(t, "worker", 500)
	f.procs.procs = []proctree.Proc{harnessChild(500, 501, "bash", "bash", "-c", "x")}
	assertStatus(t, f.m.LiveStatusOf(a), "running", StateReasonAgentCommand)
	f.procs.procs = nil
	f.now = f.now.Add(10 * time.Second)
	assertStatus(t, f.m.LiveStatusOf(a), "running", StateReasonAgentCommand)
	if n := f.procs.reads.Load(); n != 1 {
		t.Fatalf("table reads = %d; want 1 within the cache period", n)
	}
	f.now = f.now.Add(10 * time.Second)
	assertStatus(t, f.m.LiveStatusOf(a), "error", "")
	if n := f.procs.reads.Load(); n != 2 {
		t.Fatalf("table reads = %d; want 2 after the cache period", n)
	}
}

func TestStallProbeCacheResetsForANewIteration(t *testing.T) {
	f := newStallFixture(t)
	a := f.stalled(t, "worker", 500)
	assertStatus(t, f.m.LiveStatusOf(a), "error", "")
	f.iteration(t, "worker", "worker-2", 600)
	f.procs.procs = []proctree.Proc{harnessChild(600, 601, "bash", "bash", "-c", "x")}
	assertStatus(t, f.m.LiveStatusOf(a), "running", StateReasonAgentCommand)
	if n := f.procs.reads.Load(); n != 2 {
		t.Fatalf("table reads = %d; want 2", n)
	}
}

func TestNoStallProbeBelowTheThreshold(t *testing.T) {
	f := newStallFixture(t)
	a := f.stalled(t, "worker", 500)
	if err := f.as.RecordAIRequest("worker", "worker-1", f.now); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, f.m.LiveStatusOf(a), "running", "")
	if n := f.procs.reads.Load(); n != 0 {
		t.Fatalf("table reads = %d; want no probe below the threshold", n)
	}
}

func TestLiveStatusesProbeStalledAgentsTogether(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delay time.Duration
		state string
	}{
		{"table answers in time", probeTestTimeout * 8 / 10, "running"},
		{"table answers late", 3 * probeTestTimeout, "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newStallFixture(t)
			a, b := f.stalled(t, "a", 500), f.stalled(t, "b", 600)
			f.procs.delay = tc.delay
			f.procs.procs = []proctree.Proc{harnessChild(500, 501, "bash", "bash", "-c", "x"), harnessChild(600, 601, "zsh", "zsh", "-c", "y")}
			start := time.Now()
			got := f.m.LiveStatuses([]agent.Agent{a, b})
			if elapsed := time.Since(start); elapsed >= probeTestTimeout*3/2 {
				t.Fatalf("LiveStatuses took %v; want under %v", elapsed, probeTestTimeout*3/2)
			}
			reason := ""
			if tc.state == "running" {
				reason = StateReasonAgentCommand
			}
			for _, s := range got {
				assertStatus(t, s, tc.state, reason)
			}
			if n := f.procs.reads.Load(); n != 1 {
				t.Fatalf("table reads = %d; want one for the whole list", n)
			}
		})
	}
}
