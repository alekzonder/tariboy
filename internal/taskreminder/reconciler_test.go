package taskreminder

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

func TestReconcilerDisabledPolicyPublishesNothing(t *testing.T) {
	base := openReminderStore(t)
	now := mustReminderTime(t, "2026-08-21T10:05:00Z")
	insertReminderAgent(t, base, "worker", true, true, 0, "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-1", "worker", "open", "2026-08-21T09:00:00Z")
	messageBus := reminderBus(t, base, now, "worker")

	reconciler := NewReconciler(ReconcilerConfig{
		Store: base, Bus: messageBus, Clock: func() time.Time { return now },
	})
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	messages, err := messageBus.MessagesSince(bus.InboxChannel("worker"), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %#v, want none while policy is disabled", messages)
	}
}

func TestReconcilerPublishesOrdinaryInboxReminderAndSuppressesRestartDuplicate(t *testing.T) {
	base := openReminderStore(t)
	now := mustReminderTime(t, "2026-08-21T10:05:00Z")
	if err := base.ConfigSet("task_reminder", `{"enabled":true,"idle_threshold_s":300}`); err != nil {
		t.Fatal(err)
	}
	insertReminderAgent(t, base, "worker", true, true, 0, "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-2", "worker", "open", "2026-08-21T10:00:00Z")
	insertReminderTask(t, base, "REM-1", "worker", "in_progress", "2026-08-21T10:00:00Z")
	messageBus := reminderBus(t, base, now, "worker")

	reconciler := NewReconciler(ReconcilerConfig{
		Store: base, Bus: messageBus, Clock: func() time.Time { return now },
	})
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	messages, err := messageBus.MessagesSince(bus.InboxChannel("worker"), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one reminder", messages)
	}
	got := messages[0]
	if got.Channel != "agent:worker:inbox" || got.Source != "tasks" || got.Type != "task.reminder" {
		t.Fatalf("message route = channel:%q source:%q type:%q", got.Channel, got.Source, got.Type)
	}
	if got.Data["reason"] != "assigned-work-idle" || got.Data["idle_threshold_s"] != float64(300) {
		t.Fatalf("message data = %#v, want reason and threshold", got.Data)
	}
	if want := []any{"REM-1", "REM-2"}; !reflect.DeepEqual(got.Data["task_keys"], want) {
		t.Fatalf("task keys = %#v, want %#v", got.Data["task_keys"], want)
	}

	var fingerprint, idempotencyKey string
	if err := base.DB.QueryRow(`SELECT fingerprint FROM task_reminders WHERE agent='worker'`).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := base.DB.QueryRow(`SELECT idempotency_key FROM messages WHERE id=?`, got.ID).Scan(&idempotencyKey); err != nil {
		t.Fatal(err)
	}
	if idempotencyKey != "task-reminder:"+fingerprint {
		t.Fatalf("idempotency key = %q, want fingerprint-derived key", idempotencyKey)
	}

	// A replacement reconciler against the same durable store represents the
	// daemon restart boundary. The unchanged generation must stay suppressed.
	restarted := NewReconciler(ReconcilerConfig{
		Store: base, Bus: messageBus, Clock: func() time.Time { return now.Add(time.Hour) },
	})
	if err := restarted.Reconcile(context.Background()); err != nil {
		t.Fatalf("restart Reconcile: %v", err)
	}
	messages, err = messageBus.MessagesSince(bus.InboxChannel("worker"), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages after restart scan = %#v, want original only", messages)
	}
}

func TestReconcilerContinuesAfterCandidatePublishFailureAndMarksOnlySuccess(t *testing.T) {
	base := openReminderStore(t)
	now := mustReminderTime(t, "2026-08-21T10:05:00Z")
	if err := base.ConfigSet("task_reminder", `{"enabled":true,"idle_threshold_s":300}`); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"broken", "worker"} {
		insertReminderAgent(t, base, agent, true, true, 0, "2026-08-21T09:00:00Z")
		insertReminderTask(t, base, "REM-"+strings.ToUpper(agent[:1]), agent, "open", "2026-08-21T09:00:00Z")
	}
	realBus := reminderBus(t, base, now, "broken", "worker")
	publisher := &failOncePublisher{delegate: realBus, channel: bus.InboxChannel("broken")}
	reconciler := NewReconciler(ReconcilerConfig{
		Store: base, Bus: publisher, Clock: func() time.Time { return now },
	})

	if err := reconciler.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("Reconcile error = %v, want broken-agent publish error", err)
	}
	brokenMessages, err := realBus.MessagesSince(bus.InboxChannel("broken"), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	workerMessages, err := realBus.MessagesSince(bus.InboxChannel("worker"), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(brokenMessages) != 0 || len(workerMessages) != 1 {
		t.Fatalf("messages after partial failure = broken:%#v worker:%#v", brokenMessages, workerMessages)
	}
	var markedAgent string
	if err := base.DB.QueryRow(`SELECT agent FROM task_reminders`).Scan(&markedAgent); err != nil {
		t.Fatal(err)
	}
	if markedAgent != "worker" {
		t.Fatalf("marked agent = %q, want only successfully published worker", markedAgent)
	}
}

func TestReconcilerRunLogsErrorsContinuesPeriodicallyAndStops(t *testing.T) {
	base := openReminderStore(t)
	now := mustReminderTime(t, "2026-08-21T10:05:00Z")
	if err := base.ConfigSet("task_reminder", `{"enabled":"invalid","idle_threshold_s":300}`); err != nil {
		t.Fatal(err)
	}
	insertReminderAgent(t, base, "worker", true, true, 0, "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-1", "worker", "open", "2026-08-21T09:00:00Z")
	messageBus := reminderBus(t, base, now, "worker")
	var logs lockedBuffer
	reconciler := NewReconciler(ReconcilerConfig{
		Store: base, Bus: messageBus, Clock: func() time.Time { return now },
		Interval: 5 * time.Millisecond,
		Log:      slog.New(slog.NewTextHandler(&logs, nil)),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reconciler.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("Run cleanup timed out")
		}
	})

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(logs.String(), "task reminder reconciliation") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(logs.String(), "task reminder reconciliation") {
		t.Fatalf("startup error was not logged: %q", logs.String())
	}
	if err := base.ConfigSet("task_reminder", `{"enabled":true,"idle_threshold_s":300}`); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		messages, err := messageBus.MessagesSince(bus.InboxChannel("worker"), "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	messages, err := messageBus.MessagesSince(bus.InboxChannel("worker"), "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages after corrected policy = %#v, %v", messages, err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}

func reminderBus(t *testing.T, base *basestore.Store, now time.Time, agents ...string) *bus.Bus {
	t.Helper()
	messageBus := bus.New(base, func() time.Time { return now })
	for _, agent := range agents {
		if _, err := messageBus.Subscribe(agent, bus.InboxChannel(agent), bus.Matcher{}, nil); err != nil {
			t.Fatalf("subscribe %s inbox: %v", agent, err)
		}
	}
	return messageBus
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

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
