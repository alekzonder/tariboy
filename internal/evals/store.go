// Package evals is the daemon-side eval execution subsystem (spec §7.3/§8).
// store.go owns the persisted eval_results table; runner.go (Task 3) owns the
// post-iteration execution via the eval plugin type.
package evals

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Result is one eval verdict, attributed to the exact iteration + image version.
type Result struct {
	ID          string
	Iteration   string
	Agent       string
	ImageName   string
	ImageTag    string
	ImageDigest string
	EvalName    string
	EvalType    string
	Verdict     string // pass | fail | error
	Score       float64
	Detail      string
	CreatedAt   string
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

// Insert upserts a result keyed by (iteration, image_digest, eval_name): re-running
// the same eval on the same iteration+image version replaces the prior verdict.
func (s *Store) Insert(r Result) error {
	if r.CreatedAt == "" {
		r.CreatedAt = s.clock().UTC().Format(time.RFC3339Nano)
	}
	if r.ID == "" {
		r.ID = newID(nil)
	}
	_, err := s.db.Exec(`INSERT INTO eval_results
		(id, iteration, agent, image_name, image_tag, image_digest, eval_name, eval_type, verdict, score, detail, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(iteration, image_digest, eval_name) DO UPDATE SET
			verdict=excluded.verdict, score=excluded.score, detail=excluded.detail,
			eval_type=excluded.eval_type, agent=excluded.agent, created_at=excluded.created_at`,
		r.ID, r.Iteration, r.Agent, r.ImageName, r.ImageTag, r.ImageDigest,
		r.EvalName, r.EvalType, r.Verdict, r.Score, r.Detail, r.CreatedAt)
	return err
}

func (s *Store) ListByIteration(iteration string) ([]Result, error) {
	rows, err := s.db.Query(`SELECT id, iteration, agent, image_name, image_tag, image_digest,
		eval_name, eval_type, verdict, score, detail, created_at
		FROM eval_results WHERE iteration=? ORDER BY eval_name`, iteration)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func (s *Store) List(limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, iteration, agent, image_name, image_tag, image_digest,
		eval_name, eval_type, verdict, score, detail, created_at
		FROM eval_results ORDER BY created_at DESC, iteration DESC, eval_name LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func (s *Store) Get(iteration, imageDigest, evalName string) (Result, bool, error) {
	row := s.db.QueryRow(`SELECT id, iteration, agent, image_name, image_tag, image_digest,
		eval_name, eval_type, verdict, score, detail, created_at
		FROM eval_results WHERE iteration=? AND image_digest=? AND eval_name=?`,
		iteration, imageDigest, evalName)
	r, err := scanResult(row)
	if err == sql.ErrNoRows {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	return r, true, nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanResult(row rowScanner) (Result, error) {
	var r Result
	err := row.Scan(&r.ID, &r.Iteration, &r.Agent, &r.ImageName, &r.ImageTag, &r.ImageDigest,
		&r.EvalName, &r.EvalType, &r.Verdict, &r.Score, &r.Detail, &r.CreatedAt)
	return r, err
}

func scanResults(rows *sql.Rows) ([]Result, error) {
	var out []Result
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// newID is a random 16-byte hex id (surrogate primary key; the real dedup key is
// the (iteration, image_digest, eval_name) unique index).
func newID(r io.Reader) string {
	if r == nil {
		r = rand.Reader
	}
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
