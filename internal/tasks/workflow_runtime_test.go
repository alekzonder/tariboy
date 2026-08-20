package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

func runtimeWorkflowTask(
	t *testing.T,
	definition WorkflowDefinition,
	pools map[string][]string,
) (*Service, Actor, Task) {
	t.Helper()
	svc := newTestService(t)
	operator := CustomerActor("customer")
	if _, err := svc.CreateQueue(context.Background(), operator, CreateQueueInput{
		Prefix: "DEV", Name: "Development",
	}); err != nil {
		t.Fatal(err)
	}
	agents := map[string]bool{}
	for _, members := range pools {
		for _, agent := range members {
			agents[agent] = true
		}
	}
	names := make([]string, 0, len(agents))
	for agent := range agents {
		names = append(names, agent)
	}
	sort.Strings(names)
	for _, agent := range names {
		if _, err := svc.db.Exec(`
			INSERT INTO agents(name, image_ref, image_digest)
			VALUES (?, 'basic:latest', 'digest')`, agent); err != nil {
			t.Fatal(err)
		}
	}
	poolNames := make([]string, 0, len(pools))
	for pool := range pools {
		poolNames = append(poolNames, pool)
	}
	sort.Strings(poolNames)
	for _, pool := range poolNames {
		if _, err := svc.RebindAgentPool(
			context.Background(), operator, "DEV", pool, pools[pool], 0, "bind-"+pool,
		); err != nil {
			t.Fatal(err)
		}
	}
	draft, err := svc.CreateWorkflowDraft(context.Background(), operator, definition)
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.PublishWorkflowVersion(context.Background(), operator, draft.Name, draft.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateQueueWorkflow(
		context.Background(), operator, "DEV", published.ID, 0, "activate-runtime",
	); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(context.Background(), operator, CreateTaskInput{
		Queue: "DEV", Title: "runtime work",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, operator, task
}

func claimOneDefinition() WorkflowDefinition {
	return WorkflowDefinition{
		Name: "claim-one", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{
				ID: "work",
				Requirements: []WorkflowRequirement{{
					ID: "implementation", Pool: "developers", Dispatch: DispatchClaimOne,
					Outcomes: []string{"completed"},
				}},
				Transitions: []WorkflowTransition{{When: "implementation.completed", To: "done"}},
			},
			{ID: "done", Terminal: true},
		},
		Timeouts: WorkflowTimeoutPolicy{Assignment: "5m"},
	}
}

func assignmentID(assignment Assignment) string {
	return strconv.FormatInt(assignment.ID, 10)
}

func TestClaimOneIsAtomic(t *testing.T) {
	svc, _, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a", "dev-b"},
	})
	ctx := context.Background()
	for _, agent := range []string{"dev-a", "dev-b"} {
		work, err := svc.NextWork(ctx, AgentActor(agent), "dev", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(work) != 1 {
			t.Fatalf("%s work = %#v; want one shared claim-one assignment", agent, work)
		}
	}
	work, err := svc.NextWork(ctx, AgentActor("dev-a"), "DEV", 10)
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		assignment Assignment
		err        error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, agent := range []string{"dev-a", "dev-b"} {
		agent := agent
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			assignment, err := svc.ClaimAssignment(ctx, AgentActor(agent), assignmentID(work[0]), ClaimAssignmentInput{
				TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision,
				IdempotencyKey: "claim-" + agent,
			})
			results <- result{assignment: assignment, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var winner Assignment
	var successes, alreadyClaimed int
	for result := range results {
		switch ErrorCode(result.err) {
		case "":
			successes++
			winner = result.assignment
		case "assignment_already_claimed":
			alreadyClaimed++
		default:
			t.Fatalf("concurrent claim error = %v", result.err)
		}
	}
	if successes != 1 || alreadyClaimed != 1 {
		t.Fatalf("claim results successes/already-claimed = %d/%d; want 1/1", successes, alreadyClaimed)
	}
	if winner.State != AssignmentLeased || winner.LeaseOwner == "" || winner.LeaseExpiresAt == "" || winner.Revision != 2 {
		t.Fatalf("winning assignment = %#v; want revisioned lease", winner)
	}
}

func TestClaimOneIsMaterializedOwnerlessAndNextWorkDoesNotMutate(t *testing.T) {
	svc, _, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a", "dev-b"},
	})
	var before int
	var agent sql.NullString
	var assignment Assignment
	if err := svc.db.QueryRow(`
		SELECT id, requirement_execution_id, agent, attempt, state, lease_owner,
		       lease_expires_at, revision, outcome, created_at, updated_at, completed_at
		FROM task_assignments`).Scan(
		&assignment.ID, &assignment.RequirementExecutionID, &agent, &assignment.Attempt,
		&assignment.State, &assignment.LeaseOwner, &assignment.LeaseExpiresAt,
		&assignment.Revision, &assignment.Outcome, &assignment.CreatedAt,
		&assignment.UpdatedAt, &assignment.CompletedAt,
	); err != nil {
		t.Fatal(err)
	}
	if agent.Valid || assignment.State != AssignmentClaimable {
		t.Fatalf("materialized claim-one owner/state = %v/%q; want NULL/claimable", agent, assignment.State)
	}
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_assignments`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	work, err := svc.NextWork(context.Background(), AgentActor("dev-b"), "DEV", 10)
	if err != nil {
		t.Fatal(err)
	}
	var after int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_assignments`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != 1 || after != before || len(work) != 1 || work[0].ID != assignment.ID || work[0].Agent != "" {
		t.Fatalf("before/after/work = %d/%d/%#v; want read-only ownerless assignment %d", before, after, work, assignment.ID)
	}
	var wakeAssignment sql.NullInt64
	if err := svc.db.QueryRow(`
		SELECT assignment_id FROM task_workflow_outbox
		WHERE task_id = ? AND kind = 'workflow.assignment_ready' LIMIT 1`, task.ID).Scan(&wakeAssignment); err != nil {
		t.Fatal(err)
	}
	if !wakeAssignment.Valid || wakeAssignment.Int64 != assignment.ID {
		t.Fatalf("ready wake assignment = %v; want %d", wakeAssignment, assignment.ID)
	}
}

func TestOwnerlessClaimOneSurvivesPoolRebindAndFormerAgentDelete(t *testing.T) {
	svc, operator, _ := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a", "dev-b"},
	})
	pool, found, err := agentPoolByName(context.Background(), svc.db, "DEV", "developers")
	if err != nil || !found {
		t.Fatalf("developers pool = %#v/%v, err=%v", pool, found, err)
	}
	if _, err := svc.RebindAgentPool(context.Background(), operator, "DEV", "developers", []string{"dev-b"}, pool.Revision, "remove-dev-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`DELETE FROM agents WHERE name = 'dev-a'`); err != nil {
		t.Fatalf("delete former pool member with ownerless assignment: %v", err)
	}
	work, err := svc.NextWork(context.Background(), AgentActor("dev-b"), "DEV", 10)
	if err != nil || len(work) != 1 || work[0].Agent != "" {
		t.Fatalf("work after pool rebind/delete = %#v, err=%v; want frozen ownerless assignment", work, err)
	}
}

func TestNextWorkNeverReturnsAssignmentFromTerminalExecution(t *testing.T) {
	svc, _, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a"},
	})
	if _, err := svc.db.Exec(`
		UPDATE task_status_executions SET status_id = 'done'
		WHERE task_id = ? AND state = 'active'`, task.ID); err != nil {
		t.Fatal(err)
	}
	work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(work) != 0 {
		t.Fatalf("terminal execution work = %#v; want none", work)
	}
}

func TestAssignmentCompletionRequiresOwnerLeaseRevisionsOutcomeAndIdempotency(t *testing.T) {
	svc, _, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a", "dev-b"},
	})
	ctx := context.Background()
	work, err := svc.NextWork(ctx, AgentActor("dev-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("next work = %#v, err=%v", work, err)
	}
	if _, err := svc.ClaimAssignment(ctx, AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision,
	}); ErrorCode(err) != "missing_idempotency_key" {
		t.Fatalf("claim without idempotency error = %v; want missing_idempotency_key", err)
	}
	claimed, err := svc.ClaimAssignment(ctx, AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision,
		IdempotencyKey: "claim-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteAssignment(ctx, AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: claimed.Revision,
		Outcome: "completed", IdempotencyKey: "stale-task-complete",
	}); ErrorCode(err) != "revision_conflict" {
		t.Fatalf("stale task completion error = %v; want revision_conflict", err)
	}
	if _, err := svc.CompleteAssignment(ctx, AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision,
		Outcome: "completed", IdempotencyKey: "stale-complete",
	}); ErrorCode(err) != "revision_conflict" {
		t.Fatalf("stale completion error = %v; want revision_conflict", err)
	}
	if _, err := svc.CompleteAssignment(ctx, AgentActor("dev-b"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: "completed", IdempotencyKey: "wrong-owner",
	}); ErrorCode(err) != "assignment_not_owned" {
		t.Fatalf("wrong-owner completion error = %v; want assignment_not_owned", err)
	}
	if _, err := svc.CompleteAssignment(ctx, AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: "invented", IdempotencyKey: "bad-outcome",
	}); ErrorCode(err) != "invalid_outcome" {
		t.Fatalf("invalid outcome error = %v; want invalid_outcome", err)
	}
	completed, err := svc.CompleteAssignment(ctx, AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: "completed", IdempotencyKey: "complete-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.CompleteAssignment(ctx, AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: -1, AssignmentRevision: -1, Outcome: "invented", IdempotencyKey: "complete-a",
	})
	if err != nil || replayed.ID != completed.ID || replayed.Revision != completed.Revision {
		t.Fatalf("completion replay = %#v, err=%v; want %#v", replayed, err, completed)
	}
	if _, err := svc.CompleteAssignment(ctx, AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: completed.Revision,
		Outcome: "completed", IdempotencyKey: "duplicate-complete",
	}); ErrorCode(err) != "assignment_already_completed" {
		t.Fatalf("duplicate completion error = %v; want assignment_already_completed", err)
	}
}

func TestExpiredLeaseCannotComplete(t *testing.T) {
	definition := claimOneDefinition()
	definition.Timeouts.Assignment = "1m"
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"developers": {"dev-a"},
	})
	clock := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return clock }
	work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("next work = %#v, err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "claim-expired-complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Minute)
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: "completed", IdempotencyKey: "complete-expired",
	}); ErrorCode(err) != "assignment_lease_expired" {
		t.Fatalf("expired completion error = %v; want assignment_lease_expired", err)
	}
	stored, err := assignmentByID(context.Background(), svc.db, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != AssignmentLeased || stored.Revision != claimed.Revision {
		t.Fatalf("expired completion mutated assignment = %#v; want unchanged lease", stored)
	}
}

func TestExpireLeaseCreatesNewAttemptAndResumeWake(t *testing.T) {
	definition := claimOneDefinition()
	definition.Timeouts.Assignment = "1m"
	definition.Retries = WorkflowRetryPolicy{MaxAttempts: 2, OnExhausted: "done"}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"developers": {"dev-a", "dev-b"},
	})
	clock := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return clock }
	work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("next work = %#v, err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "claim-expiring",
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.LeaseExpiresAt != clock.Add(time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("lease expiry = %q; want server policy %q", claimed.LeaseExpiresAt, clock.Add(time.Minute).Format(time.RFC3339Nano))
	}
	clock = clock.Add(2 * time.Minute)
	expired, err := svc.ExpireLeases(context.Background(), clock)
	if err != nil || expired != 1 {
		t.Fatalf("expired leases = %d, err=%v; want 1", expired, err)
	}
	resumed, err := svc.NextWork(context.Background(), AgentActor("dev-b"), "DEV", 10)
	if err != nil || len(resumed) != 1 || resumed[0].Attempt != 2 || resumed[0].State != AssignmentClaimable {
		t.Fatalf("resumed work = %#v, err=%v; want claimable attempt 2", resumed, err)
	}
	var wakes int
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FROM task_workflow_outbox
		WHERE task_id = ? AND kind = 'workflow.assignment_resumed'`, task.ID).Scan(&wakes); err != nil {
		t.Fatal(err)
	}
	if wakes != 2 {
		t.Fatalf("resume wakes = %d; want frozen claim-one pool size 2", wakes)
	}
}

func TestReleaseRequiresAnUnexpiredOwnedLease(t *testing.T) {
	svc, _, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a"},
	})
	work, _ := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "claim-release",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReleaseAssignment(context.Background(), AgentActor("dev-b"), assignmentID(claimed), ReleaseAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision, IdempotencyKey: "release-wrong-owner",
	}); ErrorCode(err) != "assignment_not_owned" {
		t.Fatalf("wrong-owner release error = %v; want assignment_not_owned", err)
	}
	released, err := svc.ReleaseAssignment(context.Background(), AgentActor("dev-a"), assignmentID(claimed), ReleaseAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision, IdempotencyKey: "release-owned",
	})
	if err != nil || released.State != AssignmentReleased {
		t.Fatalf("release = %#v, err=%v; want released", released, err)
	}
	next, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	if err != nil || len(next) != 1 || next[0].Attempt != 2 {
		t.Fatalf("work after release = %#v, err=%v; want attempt 2", next, err)
	}
}

func TestNextWorkIsScopedAndDeterministicallyOrdered(t *testing.T) {
	svc, operator, first := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a"},
	})
	second, err := svc.CreateTask(context.Background(), operator, CreateTaskInput{
		Queue: "DEV", Title: "second runtime work",
	})
	if err != nil {
		t.Fatal(err)
	}
	work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "dev", 10)
	if err != nil || len(work) != 2 {
		t.Fatalf("ordered work = %#v, err=%v; want two", work, err)
	}
	if work[0].ID >= work[1].ID {
		t.Fatalf("assignment order = %d, %d; want stable ascending task order", work[0].ID, work[1].ID)
	}
	if first.ID >= second.ID {
		t.Fatalf("fixture task ids = %d, %d; want increasing", first.ID, second.ID)
	}
	if other, err := svc.NextWork(context.Background(), AgentActor("outsider"), "DEV", 10); err != nil || len(other) != 0 {
		t.Fatalf("outsider work = %#v, err=%v; want none", other, err)
	}
	if limited, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1); err != nil || len(limited) != 1 {
		t.Fatalf("limited work = %#v, err=%v; want one", limited, err)
	}
}

func TestRetryExhaustionUsesDeclaredStatus(t *testing.T) {
	definition := claimOneDefinition()
	definition.Statuses[0].Requirements[0].Outcomes = []string{"completed", "failed"}
	definition.Statuses[0].Transitions = []WorkflowTransition{
		{When: "implementation.completed", To: "done"},
		{When: "implementation.failed", To: "failed"},
	}
	definition.Statuses = append(definition.Statuses, WorkflowStatus{ID: "failed", Terminal: true})
	definition.Timeouts.Assignment = "1m"
	definition.Retries = WorkflowRetryPolicy{MaxAttempts: 1, OnExhausted: "failed"}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"developers": {"dev-a"},
	})
	clock := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return clock }
	work, _ := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	if _, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "claim-final-attempt",
	}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Minute)
	if expired, err := svc.ExpireLeases(context.Background(), clock); err != nil || expired != 1 {
		t.Fatalf("expire final attempt = %d, err=%v; want 1", expired, err)
	}
	detail, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.WorkflowStatus != "failed" || detail.Task.WorkflowRevision != task.WorkflowRevision+1 {
		t.Fatalf("retry-exhausted task = %#v; want failed at next workflow revision", detail.Task)
	}
	if next, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 10); err != nil || len(next) != 0 {
		t.Fatalf("work after retry exhaustion = %#v, err=%v; want none", next, err)
	}
}

func TestExpireLeasesClosesEveryDueLeaseBeforeRetryExhaustionTransition(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "expire-all", Version: 1, InitialStatus: "verify",
		Statuses: []WorkflowStatus{
			{
				ID: "verify", Requirements: []WorkflowRequirement{{
					ID: "qa", Pool: "qa", Dispatch: DispatchRequireAll, Outcomes: []string{"passed", "failed"},
				}},
				Transitions: []WorkflowTransition{
					{When: "qa.all(passed)", To: "done"},
					{When: "qa.any(failed)", To: "failed"},
				},
			},
			{ID: "done", Terminal: true},
			{ID: "failed", Terminal: true},
		},
		Timeouts: WorkflowTimeoutPolicy{Assignment: "1m"},
		Retries:  WorkflowRetryPolicy{MaxAttempts: 1, OnExhausted: "failed"},
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"qa": {"qa-a", "qa-b"},
	})
	clock := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return clock }
	for _, agent := range []string{"qa-a", "qa-b"} {
		work, err := svc.NextWork(context.Background(), AgentActor(agent), "DEV", 1)
		if err != nil || len(work) != 1 {
			t.Fatalf("%s work = %#v, err=%v", agent, work, err)
		}
		if _, err := svc.ClaimAssignment(context.Background(), AgentActor(agent), assignmentID(work[0]), ClaimAssignmentInput{
			TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision,
			IdempotencyKey: "claim-" + agent,
		}); err != nil {
			t.Fatal(err)
		}
	}
	clock = clock.Add(2 * time.Minute)
	if count, err := svc.ExpireLeases(context.Background(), clock); err != nil || count != 2 {
		t.Fatalf("expired leases = %d, err=%v; want both due leases", count, err)
	}
	var expired, leased int
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FILTER (WHERE state = 'expired'),
		       COUNT(*) FILTER (WHERE state = 'leased')
		FROM task_assignments`).Scan(&expired, &leased); err != nil {
		t.Fatal(err)
	}
	if expired != 2 || leased != 0 {
		t.Fatalf("assignment states expired/leased = %d/%d; want 2/0", expired, leased)
	}
}

func TestUnexpectedWorkflowSQLFailuresRollbackAssignmentMutations(t *testing.T) {
	claim := func(t *testing.T, svc *Service, task Task, key string) Assignment {
		t.Helper()
		work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
		if err != nil || len(work) != 1 {
			t.Fatalf("next work = %#v, err=%v", work, err)
		}
		claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
			TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision,
			IdempotencyKey: key,
		})
		if err != nil {
			t.Fatal(err)
		}
		return claimed
	}
	assertStillLeased := func(t *testing.T, svc *Service, claimed Assignment) {
		t.Helper()
		stored, err := assignmentByID(context.Background(), svc.db, claimed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.State != AssignmentLeased || stored.Revision != claimed.Revision || stored.Outcome != "" {
			t.Fatalf("assignment after rollback = %#v; want original lease %#v", stored, claimed)
		}
	}

	t.Run("completion reducer materialization", func(t *testing.T) {
		svc, _, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
			"developers": {"dev-a"},
		})
		claimed := claim(t, svc, task, "claim-completion-rollback")
		if _, err := svc.db.Exec(`
			CREATE TRIGGER fail_next_status_execution
			BEFORE INSERT ON task_status_executions WHEN NEW.sequence = 2
			BEGIN SELECT RAISE(ABORT, 'injected reducer failure'); END`); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CompleteAssignment(context.Background(), AgentActor("dev-a"), assignmentID(claimed), CompleteAssignmentInput{
			TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
			Outcome: "completed", IdempotencyKey: "completion-rollback",
		}); err == nil || ErrorCode(err) != "" {
			t.Fatalf("injected completion error = %v (code %q); want raw infrastructure error", err, ErrorCode(err))
		}
		assertStillLeased(t, svc, claimed)
		var requirementState, executionState string
		if err := svc.db.QueryRow(`SELECT state FROM task_requirement_executions WHERE id = ?`, claimed.RequirementExecutionID).Scan(&requirementState); err != nil {
			t.Fatal(err)
		}
		if err := svc.db.QueryRow(`SELECT state FROM task_status_executions WHERE task_id = ?`, task.ID).Scan(&executionState); err != nil {
			t.Fatal(err)
		}
		if requirementState != "pending" || executionState != "active" {
			t.Fatalf("reducer state after rollback = %q/%q; want pending/active", requirementState, executionState)
		}
	})

	for _, operation := range []string{"release", "expiry"} {
		operation := operation
		t.Run(operation+" retry materialization", func(t *testing.T) {
			svc, _, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
				"developers": {"dev-a"},
			})
			clock := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
			svc.clock = func() time.Time { return clock }
			claimed := claim(t, svc, task, "claim-"+operation+"-rollback")
			if _, err := svc.db.Exec(`
				CREATE TRIGGER fail_retry_assignment
				BEFORE INSERT ON task_assignments WHEN NEW.attempt = 2
				BEGIN SELECT RAISE(ABORT, 'injected retry failure'); END`); err != nil {
				t.Fatal(err)
			}
			var err error
			if operation == "release" {
				_, err = svc.ReleaseAssignment(context.Background(), AgentActor("dev-a"), assignmentID(claimed), ReleaseAssignmentInput{
					TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
					IdempotencyKey: "release-rollback",
				})
			} else {
				clock = clock.Add(time.Hour)
				var expired int
				expired, err = svc.ExpireLeases(context.Background(), clock)
				if expired != 0 {
					t.Fatalf("expired count on rollback = %d; want 0", expired)
				}
			}
			if err == nil || ErrorCode(err) != "" {
				t.Fatalf("injected %s error = %v (code %q); want raw infrastructure error", operation, err, ErrorCode(err))
			}
			assertStillLeased(t, svc, claimed)
		})
	}
}

func TestAssignmentIDValidationIsTyped(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), "not-an-id", ClaimAssignmentInput{
		TaskRevision: 1, AssignmentRevision: 1, IdempotencyKey: "bad-id",
	})
	if ErrorCode(err) != "invalid_assignment_id" {
		t.Fatalf("invalid assignment id error = %v; want invalid_assignment_id", err)
	}
	if fmt.Sprint(err) == "" {
		t.Fatal("typed assignment id error has no message")
	}
}
