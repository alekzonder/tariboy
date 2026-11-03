package tasks

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

var orphanStart = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

// orphanFixture is a task with one comment that old waits refer to.
type orphanFixture struct {
	st        *store.Store
	svc       *Service
	task      Task
	commentID int64
}

func newOrphanFixture(t *testing.T, customer string) orphanFixture {
	t.Helper()
	st := storetest.Open(t)
	svc := NewService(st.DB, customer, func() time.Time { return orphanStart.Add(-time.Hour) })
	ctx := context.Background()
	actor := CustomerActor(customer)
	if _, err := svc.CreateQueue(ctx, actor, CreateQueueInput{Prefix: "ORP", Name: "Orphans"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "ORP", Title: "orphans"})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := svc.AddComment(ctx, actor, task.Key, AddCommentInput{Body: "question"})
	if err != nil {
		t.Fatal(err)
	}
	return orphanFixture{st: st, svc: svc, task: task, commentID: comment.Comment.ID}
}

// insertWait opens a wait the way an old daemon stored it, without the
// existence check of current mentions.
func (f orphanFixture) insertWait(t *testing.T, principal string) int64 {
	t.Helper()
	result, err := f.svc.db.Exec(`
		INSERT INTO task_waiting_for(task_id, expected_principal, requesting_principal,
			requesting_comment_id, requested_at)
		VALUES (?, ?, 'agent:asker', ?, ?)`,
		f.task.ID, principal, f.commentID, store.FormatTime(orphanStart.Add(-time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return id
}

type waitRow struct {
	resolvedAt string
	resolving  sql.NullInt64
}

func (f orphanFixture) wait(t *testing.T, id int64) waitRow {
	t.Helper()
	var row waitRow
	if err := f.svc.db.QueryRow(`SELECT resolved_at, resolving_comment_id FROM task_waiting_for WHERE id = ?`, id).
		Scan(&row.resolvedAt, &row.resolving); err != nil {
		t.Fatal(err)
	}
	return row
}

func (f orphanFixture) run(t *testing.T) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	n, err := CloseOrphanWaits(context.Background(), f.svc.db, f.svc.customer, func() time.Time { return orphanStart }, log)
	if err != nil {
		t.Fatal(err)
	}
	return n, buf.String()
}

func closureLines(out string) []string {
	lines := []string{}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, `msg="closed wait on missing principal"`) {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestCloseOrphanWaitsClosesWaitsOnMissingAgents(t *testing.T) {
	f := newOrphanFixture(t, "customer")
	ids := []int64{
		f.insertWait(t, "agent:tariboy-pm."),
		f.insertWait(t, "agent:worker"),
		f.insertWait(t, "agent:a.b"),
	}
	m := mail.New(f.st, func() time.Time { return orphanStart }, questionDirectory{})
	letters, err := m.Send(mail.Letter{From: "agent:asker", To: "agent:worker", Kind: mail.KindTaskQuestion,
		Body: "?", NeedsReply: true, Data: map[string]any{"task_key": f.task.Key}})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := f.run(t)
	if n != 3 {
		t.Fatalf("closed = %d; want 3", n)
	}
	for _, id := range ids {
		row := f.wait(t, id)
		if row.resolvedAt != store.FormatTime(orphanStart) || row.resolving.Valid {
			t.Fatalf("wait %d = %+v; want resolved at start without a comment", id, row)
		}
	}
	got, _, err := m.Show("agent:worker", letters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Open() || got.CloseNote != "task-wait-cleanup:principal_not_found" {
		t.Fatalf("question letter = %+v; want closed by cleanup", got)
	}
}

func TestCloseOrphanWaitsKeepsExistingPrincipals(t *testing.T) {
	f := newOrphanFixture(t, "customer")
	seedAgent(t, f.svc, "worker", "")
	agentWait := f.insertWait(t, "agent:worker")
	userWait := f.insertWait(t, "user:customer")
	n, out := f.run(t)
	if n != 0 || len(closureLines(out)) != 0 {
		t.Fatalf("closed = %d, log = %q; want nothing closed", n, out)
	}
	for _, id := range []int64{agentWait, userWait} {
		if row := f.wait(t, id); row.resolvedAt != "" {
			t.Fatalf("wait %d = %+v; want open", id, row)
		}
	}
}

func TestCloseOrphanWaitsKeepsCustomerWithNonMentionLogin(t *testing.T) {
	for _, login := range []string{"john.doe@corp", "иван"} {
		f := newOrphanFixture(t, login)
		id := f.insertWait(t, "user:"+login)
		n, out := f.run(t)
		if n != 0 || len(closureLines(out)) != 0 || f.wait(t, id).resolvedAt != "" {
			t.Fatalf("login %q: closed = %d, log = %q; want the customer's wait open", login, n, out)
		}
	}
}

func TestCloseOrphanWaitsKeepsExistingAgentWithNonMentionName(t *testing.T) {
	f := newOrphanFixture(t, "customer")
	seedAgent(t, f.svc, "worker-", "")
	id := f.insertWait(t, "agent:worker-")
	if n, _ := f.run(t); n != 0 || f.wait(t, id).resolvedAt != "" {
		t.Fatalf("closed = %d; want the existing agent's wait open", n)
	}
}

func TestCloseOrphanWaitsLogsEachClosure(t *testing.T) {
	f := newOrphanFixture(t, "customer")
	dotted := f.insertWait(t, "agent:tariboy-pm.")
	missing := f.insertWait(t, "agent:worker")
	user := f.insertWait(t, "user:bob")
	_, out := f.run(t)
	t.Logf("log:\n%s", out)
	want := []string{
		"task=" + f.task.Key + " wait_id=" + itoa(dotted) + " principal=agent:tariboy-pm. reason=invalid_principal_name",
		"task=" + f.task.Key + " wait_id=" + itoa(missing) + " principal=agent:worker reason=principal_not_found",
		"task=" + f.task.Key + " wait_id=" + itoa(user) + " principal=user:bob reason=principal_not_found",
	}
	lines := closureLines(out)
	if len(lines) != len(want) {
		t.Fatalf("closure lines = %q; want %d", lines, len(want))
	}
	for i, line := range lines {
		if !strings.Contains(line, "level=INFO") || !strings.HasSuffix(line, want[i]) {
			t.Fatalf("line %d = %q; want INFO ending with %q", i, line, want[i])
		}
	}
	if !strings.Contains(out, `level=INFO msg="closed waits on missing principals" count=3`) {
		t.Fatalf("log = %q; want a summary with count=3", out)
	}
}

func TestCloseOrphanWaitsIsIdempotent(t *testing.T) {
	f := newOrphanFixture(t, "customer")
	id := f.insertWait(t, "agent:worker")
	if n, _ := f.run(t); n != 1 {
		t.Fatalf("first run closed = %d; want 1", n)
	}
	before := f.wait(t, id)
	var buf bytes.Buffer
	later := func() time.Time { return orphanStart.Add(time.Hour) }
	n, err := CloseOrphanWaits(context.Background(), f.svc.db, "customer", later, slog.New(slog.NewTextHandler(&buf, nil)))
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if n != 0 || len(closureLines(out)) != 0 || f.wait(t, id) != before {
		t.Fatalf("second run closed = %d, log = %q, wait = %+v; want no change", n, out, f.wait(t, id))
	}
	if !strings.Contains(out, "count=0") {
		t.Fatalf("log = %q; want a summary with count=0", out)
	}
}

func TestCloseOrphanWaitsKeepsUserWaitsWithoutCustomerLogin(t *testing.T) {
	f := newOrphanFixture(t, "customer")
	userWait := f.insertWait(t, "user:customer")
	otherWait := f.insertWait(t, "user:bob")
	agentWait := f.insertWait(t, "agent:worker")
	var buf bytes.Buffer
	n, err := CloseOrphanWaits(context.Background(), f.svc.db, " ", func() time.Time { return orphanStart },
		slog.New(slog.NewTextHandler(&buf, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || f.wait(t, agentWait).resolvedAt == "" {
		t.Fatalf("closed = %d; want only the missing agent's wait closed", n)
	}
	for _, id := range []int64{userWait, otherWait} {
		if row := f.wait(t, id); row.resolvedAt != "" {
			t.Fatalf("user wait %d = %+v; want open without a customer login", id, row)
		}
	}
}

func TestCloseOrphanWaitsKeepsTaskStatus(t *testing.T) {
	f := newOrphanFixture(t, "customer")
	ctx := context.Background()
	current, err := f.svc.GetTask(ctx, CustomerActor("customer"), f.task.Key)
	if err != nil {
		t.Fatal(err)
	}
	status := StatusWaitCustomer
	if _, err := f.svc.UpdateTask(ctx, CustomerActor("customer"), f.task.Key,
		UpdateTaskInput{Status: &status, Revision: current.Task.Revision}); err != nil {
		t.Fatal(err)
	}
	f.insertWait(t, "agent:tariboy-pm.")
	if n, _ := f.run(t); n != 1 {
		t.Fatalf("closed = %d; want 1", n)
	}
	after, err := f.svc.GetTask(ctx, CustomerActor("customer"), f.task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if after.Task.Status != StatusWaitCustomer {
		t.Fatalf("status = %q; want %q", after.Task.Status, StatusWaitCustomer)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
