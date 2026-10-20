package tasks

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

func triggerFixture(t *testing.T) (*Service, Actor) {
	t.Helper()
	svc := newTestService(t)
	operator := CustomerActor("customer")
	if _, err := svc.CreateQueue(context.Background(), operator, CreateQueueInput{Prefix: "DEV", Name: "Development"}); err != nil {
		t.Fatal(err)
	}
	seedAgent(t, svc, "dev-a", "")
	return svc, operator
}

func mustCreateTrigger(t *testing.T, svc *Service, operator Actor, pattern string) QueueWorkflowTrigger {
	t.Helper()
	trigger, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: pattern, Action: "create_task"})
	if err != nil {
		t.Fatal(err)
	}
	return trigger
}

func countTasksMentioning(t *testing.T, svc *Service, text string) int {
	t.Helper()
	return len(tasksMentioning(t, svc, "DEV", text))
}

func TestReconcileWorkflowObservationsRecoversCommittedBusMessage(t *testing.T) {
	svc, operator := triggerFixture(t)
	mustCreateTrigger(t, svc, operator, "metrics:api")
	// A committed message without a commit sequence: the bus always records
	// one, so only a row written before sequences existed lacks it.
	insertMessage(t, svc.db, legacyMessage{ID: "durable-1", Channel: "metrics:api", TS: "2099-01-01T00:00:00Z",
		Source: "plugin:metrics", Type: "metric.alert", Text: "API alert"})
	if n, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil || n != 1 {
		t.Fatalf("reconcile = %d, %v", n, err)
	}
	if n, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil || n != 0 {
		t.Fatalf("second reconcile = %d, %v", n, err)
	}
	if got := countTasksMentioning(t, svc, "durable-1"); got != 1 {
		t.Fatalf("triggered tasks = %d; want 1", got)
	}
}

func TestClearPendingPreservesUnreconciledTriggerMessage(t *testing.T) {
	svc, operator := triggerFixture(t)
	mustCreateTrigger(t, svc, operator, "metrics:api")
	b := bus.New(&basestore.Store{DB: svc.db}, svc.clock)
	if _, err := b.Subscribe("dev-a", "metrics:api", nil, nil); err != nil {
		t.Fatal(err)
	}
	published, err := b.Publish(bus.Message{Channel: "metrics:api", Source: "plugin:metrics", Type: "metric.alert", Text: "queue pressure"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := b.ClearPending("dev-a"); err != nil || got.DeletedDeliveries != 1 || got.DeletedMessages != 0 {
		t.Fatalf("clear result = %+v, err=%v", got, err)
	}
	if n, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil || n != 1 {
		t.Fatalf("reconcile = %d, %v", n, err)
	}
	if got := countTasksMentioning(t, svc, published.ID); got != 1 {
		t.Fatalf("triggered tasks = %d; want 1", got)
	}
}

func TestReconcileWorkflowObservationsUsesCommitSequenceNotTimestampOrID(t *testing.T) {
	svc, operator := triggerFixture(t)
	mustCreateTrigger(t, svc, operator, "metrics:*")
	ts := "2099-01-01T00:00:00Z"
	// Ids that sort against commit order and one shared timestamp: the bus
	// chooses both itself, so the rows are written directly.
	for _, message := range []struct{ id, channel string }{{"z-first", "metrics:z"}, {"a-second", "metrics:a"}} {
		insertMessage(t, svc.db, legacyMessage{ID: message.id, Channel: message.channel, TS: ts,
			Source: "plugin:metrics", Type: "alert", Sequenced: true})
	}
	if n, err := svc.ReconcileWorkflowObservations(context.Background(), 10); err != nil || n != 2 {
		t.Fatalf("reconcile = %d, %v", n, err)
	}
	if cursor := ingressCursor(t, svc); cursor != 2 {
		t.Fatalf("cursor = %d", cursor)
	}
	var first, second int64
	if tasks := tasksMentioning(t, svc, "DEV", "z-first"); len(tasks) == 1 {
		first = tasks[0].ID
	}
	if tasks := tasksMentioning(t, svc, "DEV", "a-second"); len(tasks) == 1 {
		second = tasks[0].ID
	}
	if first == 0 || second == 0 || first > second {
		t.Fatalf("task ids first/second = %d/%d; want commit order", first, second)
	}
}

func insertWorkflowIngressMessage(t *testing.T, svc *Service, id, channel, eventAt string) int64 {
	t.Helper()
	// The event time is the message timestamp, which the bus would take from
	// its clock.
	return insertMessage(t, svc.db, legacyMessage{ID: id, Channel: channel, TS: eventAt, Text: id, Sequenced: true})
}

func TestTriggerActivationSequenceExcludesEarlierCommittedMessage(t *testing.T) {
	svc, operator := triggerFixture(t)
	svc.db.SetMaxOpenConns(4)
	tx, err := svc.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := reserveWorkflowIngressWriter(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	// The message commits inside this transaction, which holds the ingress
	// writer; the bus cannot publish into a caller's transaction.
	oldSequence := insertMessage(t, tx, legacyMessage{ID: "trigger-before", Channel: "issue-provider:issues",
		TS: "2099-01-01T00:00:00Z", Text: "trigger-before", Sequenced: true})
	created := make(chan struct {
		trigger QueueWorkflowTrigger
		err     error
	}, 1)
	go func() {
		trigger, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", Action: "create_task"})
		created <- struct {
			trigger QueueWorkflowTrigger
			err     error
		}{trigger, err}
	}()
	select {
	case result := <-created:
		t.Fatalf("activation committed ahead of earlier message: %#v, %v", result.trigger, result.err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	result := <-created
	trigger, err := result.trigger, result.err
	if err != nil {
		t.Fatal(err)
	}
	newSequence := insertWorkflowIngressMessage(t, svc, "trigger-after", "issue-provider:issues", "2099-01-01T00:00:00Z")
	if trigger.CreatedAfterSequence != oldSequence || newSequence <= trigger.CreatedAfterSequence {
		t.Fatalf("trigger watermark=%d old/new=%d/%d", trigger.CreatedAfterSequence, oldSequence, newSequence)
	}
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if oldTasks, newTasks := countTasksMentioning(t, svc, "trigger-before"), countTasksMentioning(t, svc, "trigger-after"); oldTasks != 0 || newTasks != 1 {
		t.Fatalf("triggered tasks old/new=%d/%d; want 0/1", oldTasks, newTasks)
	}
}

func TestNewTriggerAtSequenceZeroUsesCommitOrderNotExternalTimestamp(t *testing.T) {
	svc, operator := triggerFixture(t)
	clearMessages(t, svc)
	trigger := mustCreateTrigger(t, svc, operator, "issue-provider:*")
	if trigger.CreatedAfterSequence != 0 || !trigger.ActivationSequenceSet {
		t.Fatalf("new zero-sequence trigger = %#v", trigger)
	}
	insertWorkflowIngressMessage(t, svc, "first-after-activation", "issue-provider:issues", "2000-01-01T00:00:00Z")
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if tasks := countTasksMentioning(t, svc, "first-after-activation"); tasks != 1 {
		t.Fatalf("first post-activation event with old external timestamp created %d tasks; want 1", tasks)
	}
}

func TestLegacyTargetWithoutActivationMarkerUsesTimestampBoundary(t *testing.T) {
	created := "2026-08-07T12:00:00.000000000Z"
	if workflowTargetEligible(10, 0, false, "2026-08-07T11:59:59.000000000Z", created) {
		t.Fatal("legacy target accepted an event older than its creation time")
	}
	if !workflowTargetEligible(10, 0, false, "2026-08-07T12:00:01.000000000Z", created) {
		t.Fatal("legacy target rejected an event newer than its creation time")
	}
}

// An observation's event_at comes from outside: any RFC 3339 offset is read
// as its instant, and a value that is not a time is refused.
func TestWorkflowObservationEventAtAcceptsAnyOffset(t *testing.T) {
	svc, operator := triggerFixture(t)
	created := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return created }
	mustCreateTrigger(t, svc, operator, "issue-provider:*")
	observe := func(id, eventAt string) error {
		return svc.ApplyWorkflowObservation(context.Background(), ApplyWorkflowObservationInput{
			EventID: id, EventAt: eventAt, Channel: "issue-provider:issues", Kind: "issue.created",
			Payload: map[string]any{"text": id, "source": "plugin:issue-provider"}})
	}
	// 13:59:59+02:00 is one second before the trigger was created.
	if err := observe("before-creation", "2026-08-07T13:59:59+02:00"); err != nil {
		t.Fatal(err)
	}
	// 14:00:00.5+02:00 is half a second after it.
	if err := observe("after-creation", "2026-08-07T14:00:00.5+02:00"); err != nil {
		t.Fatal(err)
	}
	if n := countTasksMentioning(t, svc, "before-creation"); n != 0 {
		t.Fatalf("an event before the trigger created %d tasks", n)
	}
	if n := countTasksMentioning(t, svc, "after-creation"); n != 1 {
		t.Fatalf("an event after the trigger created %d tasks; want 1", n)
	}
	err := observe("garbage", "yesterday")
	if ErrorCode(err) != "invalid_observation" || err.Error() != "event_at must be an RFC 3339 time" {
		t.Fatalf("event_at that is not a time: %v", err)
	}
}

func TestTriggerActivationBackfillsPreexistingUnsequencedMessageBeforeWatermark(t *testing.T) {
	svc, operator := triggerFixture(t)
	// A row written before commit sequences existed; the bus always records one.
	insertMessage(t, svc.db, legacyMessage{ID: "legacy-trigger-message", Channel: "issue-provider:issues",
		TS: "2099-01-01T00:00:00Z", Text: "legacy-trigger-message"})
	trigger := mustCreateTrigger(t, svc, operator, "issue-provider:*")
	repairedSequence := messageSequence(t, svc, "legacy-trigger-message")
	if repairedSequence > trigger.CreatedAfterSequence {
		t.Fatalf("repaired sequence %d is after activation watermark %d", repairedSequence, trigger.CreatedAfterSequence)
	}
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if tasks := countTasksMentioning(t, svc, "legacy-trigger-message"); tasks != 0 {
		t.Fatalf("pre-activation unsequenced message created %d tasks", tasks)
	}
}

func TestConcurrentWorkflowObservationReconcilersNeverRegressCursor(t *testing.T) {
	svc := newTestService(t)
	for i := 0; i < 20; i++ {
		// Ids that sort against commit order, which the bus never chooses.
		id := fmt.Sprintf("message-%02d", 19-i)
		insertMessage(t, svc.db, legacyMessage{ID: id, Channel: "metrics:x", TS: svc.now(),
			Source: "plugin:metrics", Type: "alert", Sequenced: true})
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				if _, err := svc.ReconcileWorkflowObservations(context.Background(), 7); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent reconcile: %v", err)
	}
	if cursor := ingressCursor(t, svc); cursor != 20 {
		t.Fatalf("cursor = %d; want 20", cursor)
	}
}

func TestZeroTargetCursorAdvanceSerializesAgainstTargetCreation(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateQueue(context.Background(), CustomerActor("customer"), CreateQueueInput{Prefix: "DEV", Name: "Development"}); err != nil {
		t.Fatal(err)
	}
	insertMessage(t, svc.db, legacyMessage{ID: "overlap-event", Channel: "issue-provider:issues", TS: svc.now(),
		Source: "plugin:issue-provider", Type: "issue.created", Text: "overlap", Sequenced: true})

	counted := make(chan struct{})
	release := make(chan struct{})
	svc.workflowIngressAfterTargetCount = func() {
		close(counted)
		<-release
	}
	reconciled := make(chan error, 1)
	go func() {
		_, err := svc.ReconcileWorkflowObservations(context.Background(), 100)
		reconciled <- err
	}()
	<-counted

	created := make(chan error, 1)
	go func() {
		// A bare row insert: the test checks that SQLite itself serializes
		// it behind the reconciler, not the service's own locking.
		_, err := svc.db.Exec(`INSERT INTO task_queue_workflow_triggers(queue_prefix,pattern,correlation_key,action,enabled,created_by,created_at,updated_at) VALUES('DEV','issue-provider:*','','create_task',1,'operator',?,?)`, svc.now(), svc.now())
		created <- err
	}()
	select {
	case err := <-created:
		t.Fatalf("target creation committed between zero-target count and cursor advance: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-reconciled; err != nil {
		t.Fatal(err)
	}
	if err := <-created; err != nil {
		t.Fatal(err)
	}
	if cursor := ingressCursor(t, svc); cursor != 1 {
		t.Fatalf("cursor = %d; want 1", cursor)
	}
}

func TestQueueWorkflowTriggerCreatesFlexibleTaskIdempotently(t *testing.T) {
	svc, operator := triggerFixture(t)
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", Action: "run_script"}); ErrorCode(err) != "invalid_trigger_action" {
		t.Fatalf("invalid action error = %v", err)
	}
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "agent:*", Action: "create_task"}); ErrorCode(err) != "invalid_channel_pattern" {
		t.Fatalf("recursive internal trigger error = %v", err)
	}
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", CorrelationKey: "issue-7", Action: "create_task"}); err != nil {
		t.Fatal(err)
	}
	notPlugin := ApplyWorkflowObservationInput{EventID: "user-event", Channel: "issue-provider:issues", Kind: "issue.created", CorrelationKey: "issue-7", Payload: map[string]any{"text": "From a user", "source": "user:someone"}}
	if err := svc.ApplyWorkflowObservation(context.Background(), notPlugin); err != nil {
		t.Fatal(err)
	}
	in := ApplyWorkflowObservationInput{EventID: "issue-provider-event-7", Channel: "issue-provider:issues", Kind: "issue.created", CorrelationKey: "issue-7", Payload: map[string]any{"text": "Fix production", "source": "plugin:issue-provider"}}
	for range 2 {
		if err := svc.ApplyWorkflowObservation(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	var flexible, fromUser int
	for _, task := range queueTasks(t, svc, "DEV") {
		if task.Title == "Fix production" && task.Status == StatusOpen && task.WorkflowDigest == "" {
			flexible++
		}
		if task.Title == "From a user" {
			fromUser++
		}
	}
	if flexible != 1 {
		t.Fatalf("trigger tasks = %d", flexible)
	}
	if fromUser != 0 {
		t.Fatalf("non-plugin message tasks = %d; want 0", fromUser)
	}
}

func TestQueueWorkflowTriggerIsOperatorOnlyAndPersistent(t *testing.T) {
	svc, operator := triggerFixture(t)
	in := CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", CorrelationKey: "issue", Action: "create_task"}
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), AgentActor("dev-a"), "DEV", in); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent trigger error = %v; want forbidden", err)
	}
	created, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", in)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.WorkflowIngressEnabled() {
		t.Fatal("trigger did not enable workflow ingress")
	}
	listed, err := svc.ListQueueWorkflowTriggers(context.Background(), operator, "DEV")
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed = %#v, %v", listed, err)
	}
	restarted := NewService(svc.db, "customer", svc.clock)
	recovered, err := restarted.ListQueueWorkflowTriggers(context.Background(), operator, "DEV")
	if err != nil || len(recovered) != 1 || recovered[0].ID != created.ID {
		t.Fatalf("restart recovery = %#v, %v", recovered, err)
	}
	if !restarted.WorkflowIngressEnabled() {
		t.Fatal("restarted service did not enable workflow ingress for a persisted trigger")
	}
	if err := svc.DeleteQueueWorkflowTrigger(context.Background(), AgentActor("dev-a"), "DEV", created.ID); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent delete = %v", err)
	}
	if err := svc.DeleteQueueWorkflowTrigger(context.Background(), operator, "DEV", created.ID); err != nil {
		t.Fatal(err)
	}
	if svc.WorkflowIngressEnabled() {
		t.Fatal("deleted last trigger left workflow ingress enabled")
	}
}
