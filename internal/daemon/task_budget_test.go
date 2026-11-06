package daemon

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestTaskBudgetHooksRefuseAndBlockTheTask(t *testing.T) {
	t.Parallel()
	st := storetest.Open(t)
	svc := tasks.NewService(st.DB, "customer", time.Now)
	ctx := context.Background()
	if err := svc.EnsureDefaultQueue(ctx); err != nil {
		t.Fatal(err)
	}
	customer := tasks.CustomerActor("customer")
	task, err := svc.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "TASK", Title: "limited"})
	if err != nil {
		t.Fatal(err)
	}
	limit := 1.0
	task, err = svc.UpdateTask(ctx, customer, task.Key, tasks.UpdateTaskInput{BudgetUSD: &limit, Revision: task.Revision})
	if err != nil {
		t.Fatal(err)
	}
	hooks := newTaskBudgetHooks(svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, over, err := hooks.check(task.Key); err != nil || over {
		t.Fatalf("under the limit: over=%v err=%v", over, err)
	}
	if _, err := st.DB.Exec(`INSERT INTO ai_requests(id, ts, agent, cost_usd, task_id) VALUES ('r1', '2026-01-01T00:00:00Z', 'dev', 1.5, ?)`, task.Key); err != nil {
		t.Fatal(err)
	}
	limitKey, over, err := hooks.check(task.Key)
	if err != nil || !over || limitKey != task.Key {
		t.Fatalf("over the limit: %q over=%v err=%v", limitKey, over, err)
	}
	hooks.exhausted(task.Key)
	hooks.exhausted(task.Key)
	hooks.wg.Wait()
	got, err := svc.GetTask(ctx, customer, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Task.ManualBlockReason, "budget exhausted: task "+task.Key) || !got.Task.Blocked {
		t.Fatalf("task = %#v", got.Task)
	}
}
