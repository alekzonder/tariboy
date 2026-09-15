package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// The customer login must be the same fixed value on every server instead of
// whatever $USER the daemon happens to run as, and adopting it must carry the
// existing tasks, notification state and customer channel over to the new
// principal rather than orphaning them.
func TestResolveCustomerLoginFixesLoginAndReconcilesExistingState(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
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
	channelBus := bus.New(st, time.Now)
	published, err := channelBus.Publish(bus.Message{
		Channel: "user:olduser", Source: "agent:worker", Type: "message", Text: "old history",
	})
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
	var channel string
	if err := st.DB.QueryRow(`SELECT channel FROM messages WHERE id = ?`, published.ID).Scan(&channel); err != nil {
		t.Fatal(err)
	}
	if channel != "user:"+tasks.DefaultCustomerLogin {
		t.Fatalf("message channel = %q, want %q", channel, "user:"+tasks.DefaultCustomerLogin)
	}
	var channels int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM channels WHERE name = ?`, "user:olduser").Scan(&channels); err != nil {
		t.Fatal(err)
	}
	if channels != 0 {
		t.Fatalf("stale channel rows = %d, want 0", channels)
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
