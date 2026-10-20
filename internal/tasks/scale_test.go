package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// seedScaleQueue returns a service whose queue BIG holds n open root tasks,
// inserted in one statement so a test reaches a task count no per-task API
// call could build quickly. The agent runner is the queue's responsible agent.
func seedScaleQueue(tb testing.TB, n int) *Service {
	tb.Helper()
	svc := NewService(storetest.Open(tb).DB, "customer", func() time.Time {
		return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	})
	if _, err := svc.CreateQueue(context.Background(), CustomerActor("customer"), CreateQueueInput{
		Prefix: "BIG", Name: "Big", ResponsibleAgent: "runner",
	}); err != nil {
		tb.Fatal(err)
	}
	if _, err := svc.db.Exec(`
		WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < ?)
		INSERT INTO tasks(task_key, queue_prefix, position, priority, title, description, status,
			author, customer, created_at, updated_at)
		SELECT 'BIG-b' || i, 'BIG', i, 'P2', 'bulk ' || i, '', 'open',
			'user:customer', 'user:customer', '2026-07-31T12:00:00Z', '2026-07-31T12:00:00Z'
		FROM seq`, n); err != nil {
		tb.Fatal(err)
	}
	return svc
}

// The visible set is joined in SQL, so no query binds a variable per task and
// the SQLite variable limit (32766) does not cap the number of tasks.
func TestListTasksScalesPast32kTasks(t *testing.T) {
	svc := seedScaleQueue(t, 40000)
	ctx := context.Background()
	for _, actor := range []Actor{CustomerActor("customer"), AgentActor("runner")} {
		page, err := svc.ListTasks(ctx, actor, ListFilter{Limit: 10})
		if err != nil {
			t.Fatalf("%s: %v", actor.Principal, err)
		}
		if len(page.Tasks) != 10 || page.NextCursor == "" || page.Tasks[0].Access != "write" {
			t.Fatalf("%s: got %d tasks, cursor %q, access %q", actor.Principal, len(page.Tasks), page.NextCursor, page.Tasks[0].Access)
		}
	}
	ready, err := svc.Ready(ctx, AgentActor("runner"), ReadyFilter{Limit: 5})
	if err != nil || len(ready) != 5 {
		t.Fatalf("ready: %d tasks, %v", len(ready), err)
	}
	detail, err := svc.GetTask(ctx, AgentActor("runner"), "BIG-b1")
	if err != nil || detail.Task.Access != "write" {
		t.Fatalf("get: %q, %v", detail.Task.Access, err)
	}
}

func BenchmarkListTasks50k(b *testing.B) {
	svc := seedScaleQueue(b, 50000)
	ctx := context.Background()
	for _, actor := range []Actor{CustomerActor("customer"), AgentActor("runner")} {
		b.Run(actor.Principal, func(b *testing.B) {
			for b.Loop() {
				if _, err := svc.ListTasks(ctx, actor, ListFilter{Limit: 200}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
