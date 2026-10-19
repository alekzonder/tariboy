package loop

import (
	"sync/atomic"
	"testing"
)

func newTestMessageWake(state uint32) *messageWake {
	w := newMessageWake()
	w.state.Store(state)
	return w
}

func probe(v bool, calls *int) func() bool {
	return func() bool {
		*calls++
		return v
	}
}

func TestMessageWakeQueue(t *testing.T) {
	tests := []struct {
		name         string
		state        uint32
		prefill      bool
		wantState    uint32
		wantSignals  int
		wantDeferred bool
	}{
		{name: "idle queues one signal", state: messageWakeIdle, wantState: messageWakeQueued, wantSignals: 1},
		{name: "idle with signal already queued stays idle", state: messageWakeIdle, prefill: true,
			wantState: messageWakeIdle, wantSignals: 1},
		{name: "queued coalesces", state: messageWakeQueued, wantState: messageWakeQueued},
		{name: "retry coalesces", state: messageWakeRetry, wantState: messageWakeRetry},
		{name: "running defers", state: messageWakeRunning, wantState: messageWakeRunning, wantDeferred: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestMessageWake(tt.state)
			if tt.prefill {
				w.ch <- struct{}{}
			}
			w.queue()
			w.queue()
			if got := w.state.Load(); got != tt.wantState {
				t.Fatalf("state = %d, want %d", got, tt.wantState)
			}
			if got := len(w.ch); got != tt.wantSignals {
				t.Fatalf("signals = %d, want %d", got, tt.wantSignals)
			}
			if got := w.deferred.Load(); got != tt.wantDeferred {
				t.Fatalf("deferred = %v, want %v", got, tt.wantDeferred)
			}
		})
	}
}

func TestMessageWakeBeginMarksRunning(t *testing.T) {
	w := newTestMessageWake(messageWakeQueued)
	w.begin()
	if got := w.state.Load(); got != messageWakeRunning {
		t.Fatalf("state = %d, want running", got)
	}
}

func TestMessageWakeFinish(t *testing.T) {
	tests := []struct {
		name        string
		deferred    bool
		wantState   uint32
		wantSignals int
	}{
		{name: "no deferred publish goes idle", wantState: messageWakeIdle},
		{name: "deferred publish requeues", deferred: true, wantState: messageWakeQueued, wantSignals: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestMessageWake(messageWakeRunning)
			w.deferred.Store(tt.deferred)
			w.finish()
			if got := w.state.Load(); got != tt.wantState {
				t.Fatalf("state = %d, want %d", got, tt.wantState)
			}
			if got := len(w.ch); got != tt.wantSignals {
				t.Fatalf("signals = %d, want %d", got, tt.wantSignals)
			}
			if w.deferred.Load() {
				t.Fatal("finish left the deferred flag set")
			}
		})
	}
}

func TestMessageWakePublishDuringRunningRequeuesOnce(t *testing.T) {
	w := newTestMessageWake(messageWakeIdle)
	w.queue()
	<-w.ch
	w.begin()
	for range 5 {
		w.queue()
	}
	if !w.deferred.Load() {
		t.Fatal("publish during running did not set deferred")
	}
	if got := len(w.ch); got != 0 {
		t.Fatalf("publish during running signalled %d times, want 0", got)
	}
	w.finish()
	if got := len(w.ch); got != 1 {
		t.Fatalf("finish queued %d signals, want exactly 1", got)
	}
	if got := w.state.Load(); got != messageWakeQueued {
		t.Fatalf("state = %d, want queued", got)
	}
}

func TestMessageWakeParkDropsDeferred(t *testing.T) {
	w := newTestMessageWake(messageWakeRunning)
	w.deferred.Store(true)
	w.park()
	if got := w.state.Load(); got != messageWakeRetry {
		t.Fatalf("state = %d, want retry", got)
	}
	if w.deferred.Load() {
		t.Fatal("park kept the deferred flag")
	}
	if got := len(w.ch); got != 0 {
		t.Fatalf("park signalled %d times, want 0", got)
	}
}

func TestMessageWakeAbandonResetsIdle(t *testing.T) {
	for _, state := range []uint32{messageWakeQueued, messageWakeRunning, messageWakeRetry} {
		w := newTestMessageWake(state)
		w.deferred.Store(true)
		w.abandon()
		if got := w.state.Load(); got != messageWakeIdle {
			t.Fatalf("from %d: state = %d, want idle", state, got)
		}
		if w.deferred.Load() {
			t.Fatalf("from %d: abandon kept the deferred flag", state)
		}
	}
}

func TestMessageWakeArmRetryCreatesAtMostOneTimer(t *testing.T) {
	f := &fakeTimerFactory{}
	w := newTestMessageWake(messageWakeRunning)
	w.armRetry(f.newTimer)
	w.armRetry(f.newTimer)
	if got := f.createdCount(); got != 1 {
		t.Fatalf("armRetry created %d timers, want 1", got)
	}
	if got := f.lastDelay(); got != messageRetryDelay {
		t.Fatalf("retry delay = %v, want %v", got, messageRetryDelay)
	}
	if got := w.state.Load(); got != messageWakeRetry {
		t.Fatalf("state = %d, want retry", got)
	}
	if w.retryC == nil || !w.retryArmed() {
		t.Fatal("armRetry did not expose the retry channel")
	}
}

func TestMessageWakeStopRetry(t *testing.T) {
	f := &fakeTimerFactory{}
	w := newTestMessageWake(messageWakeRunning)
	w.armRetry(f.newTimer)
	timer := f.lastTimer()
	w.stopRetry()
	w.stopRetry()
	if got := atomic.LoadInt32(&timer.stopped); got != 1 {
		t.Fatalf("retry timer stopped %d times, want 1", got)
	}
	if w.retryArmed() || w.retryC != nil {
		t.Fatal("stopRetry left the retry armed")
	}
}

func TestMessageWakeRetryFiredDetachesWithoutStop(t *testing.T) {
	f := &fakeTimerFactory{}
	w := newTestMessageWake(messageWakeRunning)
	w.armRetry(f.newTimer)
	timer := f.lastTimer()
	w.retryFired()
	if w.retryArmed() || w.retryC != nil {
		t.Fatal("retryFired left the retry armed")
	}
	if got := atomic.LoadInt32(&timer.stopped); got != 0 {
		t.Fatalf("fired retry timer stopped %d times, want 0", got)
	}
}

func TestMessageWakeOnConfigWake(t *testing.T) {
	tests := []struct {
		name          string
		armed         bool
		blocked       bool
		iterates      bool
		pending       bool
		wantIterCalls int
		wantPendCalls int
		wantTimers    int
		wantState     uint32
		wantBlocked   bool
		wantArmed     bool
	}{
		{name: "neither armed nor blocked is a no-op", iterates: true, pending: true,
			wantState: messageWakeRetry},
		{name: "armed and still pending rearms one retry", armed: true, blocked: true, iterates: true, pending: true,
			wantIterCalls: 1, wantPendCalls: 1, wantTimers: 2, wantState: messageWakeRetry, wantBlocked: true, wantArmed: true},
		{name: "armed and pending cleared finishes", armed: true, blocked: true, iterates: true,
			wantIterCalls: 1, wantPendCalls: 1, wantTimers: 1, wantState: messageWakeIdle},
		{name: "not iterating and blocked parks", armed: true, blocked: true,
			wantIterCalls: 1, wantTimers: 1, wantState: messageWakeRetry, wantBlocked: true},
		{name: "blocked without timer and not iterating parks", blocked: true,
			wantIterCalls: 1, wantState: messageWakeRetry, wantBlocked: true},
		{name: "armed, not blocked, not iterating finishes", armed: true,
			wantIterCalls: 1, wantTimers: 1, wantState: messageWakeIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeTimerFactory{}
			w := newTestMessageWake(messageWakeRetry)
			w.blocked = tt.blocked
			var first *fakeTimer
			if tt.armed {
				w.armRetry(f.newTimer)
				first = f.lastTimer()
			}
			var iterCalls, pendCalls int
			w.onConfigWake(f.newTimer, probe(tt.iterates, &iterCalls), probe(tt.pending, &pendCalls))
			if iterCalls != tt.wantIterCalls || pendCalls != tt.wantPendCalls {
				t.Fatalf("probe calls = (%d, %d), want (%d, %d)",
					iterCalls, pendCalls, tt.wantIterCalls, tt.wantPendCalls)
			}
			if got := f.createdCount(); got != tt.wantTimers {
				t.Fatalf("timers created = %d, want %d", got, tt.wantTimers)
			}
			if first != nil && atomic.LoadInt32(&first.stopped) != 1 {
				t.Fatal("config wake did not stop the old retry timer")
			}
			if got := w.state.Load(); got != tt.wantState {
				t.Fatalf("state = %d, want %d", got, tt.wantState)
			}
			if w.blocked != tt.wantBlocked {
				t.Fatalf("blocked = %v, want %v", w.blocked, tt.wantBlocked)
			}
			if w.retryArmed() != tt.wantArmed {
				t.Fatalf("retry armed = %v, want %v", w.retryArmed(), tt.wantArmed)
			}
		})
	}
}

func TestMessageWakeCoalesceWhileRetrying(t *testing.T) {
	tests := []struct {
		name          string
		blocked       bool
		iterates      bool
		pending       bool
		wantPendCalls int
		wantState     uint32
		wantBlocked   bool
		wantArmed     bool
	}{
		{name: "still blocked keeps the retry", blocked: true, iterates: true, pending: true,
			wantPendCalls: 1, wantState: messageWakeRetry, wantBlocked: true, wantArmed: true},
		{name: "pending cleared finishes", blocked: true, iterates: true,
			wantPendCalls: 1, wantState: messageWakeIdle},
		{name: "stopped while blocked keeps state", blocked: true,
			wantState: messageWakeRetry, wantBlocked: true},
		{name: "stopped while unblocked finishes", wantState: messageWakeIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeTimerFactory{}
			w := newTestMessageWake(messageWakeRunning)
			w.blocked = tt.blocked
			w.armRetry(f.newTimer)
			var iterCalls, pendCalls int
			w.coalesce(probe(tt.iterates, &iterCalls), probe(tt.pending, &pendCalls))
			if iterCalls != 1 || pendCalls != tt.wantPendCalls {
				t.Fatalf("probe calls = (%d, %d), want (1, %d)", iterCalls, pendCalls, tt.wantPendCalls)
			}
			if got := f.createdCount(); got != 1 {
				t.Fatalf("timers created = %d, want 1", got)
			}
			if got := w.state.Load(); got != tt.wantState {
				t.Fatalf("state = %d, want %d", got, tt.wantState)
			}
			if w.blocked != tt.wantBlocked {
				t.Fatalf("blocked = %v, want %v", w.blocked, tt.wantBlocked)
			}
			if w.retryArmed() != tt.wantArmed {
				t.Fatalf("retry armed = %v, want %v", w.retryArmed(), tt.wantArmed)
			}
		})
	}
}

func TestMessageWakeBlockAndSettle(t *testing.T) {
	f := &fakeTimerFactory{}
	w := newTestMessageWake(messageWakeRunning)
	w.block(f.newTimer)
	if !w.blocked || !w.retryArmed() || w.state.Load() != messageWakeRetry {
		t.Fatalf("block: blocked=%v armed=%v state=%d", w.blocked, w.retryArmed(), w.state.Load())
	}
	timer := f.lastTimer()

	// Not iterating while blocked: park and drop the retry.
	w.hold()
	if w.retryArmed() || atomic.LoadInt32(&timer.stopped) != 1 || w.state.Load() != messageWakeRetry || !w.blocked {
		t.Fatalf("hold while blocked: armed=%v state=%d blocked=%v", w.retryArmed(), w.state.Load(), w.blocked)
	}

	// Not iterating while unblocked: finish.
	w.blocked = false
	w.state.Store(messageWakeRunning)
	w.hold()
	if w.state.Load() != messageWakeIdle {
		t.Fatalf("hold while unblocked: state=%d, want idle", w.state.Load())
	}

	// clear unblocks, finishes, and stops the retry.
	w.block(f.newTimer)
	timer = f.lastTimer()
	w.clear()
	if w.blocked || w.retryArmed() || atomic.LoadInt32(&timer.stopped) != 1 || w.state.Load() != messageWakeIdle {
		t.Fatalf("clear: blocked=%v armed=%v state=%d", w.blocked, w.retryArmed(), w.state.Load())
	}

	// cancel abandons and stops the retry.
	w.block(f.newTimer)
	timer = f.lastTimer()
	w.deferred.Store(true)
	w.cancel()
	if w.blocked || w.retryArmed() || atomic.LoadInt32(&timer.stopped) != 1 ||
		w.state.Load() != messageWakeIdle || w.deferred.Load() {
		t.Fatalf("cancel: blocked=%v armed=%v state=%d", w.blocked, w.retryArmed(), w.state.Load())
	}
}
