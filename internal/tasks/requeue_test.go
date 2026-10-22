package tasks

import (
	"context"
	"strings"
	"testing"
)

// requeueFixture is requestFixture with a second queue OPS bound to the same
// workflow and staffed by dev-2.
func requeueFixture(t *testing.T) (*Service, Actor, Task) {
	t.Helper()
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	if _, err := svc.CreateQueue(ctx, actor, CreateQueueInput{Prefix: "OPS", Name: "Operations"}); err != nil {
		t.Fatal(err)
	}
	for _, pool := range []string{"developers", "reviewers"} {
		if _, err := svc.RebindAgentPool(ctx, actor, "OPS", pool, []string{"dev-2"}, 0, "ops-"+pool); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.SetQueueWorkflow(ctx, actor, "OPS", "development:0.1.0", 0); err != nil {
		t.Fatal(err)
	}
	return svc, actor, task
}

func TestRequeueWorkflowTaskToAQueueWithoutWorkflow(t *testing.T) {
	svc, actor, task := requeueFixture(t)
	ctx := context.Background()
	if _, err := svc.AddComment(ctx, actor, task.Key, AddCommentInput{Body: "context to keep"}); err != nil {
		t.Fatal(err)
	}
	pending := insertPendingRequest(t, svc, task)
	source, _ := taskByKey(ctx, svc.db, task.Key)

	moved, err := svc.RequeueTask(ctx, actor, task.Key, RequeueInput{Queue: "task", Revision: source.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(moved.Key, "TASK-") || moved.Queue != "TASK" || moved.Title != task.Title ||
		moved.WorkflowDigest != "" || moved.WorkflowStatus != "" || moved.Status != StatusOpen {
		t.Fatalf("moved = %#v", moved)
	}
	old, _ := taskByKey(ctx, svc.db, task.Key)
	if old.Status != StatusCancelled || old.CompletedAt == "" || old.WorkflowStatus != "develop" {
		t.Fatalf("source = %#v", old)
	}
	if state := requestState(t, svc, task, pending); state != "cancelled" {
		t.Fatalf("pending request = %s", state)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_status_visits WHERE task_id = ? AND left_at = ''`, task.ID); n != 0 {
		t.Fatalf("open visits = %d", n)
	}

	detail := taskDetail(t, svc, moved)
	if len(detail.Comments) != 2 || detail.Comments[0].Body != "context to keep" ||
		detail.Comments[0].Author != "user:customer" || detail.Comments[1].Body != "Moved from "+task.Key+"." {
		t.Fatalf("moved comments = %#v", detail.Comments)
	}
	if len(detail.Relations) != 1 || detail.Relations[0].Type != "related" ||
		detail.Relations[0].SourceKey != task.Key || detail.Relations[0].TargetKey != moved.Key {
		t.Fatalf("relations = %#v", detail.Relations)
	}
	sourceComments := taskDetail(t, svc, task).Comments
	if last := sourceComments[len(sourceComments)-1].Body; last != "Moved to queue TASK as "+moved.Key+"." {
		t.Fatalf("source comment = %q", last)
	}
	for _, payloadTask := range []Task{task, moved} {
		payload := eventPayload(t, svc, payloadTask, "task.requeued")
		if payload["from_key"] != task.Key || payload["to_key"] != moved.Key ||
			payload["from_queue"] != "DEV" || payload["to_queue"] != "TASK" {
			t.Fatalf("%s requeued payload = %v", payloadTask.Key, payload)
		}
	}
}

func TestRequeueWorkflowTaskToAnotherWorkflowQueue(t *testing.T) {
	svc, actor, task := requeueFixture(t)
	moved, err := svc.RequeueTask(context.Background(), actor, task.Key, RequeueInput{Queue: "OPS"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(moved.Key, "OPS-") || moved.WorkflowDigest == "" ||
		moved.WorkflowStatus != "develop" || moved.Assignee != "agent:dev-2" {
		t.Fatalf("moved = %#v", moved)
	}
	if old, _ := taskByKey(context.Background(), svc.db, task.Key); old.Status != StatusCancelled {
		t.Fatalf("source = %#v", old)
	}
}

func TestRequeueFlexibleTaskToAWorkflowQueue(t *testing.T) {
	svc, actor, _ := requeueFixture(t)
	ctx := context.Background()
	flexible, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "TASK", Title: "flex", Priority: "P1"})
	if err != nil {
		t.Fatal(err)
	}
	moved, err := svc.RequeueTask(ctx, actor, flexible.Key, RequeueInput{Queue: "DEV", Revision: flexible.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(moved.Key, "DEV-") || moved.WorkflowStatus != "develop" || moved.Priority != "P1" {
		t.Fatalf("moved = %#v", moved)
	}
	old, _ := taskByKey(ctx, svc.db, flexible.Key)
	if old.Status != StatusCancelled || old.CompletedAt == "" {
		t.Fatalf("source = %#v", old)
	}
}

func TestRequeueRefusals(t *testing.T) {
	svc, actor, task := requeueFixture(t)
	ctx := context.Background()
	parent, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "TASK", Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateTask(ctx, actor, CreateTaskInput{ParentKey: parent.Key, Title: "child"})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "TASK", Title: "closed"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled := StatusCancelled
	if _, err := svc.UpdateTask(ctx, actor, closed.Key, UpdateTaskInput{Status: &cancelled, Revision: closed.Revision}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		actor Actor
		key   string
		in    RequeueInput
		code  string
	}{
		{"agent", AgentActor("dev-1"), task.Key, RequeueInput{Queue: "TASK"}, "forbidden"},
		{"missing queue", actor, task.Key, RequeueInput{}, "missing_queue"},
		{"same queue", actor, task.Key, RequeueInput{Queue: "dev"}, "same_queue"},
		{"unknown queue", actor, task.Key, RequeueInput{Queue: "NOPE"}, "queue_not_found"},
		{"stale revision", actor, task.Key, RequeueInput{Queue: "TASK", Revision: 99}, "revision_conflict"},
		{"closed", actor, closed.Key, RequeueInput{Queue: "DEV"}, "task_closed"},
		{"open child", actor, parent.Key, RequeueInput{Queue: "DEV"}, "requeue_has_children"},
	} {
		if _, err := svc.RequeueTask(ctx, tc.actor, tc.key, tc.in); ErrorCode(err) != tc.code {
			t.Fatalf("%s: %v; want %s", tc.name, err, tc.code)
		}
	}
	if stored, _ := taskByKey(ctx, svc.db, task.Key); stored.Status == StatusCancelled {
		t.Fatalf("refused requeue changed the source: %#v", stored)
	}
	// A child moves on its own and becomes a root task of the target queue.
	moved, err := svc.RequeueTask(ctx, actor, child.Key, RequeueInput{Queue: "DEV"})
	if err != nil || moved.ParentKey != "" {
		t.Fatalf("child = %#v, %v", moved, err)
	}
}
