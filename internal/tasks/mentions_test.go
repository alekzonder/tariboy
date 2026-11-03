package tasks

import (
	"context"
	"testing"
)

// mentionTask returns a service with a queue the customer owns and one task
// in it.
func mentionTask(t *testing.T) (*Service, Task) {
	t.Helper()
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{
		Prefix: "MEN", Name: "Mentions", Owners: []string{"asker"},
	}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "MEN", Title: "mentions"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, task
}

func waitPrincipalsOf(result CommentResult) []string { return waitPrincipals(result.CreatedWaits) }

func TestCommentMentionStopsBeforeTrailingPunctuation(t *testing.T) {
	marks := []string{".", ",", ":", ";", "!", "?", ")", "]", "}", `"`, "'", "»", "”", "’", "`"}
	for _, mark := range marks {
		t.Run(mark, func(t *testing.T) {
			svc, task := mentionTask(t)
			seedAgent(t, svc, "tariboy-pm", "")
			result, err := svc.AddComment(context.Background(), CustomerActor("customer"), task.Key,
				AddCommentInput{Body: "Question for @agent:tariboy-pm" + mark + " thanks"})
			if err != nil {
				t.Fatal(err)
			}
			if got := waitPrincipalsOf(result); len(got) != 1 || got[0] != "agent:tariboy-pm" {
				t.Fatalf("waits = %v; want [agent:tariboy-pm]", got)
			}
		})
	}
}

func TestCommentMentionAtSentenceEndOpensWaitOnName(t *testing.T) {
	svc, task := mentionTask(t)
	seedAgent(t, svc, "tariboy-pm", "")
	result, err := svc.AddComment(context.Background(), CustomerActor("customer"), task.Key,
		AddCommentInput{Body: "For questions, mention @agent:tariboy-pm."})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitPrincipalsOf(result); len(got) != 1 || got[0] != "agent:tariboy-pm" {
		t.Fatalf("waits = %v; want [agent:tariboy-pm]", got)
	}
}

func TestCommentMentionKeepsInnerDot(t *testing.T) {
	svc, task := mentionTask(t)
	seedAgent(t, svc, "a.b", "")
	result, err := svc.AddComment(context.Background(), CustomerActor("customer"), task.Key,
		AddCommentInput{Body: "спроси @agent:a.b про это"})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitPrincipalsOf(result); len(got) != 1 || got[0] != "agent:a.b" {
		t.Fatalf("waits = %v; want [agent:a.b]", got)
	}
}

func TestCommentMentionOfMissingAgentCreatesNoWait(t *testing.T) {
	svc, task := mentionTask(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	before := assertTaskStatus(t, svc, task.Key, task.Status)
	result, err := svc.AddComment(ctx, customer, task.Key, AddCommentInput{Body: "ask @agent:ghost please"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CreatedWaits) != 0 {
		t.Fatalf("waits = %v; want none", waitPrincipalsOf(result))
	}
	after := assertTaskStatus(t, svc, task.Key, before.Task.Status)
	if len(after.Comments) != 1 || len(after.WaitingFor) != 0 {
		t.Fatalf("comments/waits = %d/%d; want 1/0", len(after.Comments), len(after.WaitingFor))
	}
	events, err := svc.ListEvents(ctx, customer, task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Kind != "task.comment_added" {
		t.Fatalf("last event = %s; want task.comment_added", last.Kind)
	}
	if mentions, _ := last.Payload["mentions"].([]any); len(mentions) != 0 {
		t.Fatalf("event mentions = %v; want none", mentions)
	}
}

func TestAskMissingPrincipalReturnsPrincipalNotFound(t *testing.T) {
	for _, principal := range []string{"agent:ghost", "ghost", "user:other"} {
		t.Run(principal, func(t *testing.T) {
			svc, task := mentionTask(t)
			ctx := context.Background()
			_, err := svc.AgentAction(ctx, AgentActor("asker"), "ask", map[string]any{
				"key": task.Key, "principal": principal, "body": "anyone?",
			})
			if ErrorCode(err) != "principal_not_found" {
				t.Fatalf("ask error = %v; want principal_not_found", err)
			}
			if status := ErrorStatus(err); status != 400 {
				t.Fatalf("ask status = %d; want 400", status)
			}
			detail, err := svc.GetTask(ctx, CustomerActor("customer"), task.Key)
			if err != nil {
				t.Fatal(err)
			}
			if len(detail.Comments) != 0 || len(detail.WaitingFor) != 0 {
				t.Fatalf("comments/waits = %d/%d; want 0/0", len(detail.Comments), len(detail.WaitingFor))
			}
		})
	}
}

// The asked principal is also the mention in the question's body: the
// question opens one wait on it, not two.
func TestAskExistingAgentOpensOneWait(t *testing.T) {
	svc, task := mentionTask(t)
	seedAgent(t, svc, "reviewer", "")
	ctx := context.Background()
	result, err := svc.AgentAction(ctx, AgentActor("asker"), "ask", map[string]any{
		"key": task.Key, "principal": "reviewer", "body": "and @agent:reviewer again?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitPrincipalsOf(result.(CommentResult)); len(got) != 1 || got[0] != "agent:reviewer" {
		t.Fatalf("waits = %v; want [agent:reviewer]", got)
	}
	detail, err := svc.GetTask(ctx, CustomerActor("customer"), task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.WaitingFor) != 1 {
		t.Fatalf("open waits = %d; want 1", len(detail.WaitingFor))
	}
}
