package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// The customer login must be the same fixed value on every server instead of
// whatever $USER the daemon happens to run as, and adopting it must carry the
// existing tasks and the customer's mail over to the new principal rather
// than orphaning them.
func TestResolveCustomerLoginFixesLoginAndReconcilesExistingState(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	st := storetest.OpenAt(t, filepath.Join(base, "tariboyd.db"))
	service := tasks.NewService(st.DB, "olduser", time.Now)
	actor := tasks.CustomerActor("olduser")
	ctx := context.Background()
	if _, err := service.CreateQueue(ctx, actor, tasks.CreateQueueInput{Prefix: "OLD", Name: "Old"}); err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(ctx, actor, tasks.CreateTaskInput{Queue: "OLD", Title: "carried over"})
	if err != nil {
		t.Fatal(err)
	}

	login, err := resolveCustomerLogin(st)
	if err != nil {
		t.Fatal(err)
	}
	if login != tasks.DefaultCustomerLogin {
		t.Fatalf("login = %q, want %q", login, tasks.DefaultCustomerLogin)
	}

	var customer string
	if err := st.DB.QueryRow(`SELECT customer FROM tasks WHERE task_key = ?`, task.Key).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	if customer != "user:"+tasks.DefaultCustomerLogin {
		t.Fatalf("task customer = %q, want %q", customer, "user:"+tasks.DefaultCustomerLogin)
	}

	// The persisted value is authoritative afterwards: a second resolve neither
	// changes the login nor repeats the reconciliation.
	again, err := resolveCustomerLogin(st)
	if err != nil || again != tasks.DefaultCustomerLogin {
		t.Fatalf("second resolve = %q, %v", again, err)
	}
	if err := st.ConfigSet(customerLoginKey, "shared"); err != nil {
		t.Fatal(err)
	}
	configured, err := resolveCustomerLogin(st)
	if err != nil || configured != "shared" {
		t.Fatalf("configured resolve = %q, %v", configured, err)
	}
}

// TestAdoptCustomerRewritesMail: adopting the customer login rewrites the
// letters to and from the old principal and the notifications not sent yet,
// so the customer's mailbox and unread counts follow the new principal.
func TestAdoptCustomerRewritesMail(t *testing.T) {
	t.Parallel()
	st := storetest.OpenAt(t, filepath.Join(t.TempDir(), "tariboyd.db"))
	service := tasks.NewService(st.DB, "olduser", time.Now)
	ctx := context.Background()
	if _, err := service.CreateQueue(ctx, tasks.CustomerActor("olduser"), tasks.CreateQueueInput{Prefix: "OLD", Name: "Old"}); err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(ctx, tasks.CustomerActor("olduser"), tasks.CreateTaskInput{Queue: "OLD", Title: "t", Assignee: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	// An unsent question notification to the old customer.
	if _, err := service.AddComment(ctx, tasks.AgentActor("worker"), task.Key, tasks.AddCommentInput{Body: "@user:olduser ?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO mail(id, sender, recipient, created_at) VALUES
		('m-in', 'agent:worker', 'user:olduser', 'x'), ('m-out', 'user:olduser', 'agent:worker', 'x'),
		('m-other', 'agent:worker', 'agent:other', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCustomerLogin(st); err != nil {
		t.Fatal(err)
	}
	want := "user:" + tasks.DefaultCustomerLogin
	for id, query := range map[string]string{
		"m-in":  `SELECT recipient FROM mail WHERE id = 'm-in'`,
		"m-out": `SELECT sender FROM mail WHERE id = 'm-out'`,
		"unsent question": `SELECT channel FROM task_notification_outbox
			WHERE message_type = 'task.question' AND published_at = ''`,
	} {
		var got string
		if err := st.DB.QueryRow(query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got != want {
			t.Errorf("%s names %q; want %q", id, got, want)
		}
	}
	var other string
	if err := st.DB.QueryRow(`SELECT recipient FROM mail WHERE id = 'm-other'`).Scan(&other); err != nil || other != "agent:other" {
		t.Fatalf("an unrelated letter changed: %q, %v", other, err)
	}
}
