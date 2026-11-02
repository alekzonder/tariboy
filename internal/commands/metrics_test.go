package commands

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/metrics"
	"github.com/alekzonder/tariboy/internal/registry"
)

// metricsFixture stores one done task without a workflow and its AI request.
func metricsFixture(t *testing.T) *registry.Ctx {
	t.Helper()
	c, _, _ := ctxWithStore(t)
	db := c.Store.DB
	at := time.Now().UTC().Add(-2 * time.Hour)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO task_queues(prefix, name, created_at, updated_at) VALUES ('DEV', 'Dev', ?, ?)`, []any{fmtTime(at), fmtTime(at)}},
		{`INSERT INTO tasks(id, task_key, queue_prefix, title, status, author, customer, created_at, updated_at, started_at, completed_at)
			VALUES (1, 'DEV-1', 'DEV', 'one', 'done', 'user:customer', 'user:customer', ?, ?, ?, ?)`,
			[]any{fmtTime(at), fmtTime(at), fmtTime(at), fmtTime(at.Add(90 * time.Minute))}},
		{`INSERT INTO iterations(id, agent, trigger, status, started_at, image_ref, image_version, image_digest)
			VALUES ('it-1', 'alice', 'manual', 'done', ?, 'developer:latest', '1.0.0', 'sha1')`, []any{fmtTime(at)}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	aiproxy.NewStore(c.Store, nil).Insert(aiproxy.AIRequest{ID: "r1", TS: fmtTime(at.Add(time.Minute)), Agent: "alice",
		Iteration: "it-1", ImageName: "developer", InputTokens: 10, CostUSD: 0.25, Status: "ok", TaskID: "DEV-1"})
	return c
}

func fmtTime(t time.Time) string { return t.Format("2006-01-02T15:04:05.000000000Z") }

func TestMetricsTaskCommand(t *testing.T) {
	t.Parallel()
	c := metricsFixture(t)
	res, err := h(t, "metrics.task")(c, registry.Params{"key": "DEV-1"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(metrics.TaskMetrics)
	if m.LeadTimeS.Value != 5400 || m.Usage.Iterations != 1 || m.Usage.CostUSD != 0.25 {
		t.Fatalf("metrics = %+v", m)
	}
	_, err = h(t, "metrics.task")(c, registry.Params{"key": "DEV-9"})
	if status, code, _, _, ok := api.ErrorResponse(err); !ok || status != 404 || code != "not_found" {
		t.Fatalf("unknown task: %v", err)
	}
}

func TestMetricsServerCommand(t *testing.T) {
	t.Parallel()
	c := metricsFixture(t)
	res, err := h(t, "metrics.server")(c, registry.Params{"image": "developer:1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(metrics.MetricsReport)
	if r.Total.Tasks.Done != 1 || r.Total.Usage.Iterations != 1 || len(r.Images) != 1 {
		t.Fatalf("report = %+v", r)
	}
	for _, bad := range []registry.Params{{"image": "developer:"}, {"since": "yesterday"}, {"workflow": ":1"}} {
		_, err := h(t, "metrics.server")(c, bad)
		if status, _, _, _, ok := api.ErrorResponse(err); !ok || status != 400 {
			t.Fatalf("%v: err = %v", bad, err)
		}
	}
}

func TestMetricsPrintsHoursAndMinutes(t *testing.T) {
	t.Parallel()
	c := metricsFixture(t)
	res, err := h(t, "metrics.task")(c, registry.Params{"key": "DEV-1"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := printTaskMetrics(&out, res.(metrics.TaskMetrics)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lead time", "1h 30m", "quality", "-", "alice", "developer:latest", "1.0.0"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("task output has no %q:\n%s", want, out.String())
		}
	}
	rep, err := h(t, "metrics.server")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := printReport(&out, rep.(metrics.MetricsReport)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"total", "alice", "developer", "1.0.0", "none", "1h 30m", "0.25"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report output has no %q:\n%s", want, out.String())
		}
	}
	if got := formatDuration(metrics.MetricsNullSeconds{Value: 12*3600 + 5*60 + 59, Valid: true}); got != "12h 05m" {
		t.Fatalf("duration = %q", got)
	}
	if got := formatDuration(metrics.MetricsNullSeconds{}); got != "-" {
		t.Fatalf("null duration = %q", got)
	}
}

func init() { typedCallSets["metrics"] = metricsTypedCalls }

func metricsTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	return metricsFixture(t), []typedCall{
		{"GET", "/api/metrics/tasks/{key}", "/api/metrics/tasks/DEV-1", nil},
		{"GET", "/api/metrics", "/api/metrics", nil},
		{"GET", "/api/metrics", "/api/metrics?agent=nobody", nil},
	}
}
