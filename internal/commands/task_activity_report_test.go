package commands

import (
	"fmt"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestTaskActivityRequiresTasksService(t *testing.T) {
	t.Parallel()
	c, _, _ := ctxWithStore(t)
	if _, err := h(t, "tasks.activity")(c, registry.Params{}); !isCode(err, "tasks_unavailable") {
		t.Fatalf("err = %#v, want tasks_unavailable", err)
	}
}

func TestTaskActivityRejectsBadBounds(t *testing.T) {
	t.Parallel()
	c, _, _ := ctxWithStore(t)
	c.Tasks = tasks.NewService(c.Store.DB, "customer", time.Now)
	for _, key := range []string{"since", "until"} {
		if _, err := h(t, "tasks.activity")(c, registry.Params{key: "yesterday"}); !isCode(err, "bad_time") {
			t.Fatalf("%s err = %#v, want bad_time", key, err)
		}
	}
}

// Catches the per-agent share counting usage on a task the report does not
// show, and losing an agent that only has task events and no usage.
func TestTaskActivityAgentShares(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	for _, name := range []string{"alice", "carol"} {
		if err := as.Create(agent.Agent{Name: name, ImageRef: "basic:latest"}); err != nil {
			t.Fatal(err)
		}
	}
	since := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	now := since.Add(time.Minute)
	taskService := tasks.NewService(c.Store.DB, "customer", func() time.Time { return now })
	c.Tasks = taskService
	customer := tasks.CustomerActor("customer")
	if _, err := taskService.CreateQueue(t.Context(), customer, tasks.CreateQueueInput{Prefix: "ACT", Name: "Activity"}); err != nil {
		t.Fatal(err)
	}
	task, err := taskService.CreateTask(t.Context(), customer, tasks.CreateTaskInput{Queue: "ACT", Title: "Only events", Assignee: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taskService.AddComment(t.Context(), tasks.AgentActor("alice"), task.Key, tasks.AddCommentInput{Body: "note"}); err != nil {
		t.Fatal(err)
	}
	ap := aiproxy.NewStore(c.Store, nil)
	if err := ap.Insert(aiproxy.AIRequest{ID: "r1", TS: since.Add(2 * time.Minute).Format(time.RFC3339Nano), Agent: "carol",
		Iteration: "it-1", Model: "m", InputTokens: 10, CostUSD: 1, Status: "ok", TaskID: "ACT-404"}); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "tasks.activity")(c, registry.Params{"since": since.Format(time.RFC3339), "until": since.Add(time.Hour).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	report := res.(TaskActivityReport)
	if len(report.Tasks) != 1 || report.Tasks[0].Key != task.Key || report.Truncated {
		t.Fatalf("tasks = %#v", report)
	}
	if report.Totals.Iterations != 0 || report.Totals.CostUSD != 0 {
		t.Fatalf("totals = %#v", report.Totals)
	}
	if len(report.Agents) != 1 || report.Agents[0].Agent != "alice" || report.Agents[0].Tasks != 1 || report.Agents[0].Iterations != 0 {
		t.Fatalf("agents = %#v", report.Agents)
	}
}

// Catches the row limit cutting the totals, or a report that is cut without
// saying so. The newest tasks are the ones kept.
func TestTaskActivityTruncatesRowsButNotTotals(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "alice", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	now := since
	taskService := tasks.NewService(c.Store.DB, "customer", func() time.Time { return now })
	c.Tasks = taskService
	customer := tasks.CustomerActor("customer")
	if _, err := taskService.CreateQueue(t.Context(), customer, tasks.CreateQueueInput{Prefix: "ACT", Name: "Activity"}); err != nil {
		t.Fatal(err)
	}
	newest := ""
	for i := range maxActivityTasks + 2 {
		now = since.Add(time.Duration(i) * time.Second)
		task, err := taskService.CreateTask(t.Context(), customer, tasks.CreateTaskInput{Queue: "ACT", Title: fmt.Sprintf("Task %d", i), Assignee: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := taskService.AddComment(t.Context(), tasks.AgentActor("alice"), task.Key, tasks.AddCommentInput{Body: "note"}); err != nil {
			t.Fatal(err)
		}
		newest = task.Key
	}
	res, err := h(t, "tasks.activity")(c, registry.Params{"since": since.Format(time.RFC3339), "until": since.Add(time.Hour).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	report := res.(TaskActivityReport)
	if len(report.Tasks) != maxActivityTasks || !report.Truncated || report.Tasks[0].Key != newest {
		t.Fatalf("rows = %d truncated = %v first = %s", len(report.Tasks), report.Truncated, report.Tasks[0].Key)
	}
	if report.Totals.Tasks != maxActivityTasks+2 || report.Totals.Comments != maxActivityTasks+2 {
		t.Fatalf("totals = %#v", report.Totals)
	}
	if len(report.Agents) != 1 || report.Agents[0].Tasks != maxActivityTasks+2 {
		t.Fatalf("agents = %#v", report.Agents)
	}
}
