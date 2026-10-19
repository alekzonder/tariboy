package loop

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/shim"
)

// countingShim answers probes while alive and counts Kill requests.
type countingShim struct {
	kills    atomic.Int32
	deadline atomic.Value
}

func (s *countingShim) Status() shim.StatusResult          { return shim.StatusResult{Running: true} }
func (s *countingShim) Kill() error                        { s.kills.Add(1); return nil }
func (s *countingShim) Screen() (string, error)            { return "", nil }
func (s *countingShim) SendKeys(shim.SendKeysParams) error { return nil }
func (s *countingShim) Report() shim.ReportResult          { return shim.ReportResult{} }
func (s *countingShim) SetHardDeadline(d string) error     { s.deadline.Store(d); return nil }

type watcherFixture struct {
	store *agent.Store
	w     iterationWatcher
	shim  *countingShim
	ln    net.Listener
}

func newWatcherFixture(t *testing.T) *watcherFixture {
	t.Helper()
	_, as, _, _ := newManager(t, &fakeRunner{})
	if err := as.Create(agent.Agent{Name: "w", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	id := "w-20260101000000-1"
	if err := as.CreateIteration(agent.Iteration{ID: id, Agent: "w", Trigger: "manual", Status: "running",
		StartedAt: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "watch")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &watcherFixture{store: as, shim: &countingShim{}}
	f.w = iterationWatcher{
		store: as, clock: time.Now, log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		agent: "w", id: id, resultPath: filepath.Join(dir, "result.json"),
		shimSock: filepath.Join(dir, "shim.sock"), poll: 5 * time.Millisecond, doneGrace: 20 * time.Millisecond,
	}
	f.ln, err = net.Listen("unix", f.w.shimSock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.ln.Close() })
	go func() { _ = shim.Serve(f.ln, f.shim) }()
	return f
}

func (f *watcherFixture) writeResultAfter(t *testing.T, d time.Duration, body string) {
	t.Helper()
	time.AfterFunc(d, func() { _ = os.WriteFile(f.w.resultPath, []byte(body), 0o600) })
}

func (f *watcherFixture) watch(t *testing.T) (watchResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return f.w.watch(ctx)
}

func TestIterationWatcherReturnsResult(t *testing.T) {
	f := newWatcherFixture(t)
	f.writeResultAfter(t, 20*time.Millisecond, `{"exit_code":3,"termination_reason":"hard_timeout"}`)
	res, err := f.watch(t)
	if err != nil || res.stale || res.softHit {
		t.Fatalf("watch = %+v, %v", res, err)
	}
	if res.result.ExitCode != 3 || res.result.TerminationReason != "hard_timeout" {
		t.Fatalf("result = %+v", res.result)
	}
	if got := f.shim.kills.Load(); got != 0 {
		t.Fatalf("kills = %d, want 0", got)
	}
}

func TestIterationWatcherSoftDeadlineAndDoneGraceSendOneKill(t *testing.T) {
	f := newWatcherFixture(t)
	if err := f.store.InitializeIterationTimeout(f.w.id, 1, 120, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetIterationDone(f.w.id, true); err != nil {
		t.Fatal(err)
	}
	f.writeResultAfter(t, 200*time.Millisecond, `{"exit_code":0}`)
	res, err := f.watch(t)
	if err != nil || !res.softHit {
		t.Fatalf("watch = %+v, %v; want the soft deadline hit", res, err)
	}
	if got := f.shim.kills.Load(); got != 1 {
		t.Fatalf("kills = %d, want 1", got)
	}
	if d, _ := f.shim.deadline.Load().(string); d == "" {
		t.Fatal("persisted hard deadline was not resynced to the shim")
	}
}

func TestIterationWatcherDoneGraceKillsOnce(t *testing.T) {
	f := newWatcherFixture(t)
	if err := f.store.SetIterationDone(f.w.id, true); err != nil {
		t.Fatal(err)
	}
	f.writeResultAfter(t, 200*time.Millisecond, `{"exit_code":0}`)
	res, err := f.watch(t)
	if err != nil || res.softHit {
		t.Fatalf("watch = %+v, %v", res, err)
	}
	if got := f.shim.kills.Load(); got != 1 {
		t.Fatalf("kills = %d, want 1", got)
	}
}

func TestIterationWatcherProbeMissesMarkStale(t *testing.T) {
	f := newWatcherFixture(t)
	f.ln.Close()
	f.w.probe = &livenessProbe{every: time.Millisecond, maxMisses: 3}
	f.w.rpcTimeout = 50 * time.Millisecond
	res, err := f.watch(t)
	if err != nil || !res.stale {
		t.Fatalf("watch = %+v, %v; want stale", res, err)
	}
}

func TestIterationWatcherDetachesOnCancel(t *testing.T) {
	f := newWatcherFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	if _, err := f.w.watch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("watch err = %v, want context.Canceled", err)
	}
}
