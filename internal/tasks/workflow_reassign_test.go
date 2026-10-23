package tasks

import (
	"context"
	"testing"
)

const assignedNotifications = `SELECT COUNT(*) FROM task_notification_outbox o JOIN task_events e ON e.sequence = o.event_sequence
	WHERE e.task_id = ? AND o.message_type = 'task.assigned' AND o.channel = ?`

func TestReassignWorkflowHandsThePoolStatusToTheChosenMember(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	setCounters(t, svc, task)
	if _, err := svc.AddComment(ctx, AgentActor("dev-1"), task.Key, AddCommentInput{Body: "@user:customer which API?"}); err != nil {
		t.Fatal(err)
	}
	signals := 0
	svc.SetGoalSignal(func() { signals++ })
	got, err := svc.ReassignWorkflow(ctx, actor, task.Key, " dev-2 ")
	if err != nil {
		t.Fatal(err)
	}
	if signals == 0 {
		t.Fatal("no goal signal after reassign")
	}
	if got.Assignee != "agent:dev-2" || got.Status != StatusInProgress || got.WaitingOn != "" ||
		got.WorkflowStatus != "develop" || got.Access != "write" {
		t.Fatalf("reassigned = %#v", got)
	}
	if agent, released := holderRow(t, svc, task, "developers"); agent != "dev-2" || released {
		t.Fatalf("holder = %s released %v", agent, released)
	}
	if c := visitCounters(t, svc, task); c[0] != 0 || c[1] != 0 {
		t.Fatalf("counters = %v", c)
	}
	if waits := openWaitRows(t, svc, task); len(waits) != 0 {
		t.Fatalf("waits = %#v", waits)
	}
	payload := eventPayload(t, svc, task, "workflow.reassigned")
	if payload["from"] != "agent:dev-1" || payload["to"] != "agent:dev-2" || payload["pool"] != "developers" ||
		payload["actor"] != "user:customer" || payload["status"] != "develop" {
		t.Fatalf("reassigned payload = %v", payload)
	}
	if n := countRows(t, svc, assignedNotifications, task.ID, principalChannel("agent:dev-2")); n != 1 {
		t.Fatalf("assignment notifications to dev-2 = %d", n)
	}
	if _, err := svc.Advance(ctx, AgentActor("dev-1"), task.Key, AdvanceInput{Outcome: "ask"}); ErrorCode(err) != "not_found" {
		t.Fatalf("old holder advance: %v", err)
	}
}

func TestReassignWorkflowToTheHolderChangesNothing(t *testing.T) {
	svc, actor, task := requestFixture(t)
	got, err := svc.ReassignWorkflow(context.Background(), actor, task.Key, "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Assignee != "agent:dev-1" || got.Revision != task.Revision {
		t.Fatalf("got = %#v", got)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_events WHERE task_id = ? AND kind = 'workflow.reassigned'`, task.ID); n != 0 {
		t.Fatalf("reassigned events = %d", n)
	}
}

func TestReassignedHolderIsStickyWhenTheStatusComesBack(t *testing.T) {
	svc, actor, task := requestFixture(t)
	if _, err := svc.ReassignWorkflow(context.Background(), actor, task.Key, "dev-2"); err != nil {
		t.Fatal(err)
	}
	enter(t, svc, task.Key, "review", "ready")
	if back := enter(t, svc, task.Key, "develop", "changes"); back.Assignee != "agent:dev-2" {
		t.Fatalf("back in develop = %#v", back)
	}
}

func TestReassignWorkflowRefusals(t *testing.T) {
	ctx := context.Background()
	svc, actor, task := requestFixture(t)
	if _, err := svc.ReassignWorkflow(ctx, AgentActor("dev-1"), task.Key, "dev-2"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent: %v", err)
	}
	if _, err := svc.ReassignWorkflow(ctx, actor, task.Key, " "); ErrorCode(err) != "invalid_agent" {
		t.Fatalf("empty agent: %v", err)
	}
	if _, err := svc.ReassignWorkflow(ctx, actor, task.Key, "reviewer-1"); ErrorCode(err) != "agent_unavailable" {
		t.Fatalf("out of pool: %v", err)
	}
	if _, err := svc.ReassignWorkflow(ctx, actor, task.Key, "ghost"); ErrorCode(err) != "agent_unavailable" {
		t.Fatalf("unknown agent: %v", err)
	}
	for _, assignments := range []string{`enabled = 0`, `error_reason = 'boom'`} {
		setAgent(t, svc, "dev-2", assignments)
		_, err := svc.ReassignWorkflow(ctx, actor, task.Key, "dev-2")
		if ErrorCode(err) != "agent_unavailable" {
			t.Fatalf("%s: %v", assignments, err)
		}
		setAgent(t, svc, "dev-2", `enabled = 1, error_reason = ''`)
	}
	pause(t, svc, task.Key, PauseIdleIterations, "")
	if _, err := svc.ReassignWorkflow(ctx, actor, task.Key, "dev-2"); ErrorCode(err) != "workflow_paused" {
		t.Fatalf("paused: %v", err)
	}
	if got := storedTask(t, svc, task.Key); got.Assignee != "agent:dev-1" {
		t.Fatalf("refusals changed the task: %#v", got)
	}
	for _, status := range []string{"approval", "merge", "done"} {
		svc, actor, task := requestFixture(t)
		enter(t, svc, task.Key, status, "")
		if _, err := svc.ReassignWorkflow(ctx, actor, task.Key, "dev-2"); ErrorCode(err) != "invalid_status_owner" {
			t.Fatalf("%s: %v", status, err)
		}
	}
	runSvc, runActor, runTask, _ := runFixture(t)
	advanceApprove(t, runSvc, runActor, runTask)
	if _, err := runSvc.ReassignWorkflow(ctx, runActor, runTask.Key, "reviewer-1"); ErrorCode(err) != "transition_pending" {
		t.Fatalf("pending: %v", err)
	}
}
