package tasks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// questionDirectory accepts the agents and the customer these tests use.
type questionDirectory struct{}

func (questionDirectory) AgentExists(string) (bool, error)     { return true, nil }
func (questionDirectory) InboxCap(string) (int, error)         { return 0, nil }
func (questionDirectory) CustomerLogin() string                { return "customer" }
func (questionDirectory) PluginInstalled(string) (bool, error) { return false, nil }
func (d questionDirectory) PluginReceivesMail(name string) (bool, error) {
	return d.PluginInstalled(name)
}
func (questionDirectory) QueueExists(string) (bool, error)            { return false, nil }
func (questionDirectory) GroupMembers(string) ([]string, bool, error) { return nil, false, nil }

// TestWaitResolutionClosesQuestionLetters: the answer on the task closes the
// question letter the customer already got, and a question notification the
// outbox has not sent yet is never sent.
func TestWaitResolutionClosesQuestionLetters(t *testing.T) {
	st := storetest.Open(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	svc := NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Questions"}); err != nil {
		t.Fatal(err)
	}
	sent, _ := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "ASK", Title: "sent", Assignee: "requester"})
	pending, _ := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "ASK", Title: "pending", Assignee: "requester"})
	for _, task := range []Task{sent, pending} {
		if _, err := svc.AddComment(ctx, AgentActor("requester"), task.Key, AddCommentInput{Body: "Need input from @user:customer"}); err != nil {
			t.Fatal(err)
		}
	}
	// The outbox already sent the first question: a letter, and its row is
	// published.
	m := mail.New(st, func() time.Time { return now }, questionDirectory{})
	letters, err := m.Send(mail.Letter{From: "agent:requester", To: "user:customer", Kind: mail.KindTaskQuestion,
		Body: "?", NeedsReply: true, Data: map[string]any{"task_key": sent.Key}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`UPDATE task_notification_outbox SET published_at = ?
		WHERE message_type = 'task.question' AND json_extract(data, '$.task_key') = ?`, "2026-07-31T12:00:00Z", sent.Key); err != nil {
		t.Fatal(err)
	}
	answer, err := svc.AddComment(ctx, customer, sent.Key, AddCommentInput{Body: "Approved"})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := m.Show("user:customer", letters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Open() || got.CloseNote != fmt.Sprintf("task-comment:%d", answer.Comment.ID) {
		t.Fatalf("question letter after the answer = %+v", got)
	}
	if _, err := svc.AddComment(ctx, customer, pending.Key, AddCommentInput{Body: "Approved"}); err != nil {
		t.Fatal(err)
	}
	var published, lastError string
	if err := st.DB.QueryRow(`SELECT published_at, last_error FROM task_notification_outbox
		WHERE message_type = 'task.question' AND json_extract(data, '$.task_key') = ?`, pending.Key).Scan(&published, &lastError); err != nil {
		t.Fatal(err)
	}
	if published == "" || lastError != "answered before delivery" {
		t.Fatalf("unsent question row = published %q, last_error %q; want it marked sent without a letter", published, lastError)
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
	detail := taskDetail(t, svc, task)
	comments, waits, revision := len(detail.Comments), len(detail.WaitingFor), detail.Task.Revision
	if comments != 1 || waits != 1 || revision != 2 {
		t.Fatalf("comments/waits/revision = %d/%d/%d, want 1/1/2", comments, waits, revision)
	}
}

func TestAssignedAgentCustomerQuestionTransitionsToWaitCustomer(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusInProgress)
	ctx := context.Background()
	result, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{
		Body: "@user:customer choose one", IdempotencyKey: "ask-1",
	})
	if err != nil || len(result.CreatedWaits) != 1 {
		t.Fatalf("add comment = %#v, %v", result, err)
	}
	detail := assertTaskStatus(t, svc, task.Key, StatusWaitCustomer)
	if detail.Task.Revision != task.Revision+1 {
		t.Fatalf("revision = %d; want %d", detail.Task.Revision, task.Revision+1)
	}
	events, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	updated, commented := events[len(events)-2], events[len(events)-1]
	if updated.Kind != "task.updated" || updated.TaskRevision != detail.Task.Revision ||
		updated.Payload["status"] != StatusWaitCustomer {
		t.Fatalf("updated event = %#v", updated)
	}
	if commented.Kind != "task.comment_added" || commented.TaskRevision != detail.Task.Revision ||
		updated.Sequence >= commented.Sequence {
		t.Fatalf("comment event = %#v after %#v", commented, updated)
	}
	// A storage check: no API lists the notification outbox.
	var outboxSequence int64
	if err := svc.db.QueryRow(`
		SELECT event_sequence FROM task_notification_outbox
		WHERE message_type = 'task.question'`).Scan(&outboxSequence); err != nil {
		t.Fatal(err)
	}
	if outboxSequence != commented.Sequence {
		t.Fatalf("question outbox sequence = %d; want comment event %d", outboxSequence, commented.Sequence)
	}

	if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{
		Body: "different retry body", IdempotencyKey: "ask-1",
	}); err != nil {
		t.Fatal(err)
	}
	replayed := assertTaskStatus(t, svc, task.Key, StatusWaitCustomer)
	replayedEvents, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Task.Revision != detail.Task.Revision || len(replayedEvents) != len(events) {
		t.Fatalf("replay revision/events = %d/%d; want %d/%d",
			replayed.Task.Revision, len(replayedEvents), detail.Task.Revision, len(events))
	}
}

func TestFinalCustomerAnswerReturnsWaitCustomerToInProgress(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusInProgress)
	ctx := context.Background()
	if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{
		Body: "@user:customer first", IdempotencyKey: "ask-1",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.AddComment(ctx, CustomerActor("customer"), task.Key, AddCommentInput{
		Body: "answered", IdempotencyKey: "answer-1",
	})
	if err != nil || len(result.ResolvedWaits) != 1 {
		t.Fatalf("answer = %#v, %v", result, err)
	}
	detail := assertTaskStatus(t, svc, task.Key, StatusInProgress)
	if detail.Task.Revision != task.Revision+2 {
		t.Fatalf("revision = %d; want %d", detail.Task.Revision, task.Revision+2)
	}
	events, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	updated, commented := events[len(events)-2], events[len(events)-1]
	if updated.Kind != "task.updated" || updated.Payload["status"] != StatusInProgress ||
		commented.Kind != "task.comment_added" || updated.TaskRevision != detail.Task.Revision ||
		commented.TaskRevision != detail.Task.Revision || updated.Sequence >= commented.Sequence {
		t.Fatalf("answer event tail = %#v, %#v", updated, commented)
	}
}

func TestCustomerAnswerTransitionsWithOtherWaitRemaining(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusInProgress)
	ctx := context.Background()
	if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{
		Body: "@user:customer decide and @agent:reviewer review",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, CustomerActor("customer"), task.Key, AddCommentInput{
		Body: "customer answer",
	}); err != nil {
		t.Fatal(err)
	}
	detail := assertTaskStatus(t, svc, task.Key, StatusInProgress)
	if len(detail.WaitingFor) != 1 || detail.WaitingFor[0].ExpectedPrincipal != "agent:reviewer" {
		t.Fatalf("remaining waits = %#v; want agent:reviewer", detail.WaitingFor)
	}
}

func TestUnrelatedCommentDoesNotResumeManualWaitCustomer(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusWaitCustomer)
	ctx := context.Background()
	before, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{
		Body: "work continues elsewhere",
	}); err != nil {
		t.Fatal(err)
	}
	assertTaskStatus(t, svc, task.Key, StatusWaitCustomer)
	after, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 || after[len(after)-1].Kind != "task.comment_added" {
		t.Fatalf("event tail after unrelated comment = %#v", after[len(before):])
	}
}

func TestCustomerMentionOfAssigneeResumesWaitCustomer(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusWaitCustomer)
	ctx := context.Background()
	before, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, CustomerActor("customer"), task.Key, AddCommentInput{
		Body: "@agent:worker continue",
	}); err != nil {
		t.Fatal(err)
	}
	assertTaskStatus(t, svc, task.Key, StatusInProgress)
	after, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+2 || after[len(after)-2].Kind != "task.updated" || after[len(after)-1].Kind != "task.comment_added" {
		t.Fatalf("event tail after summon = %#v", after[len(before):])
	}
}

func TestCustomerMentionOfHolderKeepsWorkflowCustomerStatus(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	if _, err := svc.MoveWorkflow(ctx, actor, task.Key, "approval", "decide now"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, actor, task.Key, AddCommentInput{Body: "@agent:dev-1 continue"}); err != nil {
		t.Fatal(err)
	}
	got, _ := taskByKey(ctx, svc.db, task.Key)
	if got.WorkflowStatus != "approval" || got.Category != StatusWaitCustomer || got.WaitingOn != WaitingOnCustomer {
		t.Fatalf("after the summon = %#v", got)
	}
}

func TestCustomerWaitStatusNonTransitions(t *testing.T) {
	tests := []struct {
		name                string
		actor               Actor
		mention             string
		start               string
		grantResponseAccess bool
		manualBeforeAnswer  bool
		want                string
	}{
		{name: "other agent question", actor: AgentActor("other"), mention: "@user:customer", start: StatusInProgress, grantResponseAccess: true, want: StatusInProgress},
		{name: "wait for another principal", actor: AgentActor("worker"), mention: "@agent:reviewer", start: StatusInProgress, want: StatusInProgress},
		{name: "done task", actor: AgentActor("worker"), mention: "@user:customer", start: StatusDone, want: StatusDone},
		{name: "cancelled task", actor: AgentActor("worker"), mention: "@user:customer", start: StatusCancelled, want: StatusCancelled},
		{name: "customer mentions another agent", actor: CustomerActor("customer"), mention: "@agent:reviewer", start: StatusWaitCustomer, want: StatusWaitCustomer},
		{name: "manual status before answer", actor: CustomerActor("customer"), start: StatusInProgress, manualBeforeAnswer: true, want: StatusOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, task := assignedTask(t, "worker", tt.start)
			ctx := context.Background()
			if tt.grantResponseAccess {
				if _, err := svc.AddComment(ctx, CustomerActor("customer"), task.Key, AddCommentInput{
					Body: "@agent:other please respond",
				}); err != nil {
					t.Fatal(err)
				}
			}
			if tt.manualBeforeAnswer {
				if _, err := svc.AddComment(ctx, AgentActor("worker"), task.Key, AddCommentInput{
					Body: "@user:customer question",
				}); err != nil {
					t.Fatal(err)
				}
				waiting := assertTaskStatus(t, svc, task.Key, StatusWaitCustomer)
				status := StatusOpen
				if _, err := svc.UpdateTask(ctx, CustomerActor("customer"), task.Key, UpdateTaskInput{
					Status: &status, Revision: waiting.Task.Revision,
				}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
			if err != nil {
				t.Fatal(err)
			}
			body := tt.mention
			if tt.manualBeforeAnswer {
				body = "answer after manual status"
			}
			if _, err := svc.AddComment(ctx, tt.actor, task.Key, AddCommentInput{Body: body}); err != nil {
				t.Fatal(err)
			}
			assertTaskStatus(t, svc, task.Key, tt.want)
			after, err := svc.ListEvents(ctx, CustomerActor("customer"), task.Key, 0, 20)
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != len(before)+1 || after[len(after)-1].Kind != "task.comment_added" {
				t.Fatalf("event tail after non-transition = %#v", after[len(before):])
			}
		})
	}
}

func assignedTask(t *testing.T, assignee, status string) (*Service, Task) {
	t.Helper()
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Questions"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, customer, CreateTaskInput{
		Queue: "ASK", Title: "decision", Assignee: assignee,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusOpen {
		updated, err := svc.UpdateTask(ctx, customer, task.Key, UpdateTaskInput{
			Status: &status, Revision: task.Revision,
		})
		if err != nil {
			t.Fatal(err)
		}
		task = updated
	}
	return svc, task
}

func assertTaskStatus(t *testing.T, svc *Service, key, want string) TaskDetail {
	t.Helper()
	detail, err := svc.GetTask(context.Background(), CustomerActor("customer"), key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != want {
		t.Fatalf("status = %q; want %q", detail.Task.Status, want)
	}
	return detail
}

// TestQuestionLetterClosesWhateverSender: a question the outbox sent as
// system:tasks, because its asker could no longer send, still closes when
// the question is answered on the task. The asker is in data.from.
func TestQuestionLetterClosesWhateverSender(t *testing.T) {
	st := storetest.Open(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	svc := NewService(st.DB, "customer", func() time.Time { return now })
	ctx := context.Background()
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "ASK", Name: "Questions"}); err != nil {
		t.Fatal(err)
	}
	task, _ := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "ASK", Title: "orphan", Assignee: "requester"})
	if _, err := svc.AddComment(ctx, AgentActor("requester"), task.Key, AddCommentInput{Body: "Need input from @user:customer"}); err != nil {
		t.Fatal(err)
	}
	m := mail.New(st, func() time.Time { return now }, questionDirectory{})
	letters, err := m.Send(mail.Letter{From: "system:tasks", To: "user:customer", Kind: mail.KindTaskQuestion,
		Body: "?", NeedsReply: true, Data: map[string]any{"task_key": task.Key, "from": "agent:requester"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`UPDATE task_notification_outbox SET published_at = ?
		WHERE message_type = 'task.question'`, "2026-07-31T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	// An unrelated question to the same recipient from another asker stays open.
	other, err := m.Send(mail.Letter{From: "system:tasks", To: "user:customer", Kind: mail.KindTaskQuestion,
		Body: "?", NeedsReply: true, Data: map[string]any{"task_key": task.Key, "from": "agent:someone"}})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := svc.AddComment(ctx, customer, task.Key, AddCommentInput{Body: "Approved"})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := m.Show("user:customer", letters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Open() || got.CloseNote != fmt.Sprintf("task-comment:%d", answer.Comment.ID) {
		t.Fatalf("system:tasks question letter after the answer = %+v", got)
	}
	if got, _, _ := m.Show("user:customer", other[0].ID); !got.Open() {
		t.Fatalf("question of another asker closed: %+v", got)
	}
}
