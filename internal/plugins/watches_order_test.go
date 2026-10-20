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
	"github.com/alekzonder/tariboy/internal/testutil"
)

// orderRecorder is a fake plugin socket that records the ORDER of the watch
// snapshots it receives on POST /watches. To force the delivery-ordering race
// deterministically it holds any NON-EMPTY snapshot until the test closes
// release, signalling held when one arrives, while empty snapshots ack
// immediately. Under the old goroutine-per-event push, the fast empty ([])
// delivery would then land before the held non-empty ([w]) one, leaving the
// provider reconciled to a watch that is gone.
type orderRecorder struct {
	held    chan struct{} // receives one value per non-empty snapshot that arrives
	release chan struct{} // closed by the test to ack the non-empty snapshots

	mu     sync.Mutex
	counts []int // number of watches in each snapshot, in arrival (ack) order
}

func (or *orderRecorder) serve(t *testing.T, sock string) {
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
		if len(body.Watches) > 0 {
			or.held <- struct{}{}
			select {
			case <-or.release:
			case <-r.Context().Done():
				return
			}
		}
		or.mu.Lock()
		or.counts = append(or.counts, len(body.Watches))
		or.mu.Unlock()
		w.WriteHeader(200)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
}

func newOrderRecorder() *orderRecorder {
	return &orderRecorder{held: make(chan struct{}, 8), release: make(chan struct{})}
}

// awaitHeld waits until a non-empty snapshot is held in the handler.
func (or *orderRecorder) awaitHeld(t *testing.T) {
	t.Helper()
	select {
	case <-or.held:
	case <-time.After(3 * time.Second):
		t.Fatal("no non-empty snapshot reached the provider")
	}
}

func (or *orderRecorder) recorded() []int {
	or.mu.Lock()
	defer or.mu.Unlock()
	out := make([]int, len(or.counts))
	copy(out, or.counts)
	return out
}

func (or *orderRecorder) reset() {
	or.mu.Lock()
	or.counts = nil
	or.mu.Unlock()
}

func awaitStartupPush(t *testing.T, or *orderRecorder) {
	t.Helper()
	if !testutil.WaitFor(2*time.Second, func() bool { return len(or.recorded()) > 0 }) {
		t.Fatal("provider received no startup watch reconciliation")
	}
	or.reset()
}

// waitPushDrain blocks until every serialized watch-push worker has finished
// (the pushQueue is empty) or the deadline passes. Unlike h.wg.Wait() it does
// not wait on the long-lived plugin supervisor goroutine, which also rides h.wg.
func waitPushDrain(t *testing.T, h *Host) {
	t.Helper()
	if !testutil.WaitFor(3*time.Second, func() bool {
		h.pushMu.Lock()
		defer h.pushMu.Unlock()
		return len(h.pushQueue) == 0
	}) {
		t.Fatal("watch-push workers did not drain within deadline")
	}
}

// startProvider brings up a fake running provider plugin backed by rec, serving
// its socket with h (the shared newHost/fakeRunner scaffolding).
func startProvider(t *testing.T, h *Host, ps *Store, name string, srv interface {
	serve(*testing.T, string)
}) {
	t.Helper()
	rec := sampleRecord(name)
	rec.Channels = Channels{
		Publish: []string{name + ":*"},
		Provide: []Provided{{Channel: name + ":query"}},
	}
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath(name)
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, h, name)
	srv.serve(t, sock)
	h.cfg.Runner = &fakeRunner{line: `{"name":"` + name + `","version":"0.1.0","types":["channel-source"],"protocol_version":1,"socket":"` + sock + `"}`}
	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, name)
}

// TestPushWatchesDeliversInEventOrder is the regression guard for dev-t-arn.7
// (review finding #1): a subscribe (push [w]) immediately followed by an
// unsubscribe (push []) must leave the provider reconciled to the FINAL state —
// the empty list — even when the [w] delivery is much slower than the []
// delivery. The serialized per-(plugin,channel) push guarantees this; the old
// goroutine-per-event push would let [] overtake [w] and end on [w].
func TestPushWatchesDeliversInEventOrder(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, nil)
	rec := newOrderRecorder()
	startProvider(t, h, ps, "issues", rec)
	awaitStartupPush(t, rec)

	watch := []bus.WatchInfo{{Watch: "a1b2c3", Params: map[string]any{"q": "X"}, Subscribers: []string{"dev-worker"}}}

	// Event 1: subscribe -> full list [w]. Event 2: unsubscribe -> []. Waiting
	// until the provider holds [w] means the serializer's worker picked it up
	// before [] arrives, so both are delivered (in order) rather than []
	// coalescing [w] away — this exercises the ordering path, not just the
	// final-state coalescing path.
	h.PushWatches("issues", "issues:query", watch)
	rec.awaitHeld(t)
	h.PushWatches("issues", "issues:query", nil)
	// The [] snapshot waits behind the in-flight [w] instead of being sent.
	h.pushMu.Lock()
	wp := h.pushQueue["issues\x00issues:query"]
	queued := wp != nil && wp.hasPending && len(wp.pending) == 0
	h.pushMu.Unlock()
	if !queued {
		t.Fatal("the empty snapshot was not queued behind the in-flight one")
	}
	close(rec.release)

	waitPushDrain(t, h)

	got := rec.recorded()
	if len(got) == 0 {
		t.Fatal("provider received no watch push")
	}
	// The authoritative check: the LAST snapshot the provider saw is the empty
	// one. This is what "provider converges on final subscription state" means and
	// is exactly what the ordering race broke.
	if last := got[len(got)-1]; last != 0 {
		t.Fatalf("provider's final snapshot has %d watches, want 0 (empty); full order = %v — a stale non-empty snapshot won the race", last, got)
	}
	// And a non-empty snapshot must never arrive after an empty one.
	seenEmpty := false
	for _, n := range got {
		if n == 0 {
			seenEmpty = true
		} else if seenEmpty {
			t.Fatalf("non-empty snapshot (%d watches) delivered AFTER an empty one; order = %v — out-of-order delivery", n, got)
		}
	}
}

// TestPushWatchesCoalescesStaleSnapshots verifies the other half of the
// contract: when several snapshots pile up while one is in flight, only the
// newest survives — the provider is not walked through every intermediate list.
func TestPushWatchesCoalescesStaleSnapshots(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, nil)
	rec := newOrderRecorder()
	startProvider(t, h, ps, "issues", rec)
	awaitStartupPush(t, rec)

	one := []bus.WatchInfo{{Watch: "w1", Subscribers: []string{"a"}}}
	two := []bus.WatchInfo{{Watch: "w1", Subscribers: []string{"a"}}, {Watch: "w2", Subscribers: []string{"b"}}}
	three := append(append([]bus.WatchInfo{}, two...), bus.WatchInfo{Watch: "w3", Subscribers: []string{"c"}})

	// First push is held in the handler. While it is in flight we enqueue two
	// more; only the last (three watches) should be delivered after the first
	// completes — the middle one is coalesced away.
	h.PushWatches("issues", "issues:query", one)
	rec.awaitHeld(t) // the worker grabbed `one`
	h.PushWatches("issues", "issues:query", two)
	h.PushWatches("issues", "issues:query", three)
	close(rec.release)

	waitPushDrain(t, h)

	got := rec.recorded()
	if len(got) == 0 {
		t.Fatal("provider received no watch push")
	}
	if last := got[len(got)-1]; last != 3 {
		t.Fatalf("final snapshot has %d watches, want 3 (newest); order = %v", last, got)
	}
	// The `two`-watch intermediate snapshot must have been coalesced away: at most
	// two deliveries total (the in-flight `one`, then the coalesced `three`).
	if len(got) > 2 {
		t.Fatalf("expected coalescing to at most 2 deliveries, got %d: %v", len(got), got)
	}
}
