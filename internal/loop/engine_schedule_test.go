package loop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// The engine clock in newEngine is 2026-07-06 10:00 UTC; this rule fires daily
// at 10:05 UTC.
const scheduleAt1005 = "DTSTART:20260706T000000Z\nRRULE:FREQ=DAILY;BYHOUR=10;BYMINUTE=5;BYSECOND=0"

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	if testutil.WaitFor(2*time.Second, cond) {
		return
	}
	t.Fatalf("timed out waiting for %s", what)
}

func runScheduleEngine(t *testing.T, ag agent.Agent, rule string) (*Engine, *agent.Store, *fakeTimerFactory) {
	e, as, f, _ := runScheduleEngineIdle(t, ag, rule)
	return e, as, f
}

func runScheduleEngineIdle(t *testing.T, ag agent.Agent, rule string) (*Engine, *agent.Store, *fakeTimerFactory, *idleTracker) {
	t.Helper()
	r := &fakeRunner{outcomes: []Outcome{{Status: "done", DoneFlag: true}}}
	e, as := newEngine(t, ag, r)
	if err := as.SetSchedule(ag.Name, rule); err != nil {
		t.Fatal(err)
	}
	f := &fakeTimerFactory{}
	e.newTimer = f.newTimer
	idle := trackIdle(e)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return e, as, f, idle
}

func iterationTriggers(as *agent.Store) []string {
	its, _ := as.ListIterations("smoke")
	out := make([]string, 0, len(its))
	for _, it := range its {
		out = append(out, it.Trigger)
	}
	return out
}

func TestRunScheduleOnlyAgentStartsScheduleIteration(t *testing.T) {
	t.Parallel()
	ag := baseAgent()
	ag.IntervalS = 0
	_, as, f := runScheduleEngine(t, ag, scheduleAt1005)

	waitUntil(t, "schedule timer", func() bool { return f.createdCount() >= 1 })
	if d := f.lastDelay(); d != 5*time.Minute {
		t.Fatalf("schedule timer delay = %v, want 5m", d)
	}
	f.lastTimer().ch <- time.Time{}
	waitUntil(t, "schedule iteration", func() bool { return len(iterationTriggers(as)) == 1 })
	if got := iterationTriggers(as); got[0] != "schedule" {
		t.Fatalf("triggers = %v, want [schedule]", got)
	}
}

func TestRunPicksNearestOfIntervalAndSchedule(t *testing.T) {
	t.Parallel()
	ag := baseAgent()
	ag.IntervalS = 3600
	_, as, f := runScheduleEngine(t, ag, scheduleAt1005)

	// No previous run: the interval is due at once and wins.
	waitUntil(t, "first timer", func() bool { return f.createdCount() >= 1 })
	if d := f.lastDelay(); d != 0 {
		t.Fatalf("first delay = %v, want 0", d)
	}
	f.lastTimer().ch <- time.Time{}
	waitUntil(t, "interval iteration", func() bool { return len(iterationTriggers(as)) == 1 })

	// Interval next in 1h, schedule in 5m: the schedule wins.
	waitUntil(t, "second timer", func() bool { return f.createdCount() >= 2 })
	if d := f.lastDelay(); d != 5*time.Minute {
		t.Fatalf("second delay = %v, want 5m", d)
	}
	f.lastTimer().ch <- time.Time{}
	waitUntil(t, "schedule iteration", func() bool { return len(iterationTriggers(as)) == 2 })
	got := iterationTriggers(as)
	if !(contains(got, "interval") && contains(got, "schedule")) {
		t.Fatalf("triggers = %v, want interval and schedule", got)
	}
}

func TestRunScheduleIgnoredForDisabledAgent(t *testing.T) {
	t.Parallel()
	ag := baseAgent()
	ag.IntervalS = 0
	ag.Enabled = false
	e, _, f, idle := runScheduleEngineIdle(t, ag, scheduleAt1005)
	e.Wake(WakeConfig)
	// Start, then the config wake.
	idle.wait(t, 2)
	if c := f.createdCount(); c != 0 {
		t.Fatalf("disabled agent created %d timers", c)
	}
}

func TestRunScheduleChangeRearmsTimer(t *testing.T) {
	t.Parallel()
	ag := baseAgent()
	ag.IntervalS = 0
	e, as, f, idle := runScheduleEngineIdle(t, ag, scheduleAt1005)
	waitUntil(t, "schedule timer", func() bool { return f.createdCount() >= 1 })
	first := f.lastTimer()

	if err := as.SetSchedule("smoke", "DTSTART:20260706T000000Z\nRRULE:FREQ=DAILY;BYHOUR=11;BYMINUTE=0;BYSECOND=0"); err != nil {
		t.Fatal(err)
	}
	e.Wake(WakeConfig)
	waitUntil(t, "re-armed timer", func() bool { return f.createdCount() >= 2 })
	if d := f.lastDelay(); d != time.Hour {
		t.Fatalf("re-armed delay = %v, want 1h", d)
	}
	if atomic.LoadInt32(&first.stopped) == 0 {
		t.Fatal("old schedule timer was not stopped")
	}

	// A cleared schedule leaves a schedule-only agent with no timer at all.
	if err := as.SetSchedule("smoke", ""); err != nil {
		t.Fatal(err)
	}
	e.Wake(WakeConfig)
	// Start and the two config wakes.
	idle.wait(t, 3)
	if c := f.createdCount(); c != 2 {
		t.Fatalf("cleared schedule created timers: %d", c)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
