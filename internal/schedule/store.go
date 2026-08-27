package schedule

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// seqWidth is the zero-pad width of the numeric suffix in a schedule id,
// chosen so ids sort lexicographically in (ts, seq) order.
const seqWidth = 9

var ErrNotFound = errors.New("not found")

// dbtx is the subset of *sql.DB and *sql.Tx used by nextScheduleID so it can
// run either directly or inside a transaction.
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

type Schedule struct {
	ID              string
	Agent           string
	Kind            string // oneshot | cron
	Spec            string
	Channel         string
	MessageTemplate string // JSON: {type, subject, text, data}
	NextFireAt      string // RFC3339
	Enabled         bool
	CreatedAt       string
	// CorrelationID tags a request-deadline one-shot (spec §4.2) with the
	// request's correlation id so a reply landing first can cancel exactly that
	// entry via CancelByCorrelation. Empty for ordinary schedules.
	CorrelationID string
}

type Store struct {
	db    *sql.DB
	clock func() time.Time
}

func NewStore(s *store.Store, clock func() time.Time) *Store {
	if clock == nil {
		clock = time.Now
	}
	return &Store{db: s.DB, clock: clock}
}

// Add validates Kind/Spec, computes the initial next_fire_at and inserts.
func (s *Store) Add(sch Schedule) (Schedule, error) {
	now := s.clock().UTC()
	// The id and the INSERT must be computed inside the same tx so the
	// sequence lookup is race-safe under the store's single-connection mode
	// (store.Open calls SetMaxOpenConns(1)): see nextScheduleID.
	tx, err := s.db.Begin()
	if err != nil {
		return Schedule{}, err
	}
	defer tx.Rollback()
	sch, err = add(tx, sch, now)
	if err != nil {
		return Schedule{}, err
	}
	if err := tx.Commit(); err != nil {
		return Schedule{}, err
	}
	return sch, nil
}

func (s *Store) AddTx(tx *sql.Tx, sch Schedule) (Schedule, error) {
	return add(tx, sch, s.clock().UTC())
}

func add(tx *sql.Tx, sch Schedule, now time.Time) (Schedule, error) {
	next, err := computeNext(sch.Kind, sch.Spec, now)
	if err != nil {
		return Schedule{}, err
	}
	if sch.MessageTemplate == "" {
		sch.MessageTemplate = "{}"
	}
	sch.NextFireAt = next
	sch.Enabled = true
	sch.CreatedAt = now.Format(time.RFC3339)
	sch.ID, err = nextScheduleID(tx, sch.Agent, now)
	if err != nil {
		return Schedule{}, err
	}
	_, err = tx.Exec(`INSERT INTO schedules
		(id, agent, kind, spec, channel, message_template, next_fire_at, enabled, created_at, correlation_id)
		VALUES (?,?,?,?,?,?,?,1,?,?)`,
		sch.ID, sch.Agent, sch.Kind, sch.Spec, sch.Channel, sch.MessageTemplate, sch.NextFireAt, sch.CreatedAt,
		nullable(sch.CorrelationID))
	return sch, err
}

// nextScheduleID derives a collision-safe, (ts, seq)-sortable id of the form
// sch-<agent>-<yyyymmddhhmmss>-<zero-padded seq>. The seq is MAX(existing seq
// for this agent+second)+1, computed inside the caller's tx. This mirrors
// bus.nextMessageID: using MAX (not COUNT) means pruning the oldest rows can
// never make a future seq collide with a survivor, and matching the prefix
// with substr (not LIKE) means agent names containing '_'/'%' cannot
// over-match.
func nextScheduleID(x dbtx, agent string, now time.Time) (string, error) {
	prefix := "sch-" + agent + "-" + now.UTC().Format("20060102150405") + "-"
	var maxSeq int64
	if err := x.QueryRow(`SELECT COALESCE(MAX(CAST(substr(id, ?) AS INTEGER)), 0)
		FROM schedules WHERE agent = ? AND substr(id, 1, ?) = ?`,
		len(prefix)+1, agent, len(prefix), prefix).Scan(&maxSeq); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%0*d", prefix, seqWidth, maxSeq+1), nil
}

func (s *Store) List(agent string) ([]Schedule, error) {
	rows, err := s.db.Query(`SELECT id, agent, kind, spec, channel, message_template, next_fire_at, enabled, created_at
		FROM schedules WHERE agent=? ORDER BY created_at, id`, agent)
	if err != nil {
		return nil, err
	}
	return scanSchedules(rows)
}

// CancelByCorrelation deletes every schedule tagged with correlationID (the
// request-deadline one-shot armed by Request, spec §4.2). Idempotent: a reply
// landing on a request that never had a deadline — or after the timeout already
// fired and disabled the row — finds nothing to delete and returns nil, so the
// reply cancellation is always safe. An empty correlationID matches nothing.
func (s *Store) CancelByCorrelation(correlationID string) error {
	return cancelByCorrelation(s.db, correlationID)
}

func (s *Store) CancelByCorrelationTx(tx *sql.Tx, correlationID string) error {
	return cancelByCorrelation(tx, correlationID)
}

func cancelByCorrelation(x dbtx, correlationID string) error {
	if correlationID == "" {
		return nil
	}
	_, err := x.Exec(`DELETE FROM schedules WHERE correlation_id=?`, correlationID)
	return err
}

// Cancel deletes a schedule, scoped to its owning agent so one agent can never
// cancel another agent's schedule (mirrors bus.Unsubscribe agent-scoping).
func (s *Store) Cancel(agent, id string) error {
	res, err := s.db.Exec(`DELETE FROM schedules WHERE id=? AND agent=?`, id, agent)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DueBefore returns enabled schedules whose next_fire_at is <= t.
func (s *Store) DueBefore(t time.Time) ([]Schedule, error) {
	rows, err := s.db.Query(`SELECT id, agent, kind, spec, channel, message_template, next_fire_at, enabled, created_at
		FROM schedules WHERE enabled=1 AND next_fire_at<=? ORDER BY next_fire_at, id`,
		t.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return scanSchedules(rows)
}

// MarkFired persists the post-firing state: for cron, the recomputed
// next_fire_at (still enabled); for oneshot, enabled=0.
func (s *Store) MarkFired(sch Schedule) error {
	_, err := s.db.Exec(`UPDATE schedules SET next_fire_at=?, enabled=? WHERE id=?`,
		sch.NextFireAt, boolToInt(sch.Enabled), sch.ID)
	return err
}

// computeNext derives the first firing at/after now for a new schedule.
func computeNext(kind, spec string, now time.Time) (string, error) {
	switch kind {
	case "oneshot":
		t, err := time.Parse(time.RFC3339, spec)
		if err != nil {
			return "", fmt.Errorf("oneshot spec must be RFC3339: %w", err)
		}
		return t.UTC().Format(time.RFC3339), nil
	case "cron":
		cr, err := Parse(spec)
		if err != nil {
			return "", err
		}
		next, ok := cr.Next(now) // first firing strictly after now
		if !ok {
			return "", fmt.Errorf("cron %q never fires", spec)
		}
		return next.UTC().Format(time.RFC3339), nil
	default:
		return "", fmt.Errorf("schedule kind must be oneshot|cron, got %q", kind)
	}
}

// NextAfter recomputes a cron schedule's next firing (used by the scheduler).
func NextAfter(spec string, after time.Time) (string, bool) {
	cr, err := Parse(spec)
	if err != nil {
		return "", false
	}
	next, ok := cr.Next(after)
	if !ok {
		return "", false
	}
	return next.UTC().Format(time.RFC3339), true
}

func scanSchedules(rows *sql.Rows) ([]Schedule, error) {
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var sch Schedule
		var enabled int
		if err := rows.Scan(&sch.ID, &sch.Agent, &sch.Kind, &sch.Spec, &sch.Channel,
			&sch.MessageTemplate, &sch.NextFireAt, &enabled, &sch.CreatedAt); err != nil {
			return nil, err
		}
		sch.Enabled = enabled != 0
		out = append(out, sch)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullable stores an empty string as SQL NULL so the correlation_id index holds
// only real ids (ordinary schedules stay NULL, not "").
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
