package commands

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestAgentUsageGroupingAndTotals(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "alice", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	taskService := tasks.NewService(c.Store.DB, "customer", time.Now)
	c.Tasks = taskService
	queue, err := taskService.CreateQueue(t.Context(), tasks.CustomerActor("customer"), tasks.CreateQueueInput{
		Prefix: "USE", Name: "Usage",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := taskService.CreateTask(t.Context(), tasks.CustomerActor("customer"), tasks.CreateTaskInput{
		Queue: queue.Prefix, Title: "Native epic title", Assignee: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := taskService.CreateTask(t.Context(), tasks.CustomerActor("customer"), tasks.CreateTaskInput{
		ParentKey: root.Key, Title: "Native task title", Assignee: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := taskService.CreateTask(t.Context(), tasks.CustomerActor("customer"), tasks.CreateTaskInput{
		Queue: queue.Prefix, Title: "Hidden task title",
	})
	if err != nil {
		t.Fatal(err)
	}
	ap := aiproxy.NewStore(c.Store, nil)
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	rec := func(id, task, epic string, cost float64) {
		if err := ap.Insert(aiproxy.AIRequest{ID: id, TS: now.Format(time.RFC3339Nano),
			Agent: "alice", Iteration: "it-1", Model: "claude-opus-4-8",
			InputTokens: 100, OutputTokens: 50, CostUSD: cost, Status: "ok",
			TaskID: task, EpicID: epic}); err != nil {
			t.Fatal(err)
		}
	}
	rec("r1", task.Key, root.Key, 0.10)
	rec("r2", task.Key, root.Key, 0.20)
	rec("r3", "", "", 0.05) // untagged
	rec("r4", "MISSING-1", "HIDDEN-1", 0.01)
	rec("r5", hidden.Key, hidden.Key, 0.02)

	res, err := h(t, "agent.usage")(c, registry.Params{"name": "alice", "group_by": "task"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(AgentUsage)

	// totals fold every row regardless of grouping.
	tot := m.Totals
	if tot.Requests != 5 || tot.CostUSD != 0.38 {
		t.Fatalf("totals = %+v", tot)
	}
	if tot.InputTokens != 500 || tot.OutputTokens != 250 {
		t.Fatalf("totals tokens = %+v", tot)
	}

	byKey := agentUsageRowsByKey(m.Rows)
	if got := byKey[task.Key]; got == nil || got.Requests != 2 || got.Title != "Native task title" {
		t.Fatalf("task row = %+v", got)
	}
	// NULL task_id surfaces as the 'untagged' key + title.
	if u := byKey["untagged"]; u == nil || u.Requests != 1 || u.Title != "untagged" {
		t.Fatalf("untagged row = %+v", u)
	}
	if missing := byKey["MISSING-1"]; missing == nil || missing.Title != "MISSING-1" {
		t.Fatalf("unknown task row = %+v", missing)
	}
	if inaccessible := byKey[hidden.Key]; inaccessible == nil || inaccessible.Title != hidden.Key {
		t.Fatalf("inaccessible task row = %+v", inaccessible)
	}

	res, err = h(t, "agent.usage")(c, registry.Params{"name": "alice", "group_by": "epic"})
	if err != nil {
		t.Fatal(err)
	}
	byKey = agentUsageRowsByKey(res.(AgentUsage).Rows)
	if got := byKey[root.Key]; got == nil || got.Title != "Native epic title" {
		t.Fatalf("epic row = %+v", got)
	}
	if hidden := byKey["HIDDEN-1"]; hidden == nil || hidden.Title != "HIDDEN-1" {
		t.Fatalf("unknown epic row = %+v", hidden)
	}
	if inaccessible := byKey[hidden.Key]; inaccessible == nil || inaccessible.Title != hidden.Key {
		t.Fatalf("inaccessible epic row = %+v", inaccessible)
	}
}

// agentUsageRowsByKey indexes agent.usage rows by their key.
func agentUsageRowsByKey(rows []AgentUsageRow) map[string]*AgentUsageRow {
	out := map[string]*AgentUsageRow{}
	for i := range rows {
		out[rows[i].Key] = &rows[i]
	}
	return out
}

func TestAgentUsageSeriesAndDefaults(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "bob", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	ap := aiproxy.NewStore(c.Store, nil)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	for i, ts := range []time.Time{base, base.Add(3 * time.Minute), base.Add(65 * time.Minute)} {
		if err := ap.Insert(aiproxy.AIRequest{ID: string(rune('a' + i)), TS: ts.Format(time.RFC3339Nano),
			Agent: "bob", Iteration: "it-1", InputTokens: 10, OutputTokens: 5, CostUSD: 0.01, Status: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	// Default group_by=iteration, bucket=1h.
	res, err := h(t, "agent.usage")(c, registry.Params{"name": "bob"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(AgentUsage)
	if m.GroupBy != "iteration" || m.Bucket != "1h" {
		t.Fatalf("defaults = %v/%v", m.GroupBy, m.Bucket)
	}
	series := m.Series
	// 1h buckets: 09:00 (2 reqs) and 10:00 (1 req).
	if len(series) != 2 {
		t.Fatalf("series buckets = %+v", series)
	}
	if series[0].BucketStart != "2026-07-06T09:00:00Z" || series[0].Requests != 2 {
		t.Fatalf("first bucket = %+v", series[0])
	}
	if series[0].Tokens != 30 { // 2 reqs * (10+5)
		t.Fatalf("first bucket tokens = %v", series[0].Tokens)
	}
}

func TestAgentUsageBadParams(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "alice", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	// Unknown group_by / bucket are 400s (api.UserError).
	if _, err := h(t, "agent.usage")(c, registry.Params{"name": "alice", "group_by": "nope"}); !isUserErr(err, "bad_group_by") {
		t.Fatalf("group_by: got %v", err)
	}
	if _, err := h(t, "agent.usage")(c, registry.Params{"name": "alice", "bucket": "3m"}); !isUserErr(err, "bad_bucket") {
		t.Fatalf("bucket: got %v", err)
	}
	// Unknown agent is not_found.
	if _, err := h(t, "agent.usage")(c, registry.Params{"name": "ghost"}); !isUserErr(err, "not_found") {
		t.Fatalf("unknown agent: got %v", err)
	}
}

func isUserErr(err error, code string) bool {
	ue, ok := err.(api.UserError)
	return ok && ue.Code == code
}
