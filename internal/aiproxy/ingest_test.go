package aiproxy

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestIngesterBatchFlush(t *testing.T) {
	s := newStore(t)
	ing := NewIngester(s, discardLogger())
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ing.Enqueue(sampleReq("air-"+string(rune('a'+i)), "alice", "basic", 0.01, base))
	}
	if err := ing.Flush(); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Aggregate(UsageFilter{Agent: "alice"})
	if len(rows) != 1 || rows[0].Requests != 5 {
		t.Fatalf("flushed rows = %+v", rows)
	}
}

// TestIngesterEnqueueDropsWhenFull verifies the hot path never blocks: with no
// drainer running, filling the buffer past capacity drops the extra rows rather
// than deadlocking (they are recoverable via daemon reindex from the JSONL).
func TestIngesterEnqueueDropsWhenFull(t *testing.T) {
	s := newStore(t)
	ing := NewIngester(s, discardLogger())
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	total := ingestBuffer + 100
	for i := 0; i < total; i++ {
		ing.Enqueue(sampleReq(NewRequestID(nil), "bob", "basic", 0.01, base))
	}
	if err := ing.Flush(); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Aggregate(UsageFilter{Agent: "bob"})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	if got := rows[0].Requests; got != ingestBuffer {
		t.Fatalf("buffered = %d, want %d (extras dropped)", got, ingestBuffer)
	}
}

// TestIngesterDroppedCounter asserts the atomic drop counter tracks exactly the
// rows dropped on buffer-full (the extras beyond capacity), so overflow is
// observable/rate-limitable instead of a per-row log flood.
func TestIngesterDroppedCounter(t *testing.T) {
	s := newStore(t)
	ing := NewIngester(s, discardLogger())
	if got := ing.Dropped(); got != 0 {
		t.Fatalf("initial dropped = %d, want 0", got)
	}
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	const extra = 150
	total := ingestBuffer + extra
	for i := 0; i < total; i++ {
		ing.Enqueue(sampleReq(NewRequestID(nil), "dave", "basic", 0.01, base))
	}
	if got := ing.Dropped(); got != extra {
		t.Fatalf("dropped = %d, want %d", got, extra)
	}
}

// TestIngesterRunDrainsOnCancel exercises the concurrent path under -race: a
// running drainer, many concurrent Enqueues, then ctx cancel drains and flushes
// everything buffered. Total (4000) stays under the buffer (4096) so no drops.
func TestIngesterRunDrainsOnCancel(t *testing.T) {
	s := newStore(t)
	ing := NewIngester(s, discardLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { ing.Run(ctx); close(done) }()

	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	const perGoroutine, goroutines = 1000, 4
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ing.Enqueue(sampleReq(NewRequestID(nil), "carol", "basic", 0.01, base))
			}
		}()
	}
	wg.Wait()
	cancel()
	<-done

	rows, _ := s.Aggregate(UsageFilter{Agent: "carol"})
	if len(rows) != 1 || rows[0].Requests != perGoroutine*goroutines {
		t.Fatalf("rows = %+v, want %d requests", rows, perGoroutine*goroutines)
	}
}
