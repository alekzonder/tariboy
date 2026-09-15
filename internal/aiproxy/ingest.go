package aiproxy

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

const (
	ingestBuffer = 4096
	ingestFlushN = 256
	ingestPeriod = time.Second
	// ingestWarnEvery rate-limits the drop warning: under sustained overflow we
	// warn once per this many drops instead of one line per dropped row.
	ingestWarnEvery = 256
)

// Ingester batches ai_requests rows off the hot path (spec §9): the proxy
// Enqueues metadata; a background goroutine flushes periodically or by size.
type Ingester struct {
	store   *Store
	log     *slog.Logger
	ch      chan AIRequest
	flush   chan chan error
	done    chan struct{}
	running atomic.Bool
	dropped atomic.Int64
}

func NewIngester(s *Store, log *slog.Logger) *Ingester {
	if log == nil {
		log = slog.Default()
	}
	return &Ingester{store: s, log: log, ch: make(chan AIRequest, ingestBuffer), flush: make(chan chan error), done: make(chan struct{})}
}

// Enqueue is non-blocking: a full buffer drops the row (recoverable via reindex).
// Drops are counted (see Dropped) and the warning is rate-limited so sustained
// overflow does not emit one log line per dropped row.
func (i *Ingester) Enqueue(r AIRequest) {
	select {
	case i.ch <- r:
	default:
		n := i.dropped.Add(1)
		if n%ingestWarnEvery == 1 {
			i.log.Warn("ai_requests ingest buffer full; dropping rows (rebuild via daemon reindex)",
				"id", r.ID, "dropped_total", n)
		}
	}
}

// Dropped returns the total number of rows dropped due to a full buffer.
func (i *Ingester) Dropped() int64 { return i.dropped.Load() }

func (i *Ingester) drain(batch *[]AIRequest) {
	for {
		select {
		case r := <-i.ch:
			*batch = append(*batch, r)
		default:
			return
		}
	}
}

func (i *Ingester) Run(ctx context.Context) {
	i.running.Store(true)
	defer func() {
		i.running.Store(false)
		close(i.done)
	}()
	tk := time.NewTicker(ingestPeriod)
	defer tk.Stop()
	batch := make([]AIRequest, 0, ingestFlushN)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := i.store.InsertBatch(batch); err != nil {
			i.log.Error("ai_requests batch insert", "n", len(batch), "err", err)
			return err
		}
		batch = batch[:0]
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			// Drain what is buffered, then flush and return.
			i.drain(&batch)
			_ = flush()
			return
		case r := <-i.ch:
			batch = append(batch, r)
			if len(batch) >= ingestFlushN {
				_ = flush()
			}
		case done := <-i.flush:
			i.drain(&batch)
			done <- flush()
		case <-tk.C:
			_ = flush()
		}
	}
}

// Flush drains and inserts everything currently buffered (used by tests and the
// reindex-adjacent paths).
func (i *Ingester) Flush() error {
	if i.running.Load() {
		done := make(chan error, 1)
		select {
		case i.flush <- done:
			select {
			case err := <-done:
				return err
			case <-i.done:
			}
		case <-i.done:
		}
	}
	var batch []AIRequest
	for {
		select {
		case r := <-i.ch:
			batch = append(batch, r)
		default:
			return i.store.InsertBatch(batch)
		}
	}
}
