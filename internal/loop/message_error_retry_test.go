package loop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// pendingClearingRunner fails every iteration and, like a letter that was
// answered during the run, leaves nothing pending afterwards.
type pendingClearingRunner struct{ pending *atomic.Bool }

func (r pendingClearingRunner) Run(context.Context, agent.Agent, string, string, string) (Outcome, error) {
	r.pending.Store(false)
	return Outcome{Status: "harness_error"}, nil
}

func startMessageEngine(t *testing.T, ag agent.Agent, r IterationRunner, pending func() bool) (*agent.Store, *fakeTimerFactory, func()) {
	t.Helper()
	e, as := newEngine(t, ag, r)
	e.SetMessagePeek(func() (bool, error) { return pending(), nil })
	factory := &fakeTimerFactory{}
	e.newTimer = factory.newTimer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	e.Wake(WakeMessage)
	return as, factory, func() { cancel(); <-done }
}

func waitIterations(t *testing.T, as *agent.Store, want int) []agent.Iteration {
	t.Helper()
	var its []agent.Iteration
	if !testutil.WaitFor(2*time.Second, func() bool { its, _ = as.ListIterations("smoke"); return len(its) >= want }) {
		t.Fatalf("iterations = %d, want %d", len(its), want)
	}
	return its
}

// A message iteration that ends in harness_error leaves its letters open. The
// loop shows them again after a growing delay instead of waiting for the next
// publish.
func TestMessageIterationRetriesPendingLettersAfterHarnessError(t *testing.T) {
	t.Parallel()
	ag := baseAgent()
	ag.IntervalS = 0
	r := &fakeRunner{outcomes: []Outcome{{Status: "harness_error"}, {Status: "harness_error"}, {Status: "done", DoneFlag: true}}}
	as, factory, stop := startMessageEngine(t, ag, r, func() bool { return true })
	defer stop()

	waitIterations(t, as, 1)
	waitForTimerCount(t, factory, 1)
	if got := factory.lastDelay(); got != 30*time.Second {
		t.Fatalf("first retry delay = %v, want 30s", got)
	}
	factory.lastTimer().ch <- time.Now()
	waitIterations(t, as, 2)
	waitForTimerCount(t, factory, 2)
	if got := factory.lastDelay(); got != time.Minute {
		t.Fatalf("second retry delay = %v, want 1m", got)
	}
	factory.lastTimer().ch <- time.Now()
	its := waitIterations(t, as, 3)
	for _, it := range its {
		if it.Trigger != "message" {
			t.Fatalf("retry trigger = %q, want message", it.Trigger)
		}
	}
	// A completed iteration ends the retries.
	time.Sleep(50 * time.Millisecond)
	if got := factory.createdCount(); got != 2 {
		t.Fatalf("timers after done = %d, want 2", got)
	}
}

func TestMessageErrorRetryDelayDoublesUpToTenMinutes(t *testing.T) {
	t.Parallel()
	w := newMessageWake()
	f := &fakeTimerFactory{}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, d := range want {
		w.backoff(f.newTimer)
		if got := f.lastDelay(); got != d {
			t.Fatalf("retry %d delay = %v, want %v", i+1, got, d)
		}
		w.retryFired()
	}
	w.clear()
	w.backoff(f.newTimer)
	if got := f.lastDelay(); got != 30*time.Second {
		t.Fatalf("delay after a cleared attempt = %v, want 30s", got)
	}
}

func TestMessageIterationHarnessErrorWithoutPendingLettersDoesNotRetry(t *testing.T) {
	t.Parallel()
	ag := baseAgent()
	ag.IntervalS = 0
	pending := &atomic.Bool{}
	pending.Store(true)
	as, factory, stop := startMessageEngine(t, ag, pendingClearingRunner{pending}, pending.Load)
	defer stop()
	waitIterations(t, as, 1)
	time.Sleep(50 * time.Millisecond)
	if got := factory.createdCount(); got != 0 {
		t.Fatalf("retry timers = %d, want 0", got)
	}
}

func TestMessageIterationHarnessErrorWithStopPolicyDoesNotRetry(t *testing.T) {
	t.Parallel()
	ag := baseAgent()
	ag.IntervalS = 0
	ag.OnError = "stop"
	r := &fakeRunner{outcomes: []Outcome{{Status: "harness_error"}}}
	as, factory, stop := startMessageEngine(t, ag, r, func() bool { return true })
	defer stop()
	waitIterations(t, as, 1)
	time.Sleep(50 * time.Millisecond)
	if got := factory.createdCount(); got != 0 {
		t.Fatalf("retry timers = %d, want 0", got)
	}
}
