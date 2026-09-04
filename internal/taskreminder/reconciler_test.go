package taskreminder

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

func TestReconcilerPublishesGoalThroughInbox(t *testing.T) {
	r, messageBus, _ := seededGoalReconciler(t)
	if err := r.Reconcile(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	msg := onlyGoalMessage(t, messageBus, "worker")
	if msg.Type != "task.goal" || msg.Channel != "agent:worker:inbox" || msg.Source != "tasks" ||
		msg.Data["task_key"] != "T-1" || msg.Data["reason"] != "selected" {
		t.Fatalf("message = %#v", msg)
	}
}

func TestReconcilerGoalGenerations(t *testing.T) {
	tests := []struct {
		name       string
		want       int
		wantReason string
		run        func(*testing.T, *Reconciler, *bus.Bus, *basestore.Store) error
	}{
		{
			name: "startup_duplicate", want: 1, wantReason: "selected",
			run: func(t *testing.T, r *Reconciler, _ *bus.Bus, _ *basestore.Store) error {
				if err := r.Reconcile(context.Background(), "", ""); err != nil {
					return err
				}
				return r.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "recovery_duplicate", want: 1, wantReason: "selected",
			run: func(t *testing.T, r *Reconciler, messageBus *bus.Bus, base *basestore.Store) error {
				seedTerminalIteration(t, base, "iter-7", "worker")
				if err := r.Reconcile(context.Background(), "", ""); err != nil {
					return err
				}
				restarted := NewReconciler(ReconcilerConfig{Store: base, Bus: messageBus, Clock: func() time.Time { return goalNow }})
				return restarted.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "terminal_iteration_once", want: 1, wantReason: "iteration_completed",
			run: func(t *testing.T, r *Reconciler, _ *bus.Bus, _ *basestore.Store) error {
				r.IterationCompleted("worker", "iter-7")
				r.IterationCompleted("worker", "iter-7")
				return nil
			},
		},
		{
			name: "waiting_customer", want: 0,
			run: func(t *testing.T, r *Reconciler, _ *bus.Bus, base *basestore.Store) error {
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
			run: func(t *testing.T, r *Reconciler, _ *bus.Bus, base *basestore.Store) error {
				updateTask(t, NewStore(base), "T-1", "status", "done")
				return r.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "disabled_agent", want: 0,
			run: func(t *testing.T, r *Reconciler, _ *bus.Bus, base *basestore.Store) error {
				if _, err := base.DB.Exec(`UPDATE agents SET enabled=0 WHERE name='worker'`); err != nil {
					return err
				}
				return r.Reconcile(context.Background(), "", "")
			},
		},
		{
			name: "first_publish_fails", want: 1, wantReason: "selected",
			run: func(t *testing.T, _ *Reconciler, messageBus *bus.Bus, base *basestore.Store) error {
				s := NewStore(base)
				seedAgent(t, s, "broken", goalNow.Add(-time.Hour))
				seedTask(t, s, "T-2", "agent:broken", "P0", "open", "2026-09-01T00:00:00Z")
				if _, err := messageBus.Subscribe("broken", bus.InboxChannel("broken"), bus.Matcher{}, nil); err != nil {
					return err
				}
				failing := &failOncePublisher{delegate: messageBus, channel: bus.InboxChannel("broken")}
				r := NewReconciler(ReconcilerConfig{Store: base, Bus: failing, Clock: func() time.Time { return goalNow }})
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
			messages, err := messageBus.MessagesSince(bus.InboxChannel("worker"), "", 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(messages) != tt.want {
				t.Fatalf("message count = %d, want %d: %#v", len(messages), tt.want, messages)
			}
			if tt.wantReason != "" && messages[0].Data["reason"] != tt.wantReason {
				t.Fatalf("reason = %v, want %q", messages[0].Data["reason"], tt.wantReason)
			}
			if tt.name == "terminal_iteration_once" || tt.name == "recovery_duplicate" {
				var key string
				if err := base.DB.QueryRow(`SELECT idempotency_key FROM messages WHERE id=?`, messages[0].ID).Scan(&key); err != nil {
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

func seededGoalReconciler(t *testing.T) (*Reconciler, *bus.Bus, *basestore.Store) {
	t.Helper()
	base := openGoalStore(t)
	s := NewStore(base)
	seedAgent(t, s, "worker", goalNow.Add(-time.Hour))
	seedTask(t, s, "T-1", "agent:worker", "P0", "open", "2026-09-01T00:00:00Z")
	messageBus := goalBus(t, base, "worker")
	r := NewReconciler(ReconcilerConfig{
		Store: base, Bus: messageBus, Clock: func() time.Time { return goalNow },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return r, messageBus, base
}

func goalBus(t *testing.T, base *basestore.Store, agents ...string) *bus.Bus {
	t.Helper()
	messageBus := bus.New(base, func() time.Time { return goalNow })
	for _, name := range agents {
		if _, err := messageBus.Subscribe(name, bus.InboxChannel(name), bus.Matcher{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	return messageBus
}

func onlyGoalMessage(t *testing.T, messageBus *bus.Bus, agent string) bus.Message {
	t.Helper()
	messages, err := messageBus.MessagesSince(bus.InboxChannel(agent), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one", messages)
	}
	return messages[0]
}

type failOncePublisher struct {
	delegate *bus.Bus
	channel  string
	failed   bool
}

func (p *failOncePublisher) Publish(message bus.Message) (bus.Message, error) {
	if message.Channel == p.channel && !p.failed {
		p.failed = true
		return bus.Message{}, errors.New("injected publish failure")
	}
	return p.delegate.Publish(message)
}
