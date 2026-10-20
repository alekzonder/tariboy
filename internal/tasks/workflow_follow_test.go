package tasks

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

// queueWorkflowEvents returns the payloads of the queue events of kind.
func queueWorkflowEvents(t *testing.T, svc *Service, kind string) []map[string]any {
	t.Helper()
	rows, err := svc.db.Query(`SELECT payload FROM task_events WHERE queue_prefix = 'DEV' AND kind = ? ORDER BY sequence`, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatal(err)
		}
		out = append(out, payload)
	}
	return out
}

func mustCreateDevTask(t *testing.T, svc *Service, actor Actor, title string) Task {
	t.Helper()
	task, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestSetQueueWorkflowRecordsTheTagOfTheRef(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	d1 := seedBindingImage(t, svc, "flow", "1", "developers")
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)

	bound, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow", 0)
	if err != nil || bound.Digest != d1 || bound.Tag != "latest" || bound.Revision != 1 {
		t.Fatalf("bind flow = %#v, %v; want tag latest", bound, err)
	}
	// The same digest under another tag is a change.
	versioned, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 1)
	if err != nil || versioned.Tag != "1" || versioned.Revision != 2 {
		t.Fatalf("bind flow:1 = %#v, %v; want tag 1 at revision 2", versioned, err)
	}
	pinned, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:"+d1, 2)
	if err != nil || pinned.Tag != "" || pinned.Revision != 3 {
		t.Fatalf("bind by digest = %#v, %v; want no tag at revision 3", pinned, err)
	}
	same, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:"+d1, 0)
	if err != nil || same != pinned {
		t.Fatalf("repeat bind = %#v, %v; want the no-op %#v", same, err, pinned)
	}
	if got, err := svc.GetQueueWorkflow(ctx, actor, "DEV"); err != nil || got != pinned {
		t.Fatalf("get = %#v, %v; want %#v", got, err, pinned)
	}
}

func TestCreateTaskFollowsTheBoundTag(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	d1 := seedBindingImage(t, svc, "flow", "1", "developers")
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:latest", 0); err != nil {
		t.Fatal(err)
	}
	first := mustCreateDevTask(t, svc, actor, "first")
	if first.WorkflowDigest != d1 {
		t.Fatalf("first task digest = %q; want %q", first.WorkflowDigest, d1)
	}

	// Publishing version 2 moves latest; the next task pins it.
	d2 := seedBindingImage(t, svc, "flow", "2", "developers")
	second := mustCreateDevTask(t, svc, actor, "second")
	if second.WorkflowDigest != d2 || second.WorkflowVersion != "2" {
		t.Fatalf("second task = %q %q; want %q version 2", second.WorkflowDigest, second.WorkflowVersion, d2)
	}
	var firstDigest string
	if err := svc.db.QueryRow(`SELECT workflow_digest FROM tasks WHERE task_key = ?`, first.Key).Scan(&firstDigest); err != nil || firstDigest != d1 {
		t.Fatalf("first task digest after follow = %q, %v; want %q", firstDigest, err, d1)
	}
	got, err := svc.GetQueueWorkflow(ctx, actor, "DEV")
	if err != nil || got.Digest != d2 || got.Version != "2" || got.Tag != "latest" || got.Revision != 2 {
		t.Fatalf("binding after follow = %#v, %v", got, err)
	}
	events := queueWorkflowEvents(t, svc, "queue.workflow_bound")
	if len(events) != 2 || events[1]["digest"] != d2 || events[1]["tag"] != "latest" || events[1]["followed"] != true {
		t.Fatalf("bound events = %#v; want a followed event for %s", events, d2)
	}
}

func TestPinnedBindingsDoNotFollow(t *testing.T) {
	for _, ref := range []string{"flow:1", "digest"} {
		t.Run(ref, func(t *testing.T) {
			ctx := context.Background()
			svc, actor := workflowFixture(t)
			d1 := seedBindingImage(t, svc, "flow", "1", "developers")
			mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
			if ref == "digest" {
				ref = "flow:" + d1
			}
			if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", ref, 0); err != nil {
				t.Fatal(err)
			}
			seedBindingImage(t, svc, "flow", "2", "developers")
			if task := mustCreateDevTask(t, svc, actor, "t"); task.WorkflowDigest != d1 {
				t.Fatalf("task digest = %q; want pinned %q", task.WorkflowDigest, d1)
			}
			if err := svc.FollowQueueWorkflows(ctx, "flow"); err != nil {
				t.Fatal(err)
			}
			if got, err := svc.GetQueueWorkflow(ctx, actor, "DEV"); err != nil || got.Digest != d1 || got.Revision != 1 {
				t.Fatalf("binding = %#v, %v; want pinned %q", got, err, d1)
			}
		})
	}
}

func TestFollowIsBlockedByAnEmptyPoolOrMissingSecret(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	d1 := seedBindingImage(t, svc, "flow", "1", "developers")
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow", 0); err != nil {
		t.Fatal(err)
	}
	d2 := seedBindingImage(t, svc, "flow", "2", "developers", "reviewers")
	if task := mustCreateDevTask(t, svc, actor, "blocked by pool"); task.WorkflowDigest != d1 {
		t.Fatalf("task digest = %q; want old %q while the follow is blocked", task.WorkflowDigest, d1)
	}
	blocked := queueWorkflowEvents(t, svc, "queue.workflow_follow_blocked")
	if len(blocked) != 1 || blocked[0]["reason"] != "workflow_pool_empty" || blocked[0]["digest"] != d2 ||
		!reflect.DeepEqual(blocked[0]["names"], []any{"reviewers"}) {
		t.Fatalf("blocked events = %#v", blocked)
	}

	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	d3 := seedBindingImage(t, svc, "flow", "3", "developers")
	requireSecrets(t, svc, d3, "GH_TOKEN")
	if err := svc.FollowQueueWorkflows(ctx, "flow"); err != nil {
		t.Fatal(err)
	}
	blocked = queueWorkflowEvents(t, svc, "queue.workflow_follow_blocked")
	if len(blocked) != 2 || blocked[1]["reason"] != "workflow_secret_missing" ||
		!reflect.DeepEqual(blocked[1]["names"], []any{"GH_TOKEN"}) {
		t.Fatalf("blocked events = %#v", blocked)
	}
	if got, err := svc.GetQueueWorkflow(ctx, actor, "DEV"); err != nil || got.Digest != d1 {
		t.Fatalf("binding = %#v, %v; want old %q", got, err, d1)
	}

	if err := svc.SetQueueSecret(ctx, actor, "DEV", "GH_TOKEN", "x"); err != nil {
		t.Fatal(err)
	}
	if task := mustCreateDevTask(t, svc, actor, "unblocked"); task.WorkflowDigest != d3 {
		t.Fatalf("task digest = %q; want %q once the secret exists", task.WorkflowDigest, d3)
	}
}

func TestFollowQueueWorkflowsMovesOnlyBindingsOfTheName(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	d1 := seedBindingImage(t, svc, "flow", "1", "developers")
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow", 0); err != nil {
		t.Fatal(err)
	}
	d2 := seedBindingImage(t, svc, "flow", "2", "developers")
	if err := svc.FollowQueueWorkflows(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetQueueWorkflow(ctx, actor, "DEV"); got.Digest != d1 {
		t.Fatalf("binding moved for another name: %#v", got)
	}
	// An empty name follows every tagged binding; a second call changes nothing.
	for range 2 {
		if err := svc.FollowQueueWorkflows(ctx, ""); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := svc.GetQueueWorkflow(ctx, actor, "DEV"); got.Digest != d2 || got.Revision != 2 {
		t.Fatalf("binding = %#v; want %q at revision 2", got, d2)
	}
}
