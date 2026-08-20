package storesvc

import (
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB is the store-local catalog/token database. Separate from the daemon store
// (internal/store): it must NOT link the daemon's daemon-specific migrations.
type DB struct{ sql *sql.DB }

// Open mirrors internal/store.Open's DSN/pragmas (WAL, busy_timeout, single
// writer) but embeds this package's own migrations.
func Open(path string) (*DB, error) {
	dsn := "file:" + url.PathEscape(path) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sdb.SetMaxOpenConns(1) // single writer; serialize access to the file
	db := &DB{sql: sdb}
	if err := db.migrate(); err != nil {
		sdb.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) migrate() error {
	if _, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := d.sql.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := d.sql.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// PushRow is one row of a tag's digest history.
type PushRow struct {
	Tag      string `json:"tag"`
	Digest   string `json:"digest"`
	BuiltAt  string `json:"built_at"`
	PushedAt string `json:"pushed_at"`
}

// RecordPush appends a (name,tag,digest) history row. Re-pushing an existing
// digest refreshes its timestamp rather than duplicating (idempotent).
func (d *DB) RecordPush(name, tag, digest, builtAt string) error {
	_, err := d.sql.Exec(`INSERT INTO blobs(name, tag, digest, built_at) VALUES (?,?,?,?)
		ON CONFLICT(name, tag, digest) DO UPDATE SET
			pushed_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'),
			built_at  = excluded.built_at`, name, tag, digest, builtAt)
	return err
}

// TagsFor returns every history row for a repo, ordered by tag then newest push.
// pushed_at has millisecond resolution, so back-to-back pushes can tie; rowid
// (monotonically increasing insertion order, preserved across the idempotent
// ON CONFLICT UPDATE in RecordPush) breaks ties deterministically.
func (d *DB) TagsFor(name string) ([]PushRow, error) {
	rows, err := d.sql.Query(`SELECT tag, digest, built_at, pushed_at FROM blobs
		WHERE name = ? ORDER BY tag ASC, pushed_at DESC, rowid DESC`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushRow
	for rows.Next() {
		var r PushRow
		if err := rows.Scan(&r.Tag, &r.Digest, &r.BuiltAt, &r.PushedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
