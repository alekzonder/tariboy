package operations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/store"
)

// EventType is the events hub type of an operation change: a new status or
// a new log line. The /api/operations/ws socket forwards it as a hint.
const EventType = "operation"

// Failure is an operation error with an API error code. Cause, when set, is
// the original error that Handle.Wait callers can unwrap.
type Failure struct {
	Code    string
	Message string
	Cause   error
}

func (f Failure) Error() string { return f.Code + ": " + f.Message }

func (f Failure) Unwrap() error { return f.Cause }

// RunFunc does the work of an operation. ctx ends when the daemon stops.
type RunFunc func(ctx context.Context, log *Log) (any, error)

// Handle is a started operation.
type Handle struct {
	Record Record
	done   chan struct{}
	result any
	err    error
}

// Wait returns the result of the operation, or ctx's error when ctx ends
// first. The operation itself goes on.
func (h *Handle) Wait(ctx context.Context) (any, error) {
	select {
	case <-h.done:
		return h.result, h.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Runner starts operations on the daemon's context, not on the context of the
// request that asked for them.
type Runner struct {
	store  *Store
	emit   func(events.Event)
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
}

// NewRunner returns a runner whose operations stop when parent ends or
// Shutdown runs. emit may be nil.
func NewRunner(parent context.Context, s *Store, emit func(events.Event), log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Runner{store: s, emit: emit, log: log, ctx: ctx, cancel: cancel}
}

// Store returns the store of the runner's operations.
func (r *Runner) Store() *Store { return r.store }

// Start records a running operation and runs it in the background.
func (r *Runner) Start(spec Spec, run RunFunc) (*Handle, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("the daemon is stopping")
	}
	r.wg.Add(1)
	r.mu.Unlock()
	rec, err := r.store.Create(spec)
	if err != nil {
		r.wg.Done()
		return nil, err
	}
	l, err := r.store.openLog(rec.ID, func() { r.notify(rec.ID, StatusRunning) })
	if err != nil {
		r.wg.Done()
		r.finish(rec.ID, StatusFailed, nil, "internal", "open the operation log: "+err.Error())
		return nil, err
	}
	h := &Handle{Record: rec, done: make(chan struct{})}
	r.notify(rec.ID, StatusRunning)
	go r.run(h, l, run)
	return h, nil
}

func (r *Runner) run(h *Handle, l *Log, run RunFunc) {
	defer r.wg.Done()
	started := time.Now()
	l.Printf("%s: started", h.Record.Title)
	result, err := protect(r.ctx, l, run)
	status, code, message := StatusSucceeded, "", ""
	var failure Failure
	switch {
	case err == nil:
		l.Printf("succeeded in %s", time.Since(started).Round(time.Millisecond))
	case r.ctx.Err() != nil:
		status, code, message = StatusInterrupted, "interrupted", "the daemon stopped during the operation"
		l.Printf("interrupted: %s (%v)", message, err)
	case errors.As(err, &failure):
		status, code, message = StatusFailed, failure.Code, failure.Message
	default:
		status, code, message = StatusFailed, "failed", err.Error()
	}
	if status == StatusFailed {
		l.Printf("failed: %s: %s", code, message)
	}
	if status != StatusSucceeded {
		result = nil
	}
	l.close()
	r.finish(h.Record.ID, status, result, code, message)
	h.result, h.err = result, err
	close(h.done)
}

func protect(ctx context.Context, l *Log, run RunFunc) (result any, err error) {
	defer func() {
		if p := recover(); p != nil {
			result, err = nil, Failure{Code: "internal", Message: fmt.Sprintf("panic: %v", p)}
		}
	}()
	return run(ctx, l)
}

func (r *Runner) finish(id int64, status string, result any, code, message string) {
	if err := r.store.Finish(id, status, result, code, message); err != nil {
		r.log.Error("record operation result", "id", id, "err", err)
	}
	r.notify(id, status)
}

func (r *Runner) notify(id int64, status string) {
	if r.emit == nil {
		return
	}
	r.emit(events.Event{Type: EventType, Time: store.FormatTime(time.Now()), Data: map[string]any{"id": id, "status": status}})
}

// Shutdown stops new operations, cancels the running ones and waits until
// each has recorded its result.
func (r *Runner) Shutdown() {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.cancel()
	r.wg.Wait()
}
