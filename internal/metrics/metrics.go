// Package metrics computes task, agent, image, workflow and server metrics
// on request from the tasks, workflow visits, AI requests, iterations and
// assessments. It reads the database only and stores no aggregates.
package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

var axes = []string{"image_process", "role", "task_fit"}

// DefaultPeriod is the report period when Since is not given.
const DefaultPeriod = 30 * 24 * time.Hour

// Reader computes metrics from the database.
type Reader struct {
	db  *sql.DB
	now func() time.Time
}

// New returns a Reader over db; now is the clock for open intervals and the
// default period.
func New(db *sql.DB, now func() time.Time) *Reader {
	if now == nil {
		now = time.Now
	}
	return &Reader{db: db, now: now}
}

// Filter selects the period and the data of a server report. Empty fields
// do not filter. Workflow "none" selects tasks and iterations without a
// workflow.
type Filter struct {
	Since, Until              time.Time
	Agent                     string
	Image, ImageVersion       string
	Workflow, WorkflowVersion string
}

// ParseNameVersion splits NAME[:VERSION].
func ParseNameVersion(s string) (name, version string, err error) {
	name, version, found := strings.Cut(s, ":")
	if s != "" && (name == "" || (found && version == "") || strings.Contains(version, ":")) {
		return "", "", errInvalid(fmt.Sprintf("%q is not NAME[:VERSION]", s))
	}
	return name, version, nil
}

func (r *Reader) read(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // read-only
	return fn(tx)
}

// Task returns the metrics of the task with the current or an old key.
func (r *Reader) Task(ctx context.Context, key string) (TaskMetrics, error) {
	var out TaskMetrics
	err := r.read(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE task_key = ?
			UNION ALL SELECT task_id FROM task_key_aliases WHERE old_key = ? LIMIT 1`, key, key).Scan(&id)
		if err == sql.ErrNoRows {
			return &Error{Status: http.StatusNotFound, Code: "not_found", Msg: fmt.Sprintf("task %q not found", key)}
		}
		if err != nil {
			return err
		}
		fallback, err := loadFallbackVersions(ctx, tx)
		if err != nil {
			return err
		}
		d, err := load(ctx, tx, `id = ?`, []any{id}, fallback)
		if err != nil {
			return err
		}
		out = d.taskMetrics(d.tasks[id], r.now())
		return nil
	})
	return out, err
}

// usageAcc accumulates the usage of iterations.
type usageAcc struct {
	iterations map[string]bool
	tokens     MetricsTokens
	cost       float64
}

func (u *usageAcc) add(it iterRow) {
	if u.iterations == nil {
		u.iterations = map[string]bool{}
	}
	if it.iteration != "" {
		u.iterations[it.iteration] = true
	}
	u.tokens.add(it.tokens)
	u.cost += it.cost
}

func roundCost(v float64) float64 { return math.Round(v*1e6) / 1e6 }

func (u *usageAcc) usage() MetricsUsage {
	return MetricsUsage{Iterations: int64(len(u.iterations)), Tokens: u.tokens, CostUSD: roundCost(u.cost)}
}

// imageUsage groups iterations by image version, ordered by name and version.
func imageUsage(iters []iterRow) []MetricsImageVersionUsage {
	acc := map[string]*usageAcc{}
	first := map[string]imageVer{}
	for _, it := range iters {
		k := it.image.key()
		if acc[k] == nil {
			acc[k], first[k] = &usageAcc{}, it.image
		}
		acc[k].add(it)
	}
	out := make([]MetricsImageVersionUsage, 0, len(acc))
	for _, k := range slices.Sorted(maps.Keys(acc)) {
		u, v := acc[k].usage(), first[k]
		out = append(out, MetricsImageVersionUsage{Image: v.name, ImageRef: v.ref, ImageVersion: v.version, ImageDigest: v.digest,
			Iterations: u.Iterations, Tokens: u.Tokens, CostUSD: u.CostUSD})
	}
	return out
}

func closed(status string) bool { return status == "done" || status == "cancelled" }

// statusTime sums the time in each non-terminal status: workflow visits, or
// for a task without a workflow its status changes. Open intervals run to now.
func (t *taskRec) statusTime(now time.Time) map[string]int64 {
	sum := map[string]time.Duration{}
	if len(t.visits) > 0 {
		for _, v := range t.visits {
			end := v.left
			if end.IsZero() {
				if closed(t.status) {
					continue // the terminal status of a finished task
				}
				end = now
			}
			sum[v.status] += end.Sub(v.entered)
		}
	} else {
		for i, c := range t.changes {
			if closed(c.status) {
				continue
			}
			end := now
			if i+1 < len(t.changes) {
				end = t.changes[i+1].at
			}
			sum[c.status] += end.Sub(c.at)
		}
	}
	out := map[string]int64{}
	for status, d := range sum {
		out[status] = int64(d / time.Second)
	}
	return out
}

// leadTime is the time from the first entry to in_progress to completion of
// a done task.
func (t *taskRec) leadTime() MetricsNullSeconds {
	if t.status != "done" || t.completedAt == "" {
		return MetricsNullSeconds{}
	}
	start := parseTime(t.startedAt)
	if t.startedAt == "" {
		for _, c := range t.changes {
			if c.status == "in_progress" {
				start = c.at
				break
			}
		}
	}
	if start.IsZero() {
		return MetricsNullSeconds{}
	}
	return MetricsNullSeconds{Value: int64(parseTime(t.completedAt).Sub(start) / time.Second), Valid: true}
}

// workflowAt is the workflow version of the visit open at start, else the
// task's workflow, else none.
func (d *data) workflowAt(t *taskRec, start time.Time) wfVer {
	if t == nil {
		return noWorkflow
	}
	digest := t.wfDigest
	for _, v := range t.visits {
		if !v.entered.After(start) && (v.left.IsZero() || start.Before(v.left)) && v.digest != "" {
			digest = v.digest
		}
	}
	return d.workflow(digest)
}

func (d *data) workflow(digest string) wfVer {
	if digest == "" {
		return noWorkflow
	}
	if v, ok := d.workflows[digest]; ok {
		return v
	}
	return wfVer{name: "unknown", digest: digest}
}

// taskWorkflows lists the distinct workflow versions of the task's visits.
func (d *data) taskWorkflows(t *taskRec) []wfVer {
	var out []wfVer
	seen := map[string]bool{}
	add := func(digest string) {
		v := d.workflow(digest)
		if digest != "" && !seen[v.key()] {
			seen[v.key()] = true
			out = append(out, v)
		}
	}
	for _, v := range t.visits {
		add(v.digest)
	}
	if len(out) == 0 {
		add(t.wfDigest)
	}
	return out
}

func quality(scores map[string][]int64) MetricsQuality {
	axis := func(name string) MetricsAxisScore {
		var a MetricsAxisScore
		var sum int64
		for i, s := range scores[name] {
			if i == 0 || s < a.Min {
				a.Min = s
			}
			sum += s
			a.Count++
		}
		if a.Count > 0 {
			a.Avg = math.Round(float64(sum)/float64(a.Count)*100) / 100
		}
		return a
	}
	return MetricsQuality{ImageProcess: axis("image_process"), Role: axis("role"), TaskFit: axis("task_fit")}
}

func (d *data) taskMetrics(t *taskRec, now time.Time) TaskMetrics {
	m := TaskMetrics{Key: t.key, Title: t.title, Status: t.status, StartedAt: t.startedAt, CompletedAt: t.completedAt,
		LeadTimeS: t.leadTime(), StatusTimeS: t.statusTime(now), CustomerQuestions: t.questions,
		Quality: quality(t.scores), Agents: []MetricsTaskAgentUsage{}, Images: imageUsage(t.iters), Workflows: []MetricsWorkflowRef{}}
	for _, v := range t.visits {
		m.RejectedAdvances += v.rejected
		switch v.outcome {
		case "changes_requested":
			m.ReworkByOutcome.ChangesRequested++
		case "rework":
			m.ReworkByOutcome.Rework++
		case "conflict":
			m.ReworkByOutcome.Conflict++
		}
	}
	m.ReworkRounds = m.ReworkByOutcome.ChangesRequested + m.ReworkByOutcome.Rework + m.ReworkByOutcome.Conflict
	var total usageAcc
	byAgent := map[string][]iterRow{}
	for _, it := range t.iters {
		total.add(it)
		byAgent[it.agent] = append(byAgent[it.agent], it)
	}
	m.Usage = total.usage()
	for _, agent := range slices.Sorted(maps.Keys(byAgent)) {
		var u usageAcc
		for _, it := range byAgent[agent] {
			u.add(it)
		}
		m.Agents = append(m.Agents, MetricsTaskAgentUsage{Agent: agent, Usage: u.usage(), Images: imageUsage(byAgent[agent])})
	}
	for _, v := range d.taskWorkflows(t) {
		m.Workflows = append(m.Workflows, MetricsWorkflowRef{Name: v.name, Version: v.version, Digest: v.digest})
	}
	return m
}

// Report returns the server metrics over the filter's period.
func (r *Reader) Report(ctx context.Context, f Filter) (MetricsReport, error) {
	now := r.now()
	if f.Until.IsZero() {
		f.Until = now
	}
	if f.Since.IsZero() {
		f.Since = f.Until.Add(-DefaultPeriod)
	}
	if !f.Since.Before(f.Until) {
		return MetricsReport{}, errInvalid("since must be before until")
	}
	since, until := store.FormatTime(f.Since), store.FormatTime(f.Until)
	var out MetricsReport
	err := r.read(ctx, func(tx *sql.Tx) error {
		fallback, err := loadFallbackVersions(ctx, tx)
		if err != nil {
			return err
		}
		rows, err := loadIterations(ctx, tx, `a.ts >= ? AND a.ts < ?`, []any{since, until}, fallback)
		if err != nil {
			return err
		}
		// The tasks done in the period, and the tasks of the period's
		// iterations, whose visits give an iteration its workflow version.
		inPeriod := `SELECT task_id FROM ai_requests WHERE ts >= ? AND ts < ? AND task_id IS NOT NULL`
		d, err := load(ctx, tx, `(status = 'done' AND completed_at >= ? AND completed_at < ?) OR task_key IN (`+inPeriod+`)
			OR id IN (SELECT task_id FROM task_key_aliases WHERE old_key IN (`+inPeriod+`))`,
			[]any{since, until, since, until, since, until}, fallback)
		if err != nil {
			return err
		}
		out = d.report(f, rows, since, until, now)
		return nil
	})
	return out, err
}
