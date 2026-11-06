package metrics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// withoutTaskRequests adds to the fixture, next to the request of it-x with
// task_id NULL:
//
//   - a request of it-x with an empty task_id (0.50);
//   - a request of it-3 with task_id NULL before it-3 selected DEV-3 (0.25),
//     so it-3 has requests both without a task and with one.
func withoutTaskRequests(st *store.Store) {
	add := func(id, iteration, agent, digest string, task any, at string, in int64, cost float64) {
		if _, err := st.DB.Exec(`INSERT INTO ai_requests(id, ts, agent, iteration, image_name, image_tag, image_digest, input_tokens,
			output_tokens, cache_write_tokens, cache_read_tokens, cost_usd, status, task_id)
			VALUES (?, ?, ?, ?, 'developer', 'latest', ?, ?, 0, 0, 0, ?, 'ok', ?)`, id, at, agent, iteration, digest, in, cost, task); err != nil {
			panic(err)
		}
	}
	add("nt-empty", "it-x", "carol", "sha2", "", ts(4, 0, 2), 2, 0.50)
	add("nt-before-goal", "it-3", "bob", "sha1", nil, ts(3, 0, 59), 4, 0.25)
}

func wantNoTask(t *testing.T, what string, s MetricsSlice, iterations int64, cost float64) {
	t.Helper()
	if got := s.UsageWithoutTask; got.Iterations != iterations || !near(got.CostUSD, cost) {
		t.Fatalf("%s: usage_without_task = %+v, want %d iterations, %v USD", what, got, iterations, cost)
	}
	if s.UsageWithoutTask.CostUSD > s.Usage.CostUSD {
		t.Fatalf("%s: cost without task %v exceeds cost %v", what, s.UsageWithoutTask.CostUSD, s.Usage.CostUSD)
	}
}

func TestReportUsageWithoutTask(t *testing.T) {
	t.Parallel()
	rep, err := newFixture(t, withoutTaskRequests).Report(context.Background(), period())
	if err != nil {
		t.Fatal(err)
	}
	// NULL and '' both count; it-3 counts in both usage blocks.
	wantNoTask(t, "total", rep.Total, 2, 1.75)
	if rep.Total.Usage.Iterations != 6 || !near(rep.Total.Usage.CostUSD, 2.9) || rep.Total.UsageWithoutTask.Tokens.Input != 7 {
		t.Fatalf("total = %+v", rep.Total)
	}
	wantNoTask(t, "carol", findSlice(t, rep.Agents, "carol", ""), 1, 1.5)
	wantNoTask(t, "bob", findSlice(t, rep.Agents, "bob", ""), 1, 0.25)
	wantNoTask(t, "alice", findSlice(t, rep.Agents, "alice", ""), 0, 0)
	wantNoTask(t, "image 1.0.0", findSlice(t, rep.Images, "developer", "1.0.0"), 1, 0.25)
	wantNoTask(t, "image 2.0.0", findSlice(t, rep.Images, "developer", "2.0.0"), 1, 1.5)
	wantNoTask(t, "workflow none", findSlice(t, rep.Workflows, "none", ""), 2, 1.75)
	wantNoTask(t, "workflow 0.10.1", findSlice(t, rep.Workflows, "development", "0.10.1"), 0, 0)
	for _, list := range [][]MetricsSlice{rep.Agents, rep.Images, rep.Workflows} {
		for _, s := range list {
			if s.UsageWithoutTask.CostUSD > s.Usage.CostUSD {
				t.Fatalf("slice %s %s: %+v", s.Name, s.Version, s)
			}
		}
	}
}

func TestReportUsageWithoutTaskFilters(t *testing.T) {
	t.Parallel()
	r := newFixture(t, withoutTaskRequests)
	for _, c := range []struct {
		name       string
		filter     func(*Filter)
		iterations int64
		cost       float64
	}{
		{"agent", func(f *Filter) { f.Agent = "bob" }, 1, 0.25},
		{"image version", func(f *Filter) { f.Image, f.ImageVersion = "developer", "2.0.0" }, 1, 1.5},
		{"workflow version", func(f *Filter) { f.Workflow, f.WorkflowVersion = "development", "0.10.1" }, 0, 0},
		{"workflow none", func(f *Filter) { f.Workflow = "none" }, 2, 1.75},
	} {
		f := period()
		c.filter(&f)
		rep, err := r.Report(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		wantNoTask(t, c.name, rep.Total, c.iterations, c.cost)
	}
}

func TestReportUsageWithoutTaskIsZeroNotNull(t *testing.T) {
	t.Parallel()
	f := Filter{Since: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	rep, err := newFixture(t, withoutTaskRequests).Report(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	wantNoTask(t, "total", rep.Total, 0, 0)
	raw, _ := json.Marshal(rep.Total)
	if !strings.Contains(string(raw), `"usage_without_task":{"iterations":0,`) {
		t.Fatalf("total JSON = %s", raw)
	}
}
