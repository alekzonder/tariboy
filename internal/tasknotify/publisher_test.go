package tasknotify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/tasks"
)

type fakeBus struct {
	messages []mail.Letter
	fail     bool
	// refuse is a sender the fake refuses as unknown.
	refuse string
	// refuseTo is a recipient the fake refuses as unknown.
	refuseTo string
}

func (f *fakeBus) Send(message mail.Letter) ([]mail.Letter, error) {
	if f.refuse != "" && message.From == f.refuse {
		return nil, &mail.Error{Status: 400, Code: "unknown_sender", Msg: "unknown sender"}
	}
	if f.refuseTo != "" && message.To == f.refuseTo {
		f.messages = append(f.messages, message)
		return nil, &mail.Error{Status: 400, Code: "unknown_recipient", Msg: "unknown recipient " + message.To}
	}
	f.messages = append(f.messages, message)
	if f.fail {
		return nil, errors.New("mail unavailable")
	}
	message.ID = "message-" + message.IdempotencyKey
	return []mail.Letter{message}, nil
}

func TestNotificationOutboxSendsFromCausingPrincipal(t *testing.T) {
	st := storetest.Open(t)
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
	_, err := svc.AddComment(ctx, tasks.AgentActor("worker"), assigned.Key, tasks.AddCommentInput{
		Body: "Question for @user:customer",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeBus{}
	publisher := New(st.DB, fake, func() time.Time { return now },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, customer, assigned.Key, tasks.AddCommentInput{
		Body: "Answer",
	}); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	type route struct{ from, to string }
	got := map[string]route{}
	for _, message := range fake.messages {
		got[message.Kind] = route{message.From, message.To}
		if message.IdempotencyKey == "" || message.Data["task_key"] == nil {
			t.Fatalf("%s has no idempotency key or task key: %+v", message.Kind, message)
		}
		if message.NeedsReply != (message.Kind == "task.question") {
			t.Fatalf("%s needs_reply = %v", message.Kind, message.NeedsReply)
		}
	}
	want := map[string]route{
		"task.triage":   {"user:customer", "agent:triager"},
		"task.assigned": {"user:customer", "agent:worker"},
		"task.question": {"agent:worker", "user:customer"},
		"task.answered": {"user:customer", "agent:worker"},
	}
	for kind, r := range want {
		if got[kind] != r {
			t.Fatalf("%s = %+v; want %+v (all=%v)", kind, got[kind], r, got)
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
	st := storetest.Open(t)
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

// A retry due 11ns after a second is not due 10ns after it, although the
// RFC 3339 text of the two times has different widths.
func TestRetryIsNotPublishedBeforeItIsDueWithinOneSecond(t *testing.T) {
	st := storetest.Open(t)
	second := time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC)
	now := second
	svc := tasks.NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, tasks.CreateQueueInput{
		Prefix: "RETRY", Name: "Retry", ResponsibleAgent: "triager",
	})
	_, _ = svc.CreateTask(ctx, customer, tasks.CreateTaskInput{
		Queue: "RETRY", Title: "retry me",
	})
	if _, err := st.DB.Exec("UPDATE task_notification_outbox SET next_attempt_at = ?",
		store.FormatTime(second.Add(11*time.Nanosecond))); err != nil {
		t.Fatal(err)
	}
	fake := &fakeBus{}
	publisher := New(st.DB, fake, func() time.Time { return now },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	now = second.Add(10 * time.Nanosecond)
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 0 {
		t.Fatalf("published %d messages before they were due", len(fake.messages))
	}
	now = second.Add(11 * time.Nanosecond)
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("published %d messages when due; want 1", len(fake.messages))
	}
}

// A question answered on the task before the outbox sent it never arrives;
// a sender that can no longer send falls back to system:tasks.
func TestAnsweredQuestionIsNotSentAndUnknownSenderFallsBack(t *testing.T) {
	st := storetest.Open(t)
	now := time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC)
	svc := tasks.NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: "NOTE", Name: "Notify"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "NOTE", Title: "t", Assignee: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, tasks.AgentActor("worker"), task.Key, tasks.AddCommentInput{Body: "@user:customer ?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, customer, task.Key, tasks.AddCommentInput{Body: "yes"}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeBus{refuse: "user:customer"}
	publisher := New(st.DB, fake, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	for _, message := range fake.messages {
		if message.Kind == "task.question" {
			t.Fatalf("an answered question was sent: %+v", message)
		}
		if message.From != SenderTasks {
			t.Fatalf("%s from %s; want the system:tasks fallback", message.Kind, message.From)
		}
	}
	if len(fake.messages) != 2 {
		t.Fatalf("sent %d letters; want the assignment and the answer", len(fake.messages))
	}
}

// A recipient deleted after the enqueue is a permanent error: the row is
// closed as failed after one attempt and logged once.
func TestOutboxStopsOnUnknownRecipient(t *testing.T) {
	st := storetest.Open(t)
	now := time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC)
	svc := tasks.NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, tasks.CreateQueueInput{
		Prefix: "GONE", Name: "Gone", ResponsibleAgent: "triager",
	})
	_, _ = svc.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "GONE", Title: "orphan"})

	fake := &fakeBus{refuseTo: "agent:triager"}
	var logs bytes.Buffer
	publisher := New(st.DB, fake, func() time.Time { return now }, slog.New(slog.NewTextHandler(&logs, nil)))
	for range 2 {
		if err := publisher.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Hour)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("send attempts = %d; want 1", len(fake.messages))
	}
	var published, message, lastError string
	if err := st.DB.QueryRow(`SELECT published_at, published_message, last_error FROM task_notification_outbox
		WHERE channel = 'agent:triager'`).Scan(&published, &message, &lastError); err != nil {
		t.Fatal(err)
	}
	if published == "" || message != "" || !strings.Contains(lastError, "unknown recipient") {
		t.Fatalf("row = published %q, message %q, last_error %q; want it closed as failed", published, message, lastError)
	}
	if n := strings.Count(logs.String(), "task notification dropped"); n != 1 {
		t.Fatalf("drop logged %d times; want once:\n%s", n, logs.String())
	}
}
