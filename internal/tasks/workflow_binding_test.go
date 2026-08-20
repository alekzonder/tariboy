package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
)

func workflowFixture(t *testing.T) (*Service, Actor) {
	t.Helper()
	svc := newTestService(t)
	actor := CustomerActor("customer")
	if _, err := svc.CreateQueue(context.Background(), actor, CreateQueueInput{
		Prefix: "DEV", Name: "Development",
	}); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"dev-1", "dev-2", "reviewer-1"} {
		if _, err := svc.db.Exec(`
			INSERT INTO agents(name, image_ref, image_digest)
			VALUES (?, 'basic:latest', 'digest')`, agent); err != nil {
			t.Fatal(err)
		}
	}
	return svc, actor
}

func publishDevelopmentVersion(t *testing.T, svc *Service, actor Actor, version int) WorkflowVersion {
	t.Helper()
	definition := validWorkflowDefinition()
	definition.Version = version
	created, err := svc.CreateWorkflowDraft(context.Background(), actor, definition)
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.PublishWorkflowVersion(context.Background(), actor, created.Name, created.Version)
	if err != nil {
		t.Fatal(err)
	}
	return published
}

func mustRebindPool(t *testing.T, svc *Service, actor Actor, pool string, agents []string, revision int64) AgentPool {
	t.Helper()
	got, err := svc.RebindAgentPool(context.Background(), actor, "DEV", pool, agents, revision, "pool-"+pool)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func activateDevelopmentVersion(t *testing.T, svc *Service, actor Actor, version int, revision int64) QueueWorkflowBinding {
	t.Helper()
	published := publishDevelopmentVersion(t, svc, actor, version)
	got, err := svc.ActivateQueueWorkflow(
		context.Background(), actor, "DEV", published.ID, revision, "activate-development",
	)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestActivateQueueWorkflowRequiresOperatorAndPublishedVersion(t *testing.T) {
	svc, actor := workflowFixture(t)
	definition := validWorkflowDefinition()
	draft, err := svc.CreateWorkflowDraft(context.Background(), actor, definition)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ActivateQueueWorkflow(context.Background(), AgentActor("dev-1"), "DEV", draft.ID, 0, "agent-bind"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent activation error = %v; want forbidden", err)
	}
	if _, err := svc.ActivateQueueWorkflow(context.Background(), actor, "DEV", draft.ID, 0, "draft-bind"); ErrorCode(err) != "workflow_not_published" {
		t.Fatalf("draft activation error = %v; want workflow_not_published", err)
	}
}

func TestActivateQueueWorkflowRequiresEveryLogicalPoolAndReplaysIdempotently(t *testing.T) {
	svc, actor := workflowFixture(t)
	published := publishDevelopmentVersion(t, svc, actor, 1)
	mustRebindPool(t, svc, actor, "developers", []string{"agent:dev-1"}, 0)

	if _, err := svc.ActivateQueueWorkflow(context.Background(), actor, "DEV", published.ID, 0, "activate-1"); ErrorCode(err) != "workflow_pool_empty" {
		t.Fatalf("incomplete pool activation error = %v; want workflow_pool_empty", err)
	}
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	first, err := svc.ActivateQueueWorkflow(context.Background(), actor, "dev", published.ID, 0, "activate-1")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.ActivateQueueWorkflow(context.Background(), actor, "DEV", -1, 99, "activate-1")
	if err != nil {
		t.Fatal(err)
	}
	if replayed != first || first.WorkflowName != "development" || first.WorkflowVersion != 1 || first.Revision != 1 {
		t.Fatalf("first/replayed binding = %#v / %#v", first, replayed)
	}
	got, err := svc.GetQueueWorkflow(context.Background(), actor, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("get binding = %#v; want %#v", got, first)
	}
	var events int
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FROM task_events
		WHERE queue_prefix = 'DEV' AND kind = 'task.queue_workflow_activated'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("activation events = %d; want 1", events)
	}
	second := publishDevelopmentVersion(t, svc, actor, 2)
	_, err = svc.ActivateQueueWorkflow(context.Background(), actor, "DEV", second.ID, 0, "stale-activation")
	conflict, ok := err.(*Error)
	if !ok || conflict.Code != "revision_conflict" {
		t.Fatalf("stale activation error = %#v; want revision_conflict", err)
	}
	if conflict.Data["current_revision"] != first.Revision || !reflect.DeepEqual(conflict.Data["current"], first) {
		t.Fatalf("stale activation conflict data = %#v; want current binding %#v", conflict.Data, first)
	}
}

func TestRebindAgentPoolIsRevisionedAuditedAndAffectsFutureWorkOnly(t *testing.T) {
	svc, actor := workflowFixture(t)
	developers := mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	binding := activateDevelopmentVersion(t, svc, actor, 1, 0)
	before, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "before rebind"})
	if err != nil {
		t.Fatal(err)
	}

	rebound, err := svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", []string{"agent:dev-2", "dev-2"}, developers.Revision, "rebind-developers")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", []string{"dev-1"}, 999, "rebind-developers")
	if err != nil {
		t.Fatal(err)
	}
	if len(rebound.Agents) != 1 || rebound.Agents[0] != "dev-2" || replayed.Revision != rebound.Revision {
		t.Fatalf("rebound/replayed pool = %#v / %#v", rebound, replayed)
	}
	_, err = svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", []string{"dev-1"}, developers.Revision, "stale-rebind")
	conflict, ok := err.(*Error)
	if !ok || conflict.Code != "revision_conflict" {
		t.Fatalf("stale rebind error = %#v; want revision_conflict", err)
	}
	if conflict.Data["current_revision"] != rebound.Revision || !reflect.DeepEqual(conflict.Data["current"], rebound) {
		t.Fatalf("stale rebind conflict data = %#v; want current pool %#v", conflict.Data, rebound)
	}
	if _, err := svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", nil, rebound.Revision, "empty-rebind"); ErrorCode(err) != "workflow_pool_empty" {
		t.Fatalf("empty active pool error = %v; want workflow_pool_empty", err)
	}
	after, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "after rebind"})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := func(taskID int64) []string {
		t.Helper()
		var raw string
		if err := svc.db.QueryRow(`
			SELECT re.pool_snapshot
			FROM task_requirement_executions re
			JOIN task_status_executions se ON se.id = re.status_execution_id
			WHERE se.task_id = ? AND re.requirement_id = 'implementation'`, taskID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var agents []string
		if err := json.Unmarshal([]byte(raw), &agents); err != nil {
			t.Fatal(err)
		}
		return agents
	}
	if got := snapshot(before.ID); len(got) != 1 || got[0] != "dev-1" {
		t.Fatalf("old task pool snapshot = %#v; want dev-1", got)
	}
	if got := snapshot(after.ID); len(got) != 1 || got[0] != "dev-2" {
		t.Fatalf("new task pool snapshot = %#v; want dev-2", got)
	}
	var eventRevision int64
	if err := svc.db.QueryRow(`
		SELECT task_revision FROM task_events
		WHERE queue_prefix = 'DEV' AND kind = 'task.agent_pool_rebound'
		ORDER BY sequence DESC LIMIT 1`).Scan(&eventRevision); err != nil {
		t.Fatal(err)
	}
	if eventRevision != rebound.Revision {
		t.Fatalf("pool audit revision = %d; want %d", eventRevision, rebound.Revision)
	}
	if binding.WorkflowVersionID == 0 {
		t.Fatal("fixture did not activate the workflow")
	}
}

func TestCreateTaskPinsActiveQueueWorkflowAndInitializesStatus(t *testing.T) {
	svc, actor := workflowFixture(t)
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	binding := activateDevelopmentVersion(t, svc, actor, 1, 0)

	task, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if task.WorkflowVersionID != binding.WorkflowVersionID || task.WorkflowVersion != "development@1" || task.WorkflowStatus != "implement" || task.WorkflowRevision != 1 {
		t.Fatalf("workflow = %#v; want development@1/implement revision 1", task)
	}
	var status, state string
	var versionID, revision int64
	if err := svc.db.QueryRow(`
		SELECT status_id, state, workflow_version_id, task_revision
		FROM task_status_executions WHERE task_id = ?`, task.ID).Scan(&status, &state, &versionID, &revision); err != nil {
		t.Fatal(err)
	}
	if status != "implement" || state != "active" || versionID != binding.WorkflowVersionID || revision != 1 {
		t.Fatalf("status execution = %q/%q/%d/%d", status, state, versionID, revision)
	}
	var requirements, wakes int
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FROM task_requirement_executions re
		JOIN task_status_executions se ON se.id = re.status_execution_id
		WHERE se.task_id = ?`, task.ID).Scan(&requirements); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_outbox WHERE task_id = ?`, task.ID).Scan(&wakes); err != nil {
		t.Fatal(err)
	}
	if requirements != 1 || wakes != 1 {
		t.Fatalf("requirements/wakes = %d/%d; want 1/1", requirements, wakes)
	}
	var wakeID, kind, payloadJSON string
	var routedTaskID int64
	var assignmentID sql.NullInt64
	if err := svc.db.QueryRow(`
		SELECT wake_id, task_id, assignment_id, kind, payload
		FROM task_workflow_outbox WHERE task_id = ?`, task.ID).Scan(
		&wakeID, &routedTaskID, &assignmentID, &kind, &payloadJSON,
	); err != nil {
		t.Fatal(err)
	}
	if wakeID == "" || routedTaskID != task.ID || !assignmentID.Valid || kind != "workflow.assignment_ready" {
		t.Fatalf("wake routing = %q/%d/%v/%q; want non-empty/%d/assignment/workflow.assignment_ready",
			wakeID, routedTaskID, assignmentID, kind, task.ID)
	}
	var assignmentAgent sql.NullString
	var assignmentState string
	if err := svc.db.QueryRow(`SELECT agent, state FROM task_assignments WHERE id = ?`, assignmentID.Int64).Scan(
		&assignmentAgent, &assignmentState,
	); err != nil {
		t.Fatal(err)
	}
	if assignmentAgent.Valid || assignmentState != AssignmentClaimable {
		t.Fatalf("claim-one assignment owner/state = %v/%q; want NULL/claimable", assignmentAgent, assignmentState)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["task_key"] != task.Key || payload["queue"] != "DEV" ||
		payload["requirement"] != "implementation" || payload["pool"] != "developers" ||
		payload["agent"] != "dev-1" {
		t.Fatalf("wake payload = %#v; want task/queue/requirement/pool/agent routing", payload)
	}
}

func TestManagedTaskReadsPreservePinnedWorkflow(t *testing.T) {
	svc, actor := workflowFixture(t)
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	binding := activateDevelopmentVersion(t, svc, actor, 1, 0)
	created, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "read back"})
	if err != nil {
		t.Fatal(err)
	}
	assertWorkflow := func(where string, task Task) {
		t.Helper()
		if task.WorkflowVersionID != binding.WorkflowVersionID || task.WorkflowVersion != "development@1" || task.WorkflowStatus != "implement" || task.WorkflowRevision != 1 {
			t.Fatalf("%s workflow = %#v; want development@1/implement revision 1", where, task)
		}
	}
	detail, err := svc.GetTask(context.Background(), actor, created.Key)
	if err != nil {
		t.Fatal(err)
	}
	assertWorkflow("get", detail.Task)
	page, err := svc.ListTasks(context.Background(), actor, ListFilter{Queue: "DEV", StatusView: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 1 {
		t.Fatalf("listed tasks = %#v; want one", page.Tasks)
	}
	assertWorkflow("list", page.Tasks[0])
}

func TestChildUsesCurrentQueueWorkflowAndNewVersionOnlyAffectsNewTasks(t *testing.T) {
	svc, actor := workflowFixture(t)
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	firstBinding := activateDevelopmentVersion(t, svc, actor, 1, 0)
	parent, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}

	second := publishDevelopmentVersion(t, svc, actor, 2)
	secondBinding, err := svc.ActivateQueueWorkflow(context.Background(), actor, "DEV", second.ID, firstBinding.Revision, "activate-v2")
	if err != nil {
		t.Fatal(err)
	}
	root, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "new root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{ParentKey: parent.Key, Title: "child"})
	if err != nil {
		t.Fatal(err)
	}
	var parentVersionID int64
	if err := svc.db.QueryRow(`SELECT workflow_version_id FROM tasks WHERE id = ?`, parent.ID).Scan(&parentVersionID); err != nil {
		t.Fatal(err)
	}
	if parentVersionID != firstBinding.WorkflowVersionID {
		t.Fatalf("parent workflow version id = %d; want %d", parentVersionID, firstBinding.WorkflowVersionID)
	}
	for _, task := range []Task{root, child} {
		if task.WorkflowVersionID != secondBinding.WorkflowVersionID || task.WorkflowVersion != "development@2" {
			t.Fatalf("new task workflow = %#v; want development@2", task)
		}
	}
}

func TestWorkflowTaskInitializationRollsBackWithTaskCreation(t *testing.T) {
	svc, actor := workflowFixture(t)
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	activateDevelopmentVersion(t, svc, actor, 1, 0)
	if _, err := svc.db.Exec(`
		CREATE TRIGGER fail_workflow_initialization
		BEFORE INSERT ON task_status_executions
		BEGIN
			SELECT RAISE(ABORT, 'workflow init failed');
		END`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "rollback"}); err == nil {
		t.Fatal("managed task creation succeeded despite failed workflow initialization")
	}
	var tasks, next int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE queue_prefix = 'DEV'`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT next_number FROM task_queues WHERE prefix = 'DEV'`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 || next != 1 {
		t.Fatalf("tasks/next after rollback = %d/%d; want 0/1", tasks, next)
	}
}

func TestGetQueueWorkflowReturnsTypedNotFound(t *testing.T) {
	svc, actor := workflowFixture(t)
	if _, err := svc.GetQueueWorkflow(context.Background(), actor, "DEV"); ErrorCode(err) != "queue_workflow_not_found" {
		t.Fatalf("missing binding error = %v; want queue_workflow_not_found", err)
	}
	if _, err := svc.GetQueueWorkflow(context.Background(), AgentActor("dev-1"), "DEV"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent get binding error = %v; want forbidden", err)
	}
}
