// Package retention enforces per-agent data retention (spec §12): the nightly
// maintenance run prunes iteration dirs (and their DB rows) beyond a policy of
// keep-N-iterations / keep-N-days / max-bytes, archiving each pruned
// iteration with its DB rows to tar.gz first.
package retention

import (
	"database/sql"
	"encoding/json"

	"github.com/alekzonder/tariboy/internal/store"
)

// Policy is a retention rule. A zero field means "unlimited" in the daemon
// default and "inherit the default" in a per-agent policy.
type Policy struct {
	KeepIterations int   `json:"keep_iterations"`
	KeepDays       int   `json:"keep_days"`
	MaxBytes       int64 `json:"max_bytes"`
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

// Default returns the daemon-wide policy. Unset -> zero policy (unlimited).
func (s *Store) Default() (Policy, error) {
	v, ok, err := s.cfg.ConfigGet(defaultConfigKey)
	if err != nil || !ok {
		return Policy{}, err
	}
	var p Policy
	if err := json.Unmarshal([]byte(v), &p); err != nil {
		return Policy{}, err
	}
	return p, nil
}

func (s *Store) Set(agent string, p Policy) error {
	_, err := s.db.Exec(`INSERT INTO retention_policies
		(agent, keep_iterations, keep_days, max_bytes) VALUES (?,?,?,?)
		ON CONFLICT(agent) DO UPDATE SET
			keep_iterations=excluded.keep_iterations, keep_days=excluded.keep_days,
			max_bytes=excluded.max_bytes`,
		agent, p.KeepIterations, p.KeepDays, p.MaxBytes)
	return err
}

func (s *Store) Get(agent string) (Policy, bool, error) {
	var p Policy
	err := s.db.QueryRow(`SELECT keep_iterations, keep_days, max_bytes
		FROM retention_policies WHERE agent=?`, agent).
		Scan(&p.KeepIterations, &p.KeepDays, &p.MaxBytes)
	if err == sql.ErrNoRows {
		return Policy{}, false, nil
	}
	if err != nil {
		return Policy{}, false, err
	}
	return p, true, nil
}

func (s *Store) Delete(agent string) error {
	_, err := s.db.Exec(`DELETE FROM retention_policies WHERE agent=?`, agent)
	return err
}

// Effective returns the daemon default with each non-zero per-agent field
// overriding it.
func (s *Store) Effective(agent string) (Policy, error) {
	p, err := s.Default()
	if err != nil {
		return Policy{}, err
	}
	own, _, err := s.Get(agent)
	if err != nil {
		return Policy{}, err
	}
	if own.KeepIterations != 0 {
		p.KeepIterations = own.KeepIterations
	}
	if own.KeepDays != 0 {
		p.KeepDays = own.KeepDays
	}
	if own.MaxBytes != 0 {
		p.MaxBytes = own.MaxBytes
	}
	return p, nil
}
