package tasks

import (
	"context"
	"testing"
)

// customerPoolFixture is requestFixture with CUSTOMER_AGENT_POOL=reviewers on
// DEV: its customer status "approval" is owned by the reviewers pool.
func customerPoolFixture(t *testing.T) (*Service, Actor, Task) {
	t.Helper()
	svc, actor, task := requestFixture(t)
	if _, err := svc.SetQueueEnvInfo(context.Background(), actor, "DEV", CustomerAgentPoolEnv, "reviewers"); err != nil {
		t.Fatal(err)
	}
	return svc, actor, task
}

func TestCustomerAgentPoolHoldsACustomerStatus(t *testing.T) {
	svc, actor, task := customerPoolFixture(t)
	ctx := context.Background()
	held := enter(t, svc, task.Key, "approval", "ask")
	if held.Status != StatusInProgress || held.WaitingOn != "" || held.Assignee != "agent:reviewer-1" {
		t.Fatalf("approval with a customer agent pool = %#v", held)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_waiting_for WHERE task_id = ? AND resolved_at = ''`, task.ID); n != 0 {
		t.Fatalf("open waits = %d, want none", n)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_notification_outbox WHERE message_type = 'task.question'`); n != 0 {
		t.Fatalf("question notifications = %d, want none", n)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM tasks WHERE id = ? AND workflow_owner_kind = 'pool'`, task.ID); n != 1 {
		t.Fatal("workflow_owner_kind is not pool")
	}
	view := workflowView(t, svc, held)
	if view.Owner != "pool:reviewers" || view.Holder != "agent:reviewer-1" {
		t.Fatalf("view owner = %q holder = %q", view.Owner, view.Holder)
	}
	// A later save of the task keeps the pool owner.
	if _, err := svc.AddComment(ctx, actor, task.Key, AddCommentInput{Body: "note"}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM tasks WHERE id = ? AND workflow_owner_kind = 'pool'`, task.ID); n != 1 {
		t.Fatal("a comment reset workflow_owner_kind")
	}
	if _, err := svc.Advance(ctx, AgentActor("dev-1"), task.Key, AdvanceInput{Outcome: "approved"}); err == nil {
		t.Fatal("an agent outside the pool advanced the task")
	}
	if _, err := svc.Advance(ctx, AgentActor("reviewer-1"), task.Key, AdvanceInput{Outcome: "rejected"}); err != nil {
		t.Fatal(err)
	}
	stored, _ := taskByKey(ctx, svc.db, task.Key)
	if stored.WorkflowStatus != "develop" || stored.Assignee != "agent:dev-1" {
		t.Fatalf("after rejected = %#v", stored)
	}
	// The customer keeps the decision too.
	enter(t, svc, task.Key, "approval", "ask")
	if _, err := svc.Advance(ctx, actor, task.Key, AdvanceInput{Outcome: "rejected"}); err != nil {
		t.Fatalf("customer advance in a replaced status: %v", err)
	}
}

func TestCustomerAgentPoolChangeAppliesOnTheNextEntry(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	waiting := enter(t, svc, task.Key, "approval", "ask")
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", CustomerAgentPoolEnv, "reviewers"); err != nil {
		t.Fatal(err)
	}
	if view := workflowView(t, svc, waiting); view.Owner != "customer" {
		t.Fatalf("open visit owner after env set = %q, want customer", view.Owner)
	}
	if _, err := svc.Advance(ctx, AgentActor("reviewer-1"), task.Key, AdvanceInput{Outcome: "approved"}); err == nil {
		t.Fatal("a pool agent advanced a customer visit entered before the env var was set")
	}
	if _, err := svc.Advance(ctx, actor, task.Key, AdvanceInput{Outcome: "rejected"}); err != nil {
		t.Fatal(err)
	}
	if held := enter(t, svc, task.Key, "approval", "ask"); held.Assignee != "agent:reviewer-1" {
		t.Fatalf("next entry = %#v", held)
	}
	if err := svc.RemoveQueueEnv(ctx, actor, "DEV", CustomerAgentPoolEnv); err != nil {
		t.Fatal(err)
	}
	if view := workflowView(t, svc, task); view.Owner != "pool:reviewers" {
		t.Fatalf("open visit owner after env remove = %q, want pool:reviewers", view.Owner)
	}
	if _, err := svc.Advance(ctx, AgentActor("reviewer-1"), task.Key, AdvanceInput{Outcome: "rejected"}); err != nil {
		t.Fatal(err)
	}
	if waiting := enter(t, svc, task.Key, "approval", "ask"); waiting.Status != StatusWaitCustomer {
		t.Fatalf("entry after env remove = %#v", waiting)
	}
}

func TestCustomerAgentPoolEnvIsChecked(t *testing.T) {
	svc, actor, _ := requestFixture(t)
	ctx := context.Background()
	if err := svc.SetQueueSecret(ctx, actor, "DEV", CustomerAgentPoolEnv, "reviewers"); ErrorCode(err) != "invalid_secret_key" {
		t.Fatalf("as a secret = %v; want invalid_secret_key", err)
	}
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", CustomerAgentPoolEnv, "nobody"); ErrorCode(err) != "workflow_pool_empty" {
		t.Fatalf("unknown pool = %v; want workflow_pool_empty", err)
	}
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", CustomerAgentPoolEnv, "reviewers"); err != nil {
		t.Fatal(err)
	}
	pool, err := svc.GetAgentPool(ctx, actor, "DEV", "reviewers")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebindAgentPool(ctx, actor, "DEV", "reviewers", nil, pool.Revision, "empty-reviewers"); ErrorCode(err) != "workflow_pool_empty" {
		t.Fatalf("emptying the customer agent pool = %v; want workflow_pool_empty", err)
	}
}

func TestCustomerAgentPoolIsCheckedOnBind(t *testing.T) {
	svc, actor := workflowFixture(t)
	ctx := context.Background()
	if err := svc.EnsureDefaultQueue(ctx); err != nil {
		t.Fatal(err)
	}
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	seedEngineImage(t, svc, engineDefinition())
	// An unbound queue takes any value: the pool is checked when it binds.
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", CustomerAgentPoolEnv, "approvers"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "development:0.1.0", 0); ErrorCode(err) != "workflow_pool_empty" {
		t.Fatalf("bind with an empty customer agent pool = %v; want workflow_pool_empty", err)
	}
	mustRebindPool(t, svc, actor, "approvers", []string{"reviewer-1"}, 0)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "development:0.1.0", 0); err != nil {
		t.Fatal(err)
	}
}
