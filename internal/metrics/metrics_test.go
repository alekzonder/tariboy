package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// fixtureNow is the reader clock: open visits and open status intervals run
// up to it.
var fixtureNow = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

func ts(day, hour, minute int) string {
	return store.FormatTime(time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC))
}

func exec(t *testing.T, st *store.Store, q string, args ...any) {
	t.Helper()
	if _, err := st.DB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// newFixture stores, with values counted by hand in the tests below:
//
//   - DEV-1: a development workflow task (0.10.0), done, with the outcomes
//     changes_requested and rework, two rejected advances, two questions to
//     the customer, two assessments, and iterations of two agents (alice,
//     bob) on developer 1.0.0;
//   - DEV-2: a task without a workflow, done, no assessments, started_at
//     empty (the start comes from its events), once called OLD-7: one
//     iteration on developer 1.0.0 and one, under the old key, whose
//     iterations row retention removed (its version 2.0.0 comes from it-x,
//     another iteration with the same digest);
//   - DEV-3: a development workflow task (0.10.1) still in implement;
//   - it-x: an iteration of carol on developer 2.0.0 without a task.
func newFixture(t *testing.T) *Reader {
	t.Helper()
	st := storetest.Open(t)
	exec(t, st, `INSERT INTO task_queues(prefix, name, created_at, updated_at) VALUES ('DEV', 'Dev', ?, ?)`, ts(1, 0, 0), ts(1, 0, 0))
	for _, wf := range [][]string{{"wfd1", "0.10.0"}, {"wfd2", "0.10.1"}} {
		exec(t, st, `INSERT INTO task_workflow_images(digest, name, version, manifest, built_at) VALUES (?, 'development', ?, '{}', ?)`, wf[0], wf[1], ts(1, 0, 0))
	}
	task := func(id int, key, status, digest, started, completed string) {
		var d any
		if digest != "" {
			d = digest
		}
		exec(t, st, `INSERT INTO tasks(id, task_key, queue_prefix, title, status, author, customer, workflow_digest,
			created_at, updated_at, started_at, completed_at) VALUES (?, ?, 'DEV', ?, ?, 'user:customer', 'user:customer', ?, ?, ?, ?, ?)`,
			id, key, "task "+key, status, d, ts(1, 0, 0), ts(1, 0, 0), started, completed)
	}
	task(1, "DEV-1", "done", "wfd1", ts(1, 10, 0), ts(1, 16, 0))
	task(2, "DEV-2", "done", "", "", ts(2, 5, 0))
	task(3, "DEV-3", "in_progress", "wfd2", ts(3, 0, 0), "")
	exec(t, st, `INSERT INTO task_key_aliases(old_key, task_id, created_at) VALUES ('OLD-7', 2, ?)`, ts(2, 0, 0))

	visit := func(task, seq int, status, entered, left, outcome string, rejected int, digest string) {
		exec(t, st, `INSERT INTO task_status_visits(task_id, sequence, status_id, entered_at, entered_by, left_at, outcome,
			rejected_requests, workflow_digest) VALUES (?, ?, ?, ?, 'agent:alice', ?, ?, ?, ?)`, task, seq, status, entered, left, outcome, rejected, digest)
	}
	visit(1, 1, "implement", ts(1, 10, 0), ts(1, 12, 0), "ready", 2, "wfd1")
	visit(1, 2, "review", ts(1, 12, 0), ts(1, 13, 0), "changes_requested", 0, "wfd1")
	visit(1, 3, "implement", ts(1, 13, 0), ts(1, 14, 0), "ready", 0, "wfd1")
	visit(1, 4, "review", ts(1, 14, 0), ts(1, 15, 0), "rework", 0, "wfd1")
	visit(1, 5, "implement", ts(1, 15, 0), ts(1, 15, 30), "ready", 0, "wfd1")
	visit(1, 6, "review", ts(1, 15, 30), ts(1, 16, 0), "merged", 0, "wfd1")
	visit(1, 7, "done", ts(1, 16, 0), "", "", 0, "wfd1")
	visit(3, 1, "implement", ts(3, 0, 0), "", "", 0, "wfd2")

	event := func(task int, kind, payload, at string) {
		exec(t, st, `INSERT INTO task_events(event_id, task_id, queue_prefix, kind, actor, payload, created_at)
			VALUES (?, ?, 'DEV', ?, 'user:customer', ?, ?)`, kind+at, task, kind, payload, at)
	}
	event(2, "task.created", `{"key":"DEV-2"}`, ts(2, 0, 0))
	event(2, "task.updated", `{"status":"in_progress"}`, ts(2, 1, 0))
	event(2, "task.updated", `{"status":"wait_customer"}`, ts(2, 3, 0))
	event(2, "task.updated", `{"status":"in_progress"}`, ts(2, 4, 0))
	event(2, "task.updated", `{"status":"done"}`, ts(2, 5, 0))

	exec(t, st, `INSERT INTO task_comments(id, task_id, author, body, created_at, updated_at) VALUES (1, 1, 'agent:alice', 'q', ?, ?)`, ts(1, 11, 0), ts(1, 11, 0))
	for _, w := range [][]string{{"user:customer", ts(1, 11, 0)}, {"user:customer", ts(1, 13, 30)}, {"agent:bob", ts(1, 14, 0)}} {
		exec(t, st, `INSERT INTO task_waiting_for(task_id, expected_principal, requesting_principal, requesting_comment_id, requested_at, resolved_at)
			VALUES (1, ?, 'agent:alice', 1, ?, ?)`, w[0], w[1], w[1])
	}

	assess := func(seq int, key string, axes map[string]any) {
		exec(t, st, `INSERT INTO assessments(seq, id, created_at, task_key) VALUES (?, ?, ?, ?)`, seq, "as-"+key+string(rune('a'+seq)), ts(1, 17, 0), key)
		for axis, score := range axes {
			if score == nil {
				exec(t, st, `INSERT INTO assessment_axes(assessment_seq, axis, verdict) VALUES (?, ?, 'not_applicable')`, seq, axis)
				continue
			}
			exec(t, st, `INSERT INTO assessment_axes(assessment_seq, axis, verdict, score) VALUES (?, ?, 'scored', ?)`, seq, axis, score)
		}
	}
	assess(1, "DEV-1", map[string]any{"image_process": 80, "role": 90, "task_fit": 70})
	assess(2, "DEV-1", map[string]any{"image_process": 60, "role": nil, "task_fit": 50})

	iteration := func(id, agent, version, digest, started string) {
		exec(t, st, `INSERT INTO iterations(id, agent, trigger, status, started_at, image_ref, image_version, image_digest)
			VALUES (?, ?, 'manual', 'done', ?, 'developer:latest', ?, ?)`, id, agent, started, version, digest)
	}
	iteration("it-1a", "alice", "1.0.0", "sha1", ts(1, 10, 5))
	iteration("it-1b", "bob", "1.0.0", "sha1", ts(1, 13, 5))
	iteration("it-2a", "alice", "1.0.0", "sha1", ts(2, 1, 10))
	iteration("it-3", "bob", "1.0.0", "sha1", ts(3, 1, 0))
	iteration("it-x", "carol", "2.0.0", "sha2", ts(4, 0, 0))

	n := 0
	request := func(iteration, agent, digest string, task any, at string, in, out, cw, cr int, cost float64) {
		n++
		exec(t, st, `INSERT INTO ai_requests(id, ts, agent, iteration, image_name, image_tag, image_digest, input_tokens, output_tokens,
			cache_write_tokens, cache_read_tokens, cost_usd, status, task_id) VALUES (?, ?, ?, ?, 'developer', 'latest', ?, ?, ?, ?, ?, ?, 'ok', ?)`,
			"r"+string(rune('a'+n)), at, agent, iteration, digest, in, out, cw, cr, cost, task)
	}
	request("it-1a", "alice", "sha1", "DEV-1", ts(1, 10, 6), 100, 10, 5, 50, 0.10)
	request("it-1a", "alice", "sha1", "DEV-1", ts(1, 10, 7), 100, 10, 5, 50, 0.10)
	request("it-1b", "bob", "sha1", "DEV-1", ts(1, 13, 6), 200, 20, 0, 0, 0.30)
	request("it-2a", "alice", "sha1", "DEV-2", ts(2, 1, 11), 10, 0, 0, 0, 0.20)
	request("it-2b", "alice", "sha2", "OLD-7", ts(2, 3, 30), 20, 0, 0, 0, 0.40)
	request("it-3", "bob", "sha1", "DEV-3", ts(3, 1, 1), 5, 0, 0, 0, 0.05)
	request("it-x", "carol", "sha2", nil, ts(4, 0, 1), 1, 0, 0, 0, 1.00)
	return New(st.Reader(), func() time.Time { return fixtureNow })
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestTaskMetricsWorkflowTask(t *testing.T) {
	t.Parallel()
	m, err := newFixture(t).Task(context.Background(), "DEV-1")
	if err != nil {
		t.Fatal(err)
	}
	if m.Key != "DEV-1" || m.Status != "done" || !m.LeadTimeS.Valid || m.LeadTimeS.Value != 6*3600 {
		t.Fatalf("task = %+v", m)
	}
	if m.StatusTimeS["implement"] != 12600 || m.StatusTimeS["review"] != 9000 || len(m.StatusTimeS) != 2 {
		t.Fatalf("status time = %v", m.StatusTimeS)
	}
	if m.Usage.Iterations != 2 || m.Usage.Tokens != (MetricsTokens{Input: 400, Output: 40, CacheWrite: 10, CacheRead: 100}) || !near(m.Usage.CostUSD, 0.5) {
		t.Fatalf("usage = %+v", m.Usage)
	}
	if m.ReworkRounds != 2 || m.ReworkByOutcome != (MetricsReworkByOutcome{ChangesRequested: 1, Rework: 1}) {
		t.Fatalf("rework = %d %+v", m.ReworkRounds, m.ReworkByOutcome)
	}
	if m.RejectedAdvances != 2 || m.CustomerQuestions != 2 {
		t.Fatalf("rejected = %d questions = %d", m.RejectedAdvances, m.CustomerQuestions)
	}
	q := m.Quality
	if q.ImageProcess != (MetricsAxisScore{Avg: 70, Min: 60, Count: 2}) || q.Role != (MetricsAxisScore{Avg: 90, Min: 90, Count: 1}) ||
		q.TaskFit != (MetricsAxisScore{Avg: 60, Min: 50, Count: 2}) {
		t.Fatalf("quality = %+v", q)
	}
	if len(m.Agents) != 2 || m.Agents[0].Agent != "alice" || m.Agents[0].Usage.Iterations != 1 || !near(m.Agents[0].Usage.CostUSD, 0.2) ||
		m.Agents[1].Agent != "bob" || !near(m.Agents[1].Usage.CostUSD, 0.3) {
		t.Fatalf("agents = %+v", m.Agents)
	}
	if len(m.Agents[0].Images) != 1 || m.Agents[0].Images[0].ImageRef != "developer:latest" || m.Agents[0].Images[0].ImageVersion != "1.0.0" ||
		m.Agents[0].Images[0].ImageDigest != "sha1" || m.Agents[0].Images[0].Iterations != 1 {
		t.Fatalf("agent images = %+v", m.Agents[0].Images)
	}
	if len(m.Images) != 1 || m.Images[0].Iterations != 2 {
		t.Fatalf("images = %+v", m.Images)
	}
	if len(m.Workflows) != 1 || m.Workflows[0] != (MetricsWorkflowRef{Name: "development", Version: "0.10.0", Digest: "wfd1"}) {
		t.Fatalf("workflows = %+v", m.Workflows)
	}
}

func TestTaskMetricsTaskWithoutWorkflowOldKeyAndRetainedIteration(t *testing.T) {
	t.Parallel()
	r := newFixture(t)
	m, err := r.Task(context.Background(), "OLD-7") // an old key names the same task
	if err != nil {
		t.Fatal(err)
	}
	if m.Key != "DEV-2" || !m.LeadTimeS.Valid || m.LeadTimeS.Value != 4*3600 {
		t.Fatalf("task = %+v", m)
	}
	if m.StatusTimeS["open"] != 3600 || m.StatusTimeS["in_progress"] != 10800 || m.StatusTimeS["wait_customer"] != 3600 || len(m.StatusTimeS) != 3 {
		t.Fatalf("status time = %v", m.StatusTimeS)
	}
	if m.Usage.Iterations != 2 || !near(m.Usage.CostUSD, 0.6) || m.ReworkRounds != 0 || m.RejectedAdvances != 0 {
		t.Fatalf("usage = %+v rework = %d", m.Usage, m.ReworkRounds)
	}
	if len(m.Images) != 2 || m.Images[0].ImageVersion != "1.0.0" || m.Images[1].ImageVersion != "2.0.0" || m.Images[1].ImageDigest != "sha2" ||
		m.Images[1].ImageRef != "developer" {
		t.Fatalf("images = %+v", m.Images)
	}
	if len(m.Workflows) != 0 {
		t.Fatalf("workflows = %+v", m.Workflows)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"quality":null`) {
		t.Fatalf("a task without assessments must have quality null: %s", raw)
	}
}

func TestTaskMetricsOpenTask(t *testing.T) {
	t.Parallel()
	m, err := newFixture(t).Task(context.Background(), "DEV-3")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m)
	if m.LeadTimeS.Valid || !strings.Contains(string(raw), `"lead_time_s":null`) {
		t.Fatalf("an open task has lead_time_s null: %s", raw)
	}
	if m.StatusTimeS["implement"] != int64(fixtureNow.Sub(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))/time.Second) {
		t.Fatalf("an open visit runs to now: %v", m.StatusTimeS)
	}
}

func TestTaskMetricsUnknownTask(t *testing.T) {
	t.Parallel()
	_, err := newFixture(t).Task(context.Background(), "DEV-404")
	var e *Error
	if !errors.As(err, &e) || e.Code != "not_found" {
		t.Fatalf("err = %v", err)
	}
}

func findSlice(t *testing.T, slices []MetricsSlice, name, version string) MetricsSlice {
	t.Helper()
	for _, s := range slices {
		if s.Name == name && s.Version == version {
			return s
		}
	}
	t.Fatalf("no slice %s %s in %+v", name, version, slices)
	return MetricsSlice{}
}

func period() Filter {
	return Filter{Since: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)}
}

func TestReportTotalsAndSlices(t *testing.T) {
	t.Parallel()
	rep, err := newFixture(t).Report(context.Background(), period())
	if err != nil {
		t.Fatal(err)
	}
	tot := rep.Total
	if tot.Usage.Iterations != 6 || !near(tot.Usage.CostUSD, 2.15) {
		t.Fatalf("total usage = %+v", tot.Usage)
	}
	ts := tot.Tasks
	if ts.Done != 2 || ts.LeadTimeMedianS.Value != 18000 || ts.LeadTimeAvgS.Value != 18000 || ts.Sum.Iterations != 4 ||
		!near(ts.Sum.CostUSD, 1.1) || ts.Sum.ReworkRounds != 2 || ts.Sum.RejectedAdvances != 2 || ts.Sum.CustomerQuestions != 2 {
		t.Fatalf("total tasks = %+v", ts)
	}
	if ts.Avg == nil || !near(ts.Avg.Iterations, 2) || !near(ts.Avg.CostUSD, 0.55) || ts.Quality.ImageProcess.Count != 2 {
		t.Fatalf("total averages = %+v quality = %+v", ts.Avg, ts.Quality)
	}

	v1 := findSlice(t, rep.Images, "developer", "1.0.0")
	if v1.Usage.Iterations != 4 || !near(v1.Usage.CostUSD, 0.75) || v1.Tasks.Done != 1 || v1.Tasks.Mixed != 1 || v1.Tasks.LeadTimeAvgS.Value != 21600 {
		t.Fatalf("image 1.0.0 = %+v", v1)
	}
	v2 := findSlice(t, rep.Images, "developer", "2.0.0")
	if v2.Usage.Iterations != 2 || !near(v2.Usage.CostUSD, 1.4) || v2.Tasks.Done != 0 || v2.Tasks.Mixed != 1 || v2.Tasks.LeadTimeAvgS.Valid || v2.Tasks.Avg != nil {
		t.Fatalf("image 2.0.0 = %+v", v2)
	}
	raw, _ := json.Marshal(v2)
	if !strings.Contains(string(raw), `"quality":null`) {
		t.Fatalf("a slice without assessments has quality null: %s", raw)
	}

	w0 := findSlice(t, rep.Workflows, "development", "0.10.0")
	w1 := findSlice(t, rep.Workflows, "development", "0.10.1")
	none := findSlice(t, rep.Workflows, "none", "")
	if w0.Usage.Iterations != 2 || w0.Tasks.Done != 1 || w1.Usage.Iterations != 1 || w1.Tasks.Done != 0 ||
		none.Usage.Iterations != 3 || !near(none.Usage.CostUSD, 1.6) || none.Tasks.Done != 1 {
		t.Fatalf("workflows = %+v", rep.Workflows)
	}

	alice, bob, carol := findSlice(t, rep.Agents, "alice", ""), findSlice(t, rep.Agents, "bob", ""), findSlice(t, rep.Agents, "carol", "")
	if alice.Usage.Iterations != 3 || !near(alice.Usage.CostUSD, 0.8) || alice.Tasks.Done != 2 ||
		bob.Usage.Iterations != 2 || bob.Tasks.Done != 1 || carol.Usage.Iterations != 1 || carol.Tasks.Done != 0 {
		t.Fatalf("agents = %+v", rep.Agents)
	}
}

func TestReportFilters(t *testing.T) {
	t.Parallel()
	r := newFixture(t)
	for _, c := range []struct {
		name       string
		filter     func(*Filter)
		iterations int64
		cost       float64
		done       int
	}{
		{"agent", func(f *Filter) { f.Agent = "bob" }, 2, 0.35, 1},
		{"image version", func(f *Filter) { f.Image, f.ImageVersion = "developer", "2.0.0" }, 2, 1.4, 1},
		{"image name", func(f *Filter) { f.Image = "developer" }, 6, 2.15, 2},
		{"workflow version", func(f *Filter) { f.Workflow, f.WorkflowVersion = "development", "0.10.0" }, 2, 0.5, 1},
		{"workflow none", func(f *Filter) { f.Workflow = "none" }, 3, 1.6, 1},
	} {
		f := period()
		c.filter(&f)
		rep, err := r.Report(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Total.Usage.Iterations != c.iterations || !near(rep.Total.Usage.CostUSD, c.cost) || rep.Total.Tasks.Done != c.done {
			t.Fatalf("%s: total = %+v", c.name, rep.Total)
		}
	}
}

func TestReportPeriodByCompletionAndRequestTime(t *testing.T) {
	t.Parallel()
	f := Filter{Since: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	rep, err := newFixture(t).Report(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	// Only DEV-2 is done on October 2; its lifetime usage counts in the task
	// block, the two requests of that day in the usage block.
	if rep.Total.Tasks.Done != 1 || rep.Total.Usage.Iterations != 2 || !near(rep.Total.Usage.CostUSD, 0.6) {
		t.Fatalf("total = %+v", rep.Total)
	}
	if rep.Since != store.FormatTime(f.Since) || rep.Until != store.FormatTime(f.Until) {
		t.Fatalf("period = %s %s", rep.Since, rep.Until)
	}
}

func TestReportDefaultPeriodIsThirtyDays(t *testing.T) {
	t.Parallel()
	rep, err := newFixture(t).Report(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Until != store.FormatTime(fixtureNow) || rep.Since != store.FormatTime(fixtureNow.AddDate(0, 0, -30)) || rep.Total.Tasks.Done != 2 {
		t.Fatalf("report = %s..%s %+v", rep.Since, rep.Until, rep.Total.Tasks)
	}
}

// The CLI decodes the daemon's JSON into the same types.
func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()
	r := newFixture(t)
	for _, key := range []string{"DEV-1", "DEV-2", "DEV-3"} {
		m, err := r.Task(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var back TaskMetrics
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatal(err)
		}
		again, _ := json.Marshal(back)
		if string(again) != string(raw) || back.LeadTimeS != m.LeadTimeS || back.Quality != m.Quality {
			t.Fatalf("%s round trip:\n%s\n%s", key, raw, again)
		}
	}
	rep, err := r.Report(context.Background(), period())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rep)
	var back MetricsReport
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if again, _ := json.Marshal(back); string(again) != string(raw) {
		t.Fatalf("report round trip:\n%s\n%s", raw, again)
	}
}

func TestParseNameVersion(t *testing.T) {
	t.Parallel()
	for in, want := range map[string][2]string{"developer": {"developer", ""}, "developer:0.23.1": {"developer", "0.23.1"}, "": {"", ""}} {
		name, version, err := ParseNameVersion(in)
		if err != nil || name != want[0] || version != want[1] {
			t.Fatalf("%q = %q %q %v", in, name, version, err)
		}
	}
	for _, in := range []string{"developer:", ":1.0", "a:b:c"} {
		if _, _, err := ParseNameVersion(in); err == nil {
			t.Fatalf("%q must be invalid", in)
		}
	}
}
