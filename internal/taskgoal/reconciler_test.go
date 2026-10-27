package taskgoal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

func TestReconcilerSendsGoalLetter(t *testing.T) {
	r, messageBus, _ := seededGoalReconciler(t)
	if err := r.Reconcile(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	msg := onlyGoalMessage(t, messageBus, "worker")
	if msg.Kind != "task.goal" || msg.To != "agent:worker" || msg.From != SenderGoal || !msg.NeedsReply ||
		msg.Data["task_key"] != "T-1" || msg.Data["reason"] != "selected" {
		t.Fatalf("letter = %#v", msg)
	}
}

func TestReconcilerGoalGenerations(t *testing.T) {
	tests := []struct {
		name       string
		want       int
		wantReason string
		run        func(*testing.T, *Reconciler, *mail.Mail, *basestore.Store) error
	}{
		{
			name: "startup_duplicate", want: 1, wantReason: "selected",
			run: func(t *testing.T, r *Reconciler, _ *mail.Mail, _ *basestore.Store) error {
				if err := r.Reconcile(context.Background(), "", ""); err != nil {
					return err
				}
				return r.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "recovery_duplicate", want: 1, wantReason: "selected",
			run: func(t *testing.T, r *Reconciler, messageBus *mail.Mail, base *basestore.Store) error {
				seedTerminalIteration(t, base, "iter-7", "worker")
				if err := r.Reconcile(context.Background(), "", ""); err != nil {
					return err
				}
				restarted := NewReconciler(ReconcilerConfig{Store: base, Mail: messageBus, Clock: func() time.Time { return goalNow }})
				return restarted.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "terminal_iteration_once", want: 1, wantReason: "iteration_completed",
			run: func(t *testing.T, r *Reconciler, _ *mail.Mail, _ *basestore.Store) error {
				r.IterationCompleted("worker", "iter-7")
				r.IterationCompleted("worker", "iter-7")
				return nil
			},
		},
		{
			name: "waiting_customer", want: 0,
			run: func(t *testing.T, r *Reconciler, _ *mail.Mail, base *basestore.Store) error {
				s := NewStore(base)
				if _, err := s.ReconcileAgent("worker", goalNow); err != nil {
					return err
				}
				updateTask(t, s, "T-1", "status", "wait_customer")
				seedCustomerWait(t, s, "T-1", goalNow.Add(-time.Minute), false)
				return r.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "no_candidate", want: 0,
			run: func(t *testing.T, r *Reconciler, _ *mail.Mail, base *basestore.Store) error {
				updateTask(t, NewStore(base), "T-1", "status", "done")
				return r.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "disabled_agent", want: 0,
			run: func(t *testing.T, r *Reconciler, _ *mail.Mail, base *basestore.Store) error {
				if _, err := base.DB.Exec(`UPDATE agents SET enabled=0 WHERE name='worker'`); err != nil {
					return err
				}
				return r.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "first_publish_fails", want: 1, wantReason: "selected",
			run: func(t *testing.T, _ *Reconciler, messageBus *mail.Mail, base *basestore.Store) error {
				s := NewStore(base)
				seedAgent(t, s, "broken", goalNow.Add(-time.Hour))
				seedTask(t, s, "T-2", "agent:broken", "P0", "open", "2026-09-01T00:00:00Z")
				failing := &failOncePublisher{delegate: messageBus, to: "agent:broken"}
				r := NewReconciler(ReconcilerConfig{Store: base, Mail: failing, Clock: func() time.Time { return goalNow }})
				err := r.Reconcile(context.Background(), "", "")
				if err == nil || !strings.Contains(err.Error(), "broken") {
					t.Fatalf("Reconcile error = %v, want broken-agent publish error", err)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, messageBus, base := seededGoalReconciler(t)
			if err := tt.run(t, r, messageBus, base); err != nil {
				t.Fatal(err)
			}
			messages := goalLetters(t, messageBus, "worker")
			if len(messages) != tt.want {
				t.Fatalf("message count = %d, want %d: %#v", len(messages), tt.want, messages)
			}
			if tt.wantReason != "" && messages[0].Data["reason"] != tt.wantReason {
				t.Fatalf("reason = %v, want %q", messages[0].Data["reason"], tt.wantReason)
			}
			if tt.name == "terminal_iteration_once" || tt.name == "recovery_duplicate" {
				var key string
				if err := base.DB.QueryRow(`SELECT idempotency_key FROM mail WHERE id=?`, messages[0].ID).Scan(&key); err != nil {
					t.Fatal(err)
				}
				if key != "task-goal:worker:T-1:1:iter-7" {
					t.Fatalf("idempotency key = %q", key)
				}
			}
		})
	}
}

func seedTerminalIteration(t *testing.T, base *basestore.Store, id, agentName string) {
	t.Helper()
	if _, err := base.DB.Exec(`INSERT INTO iterations(id,agent,trigger,status,started_at,ended_at)
		VALUES (?,?,'manual','done','2026-09-03T11:00:00Z','2026-09-03T11:01:00Z')`, id, agentName); err != nil {
		t.Fatal(err)
	}
}

func TestReconcilerSignalDoesNotBlockAndRunStops(t *testing.T) {
	r, _, _ := seededGoalReconciler(t)
	signaled := make(chan struct{})
	go func() {
		for range 1000 {
			r.Signal()
		}
		close(signaled)
	}()
	select {
	case <-signaled:
	case <-time.After(time.Second):
		t.Fatal("Signal blocked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestGoalLetterSuppressedWhileOpen(t *testing.T) {
	r, messageBus, _ := seededGoalReconciler(t)
	if err := r.Reconcile(context.Background(), "worker", ""); err != nil {
		t.Fatal(err)
	}
	// Rendered into a prompt, still unanswered: the letter stays open.
	if _, err := messageBus.RenderBatch("worker", 10); err != nil {
		t.Fatal(err)
	}
	if err := r.Reconcile(context.Background(), "worker", "next"); err != nil {
		t.Fatal(err)
	}
	onlyGoalMessage(t, messageBus, "worker")
}

func TestReconcilerDeliversNewGoalAfterDeadLetteredCooldown(t *testing.T) {
	now := goalNow
	_, messageBus, base := seededGoalReconciler(t)
	r := NewReconciler(ReconcilerConfig{Store: base, Mail: messageBus, Clock: func() time.Time { return now }})
	if err := r.Reconcile(context.Background(), "worker", ""); err != nil {
		t.Fatal(err)
	}
	for range 6 { // maxAttempts plus the render that dead-letters it.
		if _, err := messageBus.RenderBatch("worker", 10); err != nil {
			t.Fatal(err)
		}
	}
	s := NewStore(base)
	updateTask(t, s, "T-1", "status", "done")
	seedTask(t, s, "T-2", "agent:worker", "P0", "open", "2026-09-02T00:00:00Z")
	if err := r.Reconcile(context.Background(), "worker", "next"); err != nil {
		t.Fatal(err)
	}
	onlyGoalMessage(t, messageBus, "worker") // DLQ does not bypass cooldown.
	now = now.Add(time.Minute)
	if err := r.Reconcile(context.Background(), "worker", "next"); err != nil {
		t.Fatal(err)
	}
	messages, _, err := messageBus.List(mail.BoxInbox, "agent:worker", 10, "")
	if err != nil || len(messages) != 1 || messages[0].Data["task_key"] != "T-2" {
		t.Fatalf("inbox after cooldown = %#v, %v; want new goal T-2", messages, err)
	}
	var retained int
	if err := base.DB.QueryRow(`SELECT COUNT(*) FROM mail WHERE dead_reason='attempts'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("retained dead letters = %d, %v; want 1", retained, err)
	}
	now = now.Add(time.Minute)
	if err := r.Reconcile(context.Background(), "worker", "later"); err != nil {
		t.Fatal(err)
	}
	if messages := goalLetters(t, messageBus, "worker"); len(messages) != 2 {
		t.Fatalf("letters with an open new goal = %#v; want 2", messages)
	}
}

func TestReconcilerDeliversAgainAfterProcessedCooldown(t *testing.T) {
	now := goalNow
	_, messageBus, base := seededGoalReconciler(t)
	r := NewReconciler(ReconcilerConfig{Store: base, Mail: messageBus, Clock: func() time.Time { return now }})
	if err := r.Reconcile(context.Background(), "worker", ""); err != nil {
		t.Fatal(err)
	}
	first := onlyGoalMessage(t, messageBus, "worker")
	if _, _, err := messageBus.Reply(mail.ReplyRequest{As: "agent:worker", ID: first.ID, Text: "handled"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Reconcile(context.Background(), "worker", "next"); err != nil {
		t.Fatal(err)
	}
	if messages := goalLetters(t, messageBus, "worker"); len(messages) != 1 {
		t.Fatalf("letters before cooldown = %#v", messages)
	}
	now = now.Add(time.Minute)
	if err := r.Reconcile(context.Background(), "worker", "next"); err != nil {
		t.Fatal(err)
	}
	if messages := goalLetters(t, messageBus, "worker"); len(messages) != 2 {
		t.Fatalf("letters after cooldown = %#v", messages)
	}
}

// TestReconcilerDeliversAgainAfterIterationEndClosedTheGoal: the iteration end
// closes the Goal letter it covered; a task that still needs the agent gets a
// fresh Goal letter from the next reconciliation after the cooldown.
func TestReconcilerDeliversAgainAfterIterationEndClosedTheGoal(t *testing.T) {
	now := goalNow
	_, messageBus, base := seededGoalReconciler(t)
	r := NewReconciler(ReconcilerConfig{Store: base, Mail: messageBus, Clock: func() time.Time { return now }})
	if err := r.Reconcile(context.Background(), "worker", ""); err != nil {
		t.Fatal(err)
	}
	onlyGoalMessage(t, messageBus, "worker")
	tx, err := base.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if n, err := mail.CloseTaskNoticesTx(tx, "agent:worker", "T-1", []string{"task.goal"}, now, "iteration-end:iter-8", now); err != nil || n != 1 {
		_ = tx.Rollback()
		t.Fatalf("closed = %d, %v", n, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	r.IterationCompleted("worker", "iter-8")
	messages := goalLetters(t, messageBus, "worker")
	if len(messages) != 2 || messages[0].Data["reason"] != "iteration_completed" || messages[0].AnsweredAt != "" {
		t.Fatalf("letters after the iteration end = %#v", messages)
	}
}

func seededGoalReconciler(t *testing.T) (*Reconciler, *mail.Mail, *basestore.Store) {
	t.Helper()
	base := openGoalStore(t)
	s := NewStore(base)
	seedAgent(t, s, "worker", goalNow.Add(-time.Hour))
	seedTask(t, s, "T-1", "agent:worker", "P0", "open", "2026-09-01T00:00:00Z")
	messageBus := mail.New(base, func() time.Time { return goalNow }, goalDirectory{base})
	r := NewReconciler(ReconcilerConfig{
		Store: base, Mail: messageBus, Clock: func() time.Time { return goalNow },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return r, messageBus, base
}

// goalDirectory knows the agents of the store; nothing else is addressed.
type goalDirectory struct{ base *basestore.Store }

func (d goalDirectory) AgentExists(name string) (bool, error) {
	var n int
	err := d.base.DB.QueryRow(`SELECT COUNT(*) FROM agents WHERE name=?`, name).Scan(&n)
	return n > 0, err
}
func (goalDirectory) InboxCap(string) (int, error)                   { return 0, nil }
func (goalDirectory) CustomerLogin() string                          { return "customer" }
func (goalDirectory) PluginInstalled(string) (bool, error)           { return false, nil }
func (d goalDirectory) PluginReceivesMail(name string) (bool, error) { return d.PluginInstalled(name) }
func (goalDirectory) QueueExists(string) (bool, error)               { return false, nil }
func (goalDirectory) GroupMembers(string) ([]string, bool, error)    { return nil, false, nil }

// goalLetters is every letter sent to agent, newest first.
func goalLetters(t *testing.T, m *mail.Mail, agent string) []mail.Letter {
	t.Helper()
	letters, _, err := m.List(mail.BoxAll, "agent:"+agent, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	return letters
}

func onlyGoalMessage(t *testing.T, m *mail.Mail, agent string) mail.Letter {
	t.Helper()
	letters := goalLetters(t, m, agent)
	if len(letters) != 1 {
		t.Fatalf("letters = %#v, want one", letters)
	}
	return letters[0]
}

type failOncePublisher struct {
	delegate *mail.Mail
	to       string
	failed   bool
}

func (p *failOncePublisher) Send(letter mail.Letter) ([]mail.Letter, error) {
	if letter.To == p.to && !p.failed {
		p.failed = true
		return nil, errors.New("injected send failure")
	}
	return p.delegate.Send(letter)
}
