package commands

import (
	"errors"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestTaskActivityMergesUsageAndEventsPerTaskAndAgent(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	for _, name := range []string{"alice", "bob"} {
		if err := as.Create(agent.Agent{Name: name, ImageRef: "basic:latest"}); err != nil {
			t.Fatal(err)
		}
	}
	since := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	now := since.Add(-time.Hour)
	taskService := tasks.NewService(c.Store.DB, "customer", func() time.Time { return now })
	c.Tasks = taskService
	customer := tasks.CustomerActor("customer")
	if _, err := taskService.CreateQueue(t.Context(), customer, tasks.CreateQueueInput{Prefix: "ACT", Name: "Activity"}); err != nil {
		t.Fatal(err)
	}
	first, err := taskService.CreateTask(t.Context(), customer, tasks.CreateTaskInput{Queue: "ACT", Title: "First", Assignee: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := taskService.CreateTask(t.Context(), customer, tasks.CreateTaskInput{Queue: "ACT", Title: "Second", Assignee: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	now = since.Add(30 * time.Minute)
	if _, err := taskService.AddComment(t.Context(), tasks.AgentActor("alice"), first.Key, tasks.AddCommentInput{Body: "done part"}); err != nil {
		t.Fatal(err)
	}

	ap := aiproxy.NewStore(c.Store, nil)
	rec := func(id, agentName, iteration, task string, at time.Time, cost float64) {
		if err := ap.Insert(aiproxy.AIRequest{ID: id, TS: at.Format(time.RFC3339Nano), Agent: agentName,
			Iteration: iteration, Model: "m", InputTokens: 10, OutputTokens: 5, CostUSD: cost, Status: "ok", TaskID: task}); err != nil {
			t.Fatal(err)
		}
	}
	rec("r1", "alice", "it-1", first.Key, since.Add(time.Minute), 0.1)
	rec("r2", "alice", "it-1", first.Key, since.Add(2*time.Minute), 0.1)
	rec("r3", "bob", "it-2", second.Key, since.Add(45*time.Minute), 0.3)
	rec("r4", "alice", "it-3", "", since.Add(3*time.Minute), 5)
	rec("r5", "alice", "it-0", first.Key, since.Add(-time.Minute), 5)

	params := registry.Params{"since": since.Format(time.RFC3339), "until": since.Add(time.Hour).Format(time.RFC3339)}
	res, err := h(t, "tasks.activity")(c, params)
	if err != nil {
		t.Fatal(err)
	}
	report := res.(TaskActivityReport)
	if len(report.Tasks) != 2 || report.Tasks[0].Key != second.Key || report.Tasks[1].Key != first.Key {
		t.Fatalf("tasks by last activity = %#v", report.Tasks)
	}
	row := report.Tasks[1]
	if row.Title != "First" || row.Iterations != 1 || row.Requests != 2 || row.CostUSD != 0.2 || row.Comments != 1 ||
		len(row.Agents) != 1 || row.Agents[0] != "alice" || row.LastAt != "2026-07-06T09:30:00.000000000Z" {
		t.Fatalf("first row = %#v", row)
	}
	if report.Totals.Tasks != 2 || report.Totals.Iterations != 2 || report.Totals.Comments != 1 || report.Totals.CostUSD != 0.5 {
		t.Fatalf("totals = %#v", report.Totals)
	}
	if len(report.Agents) != 2 || report.Agents[0].Agent != "alice" || report.Agents[0].Tasks != 1 ||
		report.Agents[0].CostUSD != 0.2 || report.Agents[1].Agent != "bob" || report.Agents[1].Iterations != 1 {
		t.Fatalf("agents = %#v", report.Agents)
	}

	params["agent"] = "bob"
	res, err = h(t, "tasks.activity")(c, params)
	if err != nil {
		t.Fatal(err)
	}
	report = res.(TaskActivityReport)
	if len(report.Tasks) != 1 || report.Tasks[0].Key != second.Key || report.Agents != nil {
		t.Fatalf("bob report = %#v", report)
	}
}

func TestTaskActivityRejectsBadTime(t *testing.T) {
	t.Parallel()
	c, _, _ := ctxWithStore(t)
	c.Tasks = tasks.NewService(c.Store.DB, "customer", time.Now)
	_, err := h(t, "tasks.activity")(c, registry.Params{"since": "yesterday"})
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "bad_time" {
		t.Fatalf("err = %v", err)
	}
}
