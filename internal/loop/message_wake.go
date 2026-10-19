package loop

import (
	"sync/atomic"
	"time"
)

const messageRetryDelay = time.Second

const (
	messageWakeIdle uint32 = iota
	messageWakeQueued
	messageWakeRunning
	messageWakeRetry
)

// messageWake is the message-trigger state machine of one engine. Publishers
// call queue from any goroutine; every other method runs on the Run goroutine.
//
// state coalesces queued publishes and blocked retries while deferred preserves
// one genuinely new generation that arrives during an already-running message
// iteration. ch is separate from the best-effort config wake queue so a full
// burst of config/start/stop nudges cannot discard a pending message
// generation; state guarantees at most one signal is queued.
//
// A message retry is independent from the interval cadence. It survives trips
// around the run loop, but there is never more than one armed timer: publish
// bursts coalesce while an interactive session owns the harness.
type messageWake struct {
	state    atomic.Uint32
	deferred atomic.Bool
	ch       chan struct{}

	// Run-goroutine only.
	blocked bool
	retry   loopTimer
	retryC  <-chan time.Time
}

func newMessageWake() *messageWake {
	return &messageWake{ch: make(chan struct{}, 1)}
}

// queue records a publish: it signals ch from idle, coalesces into a queued
// wake or armed retry, and defers a publish that lands during a running
// message iteration to the next generation.
func (w *messageWake) queue() {
	for {
		switch state := w.state.Load(); state {
		case messageWakeIdle:
			if !w.state.CompareAndSwap(messageWakeIdle, messageWakeQueued) {
				continue
			}
			select {
			case w.ch <- struct{}{}:
			default:
				// This is only possible when a signal already represents the
				// pending generation (or after Run has shut down).
				w.state.Store(messageWakeIdle)
			}
			return
		case messageWakeQueued, messageWakeRetry:
			// One queued wake or retry already represents the pending burst.
			return
		case messageWakeRunning:
			// A publish during the running iteration is a future generation,
			// not part of the burst that caused the current run. The state
			// recheck closes the race with finish/park: either that transition
			// consumes the flag, or this goroutine queues/drops it under the new
			// state.
			w.deferred.Store(true)
			if w.state.Load() != messageWakeRunning &&
				w.deferred.Swap(false) {
				continue
			}
			return
		}
	}
}

// begin marks a message attempt as running.
func (w *messageWake) begin() { w.state.Store(messageWakeRunning) }

// finish returns to idle and re-queues one signal for a publish deferred while
// the attempt ran.
func (w *messageWake) finish() {
	w.state.Store(messageWakeIdle)
	if w.deferred.Swap(false) {
		w.queue()
	}
}

// park holds the pending condition for a retry; publishes coalesce into it.
func (w *messageWake) park() {
	w.state.Store(messageWakeRetry)
	w.deferred.Store(false)
}

// abandon drops all message state on shutdown.
func (w *messageWake) abandon() {
	w.state.Store(messageWakeIdle)
	w.deferred.Store(false)
}

// armRetry parks and arms the single retry timer unless one is already armed.
func (w *messageWake) armRetry(newTimer func(time.Duration) loopTimer) {
	w.park()
	if w.retry != nil {
		return
	}
	w.retry = newTimer(messageRetryDelay)
	w.retryC = w.retry.C()
}

// stopRetry stops and detaches an armed retry timer.
func (w *messageWake) stopRetry() {
	if w.retry != nil {
		w.retry.Stop()
		w.retry = nil
		w.retryC = nil
	}
}

// retryFired detaches a retry timer whose channel was drained, without Stop,
// so a still-blocked attempt can arm the next single retry.
func (w *messageWake) retryFired() {
	w.retry = nil
	w.retryC = nil
}

func (w *messageWake) retryArmed() bool { return w.retry != nil }

// block records that the pending messages could not launch and arms a retry.
func (w *messageWake) block(newTimer func(time.Duration) loopTimer) {
	w.blocked = true
	w.armRetry(newTimer)
}

// clear settles an attempt that ran or found nothing pending.
func (w *messageWake) clear() {
	w.blocked = false
	w.finish()
	w.stopRetry()
}

// hold settles an attempt while the loop does not iterate: a blocked
// condition stays parked for the next start, otherwise the wake finishes.
func (w *messageWake) hold() {
	if w.blocked {
		w.park()
	} else {
		w.finish()
	}
	w.stopRetry()
}

// cancel settles an attempt after the run context ended.
func (w *messageWake) cancel() {
	w.blocked = false
	w.abandon()
	w.stopRetry()
}

// coalesce handles a signal that arrives while a retry is armed. The armed
// retry already owns this pending condition, so extra publishes neither create
// another timer nor spin against the still-live interactive session; the
// retry is dropped only when the loop stopped or nothing is pending any more.
// iterates re-reads the config before answering.
func (w *messageWake) coalesce(iterates, pending func() bool) {
	running := iterates()
	if running && pending() {
		return
	}
	if running {
		w.blocked = false
		w.finish()
	} else if !w.blocked {
		w.finish()
	}
	w.stopRetry()
}

// onConfigWake applies a config/start/stop wake. It invalidates the old retry
// delay: re-read persisted flags (iterates), cancel the old timer, and repark
// exactly one retry only when the blocked pending condition still applies.
func (w *messageWake) onConfigWake(newTimer func(time.Duration) loopTimer, iterates, pending func() bool) {
	if w.retry == nil && !w.blocked {
		return
	}
	w.stopRetry()
	if !iterates() {
		w.hold()
		return
	}
	if pending() {
		w.armRetry(newTimer)
		return
	}
	w.clear()
}
