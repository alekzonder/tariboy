package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/store"
)

func TestShutdownWaitsForScheduler(t *testing.T) {
	base := t.TempDir()
	t.Setenv("TARIBOY_RUNTIME_DIR", t.TempDir())
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	opts := daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error"})
	opts.schedulerRun = func(ctx context.Context, _ *schedule.Scheduler) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		t.Fatal("scheduler did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(30 * time.Second):
		t.Fatal("scheduler did not observe cancellation")
	}
	select {
	case err := <-done:
		t.Fatalf("daemon returned before scheduler drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("daemon did not finish after scheduler drained")
	}
}

// TestShutdownDrainsIngesterBeforeStoreClose is the regression test for the
// M5 review finding: the AI-proxy ingester and budget-cache refresher touch
// the store (InsertBatch / Refresh) from background goroutines that were not
// awaited before defer st.Close() ran, so the ingester's final drain-flush
// could race db.Close() and silently lose the last buffered batch of
// ai_requests rows.
//
// It boots a real daemon (via Run), grabs the real *aiproxy.Ingester it wires
// up (through the unexported wireHook test seam), enqueues a batch of rows
// directly into it -- exactly the path proxy.persist takes on the hot path --
// then triggers shutdown and waits for Run to return. Only after Run has
// returned (i.e. after every deferred goroutine-drain and st.Close() ran) does
// the test reopen the on-disk store and assert every enqueued row landed. If
// the drain-before-close ordering regressed, either Run would hang (wg.Wait
// stuck) or some rows would be missing (lost to "database is closed").
func TestShutdownDrainsIngesterBeforeStoreClose(t *testing.T) {
	base := t.TempDir()
	runtimeDir := t.TempDir()
	t.Setenv("TARIBOY_RUNTIME_DIR", runtimeDir)

	ready := make(chan struct{})
	var ing *aiproxy.Ingester
	opts := daemonTestOptions(Options{BaseDir: base, Listen: "unix", HTTPAddr: "", LogLevel: "error"})
	opts.wireHook = func(i *aiproxy.Ingester, _ *aiproxy.Store) {
		ing = i
		close(ready)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()

	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		t.Fatal("daemon never wired the AI-proxy ingester")
	}

	sock := paths.New(base).Socket()
	c := client.New(sock)
	deadline := time.Now().Add(30 * time.Second)
	var upErr error
	for time.Now().Before(deadline) {
		if _, upErr = c.Call("GET", "/api/daemon/status", nil); upErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if upErr != nil {
		t.Fatalf("daemon never came up: %v", upErr)
	}

	const n = 50
	baseTS := time.Now().UTC()
	for i := 0; i < n; i++ {
		ing.Enqueue(aiproxy.AIRequest{
			ID:             fmt.Sprintf("air-shutdown-test-%d", i),
			TS:             baseTS.Format(time.RFC3339Nano),
			Agent:          "shutdowntest",
			Iteration:      "iter-1",
			ImageName:      "basic",
			ImageTag:       "latest",
			ImageDigest:    "sha256:x",
			Provider:       "anthropic",
			Model:          "claude-opus-4-8",
			InputTokens:    10,
			OutputTokens:   5,
			CostUSD:        0.001,
			LatencyMs:      1,
			Status:         "ok",
			UpstreamStatus: 200,
		})
	}

	// Trigger shutdown immediately after enqueuing: the whole point is to race
	// the ingester's final drain-flush against store close, and win.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("daemon did not shut down (ingester/refresher goroutine may be stuck)")
	}

	// Run has now returned, meaning every deferred cleanup -- including
	// cancel()+wg.Wait() for the ingester/refresher and, last, st.Close() --
	// already ran. Reopen the on-disk DB and check every enqueued row is there.
	reopened, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	aiReopened := aiproxy.NewStore(reopened, time.Now)
	rows, err := aiReopened.Aggregate(aiproxy.UsageFilter{Agent: "shutdowntest"})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) != 1 || rows[0].Requests != n {
		t.Fatalf("persisted rows = %+v, want exactly %d requests for shutdowntest "+
			"(missing rows means the final flush lost the race against store close)", rows, n)
	}
}
