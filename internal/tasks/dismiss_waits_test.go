package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// TestDismissWaitsClosesQuestionsOfADoneTask: the customer removes an
// agent's question from a closed task without an answer; the task stays done,
// the question letter closes, and the asker learns no answer is needed.
func TestDismissWaitsClosesQuestionsOfADoneTask(t *testing.T) {
	t.Parallel()
	st := storetest.Open(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	svc := NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := CustomerActor("customer")
	seedAgent(t, svc, "worker", "")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Questions"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "ASK", Title: "closed", Assignee: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{Body: "@user:customer one more thing?"}); err != nil {
		t.Fatal(err)
	}
	m := mail.New(st, func() time.Time { return now }, questionDirectory{})
	letters, err := m.Send(mail.Letter{From: "agent:worker", To: "user:customer", Kind: mail.KindTaskQuestion,
		Body: "?", NeedsReply: true, Data: map[string]any{"task_key": task.Key}})
	if err != nil {
		t.Fatal(err)
	}
	detail := assertTaskStatus(t, svc, task.Key, StatusWaitCustomer)
	if _, err := svc.CompleteTask(ctx, customer, task.Key, CompleteInput{Revision: detail.Task.Revision}); err != nil {
		t.Fatal(err)
	}
	if detail := assertTaskStatus(t, svc, task.Key, StatusDone); len(detail.WaitingFor) != 1 {
		t.Fatalf("waits of the done task = %#v; want the open question", detail.WaitingFor)
	}

	if _, err := svc.DismissWaits(ctx, AgentActor("worker"), task.Key); ErrorCode(err) == "" {
		t.Fatalf("agent dismiss error = %v; want a refusal", err)
	}
	result, err := svc.DismissWaits(ctx, customer, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ResolvedWaits) != 1 || result.ResolvedWaits[0].ResolvedAt == "" || result.Task.Status != StatusDone {
		t.Fatalf("dismiss result = %#v", result)
	}
	if detail := assertTaskStatus(t, svc, task.Key, StatusDone); len(detail.WaitingFor) != 0 {
		t.Fatalf("waits after dismiss = %#v; want none", detail.WaitingFor)
	}
	got, _, err := m.Show("user:customer", letters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Open() {
		t.Fatalf("question letter after dismiss = %+v; want closed", got)
	}
	if n := countEvents(t, svc, task, "task.waits_dismissed"); n != 1 {
		t.Fatalf("task.waits_dismissed events = %d; want 1", n)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_notification_outbox
		WHERE message_type = 'task.answered' AND channel = 'agent:worker'`); n != 1 {
		t.Fatalf("task.answered notifications to the asker = %d; want 1", n)
	}

	again, err := svc.DismissWaits(ctx, customer, task.Key)
	if err != nil || len(again.ResolvedWaits) != 0 {
		t.Fatalf("second dismiss = %#v, %v; want nothing to do", again, err)
	}
	if n := countEvents(t, svc, task, "task.waits_dismissed"); n != 1 {
		t.Fatalf("task.waits_dismissed events after a repeat = %d; want 1", n)
	}
}

// TestDismissWaitsReturnsWaitCustomerToInProgress: dismissing the only
// question hands an open task back to its assignee, like an answer does.
func TestDismissWaitsReturnsWaitCustomerToInProgress(t *testing.T) {
	t.Parallel()
	svc, task := assignedTask(t, "worker", StatusInProgress)
	ctx := context.Background()
	if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{Body: "@user:customer which one?"}); err != nil {
		t.Fatal(err)
	}
	assertTaskStatus(t, svc, task.Key, StatusWaitCustomer)
	result, err := svc.DismissWaits(ctx, CustomerActor("customer"), task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.Status != StatusInProgress {
		t.Fatalf("result status = %q; want in_progress", result.Task.Status)
	}
	assertTaskStatus(t, svc, task.Key, StatusInProgress)
}

// TestDismissWaitsKeepsWorkflowAndAgentWaits: a workflow's own question and a
// question to another agent stay open.
func TestDismissWaitsKeepsWorkflowAndAgentWaits(t *testing.T) {
	t.Parallel()
	svc, task := assignedTask(t, "worker", StatusInProgress)
	ctx := context.Background()
	if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{
		Body: "@user:customer decide and @agent:reviewer review",
	}); err != nil {
		t.Fatal(err)
	}
	// A storage setup: the workflow's question on another task, which the
	// customer answers only with an outcome.
	other, err := svc.CreateTask(ctx, CustomerActor("customer"), CreateTaskInput{Queue: "ASK", Title: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	note, err := svc.AddComment(ctx, CustomerActor("customer"), other.Key, AddCommentInput{Body: "status note"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO task_waiting_for(task_id, expected_principal, requesting_principal,
		requesting_comment_id, requested_at) VALUES (?, 'user:customer', ?, ?, '2026-07-31T12:00:00Z')`,
		other.ID, workflowActor, note.Comment.ID); err != nil {
		t.Fatal(err)
	}
	result, err := svc.DismissWaits(ctx, CustomerActor("customer"), task.Key)
	if err != nil || len(result.ResolvedWaits) != 1 || result.ResolvedWaits[0].ExpectedPrincipal != "user:customer" {
		t.Fatalf("dismiss = %#v, %v; want only the customer's question", result, err)
	}
	detail := assertTaskStatus(t, svc, task.Key, StatusInProgress)
	if len(detail.WaitingFor) != 1 || detail.WaitingFor[0].ExpectedPrincipal != "agent:reviewer" {
		t.Fatalf("waits left = %#v; want the reviewer's", detail.WaitingFor)
	}
	flow, err := svc.DismissWaits(ctx, CustomerActor("customer"), other.Key)
	if err != nil || len(flow.ResolvedWaits) != 0 {
		t.Fatalf("dismiss of a workflow question = %#v, %v; want nothing", flow, err)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_waiting_for WHERE task_id = ? AND resolved_at = ''`, other.ID); n != 1 {
		t.Fatalf("open workflow waits = %d; want 1", n)
	}
}
