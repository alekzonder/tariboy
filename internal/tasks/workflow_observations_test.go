package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func observationWorkflowTask(t *testing.T) (*Service, Actor, Task, Assignment) {
	return observationWorkflowTaskWithReactions(t, []string{"record_only", "wake_current", "hold_assignment"})
}

func observationWorkflowTaskWithReactions(t *testing.T, reactions []string) (*Service, Actor, Task, Assignment) {
	t.Helper()
	def := claimOneDefinition()
	def.Permissions.Channels.Subscribe = []string{"logs:${task.artifacts.service_id}", "metrics:*"}
	def.Permissions.Channels.Reactions = []string{"record_only", "wake_current", "create_requirement", "hold_assignment"}
	def.Observations = WorkflowObservationPolicy{OnLateEvent: "record_only", AllowedReactions: reactions}
	def.Statuses[0].Requirements[0].Produces = []string{"service_id"}
	svc, operator, task := runtimeWorkflowTask(t, def, map[string][]string{"developers": {"dev-a"}})
	work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("next work = %#v, %v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "claim-observation",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, operator, task, claimed
}

func TestWorkflowObservationCanCreateScopedOptionalRequirement(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTaskWithReactions(t, []string{"record_only", "create_requirement"})
	actor := AgentActor("dev-a")
	_, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "create_requirement", IdempotencyKey: "sub-requirement",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyWorkflowObservation(context.Background(), ApplyWorkflowObservationInput{EventID: "requirement-event", Channel: "metrics:api", Kind: "alert"}); err != nil {
		t.Fatal(err)
	}
	work, err := svc.NextWork(context.Background(), actor, "DEV", 10)
	if err != nil {
		t.Fatal(err)
	}
	var observationAssignment Assignment
	for _, candidate := range work {
		var requirement string
		if err := svc.db.QueryRow(`SELECT requirement_id FROM task_requirement_executions WHERE id=?`, candidate.RequirementExecutionID).Scan(&requirement); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(requirement, observationRequirementPrefix) {
			observationAssignment = candidate
		}
	}
	if observationAssignment.ID == 0 {
		t.Fatalf("work = %#v; missing observation requirement", work)
	}
	packet, err := svc.GetWorkPacket(context.Background(), actor, assignmentID(observationAssignment))
	if err != nil || len(packet.Observations) != 1 || packet.AllowedOutcomes[0] != "acknowledged" {
		t.Fatalf("packet = %#v, %v", packet, err)
	}
}

func TestWorkflowObservationCreatesRequirementPerDistinctEvent(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTaskWithReactions(t, []string{"record_only", "create_requirement"})
	actor := AgentActor("dev-a")
	_, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "create_requirement", IdempotencyKey: "sub-per-event",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"event/a", "event/b", "event/b"} {
		if _, err := svc.ApplyWorkflowObservation(context.Background(), ApplyWorkflowObservationInput{EventID: event, Channel: "metrics:api", Kind: "alert"}); err != nil {
			t.Fatal(err)
		}
	}
	var requirements, assignments int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_requirement_executions WHERE requirement_id LIKE '__observation:%'`).Scan(&requirements); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_assignments a JOIN task_requirement_executions r ON r.id=a.requirement_execution_id WHERE r.requirement_id LIKE '__observation:%'`).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if requirements != 2 || assignments != 2 {
		t.Fatalf("requirements/assignments = %d/%d; want 2/2", requirements, assignments)
	}
}

func TestWorkflowObservationTreatsExpiredLeaseAsLateBeforeLeaseSweep(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	actor := AgentActor("dev-a")
	sub, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "hold_assignment", IdempotencyKey: "sub-expired",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = sub
	svc.clock = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
	if _, err := svc.ApplyWorkflowObservation(context.Background(), ApplyWorkflowObservationInput{EventID: "expired-event", Channel: "metrics:api", Kind: "alert"}); err != nil {
		t.Fatal(err)
	}
	var holds int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_holds WHERE assignment_id=?`, assignment.ID).Scan(&holds); err != nil || holds != 0 {
		t.Fatalf("expired lease holds = %d, %v", holds, err)
	}
}

func TestListWorkflowSubscriptionsRequiresActiveOwnedLease(t *testing.T) {
	svc, operator, task, assignment := observationWorkflowTask(t)
	actor := AgentActor("dev-a")
	_, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "record_only", IdempotencyKey: "sub-list"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListWorkflowSubscriptions(context.Background(), Actor{}, assignmentID(assignment)); ErrorCode(err) != "forbidden" {
		t.Fatalf("empty actor error = %v", err)
	}
	if _, err := svc.ListWorkflowSubscriptions(context.Background(), AgentActor("other"), assignmentID(assignment)); ErrorCode(err) != "assignment_not_owned" {
		t.Fatalf("other agent error = %v", err)
	}
	if _, err := svc.db.Exec(`UPDATE task_assignments SET lease_expires_at='2020-01-01T00:00:00Z' WHERE id=?`, assignment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListWorkflowSubscriptions(context.Background(), actor, assignmentID(assignment)); ErrorCode(err) != "assignment_lease_expired" {
		t.Fatalf("expired owner error = %v", err)
	}
	if got, err := svc.ListWorkflowSubscriptions(context.Background(), operator, assignmentID(assignment)); err != nil || len(got) != 1 {
		t.Fatalf("operator list = %#v, %v", got, err)
	}
}

func TestWorkflowSubscriptionAllowsOnlyBoundedWildcard(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	actor := AgentActor("dev-a")
	if _, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:*", Reaction: "record_only", IdempotencyKey: "wildcard-ok"}); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"logs:*", "metrics:api:*", "*:*"} {
		_, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, Pattern: pattern, Reaction: "record_only", IdempotencyKey: "wildcard-bad-" + pattern})
		if ErrorCode(err) != "channel_not_allowed" {
			t.Fatalf("pattern %q error = %v", pattern, err)
		}
	}
}

func TestReconcileWorkflowObservationsRecoversCommittedBusMessage(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	_, err := svc.CreateWorkflowSubscription(context.Background(), AgentActor("dev-a"), assignmentID(assignment), CreateWorkflowSubscriptionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "record_only", IdempotencyKey: "sub-reconcile"})
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := json.Marshal(map[string]any{"service": "api"})
	data, _ := json.Marshal(map[string]any{"value": 7})
	if _, err := svc.db.Exec(`INSERT INTO channels(name,kind,created_at) VALUES('metrics:api','chat',?) ON CONFLICT DO NOTHING`, svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,text,data,kind) VALUES('durable-1','metrics:api',?,'plugin:metrics','metric.alert',?,NULL,?,'event')`, svc.now(), string(subject), string(data)); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil || n != 1 {
		t.Fatalf("reconcile = %d, %v", n, err)
	}
	if n, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil || n != 0 {
		t.Fatalf("second reconcile = %d, %v", n, err)
	}
	var observations int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_observations`).Scan(&observations); err != nil || observations != 1 {
		t.Fatalf("observations = %d, %v", observations, err)
	}
}

func TestReconcileWorkflowObservationsUsesCommitSequenceNotTimestampOrID(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	_, err := svc.CreateWorkflowSubscription(context.Background(), AgentActor("dev-a"), assignmentID(assignment), CreateWorkflowSubscriptionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:*", Reaction: "record_only", IdempotencyKey: "sub-sequence"})
	if err != nil {
		t.Fatal(err)
	}
	ts := svc.now()
	for _, message := range []struct{ id, channel string }{{"z-first", "metrics:z"}, {"a-second", "metrics:a"}} {
		if _, err := svc.db.Exec(`INSERT INTO channels(name,kind,created_at) VALUES(?,'chat',?) ON CONFLICT DO NOTHING`, message.channel, ts); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.db.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,kind) VALUES(?,?,?,'plugin:metrics','alert','{}','event')`, message.id, message.channel, ts); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.db.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES(?)`, message.id); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := svc.ReconcileWorkflowObservations(context.Background(), 10); err != nil || n != 2 {
		t.Fatalf("reconcile = %d, %v", n, err)
	}
	var cursor int64
	if err := svc.db.QueryRow(`SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton=1`).Scan(&cursor); err != nil || cursor != 2 {
		t.Fatalf("cursor = %d, %v", cursor, err)
	}
}

func insertWorkflowIngressMessage(t *testing.T, svc *Service, id, channel, eventAt string) int64 {
	t.Helper()
	if _, err := svc.db.Exec(`INSERT INTO channels(name,kind,created_at) VALUES(?,'chat',?) ON CONFLICT DO NOTHING`, channel, svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,text,data,kind) VALUES(?,?,?,'plugin:test','event','{}',?,'{}','event')`, id, channel, eventAt, id); err != nil {
		t.Fatal(err)
	}
	res, err := svc.db.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES(?)`, id)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return sequence
}

func TestTriggerActivationSequenceExcludesEarlierCommittedMessage(t *testing.T) {
	svc, operator, _, _ := observationWorkflowTask(t)
	svc.db.SetMaxOpenConns(4)
	tx, err := svc.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := reserveWorkflowIngressWriter(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO channels(name,kind,created_at) VALUES('issue-provider:issues','chat',?) ON CONFLICT DO NOTHING`, svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,text,data,kind) VALUES('trigger-before','issue-provider:issues','2099-01-01T00:00:00Z','plugin:test','event','{}','trigger-before','{}','event')`); err != nil {
		t.Fatal(err)
	}
	res, err := tx.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES('trigger-before')`)
	if err != nil {
		t.Fatal(err)
	}
	oldSequence, _ := res.LastInsertId()
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
	var oldTasks, newTasks int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE description LIKE '%trigger-before%'`).Scan(&oldTasks)
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE description LIKE '%trigger-after%'`).Scan(&newTasks)
	if oldTasks != 0 || newTasks != 1 {
		t.Fatalf("triggered tasks old/new=%d/%d; want 0/1", oldTasks, newTasks)
	}
}

func TestNewTriggerAtSequenceZeroUsesCommitOrderNotExternalTimestamp(t *testing.T) {
	svc, operator, _, _ := observationWorkflowTask(t)
	if _, err := svc.db.Exec(`DELETE FROM messages`); err != nil {
		t.Fatal(err)
	}
	trigger, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", Action: "create_task"})
	if err != nil {
		t.Fatal(err)
	}
	if trigger.CreatedAfterSequence != 0 || !trigger.ActivationSequenceSet {
		t.Fatalf("new zero-sequence trigger = %#v", trigger)
	}
	insertWorkflowIngressMessage(t, svc, "first-after-activation", "issue-provider:issues", "2000-01-01T00:00:00Z")
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var tasks int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE description LIKE '%first-after-activation%'`).Scan(&tasks)
	if tasks != 1 {
		t.Fatalf("first post-activation event with old external timestamp created %d tasks; want 1", tasks)
	}
}

func TestLegacyTargetWithoutActivationMarkerUsesTimestampBoundary(t *testing.T) {
	created := "2026-08-07T12:00:00Z"
	if workflowTargetEligible(10, 0, false, "2026-08-07T11:59:59Z", created) {
		t.Fatal("legacy target accepted an event older than its creation time")
	}
	if !workflowTargetEligible(10, 0, false, "2026-08-07T12:00:01Z", created) {
		t.Fatal("legacy target rejected an event newer than its creation time")
	}
}

func insertUnsequencedWorkflowMessage(t *testing.T, svc *Service, id, channel string) {
	t.Helper()
	if _, err := svc.db.Exec(`INSERT INTO channels(name,kind,created_at) VALUES(?,'chat',?) ON CONFLICT DO NOTHING`, channel, svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,text,data,kind) VALUES(?,?,?,'plugin:test','event','{}',?,'{}','event')`, id, channel, "2099-01-01T00:00:00Z", id); err != nil {
		t.Fatal(err)
	}
}

func TestTriggerActivationBackfillsPreexistingUnsequencedMessageBeforeWatermark(t *testing.T) {
	svc, operator, _, _ := observationWorkflowTask(t)
	insertUnsequencedWorkflowMessage(t, svc, "legacy-trigger-message", "issue-provider:issues")
	trigger, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", Action: "create_task"})
	if err != nil {
		t.Fatal(err)
	}
	var repairedSequence int64
	if err := svc.db.QueryRow(`SELECT sequence FROM task_workflow_message_sequence WHERE message_id='legacy-trigger-message'`).Scan(&repairedSequence); err != nil {
		t.Fatal(err)
	}
	if repairedSequence > trigger.CreatedAfterSequence {
		t.Fatalf("repaired sequence %d is after activation watermark %d", repairedSequence, trigger.CreatedAfterSequence)
	}
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var tasks int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE description LIKE '%legacy-trigger-message%'`).Scan(&tasks)
	if tasks != 0 {
		t.Fatalf("pre-activation unsequenced message created %d tasks", tasks)
	}
}

func TestSubscriptionActivationBackfillsPreexistingUnsequencedMessageBeforeWatermark(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	insertUnsequencedWorkflowMessage(t, svc, "legacy-subscription-message", "metrics:api")
	sub, err := svc.CreateWorkflowSubscription(context.Background(), AgentActor("dev-a"), assignmentID(assignment), CreateWorkflowSubscriptionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "record_only", IdempotencyKey: "legacy-sequence-sub"})
	if err != nil {
		t.Fatal(err)
	}
	var repairedSequence int64
	if err := svc.db.QueryRow(`SELECT sequence FROM task_workflow_message_sequence WHERE message_id='legacy-subscription-message'`).Scan(&repairedSequence); err != nil {
		t.Fatal(err)
	}
	if repairedSequence > sub.CreatedAfterSequence {
		t.Fatalf("repaired sequence %d is after activation watermark %d", repairedSequence, sub.CreatedAfterSequence)
	}
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var observations int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_observations WHERE payload LIKE '%legacy-subscription-message%'`).Scan(&observations)
	if observations != 0 {
		t.Fatalf("pre-activation unsequenced message created %d observations", observations)
	}
}

func TestSubscriptionActivationSequenceExcludesEarlierCommittedMessage(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	svc.db.SetMaxOpenConns(4)
	oldSequence := insertWorkflowIngressMessage(t, svc, "subscription-before", "metrics:api", "2099-01-01T00:00:00Z")
	reserved, release := make(chan struct{}), make(chan struct{})
	svc.workflowActivationAfterWriterReservation = func() { close(reserved); <-release }
	created := make(chan struct {
		sub WorkflowSubscription
		err error
	}, 1)
	go func() {
		sub, err := svc.CreateWorkflowSubscription(context.Background(), AgentActor("dev-a"), assignmentID(assignment), CreateWorkflowSubscriptionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "record_only", IdempotencyKey: "sequence-sub"})
		created <- struct {
			sub WorkflowSubscription
			err error
		}{sub, err}
	}()
	<-reserved
	inserted := make(chan struct {
		sequence int64
		err      error
	}, 1)
	go func() {
		if _, err := svc.db.Exec(`INSERT INTO channels(name,kind,created_at) VALUES('metrics:api','chat',?) ON CONFLICT DO NOTHING`, svc.now()); err != nil {
			inserted <- struct {
				sequence int64
				err      error
			}{err: err}
			return
		}
		if _, err := svc.db.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,text,data,kind) VALUES('subscription-after','metrics:api','2099-01-01T00:00:00Z','plugin:test','event','{}','subscription-after','{}','event')`); err != nil {
			inserted <- struct {
				sequence int64
				err      error
			}{err: err}
			return
		}
		res, err := svc.db.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES('subscription-after')`)
		if err != nil {
			inserted <- struct {
				sequence int64
				err      error
			}{err: err}
			return
		}
		sequence, err := res.LastInsertId()
		inserted <- struct {
			sequence int64
			err      error
		}{sequence: sequence, err: err}
	}()
	select {
	case result := <-inserted:
		t.Fatalf("message committed inside activation transaction at sequence %d: %v", result.sequence, result.err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	result := <-created
	if result.err != nil {
		t.Fatal(result.err)
	}
	sub := result.sub
	insertedResult := <-inserted
	if insertedResult.err != nil {
		t.Fatal(insertedResult.err)
	}
	newSequence := insertedResult.sequence
	if sub.CreatedAfterSequence != oldSequence || newSequence <= sub.CreatedAfterSequence {
		t.Fatalf("subscription watermark=%d old/new=%d/%d", sub.CreatedAfterSequence, oldSequence, newSequence)
	}
	if _, err := svc.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var oldObservations, newObservations int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_observations WHERE payload LIKE '%subscription-before%'`).Scan(&oldObservations)
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_observations WHERE payload LIKE '%subscription-after%'`).Scan(&newObservations)
	if oldObservations != 0 || newObservations != 1 {
		t.Fatalf("observations old/new=%d/%d; want 0/1", oldObservations, newObservations)
	}
}

func TestConcurrentWorkflowObservationReconcilersNeverRegressCursor(t *testing.T) {
	svc := newTestService(t)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("message-%02d", 19-i)
		if _, err := svc.db.Exec(`INSERT INTO channels(name,kind,created_at) VALUES('metrics:x','chat',?) ON CONFLICT DO NOTHING`, svc.now()); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.db.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,kind) VALUES(?,'metrics:x',?,'plugin:metrics','alert','{}','event')`, id, svc.now()); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.db.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES(?)`, id); err != nil {
			t.Fatal(err)
		}
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
	var cursor int64
	if err := svc.db.QueryRow(`SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton=1`).Scan(&cursor); err != nil || cursor != 20 {
		t.Fatalf("cursor = %d, %v; want 20", cursor, err)
	}
}

func TestZeroTargetCursorAdvanceSerializesAgainstTargetCreation(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.db.Exec(`INSERT INTO task_queues(prefix,name,created_at,updated_at) VALUES('DEV','Development',?,?)`, svc.now(), svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO channels(name,kind,created_at) VALUES('issue-provider:issues','chat',?)`, svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,text,data,kind) VALUES('overlap-event','issue-provider:issues',?,'plugin:issue-provider','issue.created','{}','overlap','{}','event')`, svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES('overlap-event')`); err != nil {
		t.Fatal(err)
	}

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
	var cursor int64
	if err := svc.db.QueryRow(`SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton=1`).Scan(&cursor); err != nil || cursor != 1 {
		t.Fatalf("cursor = %d, %v; want 1", cursor, err)
	}
}

func TestQueueWorkflowTriggerCreatesManagedTaskIdempotently(t *testing.T) {
	svc, operator, _, _ := observationWorkflowTask(t)
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", Action: "run_script"}); ErrorCode(err) != "invalid_trigger_action" {
		t.Fatalf("invalid action error = %v", err)
	}
	if _, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "agent:*", Action: "create_task"}); ErrorCode(err) != "invalid_channel_pattern" {
		t.Fatalf("recursive internal trigger error = %v", err)
	}
	_, err := svc.CreateQueueWorkflowTrigger(context.Background(), operator, "DEV", CreateQueueWorkflowTriggerInput{Pattern: "issue-provider:*", CorrelationKey: "issue-7", Action: "create_task"})
	if err != nil {
		t.Fatal(err)
	}
	in := ApplyWorkflowObservationInput{EventID: "issue-provider-event-7", Channel: "issue-provider:issues", Kind: "issue.created", CorrelationKey: "issue-7", Payload: map[string]any{"text": "Fix production", "source": "plugin:issue-provider"}}
	for range 2 {
		if _, err := svc.ApplyWorkflowObservation(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE queue_prefix='DEV' AND title='Fix production' AND workflow_version_id IS NOT NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("trigger tasks = %d, %v", count, err)
	}
}

func TestQueueWorkflowTriggerIsOperatorOnlyAndPersistent(t *testing.T) {
	svc, operator, _, _ := observationWorkflowTask(t)
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

func TestWorkflowSubscriptionResolvesArtifactAndCannotWidenScope(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	actor := AgentActor("dev-a")
	if _, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "logs:api", Reaction: "wake_current", IdempotencyKey: "before-artifact",
	}); ErrorCode(err) != "channel_not_allowed" {
		t.Fatalf("missing artifact expansion error = %v", err)
	}
	artifact, err := svc.AddArtifact(context.Background(), actor, assignmentID(assignment), AddArtifactInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Name: "service_id", Type: ArtifactMarkdown, Content: "api", IdempotencyKey: "service-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = artifact
	created, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "logs:api", CorrelationKey: "incident-7", Reaction: "wake_current", IdempotencyKey: "subscribe-logs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Pattern != "logs:api" || created.AssignmentID != assignment.ID || created.TaskKey != task.Key {
		t.Fatalf("subscription = %#v", created)
	}
	for _, pattern := range []string{"logs:*", "logs:other", "logs:api:extra"} {
		_, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
			TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, Pattern: pattern, Reaction: "record_only", IdempotencyKey: "bad-" + pattern,
		})
		if ErrorCode(err) != "channel_not_allowed" {
			t.Fatalf("pattern %q error = %v; want channel_not_allowed", pattern, err)
		}
	}
}

func TestWorkflowObservationIsDeduplicatedAndAppliesDeclaredReaction(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	actor := AgentActor("dev-a")
	sub, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "hold_assignment", IdempotencyKey: "sub-metrics",
	})
	if err != nil {
		t.Fatal(err)
	}
	in := ApplyWorkflowObservationInput{EventID: "message-1", Channel: "metrics:api", Kind: "alert", CorrelationKey: "", Payload: map[string]any{"value": 99}}
	first, err := svc.ApplyWorkflowObservation(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ApplyWorkflowObservation(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID || first[0].SubscriptionID != sub.ID {
		t.Fatalf("observations = %#v / %#v", first, second)
	}
	var holds int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_holds WHERE assignment_id = ? AND released_at = ''`, assignment.ID).Scan(&holds); err != nil || holds != 1 {
		t.Fatalf("holds = %d, %v", holds, err)
	}
	if _, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision + 2, AssignmentRevision: assignment.Revision + 1, Pattern: "metrics:api", Reaction: "create_requirement", IdempotencyKey: "reaction-not-declared",
	}); ErrorCode(err) != "workflow_reaction_not_allowed" {
		t.Fatalf("reaction error = %v", err)
	}
}

func TestWorkflowSubscriptionCancellationAndLateObservationRecordOnly(t *testing.T) {
	svc, _, task, assignment := observationWorkflowTask(t)
	actor := AgentActor("dev-a")
	sub, err := svc.CreateWorkflowSubscription(context.Background(), actor, assignmentID(assignment), CreateWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Pattern: "metrics:api", Reaction: "hold_assignment", IdempotencyKey: "sub-cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelWorkflowSubscription(context.Background(), actor, assignmentID(assignment), sub.ID, CancelWorkflowSubscriptionInput{
		TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, IdempotencyKey: "cancel-sub",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ApplyWorkflowObservation(context.Background(), ApplyWorkflowObservationInput{EventID: "cancelled-event", Channel: "metrics:api", Kind: "alert"})
	if err != nil || len(got) != 0 {
		t.Fatalf("cancelled observations = %#v, %v", got, err)
	}

	// A subscription bound to an assignment that is no longer in the current
	// status records a late event but cannot retain its active reaction.
	if _, err := svc.db.Exec(`UPDATE task_workflow_subscriptions SET state='active', cancelled_at='' WHERE id=?`, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`UPDATE task_assignments SET state='completed' WHERE id=?`, assignment.ID); err != nil {
		t.Fatal(err)
	}
	got, err = svc.ApplyWorkflowObservation(context.Background(), ApplyWorkflowObservationInput{EventID: "late-event", Channel: "metrics:api", Kind: "alert"})
	if err != nil || len(got) != 1 {
		t.Fatalf("late observation = %#v, %v", got, err)
	}
	var holds int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_holds WHERE assignment_id=?`, assignment.ID).Scan(&holds)
	if holds != 0 {
		t.Fatalf("late holds = %d; want 0", holds)
	}
}
