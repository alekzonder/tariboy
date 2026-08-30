package tasks

import (
	"context"
	"testing"
)

func TestQuestionNotificationProjectsRequestingPrincipal(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Questions"})
	task, _ := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "ASK", Title: "decision", Assignee: "requester"})
	if _, err := svc.AddComment(ctx, AgentActor("requester"), task.Key, AddCommentInput{
		Body: "Need input from @user:customer",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListNotifications(ctx, customer, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RequestingPrincipal != "agent:requester" {
		t.Fatalf("notifications = %#v", got)
	}
}

func TestAnswerMarksResolvedQuestionNotificationRead(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Questions"})
	task, _ := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "ASK", Title: "decision", Assignee: "requester"})
	if _, err := svc.AddComment(ctx, AgentActor("requester"), task.Key, AddCommentInput{
		Body: "Need input from @user:customer",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, customer, task.Key, AddCommentInput{Body: "Approved"}); err != nil {
		t.Fatal(err)
	}
	notifications, err := svc.ListNotifications(ctx, customer, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 || notifications[0].ReadAt == "" {
		t.Fatalf("notifications = %#v; want resolved question marked read", notifications)
	}
}

func TestAnswerResolvesOnlyAuthorsOpenWaitsAndMentionGrantsResponseAccess(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Questions"})
	task, _ := svc.CreateTask(ctx, customer, CreateTaskInput{
		Queue: "ASK", Title: "decision", Assignee: "requester",
	})

	question, err := svc.AddComment(ctx, AgentActor("requester"), task.Key, AddCommentInput{
		Body: "Need input from @agent:alice and @user:customer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(question.CreatedWaits) != 2 {
		t.Fatalf("created waits = %d; want 2", len(question.CreatedWaits))
	}
	aliceView, err := svc.GetTask(ctx, AgentActor("alice"), task.Key)
	if err != nil {
		t.Fatalf("mentioned agent cannot open task: %v", err)
	}
	if aliceView.Task.Access != "respond" {
		t.Fatalf("mentioned agent access = %q; want respond", aliceView.Task.Access)
	}
	renamed := "must not be allowed"
	if _, err := svc.UpdateTask(ctx, AgentActor("alice"), task.Key, UpdateTaskInput{
		Title: &renamed, Revision: aliceView.Task.Revision,
	}); ErrorCode(err) != "not_found" {
		t.Fatalf("mentioned agent update error = %v; want not_found", err)
	}

	answer, err := svc.AddComment(ctx, AgentActor("alice"), task.Key, AddCommentInput{
		Body: "My answer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.ResolvedWaits) != 1 ||
		answer.ResolvedWaits[0].ExpectedPrincipal != "agent:alice" {
		t.Fatalf("resolved waits = %#v; want only agent:alice", answer.ResolvedWaits)
	}
	detail, err := svc.GetTask(ctx, customer, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.WaitingFor) != 1 ||
		detail.WaitingFor[0].ExpectedPrincipal != "user:customer" {
		t.Fatalf("remaining waits = %#v; want only user:customer", detail.WaitingFor)
	}

	userAnswer, err := svc.AddComment(ctx, customer, task.Key, AddCommentInput{
		Body: "Customer answer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(userAnswer.ResolvedWaits) != 1 {
		t.Fatalf("customer resolved waits = %d; want 1", len(userAnswer.ResolvedWaits))
	}
	detail, err = svc.GetTask(ctx, customer, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.WaitingFor) != 0 || len(detail.Comments) != 3 {
		t.Fatalf("detail waits/comments = %d/%d; want 0/3",
			len(detail.WaitingFor), len(detail.Comments))
	}
}

func TestMalformedMentionRemainsPlainComment(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "TEXT", Name: "Text"})
	task, _ := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "TEXT", Title: "text"})

	result, err := svc.AddComment(ctx, customer, task.Key, AddCommentInput{
		Body: "not principals: @alice @agent: @unknown:name",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CreatedWaits) != 0 {
		t.Fatalf("malformed mentions created waits: %#v", result.CreatedWaits)
	}
}

func TestAddCommentIdempotencyReplaysWithoutDuplicateWaitOrRevision(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Ask"})
	task, _ := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "ASK", Title: "question"})

	first, err := svc.AddComment(ctx, customer, task.Key, AddCommentInput{
		Body: "@agent:alice need input", IdempotencyKey: "comment-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.AddComment(ctx, customer, task.Key, AddCommentInput{
		Body: "different retry body", IdempotencyKey: "comment-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Comment.ID != first.Comment.ID || replayed.Comment.Body != first.Comment.Body {
		t.Fatalf("replayed = %#v, first = %#v", replayed, first)
	}
	var comments, waits, revision int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_comments WHERE task_id = ?`, task.ID).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_waiting_for WHERE task_id = ?`, task.ID).Scan(&waits); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT revision FROM tasks WHERE id = ?`, task.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if comments != 1 || waits != 1 || revision != 2 {
		t.Fatalf("comments/waits/revision = %d/%d/%d, want 1/1/2", comments, waits, revision)
	}
}
