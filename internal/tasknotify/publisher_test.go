package tasknotify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

type fakeBus struct {
	messages []bus.Message
	fail     bool
}

func (f *fakeBus) Publish(message bus.Message) (bus.Message, error) {
	f.messages = append(f.messages, message)
	if f.fail {
		return bus.Message{}, errors.New("bus unavailable")
	}
	message.ID = "message-" + message.IdempotencyKey
	return message, nil
}

func TestFlushPublishesAssignmentQuestionAnswerAndTriageChannels(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC)
	svc := tasks.NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, tasks.CreateQueueInput{
		Prefix: "NOTE", Name: "Notify", ResponsibleAgent: "triager",
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.CreateTask(ctx, customer, tasks.CreateTaskInput{
		Queue: "NOTE", Title: "unassigned",
	})
	assigned, _ := svc.CreateTask(ctx, customer, tasks.CreateTaskInput{
		Queue: "NOTE", Title: "assigned", Assignee: "worker",
	})
	_, err = svc.AddComment(ctx, tasks.AgentActor("worker"), assigned.Key, tasks.AddCommentInput{
		Body: "Question for @user:customer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, customer, assigned.Key, tasks.AddCommentInput{
		Body: "Answer",
	}); err != nil {
		t.Fatal(err)
	}

	fake := &fakeBus{}
	publisher := New(st.DB, fake, func() time.Time { return now },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, message := range fake.messages {
		got[message.Type] = message.Channel
		if message.IdempotencyKey == "" {
			t.Fatalf("%s has no idempotency key", message.Type)
		}
	}
	want := map[string]string{
		"task.triage":   "agent:triager:inbox",
		"task.assigned": "agent:worker:inbox",
		"task.question": "user:customer",
		"task.answered": "agent:worker:inbox",
	}
	for typ, channel := range want {
		if got[typ] != channel {
			t.Fatalf("%s channel = %q; want %q (all=%v)", typ, got[typ], channel, got)
		}
	}
	count := len(fake.messages)
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != count {
		t.Fatalf("second flush published %d more messages", len(fake.messages)-count)
	}
}

func TestFailedPublishIsRetriedAfterBackoff(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC)
	svc := tasks.NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, tasks.CreateQueueInput{
		Prefix: "RETRY", Name: "Retry", ResponsibleAgent: "triager",
	})
	_, _ = svc.CreateTask(ctx, customer, tasks.CreateTaskInput{
		Queue: "RETRY", Title: "retry me",
	})

	fake := &fakeBus{fail: true}
	publisher := New(st.DB, fake, func() time.Time { return now },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("attempts = %d; want 1", len(fake.messages))
	}
	fake.fail = false
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("retried before backoff elapsed")
	}
	now = now.Add(3 * time.Second)
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 2 {
		t.Fatalf("attempts after backoff = %d; want 2", len(fake.messages))
	}
}

func TestFlushPublishesDurableWorkflowWakeAfterFailureAndRestart(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	svc := tasks.NewService(st.DB, "customer", clock)
	if err := agent.NewStore(st).Create(agent.Agent{Name: "worker", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	actor := tasks.CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, actor, tasks.CreateQueueInput{Prefix: "FLOW", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	def := tasks.WorkflowDefinition{Name: "wake", Version: 1, InitialStatus: "work", Statuses: []tasks.WorkflowStatus{
		{ID: "work", Requirements: []tasks.WorkflowRequirement{{ID: "do", Pool: "workers", Dispatch: tasks.DispatchClaimOne, Outcomes: []string{"done"}}}, Transitions: []tasks.WorkflowTransition{{When: "do.done", To: "done"}}},
		{ID: "done", Terminal: true, Requirements: []tasks.WorkflowRequirement{}, Transitions: []tasks.WorkflowTransition{}},
	}}
	draft, err := svc.CreateWorkflowDraft(ctx, actor, def)
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.PublishWorkflowVersion(ctx, actor, draft.Name, draft.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebindAgentPool(ctx, actor, "FLOW", "workers", []string{"worker"}, 0, "pool"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateQueueWorkflow(ctx, actor, "FLOW", published.ID, 0, "bind"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTask(ctx, actor, tasks.CreateTaskInput{Queue: "FLOW", Title: "wake me"}); err != nil {
		t.Fatal(err)
	}

	failing := &fakeBus{fail: true}
	first := New(st.DB, failing, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := first.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var publishedAt string
	if err := st.DB.QueryRow(`SELECT attempts, published_at FROM task_workflow_outbox`).Scan(&attempts, &publishedAt); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || publishedAt != "" {
		t.Fatalf("failed workflow wake attempts/published=%d/%q", attempts, publishedAt)
	}

	now = now.Add(3 * time.Second)
	recoveredBus := &fakeBus{}
	restarted := New(st.DB, recoveredBus, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := restarted.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(recoveredBus.messages) != 1 {
		t.Fatalf("recovered workflow wakes=%d, want 1", len(recoveredBus.messages))
	}
	message := recoveredBus.messages[0]
	if message.Channel != "agent:worker:inbox" || message.Type != "workflow.assignment_ready" || message.IdempotencyKey == "" {
		t.Fatalf("workflow wake=%#v", message)
	}
	if err := st.DB.QueryRow(`SELECT published_at FROM task_workflow_outbox`).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if publishedAt == "" {
		t.Fatal("workflow wake was not marked published")
	}
}
