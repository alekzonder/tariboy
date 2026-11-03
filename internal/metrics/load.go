package metrics

import (
	"cmp"
	"context"
	"database/sql"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/assessment"
	"github.com/alekzonder/tariboy/internal/store"
)

// UnknownVersion is the image version of an iteration whose version no
// iterations row records.
const UnknownVersion = "unknown"

// imageVer identifies the image version an iteration ran on.
type imageVer struct{ name, version, ref, digest string }

func (v imageVer) key() string { return v.name + "\x00" + v.version }

// wfVer identifies a workflow version; name is "none" for no workflow.
type wfVer struct{ name, version, digest string }

func (v wfVer) key() string { return v.name + "\x00" + v.version }

var noWorkflow = wfVer{name: "none"}

// iterRow is the AI usage of one iteration of one agent for one task key.
type iterRow struct {
	iteration, agent, taskKey string
	start                     time.Time
	tokens                    MetricsTokens
	cost                      float64
	image                     imageVer
}

type visit struct {
	status, outcome, digest string
	entered, left           time.Time // left is zero for an open visit
	rejected                int64
}

type statusChange struct {
	status string
	at     time.Time
}

type taskRec struct {
	id                                     int64
	key, title, status, customer, wfDigest string
	startedAt, completedAt                 string
	visits                                 []visit
	changes                                []statusChange
	iters                                  []iterRow
	questions                              int64
	scores                                 map[string][]int64
}

// data is one consistent read of the tasks in scope and their sources.
type data struct {
	tasks     map[int64]*taskRec
	keys      map[string]int64 // current and old task keys
	workflows map[string]wfVer // by digest
}

func parseTime(s string) time.Time {
	t, err := store.ParseTime(s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// load reads the tasks matching cond (a condition on tasks, with args) and
// everything their metrics need, in one read transaction.
func load(ctx context.Context, tx *sql.Tx, cond string, args []any, fallbackVersions map[string]string) (*data, error) {
	d := &data{tasks: map[int64]*taskRec{}, keys: map[string]int64{}, workflows: map[string]wfVer{}}
	if err := query(ctx, tx, `SELECT digest, name, version FROM task_workflow_images`, nil, func(r *sql.Rows) error {
		var v wfVer
		if err := r.Scan(&v.digest, &v.name, &v.version); err != nil {
			return err
		}
		d.workflows[v.digest] = v
		return nil
	}); err != nil {
		return nil, err
	}
	if err := query(ctx, tx, `SELECT id, task_key, title, status, customer, COALESCE(workflow_digest, ''), started_at, completed_at
		FROM tasks WHERE `+cond, args, func(r *sql.Rows) error {
		t := &taskRec{scores: map[string][]int64{}}
		if err := r.Scan(&t.id, &t.key, &t.title, &t.status, &t.customer, &t.wfDigest, &t.startedAt, &t.completedAt); err != nil {
			return err
		}
		d.tasks[t.id], d.keys[t.key] = t, t.id
		return nil
	}); err != nil {
		return nil, err
	}
	// The ids and keys go to the later queries as lists: cond can scan
	// ai_requests, which has no index by task, and must run once.
	ids, idArgs := inList(slices.Collect(maps.Keys(d.tasks)))
	if err := query(ctx, tx, `SELECT old_key, task_id FROM task_key_aliases WHERE task_id IN (`+ids+`)`, idArgs, func(r *sql.Rows) error {
		var key string
		var id int64
		if err := r.Scan(&key, &id); err != nil {
			return err
		}
		d.keys[key] = id
		return nil
	}); err != nil {
		return nil, err
	}
	keys, keyArgs := inList(slices.Collect(maps.Keys(d.keys)))
	steps := []struct {
		q    string
		args []any
		scan func(*sql.Rows) error
	}{
		{`SELECT task_id, status_id, outcome, workflow_digest, entered_at, left_at, rejected_requests FROM task_status_visits
			WHERE task_id IN (` + ids + `) ORDER BY task_id, sequence`, idArgs, func(r *sql.Rows) error {
			var id int64
			var v visit
			var entered, left string
			if err := r.Scan(&id, &v.status, &v.outcome, &v.digest, &entered, &left, &v.rejected); err != nil {
				return err
			}
			v.entered = parseTime(entered)
			if left != "" {
				v.left = parseTime(left)
			}
			d.tasks[id].visits = append(d.tasks[id].visits, v)
			return nil
		}},
		{`SELECT task_id, CASE WHEN kind IN ('task.created', 'task.imported') THEN COALESCE(json_extract(payload, '$.status'), 'open')
				ELSE json_extract(payload, '$.status') END, created_at
			FROM task_events WHERE task_id IN (` + ids + `) AND json_valid(payload)
				AND (kind IN ('task.created', 'task.imported') OR json_type(payload, '$.status') = 'text') ORDER BY sequence`, idArgs, func(r *sql.Rows) error {
			var id int64
			var c statusChange
			var at string
			if err := r.Scan(&id, &c.status, &at); err != nil {
				return err
			}
			c.at = parseTime(at)
			d.tasks[id].changes = append(d.tasks[id].changes, c)
			return nil
		}},
		{`SELECT w.task_id, COUNT(*) FROM task_waiting_for w JOIN tasks t ON t.id = w.task_id
			WHERE w.task_id IN (` + ids + `) AND w.expected_principal = t.customer GROUP BY w.task_id`, idArgs, func(r *sql.Rows) error {
			var id, n int64
			if err := r.Scan(&id, &n); err != nil {
				return err
			}
			d.tasks[id].questions = n
			return nil
		}},
		// Only the latest assessment of each iteration and task counts: a
		// reassessment replaces the earlier record (assessment.IsLatest).
		{`SELECT a.task_key, x.axis, x.score FROM assessments a JOIN assessment_axes x ON x.assessment_seq = a.seq
			WHERE x.verdict = 'scored' AND a.task_key IN (` + keys + `) AND ` + assessment.IsLatest, keyArgs, func(r *sql.Rows) error {
			var key, axis string
			var score int64
			if err := r.Scan(&key, &axis, &score); err != nil {
				return err
			}
			if t := d.task(key); t != nil {
				t.scores[axis] = append(t.scores[axis], score)
			}
			return nil
		}},
	}
	for _, s := range steps {
		if err := query(ctx, tx, s.q, s.args, s.scan); err != nil {
			return nil, err
		}
	}
	iters, err := loadIterations(ctx, tx, `a.task_id IN (`+keys+`)`, keyArgs, fallbackVersions)
	if err != nil {
		return nil, err
	}
	for _, it := range iters {
		if t := d.task(it.taskKey); t != nil {
			t.iters = append(t.iters, it)
		}
	}
	return d, nil
}

func (d *data) task(key string) *taskRec {
	if id, ok := d.keys[key]; ok {
		return d.tasks[id]
	}
	return nil
}

// inList returns the placeholders and the arguments of an IN list.
// shortcut: one argument per value, under SQLite's 32766 variable limit for
// the tasks one report period completes; batch the list if that grows.
func inList[T any](values []T) (string, []any) {
	args := make([]any, len(values))
	for i, v := range values {
		args[i] = v
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(values)), ","), args
}

func query(ctx context.Context, tx *sql.Tx, q string, args []any, scan func(*sql.Rows) error) error {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// loadFallbackVersions maps an image digest to the version an iterations
// row recorded for it: the version of an iteration whose row retention
// removed.
func loadFallbackVersions(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	out := map[string]string{}
	err := query(ctx, tx, `SELECT image_digest, MAX(image_version) FROM iterations
		WHERE image_digest <> '' AND image_version <> '' GROUP BY image_digest`, nil, func(r *sql.Rows) error {
		var digest, version string
		if err := r.Scan(&digest, &version); err != nil {
			return err
		}
		out[digest] = version
		return nil
	})
	return out, err
}

// loadIterations sums ai_requests matching cond per iteration, agent and task
// key, with the image version of the iteration.
func loadIterations(ctx context.Context, tx *sql.Tx, cond string, args []any, fallbackVersions map[string]string) ([]iterRow, error) {
	var out []iterRow
	err := query(ctx, tx, `SELECT a.iteration, a.agent, COALESCE(a.task_id, ''), MIN(a.ts),
			SUM(a.input_tokens), SUM(a.output_tokens), SUM(a.cache_write_tokens), SUM(a.cache_read_tokens), SUM(a.cost_usd),
			MAX(a.image_name), MAX(a.image_tag), MAX(a.image_digest),
			COALESCE(i.started_at, ''), COALESCE(i.image_ref, ''), COALESCE(i.image_version, ''), COALESCE(i.image_digest, '')
		FROM ai_requests a LEFT JOIN iterations i ON i.id = a.iteration
		WHERE `+cond+` GROUP BY a.iteration, a.agent, a.task_id`, args, func(r *sql.Rows) error {
		var it iterRow
		var firstTS, aiName, aiTag, aiDigest, started, ref, version, digest string
		if err := r.Scan(&it.iteration, &it.agent, &it.taskKey, &firstTS, &it.tokens.Input, &it.tokens.Output,
			&it.tokens.CacheWrite, &it.tokens.CacheRead, &it.cost, &aiName, &aiTag, &aiDigest, &started, &ref, &version, &digest); err != nil {
			return err
		}
		if ref == "" { // no iterations row: retention removed it
			ref, digest, version = aiName, aiDigest, ""
			if aiTag != "" {
				ref += ":" + aiTag
			}
		}
		if version == "" {
			version = cmp.Or(fallbackVersions[digest], UnknownVersion)
		}
		it.image = imageVer{name: imageName(ref), version: version, ref: ref, digest: digest}
		if started == "" {
			started = firstTS
		}
		it.start = parseTime(started)
		out = append(out, it)
		return nil
	})
	return out, err
}

// imageName strips the tag or digest from an image reference.
func imageName(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}
