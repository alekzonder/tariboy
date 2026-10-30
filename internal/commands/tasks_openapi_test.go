package commands

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func init() { typedCallSets["tasks"] = tasksTypedCalls }

// tasksTypedCalls drives the native task routes the shared taskTypedCalls
// table leaves out against the tasks golden fixture. Each mutation acts on its
// own task with the revision read here, so the calls run in any order.
func tasksTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	t.Helper()
	c, svc := tasksFixture(t)
	customer := tasks.CustomerActor("customer")
	ctx := t.Context()
	if _, err := svc.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: "TYP", Name: "Typed", Owners: []string{"dev-a"}}); err != nil {
		t.Fatal(err)
	}
	newTask := func(title string) (string, int64) {
		t.Helper()
		task, err := svc.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "TYP", Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return task.Key, task.Revision
	}
	updated, updatedRev := newTask("Update me")
	claimed, claimedRev := newTask("Claim me")
	moved, movedRev := newTask("Move me")
	completed, completedRev := newTask("Complete me")
	related, relatedRev := newTask("Relate me")
	target, _ := newTask("Target")
	unrelated, _ := newTask("Unrelate me")
	relation, err := svc.AddRelation(ctx, customer, unrelated, tasks.RelationInput{TargetKey: target, Type: "related", Revision: 1, IdempotencyKey: "rel"})
	if err != nil {
		t.Fatal(err)
	}
	unrelatedDetail, err := svc.GetTask(ctx, customer, unrelated)
	if err != nil {
		t.Fatal(err)
	}
	// An agent's question to the customer files a notification and a wait.
	if _, err := svc.AddComment(ctx, tasks.AgentActor("dev-a"), target, tasks.AddCommentInput{Body: "Need input from @user:customer"}); err != nil {
		t.Fatal(err)
	}
	trigger, err := svc.CreateQueueWorkflowTrigger(ctx, customer, "TYP", tasks.CreateQueueWorkflowTriggerInput{Pattern: "plugin:*", Action: "create_task"})
	if err != nil {
		t.Fatal(err)
	}
	doomed, err := svc.RebindAgentPool(ctx, customer, "TYP", "doomed", []string{"dev-a"}, 0, "doomed")
	if err != nil {
		t.Fatal(err)
	}
	// The imported bundle comes from a second daemon with the same queue.
	_, source := tasksFixture(t)
	if _, err := source.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: "TYP", Name: "Typed"}); err != nil {
		t.Fatal(err)
	}
	exported, err := source.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "TYP", Title: "Exported"})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := source.ExportTask(ctx, customer, exported.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := aiproxy.NewStore(c.Store, nil).Insert(aiproxy.AIRequest{ID: "r1", TS: tasksGoldenClock.Add(time.Minute).Format(time.RFC3339Nano),
		Agent: "dev-a", Iteration: "it-1", Model: "m", InputTokens: 10, OutputTokens: 5, CostUSD: 0.25, Status: "ok", TaskID: claimed}); err != nil {
		t.Fatal(err)
	}
	window := url.Values{"since": {tasksGoldenClock.Add(-time.Hour).Format(time.RFC3339)}, "until": {tasksGoldenClock.Add(time.Hour).Format(time.RFC3339)}}
	agentWindow := url.Values{"since": window["since"], "until": window["until"], "agent": {"dev-a"}}
	return c, []typedCall{
		{"GET", "/api/task-queues", "/api/task-queues", nil},
		{"POST", "/api/task-queues", "/api/task-queues", map[string]any{"prefix": "NEW", "name": "New"}},
		{"GET", "/api/task-queues/{prefix}", "/api/task-queues/TYP", nil},
		{"PATCH", "/api/task-queues/{prefix}", "/api/task-queues/NEW", map[string]any{"description": "d", "revision": 1}},
		{"GET", "/api/tasks", "/api/tasks?queue=TYP", nil},
		{"GET", "/api/tasks", "/api/tasks?queue=TYP&limit=1", nil},
		{"POST", "/api/tasks", "/api/tasks", map[string]any{"queue": "TYP", "title": "Created", "pull_request": "https://example.test/o/r/pull/1"}},
		{"GET", "/api/tasks/{key}", "/api/tasks/" + target, nil},
		{"GET", "/api/tasks/{key}", "/api/tasks/" + unrelated, nil},
		{"PATCH", "/api/tasks/{key}", "/api/tasks/" + updated, map[string]any{"title": "Updated", "revision": updatedRev}},
		{"GET", "/api/tasks/{key}/export", "/api/tasks/" + target + "/export", nil},
		{"POST", "/api/tasks/import", "/api/tasks/import", map[string]any{"bundle": bundle}},
		{"POST", "/api/tasks/{key}/claim", "/api/tasks/" + claimed + "/claim", map[string]any{"revision": claimedRev}},
		{"POST", "/api/tasks/{key}/move", "/api/tasks/" + moved + "/move", map[string]any{"parent_key": target, "revision": movedRev}},
		{"POST", "/api/tasks/{key}/complete", "/api/tasks/" + completed + "/complete", map[string]any{"revision": completedRev}},
		{"GET", "/api/tasks/{key}/comments", "/api/tasks/" + target + "/comments", nil},
		{"GET", "/api/tasks/{key}/comments", "/api/tasks/" + updated + "/comments", nil},
		{"POST", "/api/tasks/{key}/comments", "/api/tasks/" + target + "/comments", map[string]any{"body": "Answered"}},
		{"GET", "/api/tasks/{key}/relations", "/api/tasks/" + unrelated + "/relations", nil},
		{"POST", "/api/tasks/{key}/relations", "/api/tasks/" + related + "/relations", map[string]any{"target_key": target, "type": "related", "revision": relatedRev, "idempotency_key": "add"}},
		{"DELETE", "/api/tasks/{key}/relations", fmt.Sprintf("/api/tasks/%s/relations?relation_id=%d&revision=%d&idempotency_key=del", unrelated, relation.ID, unrelatedDetail.Task.Revision), nil},
		{"GET", "/api/tasks/{key}/events", "/api/tasks/" + target + "/events", nil},
		{"GET", "/api/tasks/{key}/events", "/api/tasks/" + target + "/events?after=1000", nil},
		{"GET", "/api/task-principals", "/api/task-principals", nil},
		{"DELETE", "/api/task-queues/{queue}/workflow-triggers/{id}", fmt.Sprintf("/api/task-queues/TYP/workflow-triggers/%d", trigger.ID), nil},
		{"DELETE", "/api/task-queues/{queue}/pools/{pool}", fmt.Sprintf("/api/task-queues/TYP/pools/doomed?revision=%d&idempotency_key=del-pool", doomed.Revision), nil},
		{"POST", "/api/tasks/{key}/workflow/upgrade", "/api/tasks/DEV-1/workflow/upgrade", map[string]any{"reason": "new rules"}},
		{"POST", "/api/tasks/{key}/workflow/reassign", "/api/tasks/DEV-1/workflow/reassign", map[string]any{"agent": "dev-2"}},
		{"GET", "/api/task-activity", "/api/task-activity?" + window.Encode(), nil},
		{"GET", "/api/task-activity", "/api/task-activity?" + agentWindow.Encode(), nil},
	}
}
