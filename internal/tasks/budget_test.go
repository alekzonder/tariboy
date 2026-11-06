package tasks

import (
	"context"
	"math"
	"strings"
	"testing"
)

// spend records one AI request of cost on the task key, as the proxy does.
func spend(t *testing.T, svc *Service, key string, cost float64) {
	t.Helper()
	if _, err := svc.db.Exec(`INSERT INTO ai_requests(id, ts, agent, cost_usd, task_id)
		VALUES (lower(hex(randomblob(8))), '2026-07-31T12:00:00.000000000Z', 'dev-1', ?, ?)`, cost, key); err != nil {
		t.Fatal(err)
	}
}

func setBudget(t *testing.T, svc *Service, key string, usd float64) Task {
	t.Helper()
	task := storedTask(t, svc, key)
	updated, err := svc.UpdateTask(context.Background(), operatorOf(svc), key, UpdateTaskInput{BudgetUSD: &usd, Revision: task.Revision})
	if err != nil {
		t.Fatal(err)
	}
	return updated
}

func budgetTree(t *testing.T) (*Service, Task, Task) {
	t.Helper()
	svc := newTestService(t)
	ctx := context.Background()
	actor := operatorOf(svc)
	if err := svc.EnsureDefaultQueue(ctx); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "TASK", Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateTask(ctx, actor, CreateTaskInput{ParentKey: parent.Key, Title: "child"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, parent, child
}

func TestUpdateTaskBudgetIsCustomerOnly(t *testing.T) {
	t.Parallel()
	svc, parent, _ := budgetTree(t)
	ctx := context.Background()
	if got := setBudget(t, svc, parent.Key, 5); got.BudgetUSD != 5 {
		t.Fatalf("budget = %v", got.BudgetUSD)
	}
	if got := storedTask(t, svc, parent.Key); got.BudgetUSD != 5 {
		t.Fatalf("stored budget = %v", got.BudgetUSD)
	}
	if fields := jsonFields(t, storedTask(t, svc, parent.Key)); fields["budget_usd"] != 5.0 {
		t.Fatalf("json = %v", fields)
	}
	current := storedTask(t, svc, parent.Key)
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := svc.UpdateTask(ctx, operatorOf(svc), parent.Key, UpdateTaskInput{BudgetUSD: &bad, Revision: current.Revision}); ErrorCode(err) != "invalid_budget" {
			t.Fatalf("budget %v: %v", bad, err)
		}
	}
	if _, err := svc.db.Exec(`UPDATE tasks SET assignee = 'agent:dev-1' WHERE id = ?`, current.ID); err != nil {
		t.Fatal(err)
	}
	seedAgent(t, svc, "dev-1", "")
	more := 50.0
	if _, err := svc.UpdateTask(ctx, AgentActor("dev-1"), parent.Key, UpdateTaskInput{BudgetUSD: &more, Revision: current.Revision}); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent budget: %v", err)
	}
}

func TestExhaustedBudget(t *testing.T) {
	t.Parallel()
	svc, parent, child := budgetTree(t)
	ctx := context.Background()
	check := func(key string) (BudgetBlock, bool) {
		t.Helper()
		block, ok, err := svc.ExhaustedBudget(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		return block, ok
	}
	spend(t, svc, child.Key, 3)
	if _, ok := check(child.Key); ok {
		t.Fatal("no limit blocked")
	}
	setBudget(t, svc, parent.Key, 5)
	if _, ok := check(child.Key); ok {
		t.Fatal("blocked under the limit")
	}
	// An old key of the child still counts toward the parent.
	if _, err := svc.db.Exec(`INSERT INTO task_key_aliases(old_key, task_id, created_at) VALUES ('TASK-old', ?, '')`, child.ID); err != nil {
		t.Fatal(err)
	}
	spend(t, svc, "TASK-old", 1)
	spend(t, svc, parent.Key, 1)
	block, ok := check(child.Key)
	if !ok || block.Key != parent.Key || block.BudgetUSD != 5 || block.SpentUSD != 5 {
		t.Fatalf("block = %#v %v", block, ok)
	}
	if block, ok := check("TASK-old"); !ok || block.Key != parent.Key {
		t.Fatalf("alias block = %#v %v", block, ok)
	}
	// The nearest exhausted limit wins.
	setBudget(t, svc, child.Key, 2)
	if block, ok := check(child.Key); !ok || block.Key != child.Key || block.SpentUSD != 4 {
		t.Fatalf("nearest block = %#v %v", block, ok)
	}
	setBudget(t, svc, child.Key, 0)
	setBudget(t, svc, parent.Key, 0)
	if _, ok := check(child.Key); ok {
		t.Fatal("zero limit blocked")
	}
	if _, ok := check("TASK-MISSING"); ok {
		t.Fatal("unknown key blocked")
	}
	status, err := svc.TaskBudget(ctx, operatorOf(svc), parent.Key)
	if err != nil {
		t.Fatal(err)
	}
	if status.Key != parent.Key || status.BudgetUSD != 0 || status.SpentUSD != 5 || status.Exhausted {
		t.Fatalf("status = %#v", status)
	}
}

func TestBudgetExhaustedBlocksAFlexibleTask(t *testing.T) {
	t.Parallel()
	svc, parent, child := budgetTree(t)
	ctx := context.Background()
	setBudget(t, svc, parent.Key, 1)
	spend(t, svc, child.Key, 2)
	if err := svc.HandleBudgetExhausted(ctx, child.Key); err != nil {
		t.Fatal(err)
	}
	blocked := storedTask(t, svc, child.Key)
	want := "budget exhausted: task " + parent.Key + " spent $2.00 of $1.00"
	if blocked.ManualBlockReason != want || !blocked.Blocked || blocked.Revision != child.Revision+1 {
		t.Fatalf("blocked = %#v", blocked)
	}
	detail := taskDetail(t, svc, blocked)
	if len(detail.Comments) != 1 || !strings.Contains(detail.Comments[0].Body, "@"+blocked.Customer) ||
		!strings.Contains(detail.Comments[0].Body, want) {
		t.Fatalf("comments = %#v", detail.Comments)
	}
	// A second refusal changes nothing.
	if err := svc.HandleBudgetExhausted(ctx, child.Key); err != nil {
		t.Fatal(err)
	}
	if again := storedTask(t, svc, child.Key); again.Revision != blocked.Revision {
		t.Fatalf("second call changed the task: %#v", again)
	}
	// An agent cannot lift the block.
	if _, err := svc.db.Exec(`UPDATE tasks SET assignee = 'agent:dev-1' WHERE id = ?`, child.ID); err != nil {
		t.Fatal(err)
	}
	seedAgent(t, svc, "dev-1", "")
	empty := ""
	if _, err := svc.UpdateTask(ctx, AgentActor("dev-1"), child.Key, UpdateTaskInput{ManualBlockReason: &empty, Revision: blocked.Revision}); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent unblock: %v", err)
	}
	// A manual block with other text stays when the limit is raised.
	sibling, err := svc.CreateTask(ctx, operatorOf(svc), CreateTaskInput{ParentKey: parent.Key, Title: "sibling"})
	if err != nil {
		t.Fatal(err)
	}
	manual := "waiting for hardware"
	if _, err := svc.UpdateTask(ctx, operatorOf(svc), sibling.Key, UpdateTaskInput{ManualBlockReason: &manual, Revision: sibling.Revision}); err != nil {
		t.Fatal(err)
	}
	setBudget(t, svc, parent.Key, 1.5)
	if still := storedTask(t, svc, child.Key); still.ManualBlockReason != want {
		t.Fatalf("raised below the spend unblocked: %#v", still)
	}
	setBudget(t, svc, parent.Key, 10)
	if lifted := storedTask(t, svc, child.Key); lifted.ManualBlockReason != "" || lifted.Blocked {
		t.Fatalf("raised limit kept the block: %#v", lifted)
	}
	if kept := storedTask(t, svc, sibling.Key); kept.ManualBlockReason != manual {
		t.Fatalf("manual block changed: %#v", kept)
	}
}

func TestBudgetExhaustedPausesAWorkflowTask(t *testing.T) {
	t.Parallel()
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	setBudget(t, svc, task.Key, 1)
	spend(t, svc, task.Key, 1)
	if err := svc.HandleBudgetExhausted(ctx, task.Key); err != nil {
		t.Fatal(err)
	}
	paused := storedTask(t, svc, task.Key)
	if paused.WorkflowPausedReason != PauseBudgetExhausted || paused.ManualBlockReason != "" {
		t.Fatalf("paused = %#v", paused)
	}
	waits := openWaitRows(t, svc, task)
	if len(waits) != 1 || !strings.Contains(waits[0].body, "spent $1.00 of $1.00") {
		t.Fatalf("waits = %#v", waits)
	}
	if err := svc.HandleBudgetExhausted(ctx, task.Key); err != nil {
		t.Fatal(err)
	}
	if again := storedTask(t, svc, task.Key); again.Revision != paused.Revision {
		t.Fatalf("second call changed the task: %#v", again)
	}
	if _, err := svc.ResumeWorkflow(ctx, actor, task.Key, ResumeContinue); ErrorCode(err) != "budget_exhausted" || ErrorStatus(err) != 409 ||
		!strings.Contains(err.Error(), "spent $1.00 of $1.00") {
		t.Fatalf("resume over the limit: %v", err)
	}
	setBudget(t, svc, task.Key, 2)
	if resumed, err := svc.ResumeWorkflow(ctx, actor, task.Key, ResumeContinue); err != nil || resumed.WorkflowPausedReason != "" {
		t.Fatalf("resume = %#v, %v", resumed, err)
	}
}
