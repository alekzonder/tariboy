package taskgoal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/mail"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

// seedIteration appends an iteration of worker that ended at ended ("" while
// it runs).
func seedIteration(t *testing.T, base *basestore.Store, id string, status agent.IterationStatus, ended time.Time) {
	t.Helper()
	endedAt := ""
	if status != agent.IterationRunning {
		endedAt = basestore.FormatTime(ended)
	}
	if _, err := base.DB.Exec(`INSERT INTO iterations(id,agent,trigger,status,started_at,ended_at)
		VALUES (?,'worker','message',?,?,?)`, id, string(status), basestore.FormatTime(ended.Add(-time.Second)), endedAt); err != nil {
		t.Fatal(err)
	}
}

func TestFailureStreak(t *testing.T) {
	ended := goalNow.Add(-time.Minute)
	cases := []struct {
		name     string
		statuses []agent.IterationStatus // oldest first
		// LastEnded is read only for a failed newest iteration.
		want FailureStreak
	}{
		{"no iterations", nil, FailureStreak{}},
		{"done resets", []agent.IterationStatus{"harness_error", "timeout", "done"},
			FailureStreak{Count: 0, LastID: "it-3", LastStatus: "done"}},
		{"failures in a row", []agent.IterationStatus{"done", "harness_error", "timeout", "no_i_am_done"},
			FailureStreak{Count: 3, LastID: "it-4", LastStatus: "no_i_am_done", LastEnded: ended}},
		{"killed and failed are skipped", []agent.IterationStatus{"timeout", "killed", "harness_error", "failed", "no_i_am_done"},
			FailureStreak{Count: 3, LastID: "it-5", LastStatus: "no_i_am_done", LastEnded: ended}},
		{"last killed", []agent.IterationStatus{"timeout", "killed"},
			FailureStreak{Count: 1, LastID: "it-2", LastStatus: "killed"}},
		{"running", []agent.IterationStatus{"timeout", "running"},
			FailureStreak{Count: 1, LastID: "it-2", LastStatus: "running", Running: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := openGoalStore(t)
			s := NewStore(base)
			seedAgent(t, s, "worker", goalNow.Add(-time.Hour))
			for i, status := range tc.statuses {
				seedIteration(t, base, fmt.Sprintf("it-%d", i+1), status, ended)
			}
			got, err := s.FailureStreak("worker")
			if err != nil {
				t.Fatal(err)
			}
			tc.want.Cooldown = time.Minute
			if got != tc.want {
				t.Fatalf("streak = %#v, want %#v", got, tc.want)
			}
		})
	}
	t.Run("halted", func(t *testing.T) {
		base := openGoalStore(t)
		s := NewStore(base)
		seedAgent(t, s, "worker", goalNow.Add(-time.Hour))
		execGoalSQL(t, s, `UPDATE agents SET error_reason='harness error (on_error=stop)' WHERE name='worker'`)
		seedIteration(t, base, "it-1", "harness_error", ended)
		got, err := s.FailureStreak("worker")
		if err != nil || !got.Halted || got.Count != 1 {
			t.Fatalf("streak = %#v, %v; want a halted failure", got, err)
		}
	})
}

func TestRedeliveryPause(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, d := range want {
		if got := redeliveryPause(time.Minute, i+1); got != d {
			t.Fatalf("pause(60s, %d) = %v, want %v", i+1, got, d)
		}
	}
	if got := redeliveryPause(15*time.Minute, 3); got != 15*time.Minute {
		t.Fatalf("pause(900s, 3) = %v; the bound never shortens the configured cooldown", got)
	}
	if got := redeliveryPause(time.Second, 64); got != redeliveryPauseMax {
		t.Fatalf("pause(1s, 64) = %v, want %v", got, redeliveryPauseMax)
	}
	if got := redeliveryPause(time.Second, 1<<30); got != redeliveryPauseMax {
		t.Fatalf("pause(1s, 2^30) = %v, want %v", got, redeliveryPauseMax)
	}
}

// fakeAudit records the events written for one agent.
type fakeAudit struct {
	mu     sync.Mutex
	events []auditEvent
}

type auditEvent struct {
	agent, typ, source, iterationID string
	data                            map[string]any
}

func (a *fakeAudit) For(agentName string) Recorder { return auditFor{a, agentName} }

func (a *fakeAudit) all() []auditEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]auditEvent(nil), a.events...)
}

type auditFor struct {
	a     *fakeAudit
	agent string
}

func (r auditFor) Record(typ, source, iterationID string, data map[string]any) int64 {
	r.a.mu.Lock()
	defer r.a.mu.Unlock()
	r.a.events = append(r.a.events, auditEvent{r.agent, typ, source, iterationID, data})
	return int64(len(r.a.events))
}

// fakeTimers records the timers the reconciler arms.
type fakeTimers struct {
	mu     sync.Mutex
	armed  []time.Duration
	fns    []func()
	stoped []bool
}

func (f *fakeTimers) after(d time.Duration, fn func()) Stopper {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.armed = append(f.armed, d)
	f.fns = append(f.fns, fn)
	f.stoped = append(f.stoped, false)
	return fakeStopper{f, len(f.armed) - 1}
}

type fakeStopper struct {
	f *fakeTimers
	i int
}

func (s fakeStopper) Stop() bool {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	was := !s.f.stoped[s.i]
	s.f.stoped[s.i] = true
	return was
}

// redeliveryFixture is a worker with Goal T-1, its Goal letter already shown
// and closed by the iteration end, and a fake clock, audit log and timers.
type redeliveryFixture struct {
	t      *testing.T
	base   *basestore.Store
	mail   *mail.Mail
	audit  *fakeAudit
	timers *fakeTimers
	now    time.Time
	r      *Reconciler
	seq    int
}

func newRedeliveryFixture(t *testing.T) *redeliveryFixture {
	t.Helper()
	f := &redeliveryFixture{t: t, base: openGoalStore(t), audit: &fakeAudit{}, timers: &fakeTimers{}, now: goalNow}
	s := NewStore(f.base)
	seedAgent(t, s, "worker", goalNow.Add(-time.Hour))
	seedTask(t, s, "T-1", "agent:worker", "P0", "in_progress", "2026-09-01T00:00:00Z")
	execGoalSQL(t, s, `UPDATE agents SET goal_delivery_cooldown_s=60 WHERE name='worker'`)
	f.mail = mail.New(f.base, func() time.Time { return f.now }, goalDirectory{f.base})
	f.r = f.reconciler()
	return f
}

func (f *redeliveryFixture) reconciler() *Reconciler {
	return NewReconciler(ReconcilerConfig{
		Store: f.base, Mail: f.mail, Clock: func() time.Time { return f.now },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Audit: f.audit.For, AfterFunc: f.timers.after,
	})
}

// fail records a finished iteration with status that ends now and closes the
// open Goal letters, as the iteration end does; it returns the iteration ID.
func (f *redeliveryFixture) end(status agent.IterationStatus) string {
	f.t.Helper()
	f.seq++
	id := fmt.Sprintf("worker-%d", f.seq)
	seedIteration(f.t, f.base, id, status, f.now)
	tx, err := f.base.DB.Begin()
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := mail.CloseTaskNoticesTx(tx, "agent:worker", "T-1", []string{"task.goal"}, f.now, "iteration-end:"+id, f.now); err != nil {
		_ = tx.Rollback()
		f.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		f.t.Fatal(err)
	}
	f.r.IterationCompleted("worker", id)
	return id
}

func (f *redeliveryFixture) reconcile() {
	f.t.Helper()
	if err := f.r.Reconcile(context.Background(), "", ""); err != nil {
		f.t.Fatal(err)
	}
}

// retries is the number of Goal letters with reason retry.
func (f *redeliveryFixture) retries() int {
	f.t.Helper()
	n := 0
	for _, l := range goalLetters(f.t, f.mail, "worker") {
		if l.Kind == "task.goal" && l.Data["reason"] == "retry" {
			n++
		}
	}
	return n
}

// expectPause checks that no retry goes out before pause and exactly one at
// pause, then returns the event written for it.
func (f *redeliveryFixture) expectPause(pause time.Duration) auditEvent {
	f.t.Helper()
	before, events := f.retries(), len(f.audit.all())
	start := f.now
	f.now = start.Add(pause - time.Second)
	f.reconcile()
	if got := f.retries(); got != before {
		f.t.Fatalf("retry letters %v after the failure = %d, want %d (pause %v)", pause-time.Second, got, before, pause)
	}
	f.now = start.Add(pause)
	f.reconcile()
	f.reconcile() // a second pass sends nothing more
	if got := f.retries(); got != before+1 {
		f.t.Fatalf("retry letters at the pause %v = %d, want %d", pause, got, before+1)
	}
	all := f.audit.all()
	if len(all) != events+1 {
		f.t.Fatalf("audit events = %#v, want one more", all)
	}
	return all[len(all)-1]
}

func TestReconcileRedeliversAfterHarnessError(t *testing.T) {
	testRedeliversAfter(t, agent.IterationHarnessError)
}

func TestReconcileRedeliversAfterTimeout(t *testing.T) {
	testRedeliversAfter(t, agent.IterationTimeout)
}

func TestReconcileRedeliversAfterNoIAmDone(t *testing.T) {
	testRedeliversAfter(t, agent.IterationNoIAmDone)
}

func testRedeliversAfter(t *testing.T, status agent.IterationStatus) {
	t.Helper()
	f := newRedeliveryFixture(t)
	f.reconcile() // the first Goal letter
	f.now = f.now.Add(2 * time.Minute)
	id := f.end(status)
	if got := f.retries(); got != 0 {
		t.Fatalf("retry letters right after the failure = %d", got)
	}
	ev := f.expectPause(time.Minute)
	if ev.agent != "worker" || ev.typ != "goal_redelivered" || ev.source != "system" || ev.iterationID != id ||
		ev.data["task_key"] != "T-1" || ev.data["failures"] != 1 || ev.data["pause_s"] != 60 || ev.data["last_status"] != string(status) {
		t.Fatalf("event = %#v", ev)
	}
	letters := goalLetters(t, f.mail, "worker")
	if letters[0].Data["reason"] != "retry" || letters[0].AnsweredAt != "" {
		t.Fatalf("newest letter = %#v, want an open retry", letters[0])
	}
}

func TestReconcileRedeliveryBackoffAndReset(t *testing.T) {
	f := newRedeliveryFixture(t)
	f.reconcile()
	statuses := []agent.IterationStatus{"harness_error", "timeout", "no_i_am_done", "harness_error", "timeout", "harness_error"}
	pauses := []int{60, 120, 240, 480, 600, 600}
	for i, status := range statuses {
		f.end(status)
		ev := f.expectPause(time.Duration(pauses[i]) * time.Second)
		if ev.data["failures"] != i+1 || ev.data["pause_s"] != pauses[i] || ev.data["last_status"] != string(status) {
			t.Fatalf("failure %d event = %#v", i+1, ev)
		}
	}
	f.end(agent.IterationDone)
	f.now = f.now.Add(time.Minute)
	f.end(agent.IterationTimeout)
	if ev := f.expectPause(time.Minute); ev.data["failures"] != 1 || ev.data["pause_s"] != 60 {
		t.Fatalf("event after done = %#v; done resets the count", ev)
	}
}

func TestReconcileRedeliverySurvivesRestart(t *testing.T) {
	f := newRedeliveryFixture(t)
	f.reconcile()
	for _, pause := range []time.Duration{time.Minute, 2 * time.Minute} {
		f.end(agent.IterationTimeout)
		f.expectPause(pause)
	}
	f.end(agent.IterationHarnessError)
	// A new reconciler on the same database reads the count back: n = 3.
	f.r = f.reconciler()
	if ev := f.expectPause(4 * time.Minute); ev.data["failures"] != 3 {
		t.Fatalf("event after restart = %#v", ev)
	}
}

func TestReconcileRedeliverySkipsRunningAndHalted(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		f := newRedeliveryFixture(t)
		f.reconcile()
		f.end(agent.IterationTimeout)
		f.expectPause(time.Minute)
		// The retry letter started a new iteration that is still running and
		// shows it: nothing more goes out.
		seedIteration(t, f.base, "worker-run", agent.IterationRunning, f.now)
		f.now = f.now.Add(time.Hour)
		f.reconcile()
		if got, events := f.retries(), len(f.audit.all()); got != 1 || events != 1 {
			t.Fatalf("retries = %d events = %d while running; want 1 and 1", got, events)
		}
	})
	t.Run("halted", func(t *testing.T) {
		f := newRedeliveryFixture(t)
		f.reconcile()
		execGoalSQL(t, NewStore(f.base), `UPDATE agents SET error_reason='harness error (on_error=stop)' WHERE name='worker'`)
		f.end(agent.IterationHarnessError)
		f.now = f.now.Add(time.Hour)
		f.reconcile()
		if got, n := len(goalLetters(t, f.mail, "worker")), len(f.audit.all()); got != 1 || n != 0 {
			t.Fatalf("letters = %d events = %d for a halted loop; want 1 and 0", got, n)
		}
		// Start clears the halt: the next pass sends the Goal again.
		execGoalSQL(t, NewStore(f.base), `UPDATE agents SET error_reason='' WHERE name='worker'`)
		f.reconcile()
		if got := f.retries(); got != 1 {
			t.Fatalf("retries after start = %d, want 1", got)
		}
	})
	t.Run("killed", func(t *testing.T) {
		f := newRedeliveryFixture(t)
		f.reconcile()
		f.now = f.now.Add(2 * time.Minute)
		f.end(agent.IterationKilled)
		// The usual delivery applies: no retry letter and no event.
		if got, n := f.retries(), len(f.audit.all()); got != 0 || n != 0 {
			t.Fatalf("retries = %d events = %d after killed", got, n)
		}
		if got := len(goalLetters(t, f.mail, "worker")); got != 2 {
			t.Fatalf("letters after killed = %d, want the usual next Goal", got)
		}
	})
}

func TestReconcileRedeliveryArmsTimer(t *testing.T) {
	f := newRedeliveryFixture(t)
	f.reconcile()
	f.end(agent.IterationTimeout)
	f.timers.mu.Lock()
	armed := append([]time.Duration(nil), f.timers.armed...)
	f.timers.mu.Unlock()
	if len(armed) != 1 || armed[0] != time.Minute {
		t.Fatalf("timers = %v, want one for the 1m pause", armed)
	}
	// A later due date keeps the armed timer.
	f.now = f.now.Add(10 * time.Second)
	f.reconcile()
	f.timers.mu.Lock()
	if len(f.timers.armed) != 1 {
		t.Fatalf("timers = %v; a later pass re-armed the same due date", f.timers.armed)
	}
	fire := f.timers.fns[0]
	f.timers.mu.Unlock()
	// The timer wakes a full pass through Signal.
	fire()
	select {
	case <-f.r.signals:
	default:
		t.Fatal("the timer did not signal the reconciler")
	}
	// After it fired, the next pass arms a timer again.
	f.reconcile()
	f.timers.mu.Lock()
	defer f.timers.mu.Unlock()
	if len(f.timers.armed) != 2 || f.timers.armed[1] != 50*time.Second {
		t.Fatalf("timers = %v, want a second one for the remaining 50s", f.timers.armed)
	}
}

func TestReconcileRedeliveryEarlierDueRearms(t *testing.T) {
	f := newRedeliveryFixture(t)
	f.r.armRedelivery(f.now.Add(5 * time.Minute))
	f.r.armRedelivery(f.now.Add(time.Minute))
	f.r.armRedelivery(f.now.Add(3 * time.Minute))
	f.timers.mu.Lock()
	defer f.timers.mu.Unlock()
	if len(f.timers.armed) != 2 || f.timers.armed[1] != time.Minute || !f.timers.stoped[0] || f.timers.stoped[1] {
		t.Fatalf("timers = %v stopped = %v; an earlier due date replaces the timer", f.timers.armed, f.timers.stoped)
	}
}
