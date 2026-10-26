package tasks

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
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

// queueDirectory knows the queues of the service and the plugin metrics.
type queueDirectory struct{ svc *Service }

func (queueDirectory) AgentExists(string) (bool, error)            { return false, nil }
func (queueDirectory) InboxCap(string) (int, error)                { return 0, nil }
func (queueDirectory) CustomerLogin() string                       { return "customer" }
func (queueDirectory) PluginInstalled(name string) (bool, error)   { return name == "metrics", nil }
func (queueDirectory) GroupMembers(string) ([]string, bool, error) { return nil, false, nil }
func (d queueDirectory) QueueExists(prefix string) (bool, error) {
	var n int
	err := d.svc.db.QueryRow(`SELECT COUNT(*) FROM task_queues WHERE prefix = ?`, prefix).Scan(&n)
	return n > 0, err
}

// TestQueueTriggerFiresForQueueRecipient: a letter to queue:DEV from a sender
// the trigger's glob matches, with the trigger's correlation key, creates one
// task titled by its subject; a letter to another queue, from another sender
// or with another key creates none.
func TestQueueTriggerFiresForQueueRecipient(t *testing.T) {
	svc, operator := triggerFixture(t)
	if _, err := svc.CreateQueue(context.Background(), operator, CreateQueueInput{Prefix: "OPS", Name: "Ops"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV",
		CreateQueueWorkflowTriggerInput{Pattern: "plugin:*", CorrelationKey: "issue-7", Action: "create_task"}); err != nil {
		t.Fatal(err)
	}
	m := mail.New(&basestore.Store{DB: svc.db}, svc.clock, queueDirectory{svc})
	send := func(to, subject, key string) mail.Letter {
		t.Helper()
		out, err := m.Send(mail.Letter{From: "plugin:metrics", To: to, Subject: subject, Body: "details",
			Kind: "issue.created", Data: map[string]any{"correlation_key": key}})
		if err != nil {
			t.Fatal(err)
		}
		if out[0].AnsweredAt == "" {
			t.Fatalf("a letter to a queue is stored open: %+v", out[0])
		}
		return out[0]
	}
	fires := send("queue:DEV", "Fix production", "issue-7")
	send("queue:OPS", "Other queue", "issue-7")
	send("queue:DEV", "Other key", "issue-8")
	for range 2 {
		if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
			t.Fatal(err)
		}
	}
	tasks := queueTasks(t, svc, "DEV")
	if len(tasks) != 1 || tasks[0].Title != "Fix production" || !strings.Contains(tasks[0].Description, fires.ID) ||
		!strings.Contains(tasks[0].Description, "plugin:metrics") {
		t.Fatalf("DEV tasks = %#v; want one task from %s", tasks, fires.ID)
	}
	if n := len(queueTasks(t, svc, "OPS")); n != 0 {
		t.Fatalf("OPS tasks = %d; OPS has no trigger", n)
	}
}

func TestReconcileWorkflowObservationsRecoversCommittedLetter(t *testing.T) {
	svc, operator := triggerFixture(t)
	mustCreateTrigger(t, svc, operator, "plugin:metrics")
	sendQueueLetter(t, svc, queueLetter{ID: "durable-1", Sender: "plugin:metrics", CreatedAt: "2099-01-01T00:00:00Z",
		Kind: "metric.alert", Body: "API alert"})
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

func TestReconcileWorkflowObservationsUsesCommitSequenceNotTimestampOrID(t *testing.T) {
	svc, operator := triggerFixture(t)
	mustCreateTrigger(t, svc, operator, "plugin:*")
	at := "2099-01-01T00:00:00Z"
	// Ids that sort against commit order and one shared time: mail chooses
	// both itself, so the rows are written directly.
	for _, id := range []string{"z-first", "a-second"} {
		sendQueueLetter(t, svc, queueLetter{ID: id, Sender: "plugin:metrics", CreatedAt: at, Kind: "alert"})
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

func TestTriggerActivationSequenceExcludesEarlierCommittedLetter(t *testing.T) {
	svc, operator := triggerFixture(t)
	svc.db.SetMaxOpenConns(4)
	tx, err := svc.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := reserveWorkflowIngressWriter(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	// The letter commits inside this transaction, which holds the ingress
	// writer.
	oldSequence := insertQueueLetter(t, tx, queueLetter{ID: "trigger-before", CreatedAt: "2099-01-01T00:00:00Z", Body: "trigger-before"})
	created := make(chan struct {
		trigger QueueWorkflowTrigger
		err     error
	}, 1)
	go func() {
		trigger, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "plugin:*", Action: "create_task"})
		created <- struct {
			trigger QueueWorkflowTrigger
			err     error
		}{trigger, err}
	}()
	select {
	case result := <-created:
		t.Fatalf("activation committed ahead of earlier letter: %#v, %v", result.trigger, result.err)
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
	newSequence := sendQueueLetter(t, svc, queueLetter{ID: "trigger-after", CreatedAt: "2099-01-01T00:00:00Z", Body: "trigger-after"})
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
	trigger := mustCreateTrigger(t, svc, operator, "plugin:*")
	if trigger.CreatedAfterSequence != 0 || !trigger.ActivationSequenceSet {
		t.Fatalf("new zero-sequence trigger = %#v", trigger)
	}
	sendQueueLetter(t, svc, queueLetter{ID: "first-after-activation", CreatedAt: "2000-01-01T00:00:00Z", Body: "first-after-activation"})
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if tasks := countTasksMentioning(t, svc, "first-after-activation"); tasks != 1 {
		t.Fatalf("first post-activation letter with an old time created %d tasks; want 1", tasks)
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
	mustCreateTrigger(t, svc, operator, "plugin:*")
	observe := func(id, eventAt string) error {
		return svc.ApplyWorkflowObservation(context.Background(), ApplyWorkflowObservationInput{
			EventID: id, EventAt: eventAt, Sender: "plugin:issue-provider", Queue: "DEV", Kind: "issue.created", Body: id})
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

func TestConcurrentWorkflowObservationReconcilersNeverRegressCursor(t *testing.T) {
	svc := newTestService(t)
	for i := 0; i < 20; i++ {
		// Ids that sort against commit order, which mail never chooses.
		sendQueueLetter(t, svc, queueLetter{ID: fmt.Sprintf("message-%02d", 19-i), Sender: "plugin:metrics", Kind: "alert"})
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
	sendQueueLetter(t, svc, queueLetter{ID: "overlap-event", Sender: "plugin:issue-provider", Kind: "issue.created", Body: "overlap"})

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
		_, err := svc.db.Exec(`INSERT INTO task_queue_workflow_triggers(queue_prefix,pattern,correlation_key,action,enabled,created_by,created_at,updated_at) VALUES('DEV','plugin:*','','create_task',1,'operator',?,?)`, svc.now(), svc.now())
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
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "plugin:*", Action: "run_script"}); ErrorCode(err) != "invalid_trigger_action" {
		t.Fatalf("invalid action error = %v", err)
	}
	for _, pattern := range []string{"issue-provider:*", "group:dev", "plugin:", "plugin:a*", "queue:DEV"} {
		if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: pattern, Action: "create_task"}); ErrorCode(err) != "invalid_sender_pattern" {
			t.Fatalf("pattern %q error = %v; want invalid_sender_pattern", pattern, err)
		}
	}
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "plugin:*", CorrelationKey: "issue-7", Action: "create_task"}); err != nil {
		t.Fatal(err)
	}
	fromUser := ApplyWorkflowObservationInput{EventID: "user-event", Sender: "user:someone", Queue: "DEV", Kind: "issue.created", CorrelationKey: "issue-7", Subject: "From a user"}
	if err := svc.ApplyWorkflowObservation(context.Background(), fromUser); err != nil {
		t.Fatal(err)
	}
	in := ApplyWorkflowObservationInput{EventID: "issue-provider-event-7", Sender: "plugin:issue-provider", Queue: "DEV", Kind: "issue.created", CorrelationKey: "issue-7", Subject: "Fix production"}
	for range 2 {
		if err := svc.ApplyWorkflowObservation(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	var flexible, users int
	for _, task := range queueTasks(t, svc, "DEV") {
		if task.Title == "Fix production" && task.Status == StatusOpen && task.WorkflowDigest == "" {
			flexible++
		}
		if task.Title == "From a user" {
			users++
		}
	}
	if flexible != 1 {
		t.Fatalf("trigger tasks = %d", flexible)
	}
	if users != 0 {
		t.Fatalf("tasks from a sender the glob does not match = %d; want 0", users)
	}
}

func TestQueueWorkflowTriggerIsOperatorOnlyAndPersistent(t *testing.T) {
	svc, operator := triggerFixture(t)
	in := CreateQueueWorkflowTriggerInput{Pattern: "plugin:*", CorrelationKey: "issue", Action: "create_task"}
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
