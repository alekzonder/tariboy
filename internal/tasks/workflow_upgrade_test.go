package tasks

import (
	"context"
	"testing"
)

// upgradeFixture is requestFixture with version 0.2.0 of the same workflow
// published and bound to DEV, so the task stays pinned to 0.1.0.
func upgradeFixture(t *testing.T) (*Service, Actor, Task, string) {
	t.Helper()
	svc, actor, task := requestFixture(t)
	def := requestDefinition()
	def.WorkflowVersion = "0.2.0"
	digest := seedEngineImage(t, svc, def)
	if _, err := svc.SetQueueWorkflow(context.Background(), actor, "DEV", "development:0.2.0", 1); err != nil {
		t.Fatal(err)
	}
	return svc, actor, task, digest
}

func TestUpgradeWorkflowRestartsTheTaskAtTheQueueVersion(t *testing.T) {
	svc, actor, task, d2 := upgradeFixture(t)
	ctx := context.Background()
	d1 := task.WorkflowDigest
	if _, err := svc.UpgradeWorkflow(ctx, AgentActor("dev-1"), task.Key, ""); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent: %v", err)
	}
	if _, err := svc.SetArtifact(ctx, AgentActor("dev-1"), task.Key, "plan", "old plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveWorkflow(ctx, actor, task.Key, "review", "hand over"); err != nil {
		t.Fatal(err)
	}
	moved, _ := taskByKey(ctx, svc.db, task.Key)
	pending := insertPendingRequest(t, svc, moved)
	forceTask(t, svc, moved, `workflow_paused_reason = 'idle'`)

	upgraded, err := svc.UpgradeWorkflow(ctx, actor, task.Key, " new rules ")
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.WorkflowDigest != d2 || upgraded.WorkflowVersion != "0.2.0" || upgraded.WorkflowStatus != "develop" ||
		upgraded.Assignee != "agent:dev-1" || upgraded.WorkflowPausedReason != "" || upgraded.Status != StatusInProgress {
		t.Fatalf("upgraded = %#v", upgraded)
	}
	if state := requestState(t, svc, upgraded, pending); state != "cancelled" {
		t.Fatalf("pending request = %s", state)
	}
	payload := eventPayload(t, svc, upgraded, "workflow.upgraded")
	if payload["from_version"] != "0.1.0" || payload["to_version"] != "0.2.0" || payload["from_digest"] != d1 ||
		payload["to_digest"] != d2 || payload["status"] != "review" || payload["actor"] != "user:customer" ||
		payload["reason"] != "new rules" {
		t.Fatalf("upgraded payload = %v", payload)
	}
	view := workflowView(t, svc, upgraded)
	if view.Digest != d2 || view.QueueDigest != d2 || view.QueueVersion != "0.2.0" {
		t.Fatalf("view = %s %s %s", view.Digest, view.QueueDigest, view.QueueVersion)
	}
	if len(view.Artifacts) != 1 || view.Artifacts[0].Value != "old plan" {
		t.Fatalf("artifacts = %#v", view.Artifacts)
	}
	visits := view.Visits
	if len(visits) != 3 {
		t.Fatalf("visits = %#v", visits)
	}
	if visits[0].WorkflowDigest != d1 || visits[0].WorkflowVersion != "0.1.0" ||
		visits[1].WorkflowDigest != d1 || visits[1].LeftAt == "" {
		t.Fatalf("old visits = %#v", visits[:2])
	}
	if visits[1].Message != "workflow upgraded from 0.1.0 to 0.2.0: new rules" {
		t.Fatalf("closed visit message = %q", visits[1].Message)
	}
	if last := visits[2]; last.Status != "develop" || last.WorkflowDigest != d2 || last.WorkflowVersion != "0.2.0" || last.LeftAt != "" {
		t.Fatalf("new visit = %#v", last)
	}

	// The task is on the queue's version now.
	if _, err := svc.UpgradeWorkflow(ctx, actor, task.Key, ""); ErrorCode(err) != "workflow_current" || ErrorStatus(err) != 409 {
		t.Fatalf("current: %v", err)
	}
}

func TestUpgradeWorkflowRefusals(t *testing.T) {
	svc, actor, task, d2 := upgradeFixture(t)
	ctx := context.Background()
	flexible, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "TASK", Title: "flex"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpgradeWorkflow(ctx, actor, flexible.Key, ""); ErrorCode(err) != "workflow_not_bound" {
		t.Fatalf("flexible: %v", err)
	}

	requireSecrets(t, svc, d2, "GH_TOKEN")
	if _, err := svc.UpgradeWorkflow(ctx, actor, task.Key, ""); ErrorCode(err) != "workflow_secret_missing" {
		t.Fatalf("secret: %v", err)
	}
	stored, _ := taskByKey(ctx, svc.db, task.Key)
	if stored.WorkflowDigest != task.WorkflowDigest || stored.WorkflowStatus != "develop" || stored.Revision != task.Revision {
		t.Fatalf("refused upgrade changed the task: %#v", stored)
	}
	requireSecrets(t, svc, d2)

	if _, err := svc.db.Exec(`DELETE FROM task_queue_workflows WHERE queue_prefix = 'DEV'`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpgradeWorkflow(ctx, actor, task.Key, ""); ErrorCode(err) != "workflow_unbound" {
		t.Fatalf("unbound: %v", err)
	}
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "development:0.2.0", 0); err != nil {
		t.Fatal(err)
	}
	if view := workflowView(t, svc, task); view.QueueDigest != d2 || view.Digest == d2 {
		t.Fatalf("view before upgrade = %s / queue %s", view.Digest, view.QueueDigest)
	}

	if _, err := svc.CancelWorkflowTask(ctx, actor, task.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpgradeWorkflow(ctx, actor, task.Key, ""); ErrorCode(err) != "workflow_closed" {
		t.Fatalf("closed: %v", err)
	}
}
