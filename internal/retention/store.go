// Package retention enforces per-agent data retention (spec §12): a background
// daemon task prunes iteration dirs (and their DB rows) beyond a policy of
// keep-N-iterations / keep-N-days / max-bytes, archiving to tar.gz by default.
package retention

import (
	"database/sql"
	"encoding/json"

	"github.com/alekzonder/tariboy/internal/store"
)

// Policy is a retention rule. Zero fields mean "unlimited / inherit"; Archive
// controls whether a pruned iteration dir is tar.gz'd before deletion.
type Policy struct {
	KeepIterations int   `json:"keep_iterations"`
	KeepDays       int   `json:"keep_days"`
	MaxBytes       int64 `json:"max_bytes"`
	Archive        bool  `json:"archive"`
}

const defaultConfigKey = "retention_default"

type Store struct {
	db  *sql.DB
	cfg *store.Store
}

func NewStore(s *store.Store) *Store { return &Store{db: s.DB, cfg: s} }

func (s *Store) SetDefault(p Policy) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.cfg.ConfigSet(defaultConfigKey, string(b))
}

// Default returns the daemon-wide policy. Unset -> zero policy with Archive
// true (unlimited retention, archive-on).
func (s *Store) Default() (Policy, error) {
	v, ok, err := s.cfg.ConfigGet(defaultConfigKey)
	if err != nil {
		return Policy{}, err
	}
	if !ok {
		return Policy{Archive: true}, nil
	}
	var p Policy
	if err := json.Unmarshal([]byte(v), &p); err != nil {
		return Policy{}, err
	}
	return p, nil
}

func (s *Store) Set(agent string, p Policy) error {
	_, err := s.db.Exec(`INSERT INTO retention_policies
		(agent, keep_iterations, keep_days, max_bytes, archive) VALUES (?,?,?,?,?)
		ON CONFLICT(agent) DO UPDATE SET
			keep_iterations=excluded.keep_iterations, keep_days=excluded.keep_days,
			max_bytes=excluded.max_bytes, archive=excluded.archive`,
		agent, p.KeepIterations, p.KeepDays, p.MaxBytes, b2i(p.Archive))
	return err
}

func (s *Store) Get(agent string) (Policy, bool, error) {
	var p Policy
	var arch int
	err := s.db.QueryRow(`SELECT keep_iterations, keep_days, max_bytes, archive
		FROM retention_policies WHERE agent=?`, agent).
		Scan(&p.KeepIterations, &p.KeepDays, &p.MaxBytes, &arch)
	if err == sql.ErrNoRows {
		return Policy{}, false, nil
	}
	if err != nil {
		return Policy{}, false, err
	}
	p.Archive = arch != 0
	return p, true, nil
}

func (s *Store) Delete(agent string) error {
	_, err := s.db.Exec(`DELETE FROM retention_policies WHERE agent=?`, agent)
	return err
}

// Effective returns the per-agent policy if set, else the daemon default.
func (s *Store) Effective(agent string) (Policy, error) {
	p, ok, err := s.Get(agent)
	if err != nil {
		return Policy{}, err
	}
	if ok {
		return p, nil
	}
	return s.Default()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
