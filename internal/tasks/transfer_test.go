package tasks

import (
	"context"
	"testing"
)

func transferFixture(t *testing.T) (*Service, Actor, Task, Task) {
	t.Helper()
	svc := newTestService(t)
	ctx := context.Background()
	actor := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, actor, CreateQueueInput{Prefix: "MOVE", Name: "Move"}); err != nil {
		t.Fatal(err)
	}
	root, err := svc.CreateTask(ctx, actor, CreateTaskInput{
		Queue: "MOVE", Title: "root", Description: "root body", Priority: PriorityP1,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateTask(ctx, actor, CreateTaskInput{ParentKey: root.Key, Title: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, actor, root.Key, AddCommentInput{Body: "first note"}); err != nil {
		t.Fatal(err)
	}
	return svc, actor, root, child
}

func TestExportImportRoundTripPreservesKeysTreeAndComments(t *testing.T) {
	source, actor, root, child := transferFixture(t)
	ctx := context.Background()

	bundle, err := source.ExportTask(ctx, actor, root.Key)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.RootKey != root.Key || bundle.Queue != "MOVE" || len(bundle.Tasks) != 2 {
		t.Fatalf("bundle = %#v", bundle)
	}

	target := newTestService(t)
	if _, err := target.CreateQueue(ctx, actor, CreateQueueInput{Prefix: "MOVE", Name: "Move"}); err != nil {
		t.Fatal(err)
	}
	imported, err := target.ImportTask(ctx, actor, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Key != root.Key {
		t.Fatalf("imported root key = %q, want %q", imported.Key, root.Key)
	}
	detail, err := target.GetTask(ctx, actor, root.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Title != "root" || detail.Task.Description != "root body" || detail.Task.Priority != PriorityP1 {
		t.Fatalf("imported root = %#v", detail.Task)
	}
	if len(detail.Comments) != 1 || detail.Comments[0].Body != "first note" ||
		detail.Comments[0].Author != "user:customer" {
		t.Fatalf("imported comments = %#v", detail.Comments)
	}
	if detail.Descendants != 1 {
		t.Fatalf("imported descendants = %d, want 1", detail.Descendants)
	}
	childDetail, err := target.GetTask(ctx, actor, child.Key)
	if err != nil {
		t.Fatal(err)
	}
	if childDetail.Task.ParentKey != root.Key {
		t.Fatalf("imported child parent = %q, want %q", childDetail.Task.ParentKey, root.Key)
	}
}

func TestImportRejectsMissingQueue(t *testing.T) {
	source, actor, root, _ := transferFixture(t)
	ctx := context.Background()
	bundle, err := source.ExportTask(ctx, actor, root.Key)
	if err != nil {
		t.Fatal(err)
	}
	target := newTestService(t)
	if _, err := target.ImportTask(ctx, actor, bundle); ErrorCode(err) != "queue_not_found" {
		t.Fatalf("import into a daemon without the queue: %v", err)
	}
}

func TestImportRejectsKeyAlreadyPresent(t *testing.T) {
	source, actor, root, _ := transferFixture(t)
	ctx := context.Background()
	bundle, err := source.ExportTask(ctx, actor, root.Key)
	if err != nil {
		t.Fatal(err)
	}
	// Importing back into the daemon the bundle came from is the clearest form
	// of a key that is already in use.
	if _, err := source.ImportTask(ctx, actor, bundle); ErrorCode(err) != "task_key_taken" {
		t.Fatalf("import over an existing key: %v", err)
	}
}

func TestImportRejectsAgentActor(t *testing.T) {
	source, actor, root, _ := transferFixture(t)
	ctx := context.Background()
	bundle, err := source.ExportTask(ctx, actor, root.Key)
	if err != nil {
		t.Fatal(err)
	}
	target := newTestService(t)
	if _, err := target.CreateQueue(ctx, actor, CreateQueueInput{Prefix: "MOVE", Name: "Move"}); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ImportTask(ctx, AgentActor("worker"), bundle); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent import: %v", err)
	}
}
