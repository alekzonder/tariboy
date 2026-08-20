// Package groups is the daemon-side group entity + membership reconciler
// (spec §4). store.go owns the persisted groups table; provisioner.go
// (Task 3/4) owns the derived channels, shared dir, and subscriptions.
package groups

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/store"
)

// ErrInvalidName marks a group name that fails agent.ValidName (the shared
// path-traversal guard).
var ErrInvalidName = errors.New("invalid group name")

// Group is one collaboration unit. Channel names and the shared-dir path are
// derived from Name (see provisioner.go), not stored.
type Group struct {
	Name      string
	Lead      string
	CreatedAt string
	Settings  map[string]any
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

func (s *Store) Upsert(g Group) error {
	if !agent.ValidName(g.Name) {
		return fmt.Errorf("%w %q: must match ^[a-z0-9][a-z0-9_-]*$", ErrInvalidName, g.Name)
	}
	settings := "{}"
	if len(g.Settings) > 0 {
		b, err := json.Marshal(g.Settings)
		if err != nil {
			return err
		}
		settings = string(b)
	}
	created := g.CreatedAt
	if created == "" {
		created = s.clock().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.Exec(`INSERT INTO groups(name, lead, created_at, settings)
		VALUES (?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET lead=excluded.lead, settings=excluded.settings`,
		g.Name, g.Lead, created, settings)
	return err
}

func (s *Store) Get(name string) (Group, bool, error) {
	row := s.db.QueryRow(`SELECT name, lead, created_at, settings FROM groups WHERE name=?`, name)
	g, err := scanGroup(row)
	if err == sql.ErrNoRows {
		return Group{}, false, nil
	}
	if err != nil {
		return Group{}, false, err
	}
	return g, true, nil
}

func (s *Store) List() ([]Group, error) {
	rows, err := s.db.Query(`SELECT name, lead, created_at, settings FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) SetLead(name, lead string) error {
	_, err := s.db.Exec(`UPDATE groups SET lead=? WHERE name=?`, lead, name)
	return err
}

func (s *Store) Delete(name string) error {
	_, err := s.db.Exec(`DELETE FROM groups WHERE name=?`, name)
	return err
}

type rowScanner interface{ Scan(dest ...any) error }

func scanGroup(row rowScanner) (Group, error) {
	var g Group
	var settings string
	if err := row.Scan(&g.Name, &g.Lead, &g.CreatedAt, &settings); err != nil {
		return Group{}, err
	}
	if settings != "" && settings != "{}" {
		_ = json.Unmarshal([]byte(settings), &g.Settings)
	}
	return g, nil
}
