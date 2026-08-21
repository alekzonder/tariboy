// Package aiproxy is the in-daemon AI reverse proxy (spec §9): per-iteration
// attribution tokens, exact usage accounting, cost from a pricing table, a
// hot-path JSONL transcript with a background DB ingester, routing and budgets.
// store.go owns the ai_requests metadata table plus pricing/budget rows.
package aiproxy

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// roundCost rounds a USD cost sum to 6 decimal places to avoid float64
// summation drift (e.g. 0.10+0.20 landing on 0.30000000000000004 instead of
// the nearest representable 0.3) leaking into usage/budget comparisons.
func roundCost(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// nullStr maps an empty string to a SQL NULL so untagged task/epic columns
// store NULL rather than "" (spec: NULL when untagged).
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AIRequest is one proxied request's metadata (spec §9). Bodies are NOT stored
// here; they live in iterations/<id>/proxy-transcript.jsonl.
type AIRequest struct {
	ID               string
	TS               string // RFC3339Nano
	Agent            string
	Iteration        string
	ImageName        string
	ImageTag         string
	ImageDigest      string
	Provider         string // anthropic | openai
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheWriteTokens int
	CacheReadTokens  int
	CostUSD          float64
	LatencyMs        int
	Status           string // ok | budget_block | auth_error | upstream_error
	UpstreamStatus   int
	TaskID           string // native task key; "" when untagged
	EpicID           string // native top-level root key; "" when untagged
	GroupID          string // request-time group identifier; "" when ungrouped
	GroupName        string // request-time group display name; "" when ungrouped
}

type Store struct {
	db                   *sql.DB
	clock                func() time.Time
	reportProjectionHook func()
}

func NewStore(s *store.Store, clock func() time.Time) *Store {
	if clock == nil {
		clock = time.Now
	}
	return &Store{db: s.DB, clock: clock}
}

// setReportProjectionHook is a private test seam for forcing a concurrent WAL
// write after the report's first projection has established its read snapshot.
func (s *Store) setReportProjectionHook(hook func()) {
	s.reportProjectionHook = hook
}

// NewRequestID returns a random hex id for an ai_requests row. rand==nil uses
// crypto/rand (the production path); tests pass a deterministic reader.
func NewRequestID(r io.Reader) string {
	if r == nil {
		r = rand.Reader
	}
	var b [12]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		// Fall back to the clock-free unique-enough hex of a fresh crypto read.
		_, _ = io.ReadFull(rand.Reader, b[:])
	}
	return "air-" + hex.EncodeToString(b[:])
}

func (s *Store) Insert(r AIRequest) error { return s.InsertBatch([]AIRequest{r}) }

func (s *Store) InsertBatch(rs []AIRequest) error {
	if len(rs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO ai_requests
		(id, ts, agent, iteration, image_name, image_tag, image_digest, provider, model,
		 input_tokens, output_tokens, cache_write_tokens, cache_read_tokens,
		 cost_usd, latency_ms, status, upstream_status, task_id, epic_id, group_id, group_name)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range rs {
		if _, err := stmt.Exec(r.ID, r.TS, r.Agent, r.Iteration, r.ImageName, r.ImageTag,
			r.ImageDigest, r.Provider, r.Model, r.InputTokens, r.OutputTokens, r.CacheWriteTokens,
			r.CacheReadTokens, r.CostUSD, r.LatencyMs, r.Status, r.UpstreamStatus,
			nullStr(r.TaskID), nullStr(r.EpicID), nullStr(r.GroupID), nullStr(r.GroupName)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteAll() error {
	_, err := s.db.Exec(`DELETE FROM ai_requests`)
	return err
}

type UsageFilter struct {
	Agent string
	Image string
	Since string // RFC3339 (inclusive lower bound)
	Until string // RFC3339 (exclusive upper bound)
	Group string // group snapshot id; UngroupedFilter selects NULL snapshots
}

const UngroupedFilter = "__ungrouped__"

type UsageRow struct {
	Agent            string
	ImageName        string
	GroupID          string
	GroupName        string
	Requests         int
	InputTokens      int
	OutputTokens     int
	CacheWriteTokens int
	CacheReadTokens  int
	CostUSD          float64
}

// IterationUsageRow aggregates exactly one historical iteration. It is kept
// separate from UsageFilter because operator judge views must not accidentally
// include the judge agents' own requests.
type IterationUsageRow struct {
	Iteration                                                              string
	Requests, InputTokens, OutputTokens, CacheWriteTokens, CacheReadTokens int
	CostUSD                                                                float64
}

func (s *Store) AggregateIterations(ids []string) ([]IterationUsageRow, error) {
	if len(ids) == 0 {
		return []IterationUsageRow{}, nil
	}
	q := `SELECT iteration,COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(cache_write_tokens),0),COALESCE(SUM(cache_read_tokens),0),COALESCE(SUM(cost_usd),0) FROM ai_requests WHERE iteration IN (` + strings.TrimRight(strings.Repeat("?,", len(ids)), ",") + `) GROUP BY iteration ORDER BY iteration`
	args := make([]any, len(ids))
	for i := range ids {
		args[i] = ids[i]
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IterationUsageRow{}
	for rows.Next() {
		var x IterationUsageRow
		if err := rows.Scan(&x.Iteration, &x.Requests, &x.InputTokens, &x.OutputTokens, &x.CacheWriteTokens, &x.CacheReadTokens, &x.CostUSD); err != nil {
			return nil, err
		}
		x.CostUSD = roundCost(x.CostUSD)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (f UsageFilter) where() (string, []any) {
	var conds []string
	var args []any
	if f.Agent != "" {
		conds = append(conds, "agent=?")
		args = append(args, f.Agent)
	}
	if f.Image != "" {
		conds = append(conds, "image_name=?")
		args = append(args, f.Image)
	}
	if f.Group == UngroupedFilter {
		conds = append(conds, "group_id IS NULL")
	} else if f.Group != "" {
		conds = append(conds, "group_id=?")
		args = append(args, f.Group)
	}
	// Window bounds must compare chronologically, not lexically. ts is stored as
	// RFC3339Nano text whose fractional part is variable-width (trailing zeros
	// stripped), so a plain string compare against a bound the UI padded to a
	// different width admits/drops the wrong sub-second rows: '..:00.123456Z'
	// sorts before the bound '..:00.123Z' because '4' < 'Z', yet is
	// chronologically later. Normalizing both sides through strftime to a
	// fixed-width, offset-aware UTC millisecond form ('YYYY-MM-DDTHH:MM:SS.SSS')
	// makes the text compare equal the time compare — exact for the UI's
	// millisecond bounds, with no floating-point fuzz. (Series buckets via
	// strftime('%s', ts) for the same offset-aware reason.)
	const (
		tsColNorm = "strftime('%Y-%m-%dT%H:%M:%f',ts)"
		tsArgNorm = "strftime('%Y-%m-%dT%H:%M:%f',?)"
	)
	if f.Since != "" {
		conds = append(conds, tsColNorm+">="+tsArgNorm)
		args = append(args, f.Since)
	}
	if f.Until != "" {
		conds = append(conds, tsColNorm+"<"+tsArgNorm)
		args = append(args, f.Until)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

type rowQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

func (s *Store) Aggregate(f UsageFilter) ([]UsageRow, error) {
	return aggregate(s.db, f)
}

func aggregate(q rowQueryer, f UsageFilter) ([]UsageRow, error) {
	where, args := f.where()
	rows, err := q.Query(`SELECT agent, image_name, COALESCE(group_id,''), COALESCE(group_name,''), COUNT(*),
		COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		COALESCE(SUM(cache_write_tokens),0), COALESCE(SUM(cache_read_tokens),0),
		COALESCE(SUM(cost_usd),0)
		FROM ai_requests`+where+`
		GROUP BY agent, image_name, group_id, group_name
		ORDER BY agent, image_name, group_id, group_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageRow
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.Agent, &r.ImageName, &r.GroupID, &r.GroupName, &r.Requests, &r.InputTokens, &r.OutputTokens,
			&r.CacheWriteTokens, &r.CacheReadTokens, &r.CostUSD); err != nil {
			return nil, err
		}
		r.CostUSD = roundCost(r.CostUSD)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RequestRow is one recent historical request projected for server Usage.
type RequestRow struct {
	ID               string
	TS               string
	Agent            string
	ImageName        string
	Provider         string
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheWriteTokens int
	CacheReadTokens  int
	CostUSD          float64
	Status           string
	GroupID          string
	GroupName        string
}

const maxUsageRequests = 200

// Requests returns the newest matching request snapshots. The store owns the
// cap so every caller remains bounded even if it supplies a larger limit.
// Proxy and reindex rows use canonical UTC RFC3339Nano. SQLite date functions
// lose sub-millisecond precision, so the query orders their fixed seconds
// prefix and a right-zero-padded nanosecond fraction before applying LIMIT.
func (s *Store) Requests(f UsageFilter, limit int) ([]RequestRow, error) {
	return requests(s.db, f, limit)
}

func requests(q rowQueryer, f UsageFilter, limit int) ([]RequestRow, error) {
	if limit <= 0 || limit > maxUsageRequests {
		limit = maxUsageRequests
	}
	where, args := f.where()
	args = append(args, limit)
	rows, err := q.Query(`SELECT id, ts, agent, image_name, provider, model,
		input_tokens, output_tokens, cache_write_tokens, cache_read_tokens,
		cost_usd, status, COALESCE(group_id,''), COALESCE(group_name,'')
		FROM ai_requests`+where+`
		ORDER BY substr(ts,1,19) DESC,
			CASE WHEN substr(ts,20,1)='.'
				THEN substr(substr(ts,21,length(ts)-21)||'000000000',1,9)
				ELSE '000000000'
			END DESC,
			id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RequestRow{}
	for rows.Next() {
		var r RequestRow
		if err := rows.Scan(&r.ID, &r.TS, &r.Agent, &r.ImageName, &r.Provider, &r.Model,
			&r.InputTokens, &r.OutputTokens, &r.CacheWriteTokens, &r.CacheReadTokens,
			&r.CostUSD, &r.Status, &r.GroupID, &r.GroupName); err != nil {
			return nil, err
		}
		r.CostUSD = roundCost(r.CostUSD)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Report reads every server Usage projection from one SQLite transaction, so
// the aggregates, series, and recent requests share one WAL snapshot.
func (s *Store) Report(f UsageFilter, bucket string, limit int) ([]UsageRow, []SeriesRow, []RequestRow, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, nil, err
	}
	rollback := func(cause error) error {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(cause, fmt.Errorf("rollback usage report: %w", rollbackErr))
		}
		return cause
	}

	rows, err := aggregate(tx, f)
	if err != nil {
		return nil, nil, nil, rollback(err)
	}
	if s.reportProjectionHook != nil {
		s.reportProjectionHook()
	}
	seriesRows, err := series(tx, f, bucket)
	if err != nil {
		return nil, nil, nil, rollback(err)
	}
	requestRows, err := requests(tx, f, limit)
	if err != nil {
		return nil, nil, nil, rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, nil, rollback(err)
	}
	return rows, seriesRows, requestRows, nil
}

// ErrBadGroupBy / ErrBadBucket flag an unrecognized grouping key or series
// bucket so a REST layer can turn them into a 400 rather than a 500.
var (
	ErrBadGroupBy = errors.New("unknown group_by")
	ErrBadBucket  = errors.New("unknown bucket")
)

// GroupRow is one AggregateBy bucket: the grouped column value plus request
// count, the four token sums, and cost. Key is "" when the grouped column is
// SQL NULL (an untagged task/epic) — the caller decides how to label that.
type GroupRow struct {
	Key              string
	Requests         int
	InputTokens      int
	OutputTokens     int
	CacheWriteTokens int
	CacheReadTokens  int
	CostUSD          float64
}

// groupByColumn maps a public grouping key to its ai_requests column. The
// whitelist is the injection guard: only these four values ever reach the SQL.
func groupByColumn(groupBy string) (string, bool) {
	switch groupBy {
	case "iteration":
		return "iteration", true
	case "task":
		return "task_id", true
	case "epic":
		return "epic_id", true
	case "model":
		return "model", true
	}
	return "", false
}

// AggregateBy groups the filtered ai_requests by one column (iteration, task,
// epic, or model) and returns per-group request/token/cost sums. Task/epic keys
// come back as bare ids (or "" when NULL); title
// resolution is the caller's job.
func (s *Store) AggregateBy(f UsageFilter, groupBy string) ([]GroupRow, error) {
	col, ok := groupByColumn(groupBy)
	if !ok {
		return nil, ErrBadGroupBy
	}
	where, args := f.where()
	rows, err := s.db.Query(`SELECT COALESCE(`+col+`,''), COUNT(*),
		COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		COALESCE(SUM(cache_write_tokens),0), COALESCE(SUM(cache_read_tokens),0),
		COALESCE(SUM(cost_usd),0)
		FROM ai_requests`+where+`
		GROUP BY `+col+` ORDER BY `+col, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupRow
	for rows.Next() {
		var r GroupRow
		if err := rows.Scan(&r.Key, &r.Requests, &r.InputTokens, &r.OutputTokens,
			&r.CacheWriteTokens, &r.CacheReadTokens, &r.CostUSD); err != nil {
			return nil, err
		}
		r.CostUSD = roundCost(r.CostUSD)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SeriesRow is one time bucket: its UTC start (RFC3339), request count, total
// tokens (all four kinds summed), and cost.
type SeriesRow struct {
	BucketStart string
	Requests    int
	Tokens      int
	CostUSD     float64
}

// bucketSeconds maps a public bucket key to its width in seconds. Whitelisted so
// only a trusted integer is interpolated into the SQL time expression.
func bucketSeconds(bucket string) (int, bool) {
	switch bucket {
	case "5m":
		return 300, true
	case "15m":
		return 900, true
	case "1h":
		return 3600, true
	case "1d":
		return 86400, true
	}
	return 0, false
}

// Series buckets the filtered ai_requests into fixed UTC time windows. ts is
// stored as RFC3339Nano text; strftime('%s', ts) parses it (offset-aware) to a
// unix epoch, which we floor to the bucket width and re-format as a UTC
// RFC3339 bucket start. Rows come back ordered by bucket start.
func (s *Store) Series(f UsageFilter, bucket string) ([]SeriesRow, error) {
	return series(s.db, f, bucket)
}

func series(q rowQueryer, f UsageFilter, bucket string) ([]SeriesRow, error) {
	secs, ok := bucketSeconds(bucket)
	if !ok {
		return nil, ErrBadBucket
	}
	where, args := f.where()
	floor := fmt.Sprintf("(strftime('%%s', ts)/%d)*%d", secs, secs)
	rows, err := q.Query(`SELECT strftime('%Y-%m-%dT%H:%M:%SZ', `+floor+`, 'unixepoch'),
		COUNT(*),
		COALESCE(SUM(input_tokens+output_tokens+cache_write_tokens+cache_read_tokens),0),
		COALESCE(SUM(cost_usd),0)
		FROM ai_requests`+where+`
		GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeriesRow
	for rows.Next() {
		var r SeriesRow
		if err := rows.Scan(&r.BucketStart, &r.Requests, &r.Tokens, &r.CostUSD); err != nil {
			return nil, err
		}
		r.CostUSD = roundCost(r.CostUSD)
		out = append(out, r)
	}
	return out, rows.Err()
}

// IterationUsage sums the tokens and cost recorded for one iteration. Used
// best-effort as OTel span attributes (spec §14); the authoritative accounting
// view is Aggregate/usage.
func (s *Store) IterationUsage(iteration string) (inTok, outTok int, costUSD float64, err error) {
	row := s.db.QueryRow(`SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		COALESCE(SUM(cost_usd),0) FROM ai_requests WHERE iteration=?`, iteration)
	if err = row.Scan(&inTok, &outTok, &costUSD); err != nil {
		return 0, 0, 0, err
	}
	return inTok, outTok, costUSD, nil
}

// AgentGroups returns the agent->group map for every agent with a non-empty
// group. It is the group->members input the budget cache needs; the agents
// table lives in the same DB, so this is a single scoped read.
func (s *Store) AgentGroups() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT name, "group" FROM agents WHERE "group" != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, group string
		if err := rows.Scan(&name, &group); err != nil {
			return nil, err
		}
		out[name] = group
	}
	return out, rows.Err()
}

// CurrentGroup returns the request-time group snapshot for an agent. Groups
// currently use their name as both stable identifier and display name.
func (s *Store) CurrentGroup(agent string) (id, name string, err error) {
	var group string
	if err := s.db.QueryRow(`SELECT "group" FROM agents WHERE name=?`, agent).Scan(&group); err != nil {
		return "", "", err
	}
	return group, group, nil
}

// GroupCostSince sums cost_usd over a group's members since a cutoff. An empty
// member list sums to zero (no rows), never an error.
func (s *Store) GroupCostSince(members []string, since time.Time) (float64, error) {
	if len(members) == 0 {
		return 0, nil
	}
	q := `SELECT COALESCE(SUM(cost_usd),0) FROM ai_requests WHERE ts>=? AND agent IN (`
	args := []any{since.UTC().Format(time.RFC3339Nano)}
	for i, m := range members {
		if i > 0 {
			q += ","
		}
		q += "?"
		args = append(args, m)
	}
	q += ")"
	var c float64
	err := s.db.QueryRow(q, args...).Scan(&c)
	return roundCost(c), err
}

// CostSince sums cost_usd for an agent (empty = all agents) since a cutoff.
func (s *Store) CostSince(agent string, since time.Time) (float64, error) {
	// ts is variable-width RFC3339Nano text, so comparing it directly to a
	// whole-second calendar boundary misorders fractional timestamps (the dot
	// sorts before the boundary's trailing Z). Normalize both sides through
	// SQLite's offset-aware time parser before comparing.
	q := `SELECT COALESCE(SUM(cost_usd),0) FROM ai_requests
		WHERE strftime('%Y-%m-%dT%H:%M:%f',ts)>=strftime('%Y-%m-%dT%H:%M:%f',?)`
	args := []any{since.UTC().Format(time.RFC3339Nano)}
	if agent != "" {
		q += ` AND agent=?`
		args = append(args, agent)
	}
	var c float64
	err := s.db.QueryRow(q, args...).Scan(&c)
	return roundCost(c), err
}
