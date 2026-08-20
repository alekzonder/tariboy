package plugins

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
)

// watchRecorder is a fake plugin socket serving /health and /watches; it records
// the last push body so a test can assert what the daemon delivered.
type watchRecorder struct {
	mu   sync.Mutex
	last ChannelWatchesDTO
	hits int
}

func (wr *watchRecorder) serve(t *testing.T, sock string) {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/watches", func(w http.ResponseWriter, r *http.Request) {
		var body ChannelWatchesDTO
		_ = json.NewDecoder(r.Body).Decode(&body)
		wr.mu.Lock()
		wr.last, wr.hits = body, wr.hits+1
		wr.mu.Unlock()
		w.WriteHeader(200)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
}

func (wr *watchRecorder) snapshot() (ChannelWatchesDTO, int) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	return wr.last, wr.hits
}

func (wr *watchRecorder) reset() {
	wr.mu.Lock()
	wr.last, wr.hits = ChannelWatchesDTO{}, 0
	wr.mu.Unlock()
}

// TestHostPushWatchesReachesRunningPlugin is the §6.2 push integration: a
// running provider plugin receives POST /watches carrying the channel's full
// current watch list.
func TestHostPushWatchesReachesRunningPlugin(t *testing.T) {
	h, _, ps := newHost(t, nil)
	rec := sampleRecord("issues")
	rec.Channels = Channels{
		Publish: []string{"issues:*"},
		Provide: []Provided{{Channel: "issues:query"}},
	}
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("issues")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, h, "issues")
	wr := &watchRecorder{}
	wr.serve(t, sock)
	h.cfg.Runner = &fakeRunner{line: `{"name":"issues","version":"0.1.0","types":["channel-source"],"protocol_version":1,"socket":"` + sock + `"}`}

	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "issues")
	startupDeadline := time.Now().Add(2 * time.Second)
	for {
		_, hits := wr.snapshot()
		if hits > 0 {
			break
		}
		if time.Now().After(startupDeadline) {
			t.Fatal("plugin never received startup watch reconciliation")
		}
		time.Sleep(10 * time.Millisecond)
	}

	h.PushWatches("issues", "issues:query", []bus.WatchInfo{
		{Watch: "a1b2c3", Params: map[string]any{"query": "PROJ"}, Subscribers: []string{"dev-manager", "dev-worker"}},
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		got, hits := wr.snapshot()
		if hits > 0 && len(got.Watches) == 1 {
			if got.Channel != "issues:query" || len(got.Watches) != 1 {
				t.Fatalf("push body = %+v", got)
			}
			if got.Watches[0].Watch != "a1b2c3" || len(got.Watches[0].Subscribers) != 2 {
				t.Fatalf("pushed watch = %+v", got.Watches[0])
			}
			if got.Watches[0].Params["query"] != "PROJ" {
				t.Fatalf("params not carried in push: %+v", got.Watches[0].Params)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("plugin never received the watch push")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHostRunningTransitionPushesCurrentWatches(t *testing.T) {
	h, b, ps := newHost(t, nil)
	rec := sampleRecord("issues")
	rec.Channels = Channels{Provide: []Provided{{Channel: "issues:query"}}}
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SubscribeParams("dev", "issues:query", bus.Matcher{}, nil,
		map[string]any{"query": "PROJ"}); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("issues")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, h, "issues")
	wr := &watchRecorder{}
	wr.serve(t, sock)
	h.cfg.Runner = &fakeRunner{line: `{"name":"issues","version":"0.1.0","types":["channel-source"],"protocol_version":1,"socket":"` + sock + `"}`}

	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "issues")
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, hits := wr.snapshot()
		if hits > 0 {
			if got.Channel != "issues:query" || len(got.Watches) != 1 || got.Watches[0].Params["query"] != "PROJ" {
				t.Fatalf("startup watch push = %+v", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("running transition did not reconcile current provider watches")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStartupRefreshReadsCurrentWatchesAtDeliveryBoundary(t *testing.T) {
	h, b, ps := newHost(t, nil)
	rec := sampleRecord("issues")
	rec.Channels = Channels{Provide: []Provided{{Channel: "issues:query"}}}
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("issues")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, h, "issues")
	wr := &watchRecorder{}
	wr.serve(t, sock)
	h.cfg.Runner = &fakeRunner{line: `{"name":"issues","version":"0.1.0","types":["channel-source"],"protocol_version":1,"socket":"` + sock + `"}`}
	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "issues")
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, hits := wr.snapshot()
		if hits > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial startup refresh missing")
		}
		time.Sleep(5 * time.Millisecond)
	}
	wr.reset()

	// Hold the delivery queue so startup refresh is requested first but cannot
	// read. Commit the subscription before allowing that refresh to proceed.
	h.pushMu.Lock()
	queued := make(chan struct{})
	go func() {
		close(queued)
		h.pushCurrentWatches("issues", "issues:query")
	}()
	<-queued
	if _, err := b.SubscribeParams("dev", "issues:query", bus.Matcher{}, nil,
		map[string]any{"query": "LATEST"}); err != nil {
		h.pushMu.Unlock()
		t.Fatal(err)
	}
	h.pushMu.Unlock()

	deadline = time.Now().Add(2 * time.Second)
	for {
		got, hits := wr.snapshot()
		if hits > 0 && len(got.Watches) == 1 {
			if got.Watches[0].Params["query"] != "LATEST" {
				t.Fatalf("startup refresh delivered stale watches: %+v", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("startup refresh did not deliver subscription committed before delivery")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestHostPushWatchesPluginNotRunning is a no-op fast path: pushing to a plugin
// that is not running returns immediately (the pull path reconciles later)
// rather than blocking or panicking.
func TestHostPushWatchesPluginNotRunning(t *testing.T) {
	h, _, _ := newHost(t, nil)
	h.baseCtx = context.Background()
	done := make(chan struct{})
	go func() {
		h.PushWatches("ghost", "issues:query", nil)
		h.wgWaitForTest()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PushWatches to a non-running plugin should return promptly")
	}
}

// wgWaitForTest drains the host's push goroutines so the not-running test can
// assert the goroutine exited without touching unexported fields elsewhere.
func (h *Host) wgWaitForTest() { h.wg.Wait() }
