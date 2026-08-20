// Package plugins is the external plugin host (spec §7): tariboyd owns the
// lifecycle of executable plugins installed under <base>/plugins/<name>/. It
// starts, supervises (health/backoff/graceful stop), and drains the two
// bus-facing plugin types (channel-source publishes in, channel-sink delivers
// out). store.go owns the persisted plugins table so enabled plugins are
// re-launched after a restart.
package plugins

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Record is one installed plugin's persisted metadata + lifecycle state.
type Record struct {
	Name            string
	Version         string
	Types           []string
	ProtocolVersion int
	Exec            string
	SourcePath      string
	Channels        Channels
	Enabled         bool
	InstalledAt     string
	State           string // installed | running | unhealthy | stopped
	Health          string // json blob
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

func (s *Store) Upsert(r Record) error {
	types, _ := json.Marshal(r.Types)
	chans, _ := json.Marshal(r.Channels)
	installed := r.InstalledAt
	if installed == "" {
		installed = s.clock().UTC().Format(time.RFC3339Nano)
	}
	if r.State == "" {
		r.State = "installed"
	}
	if r.Health == "" {
		r.Health = "{}"
	}
	_, err := s.db.Exec(`INSERT INTO plugins
		(name, version, types, protocol_version, exec, source_path, channels, enabled, installed_at, state, health)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET
			version=excluded.version, types=excluded.types, protocol_version=excluded.protocol_version,
			exec=excluded.exec, source_path=excluded.source_path, channels=excluded.channels,
			enabled=excluded.enabled, state=excluded.state, health=excluded.health`,
		r.Name, r.Version, string(types), r.ProtocolVersion, r.Exec, r.SourcePath, string(chans),
		boolToInt(r.Enabled), installed, r.State, r.Health)
	return err
}

func (s *Store) Get(name string) (Record, bool, error) {
	row := s.db.QueryRow(`SELECT name, version, types, protocol_version, exec, source_path,
		channels, enabled, installed_at, state, health FROM plugins WHERE name=?`, name)
	r, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return r, true, nil
}

func (s *Store) List() ([]Record, error) {
	rows, err := s.db.Query(`SELECT name, version, types, protocol_version, exec, source_path,
		channels, enabled, installed_at, state, health FROM plugins ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SetEnabled(name string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE plugins SET enabled=? WHERE name=?`, boolToInt(enabled), name)
	return err
}

func (s *Store) SetActiveVersion(name, version string) error {
	if version == "" {
		return fmt.Errorf("plugin %s has empty version", name)
	}
	_, err := s.db.Exec(`INSERT INTO plugin_active_versions(name,version) VALUES(?,?)
		ON CONFLICT(name) DO UPDATE SET version=excluded.version`, name, version)
	return err
}

func (s *Store) ActiveVersion(name string) (string, bool, error) {
	var version string
	err := s.db.QueryRow(`SELECT version FROM plugin_active_versions WHERE name=?`, name).Scan(&version)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return version, true, nil
}

func (s *Store) ClearActiveVersion(name string) error {
	_, err := s.db.Exec(`DELETE FROM plugin_active_versions WHERE name=?`, name)
	return err
}

func (s *Store) SetState(name, state, healthJSON string) error {
	if healthJSON == "" {
		healthJSON = "{}"
	}
	_, err := s.db.Exec(`UPDATE plugins SET state=?, health=? WHERE name=?`, state, healthJSON, name)
	return err
}

func (s *Store) Delete(name string) error {
	_, err := s.db.Exec(`DELETE FROM plugins WHERE name=?`, name)
	return err
}

type rowScanner interface{ Scan(dest ...any) error }

func scanRecord(row rowScanner) (Record, error) {
	var r Record
	var types, chans string
	var enabled int
	if err := row.Scan(&r.Name, &r.Version, &types, &r.ProtocolVersion, &r.Exec, &r.SourcePath,
		&chans, &enabled, &r.InstalledAt, &r.State, &r.Health); err != nil {
		return Record{}, err
	}
	if err := json.Unmarshal([]byte(types), &r.Types); err != nil {
		return Record{}, fmt.Errorf("plugin %s: bad types json: %w", r.Name, err)
	}
	if err := json.Unmarshal([]byte(chans), &r.Channels); err != nil {
		return Record{}, fmt.Errorf("plugin %s: bad channels json: %w", r.Name, err)
	}
	r.Enabled = enabled != 0
	return r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
