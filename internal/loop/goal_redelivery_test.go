package loop

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/taskgoal"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// goalLoop is an engine wired to real mail, the Goal reconciler and the
// iteration end of the tasks service on one isolated database, as the daemon
// wires them, with a shared fake clock and fake timers.
type goalLoop struct {
	t       *testing.T
	st      *store.Store
	as      *agent.Store
	mail    *mail.Mail
	goal    *taskgoal.Reconciler
	engine  *Engine
	timers  *fakeTimerFactory // the engine's message retry
	goalDue []time.Duration   // the reconciler's redelivery timers
	mu      sync.Mutex
	now     time.Time
}

func (g *goalLoop) clock() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.now
}

func (g *goalLoop) advance(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.now = g.now.Add(d)
}

// wakingSender wakes the engine for each letter, as mail does for the
// daemon's agents.
type wakingSender struct{ g *goalLoop }

func (s wakingSender) Send(l mail.Letter) ([]mail.Letter, error) {
	out, err := s.g.mail.Send(l)
	if err == nil {
		s.g.engine.Wake(WakeMessage)
	}
	return out, err
}

// letterRunner shows the open letters, as a prompt does, then ends with the
// next outcome; a done outcome answers every letter and finishes the task.
type letterRunner struct {
	g        *goalLoop
	mu       sync.Mutex
	outcomes []Outcome
	calls    int
}

func (r *letterRunner) Run(context.Context, agent.Agent, string, string, string) (Outcome, error) {
	r.mu.Lock()
	o := r.outcomes[min(r.calls, len(r.outcomes)-1)]
	r.calls++
	r.mu.Unlock()
	if _, err := r.g.mail.RenderBatch("smoke", 10); err != nil {
		r.g.t.Error(err)
	}
	if o.Status == "done" {
		ts := store.FormatTime(r.g.clock())
		if _, err := r.g.st.DB.Exec(`UPDATE mail SET answered_at=? WHERE recipient='agent:smoke' AND answered_at=''`, ts); err != nil {
			r.g.t.Error(err)
		}
		if _, err := r.g.st.DB.Exec(`UPDATE tasks SET status='done' WHERE task_key='T-1'`); err != nil {
			r.g.t.Error(err)
		}
	}
	return o, nil
}

func startGoalLoop(t *testing.T, outcomes ...Outcome) *goalLoop {
	t.Helper()
	g := &goalLoop{t: t, st: storetest.Open(t), now: time.Date(2026, 11, 3, 22, 0, 55, 0, time.UTC), timers: &fakeTimerFactory{}}
	ag := baseAgent()
	ag.IntervalS = 0
	g.as = agent.NewStore(g.st)
	if err := g.as.Create(ag); err != nil {
		t.Fatal(err)
	}
	ts := store.FormatTime(g.now)
	for _, q := range []string{
		`UPDATE agents SET goal_enabled=1, goal_delivery_cooldown_s=60 WHERE name='smoke'`,
		`INSERT INTO task_queues(prefix,name,created_at,updated_at) VALUES ('T','Tasks','` + ts + `','` + ts + `')`,
		`INSERT INTO tasks(task_key,queue_prefix,priority,title,description,status,author,customer,assignee,created_at,updated_at)
			VALUES ('T-1','T','P1','goal','goal','in_progress','user:customer','user:customer','agent:smoke','` + ts + `','` + ts + `')`,
	} {
		if _, err := g.st.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	g.mail = mail.New(g.st, g.clock, agentStoreDirectory{g.as})
	g.goal = taskgoal.NewReconciler(taskgoal.ReconcilerConfig{
		Store: g.st, Mail: wakingSender{g}, Clock: g.clock,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AfterFunc: func(d time.Duration, _ func()) taskgoal.Stopper {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.goalDue = append(g.goalDue, d)
			return time.AfterFunc(time.Hour, func() {})
		},
	})
	taskService := tasks.NewService(g.st.DB, "customer", g.clock)
	g.engine = NewEngine(ag, g.as, &letterRunner{g: g, outcomes: outcomes}, slog.New(slog.NewTextHandler(io.Discard, nil)), g.clock)
	g.engine.newTimer = g.timers.newTimer
	g.engine.SetMessagePeek(func() (bool, error) {
		n, err := g.mail.OpenCount("smoke")
		return n > 0, err
	})
	// The Manager's report of a terminal iteration: the iteration end, then
	// the Goal wake.
	g.engine.SetIterationCompleted(func(name, id string) {
		it, err := g.as.GetIteration(name, id)
		if err != nil {
			t.Error(err)
		}
		end := tasks.IterationEnd{Agent: name, IterationID: id, GoalTaskKey: "T-1", StartedAt: g.clock(), FinishedAt: g.clock(),
			HarnessError: it.Status == agent.IterationHarnessError}
		if err := taskService.RecordIterationEnd(context.Background(), end); err != nil {
			t.Error(err)
		}
		g.goal.IterationCompleted(name, id)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { g.engine.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return g
}

func (g *goalLoop) reconcile() {
	g.t.Helper()
	if err := g.goal.Reconcile(context.Background(), "", ""); err != nil {
		g.t.Fatal(err)
	}
}

// iterations waits until want iterations finished and checks that no more
// start; it returns them oldest first.
func (g *goalLoop) iterations(want int) []agent.Iteration {
	g.t.Helper()
	var its []agent.Iteration
	finished := func() bool {
		its, _ = g.as.ListIterations("smoke")
		n := 0
		for _, it := range its {
			if it.Status != agent.IterationRunning {
				n++
			}
		}
		return n >= want
	}
	if !testutil.WaitFor(2*time.Second, finished) {
		g.t.Fatalf("iterations = %#v, want %d finished", its, want)
	}
	time.Sleep(50 * time.Millisecond)
	if its, _ = g.as.ListIterations("smoke"); len(its) != want {
		g.t.Fatalf("iterations = %d, want %d", len(its), want)
	}
	return its
}

func (g *goalLoop) goalLetters() []mail.Letter {
	g.t.Helper()
	letters, _, err := g.mail.List(mail.BoxAll, "agent:smoke", 100, "")
	if err != nil {
		g.t.Fatal(err)
	}
	var out []mail.Letter
	for _, l := range letters {
		if l.Kind == "task.goal" {
			out = append(out, l)
		}
	}
	return out
}

// The bob case: an iteration woken by the Goal letter ends in harness_error
// after two seconds. The letter it showed no longer stays open and blocks
// every later delivery: after the cooldown the reconciler sends the Goal
// again and the engine runs exactly one new message iteration.
func TestGoalRedeliveryAfterHarnessErrorBob(t *testing.T) {
	g := startGoalLoop(t, Outcome{Status: "harness_error"}, Outcome{Status: "done", DoneFlag: true})
	g.reconcile()
	its := g.iterations(1)
	if its[0].Status != agent.IterationHarnessError || its[0].Trigger != "message" {
		t.Fatalf("first iteration = %#v", its[0])
	}
	// The completion hook runs after the iteration row is final: it ends by
	// arming the redelivery timer.
	var due []time.Duration
	if !testutil.WaitFor(2*time.Second, func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		due = append([]time.Duration(nil), g.goalDue...)
		return len(due) > 0
	}) || len(due) != 1 || due[0] != time.Minute {
		t.Fatalf("redelivery timers = %v, want one for the 1m cooldown", due)
	}
	letters := g.goalLetters()
	if len(letters) != 1 || letters[0].AnsweredAt == "" || letters[0].Attempts != 1 {
		t.Fatalf("goal letters = %#v; the letter the failed iteration showed must be closed", letters)
	}
	time.Sleep(50 * time.Millisecond)
	if got := g.timers.createdCount(); got != 0 {
		t.Fatalf("engine retry timers = %d; only the reconciler wakes the agent", got)
	}

	g.advance(59 * time.Second)
	g.reconcile()
	g.iterations(1)
	g.advance(time.Second)
	g.reconcile()
	g.reconcile()
	its = g.iterations(2)
	if next := its[1]; next.Trigger != "message" || next.Status != agent.IterationDone {
		t.Fatalf("redelivered iteration = %#v", next)
	}
	if letters := g.goalLetters(); len(letters) != 2 || letters[0].Data["reason"] != "retry" {
		t.Fatalf("goal letters = %#v, want a retry", letters)
	}
}

// With another open letter the engine's own retry runs the next iteration;
// the Goal redelivery adds no second iteration for the same failure.
func TestGoalRedeliveryCoalescesWithMessageRetry(t *testing.T) {
	g := startGoalLoop(t, Outcome{Status: "harness_error"}, Outcome{Status: "done", DoneFlag: true})
	if _, err := g.mail.Send(mail.Letter{From: "user:customer", To: "agent:smoke", Subject: "hello", Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	g.reconcile()
	g.iterations(1)
	waitForTimerCount(t, g.timers, 1)
	if got := g.timers.lastDelay(); got != 30*time.Second {
		t.Fatalf("engine retry = %v, want 30s", got)
	}
	g.advance(30 * time.Second)
	g.timers.lastTimer().ch <- g.clock()
	g.iterations(2)
	g.advance(time.Hour)
	g.reconcile()
	g.iterations(2)
	for _, l := range g.goalLetters() {
		if l.Data["reason"] == "retry" {
			t.Fatalf("retry letter %#v after the engine retry already ran the agent", l)
		}
	}
}
