package storesvc

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/store"
)

// legacyStoreSchema is the Store-service schema before 0002: the 0001 tables
// and a schema_migrations table, all with millisecond defaults.
var legacyStoreSchema = []string{
	`CREATE TABLE schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`,
	`CREATE TABLE blobs (
    name       TEXT NOT NULL,
    tag        TEXT NOT NULL,
    digest     TEXT NOT NULL,
    built_at   TEXT NOT NULL DEFAULT '',
    pushed_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (name, tag, digest)
)`,
	`CREATE INDEX idx_blobs_name ON blobs(name, tag, pushed_at)`,
	`CREATE TABLE tokens (
    token_sha256 TEXT PRIMARY KEY,
    scope        TEXT NOT NULL,
    label        TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
)`,
	`INSERT INTO schema_migrations(name, applied_at) VALUES ('0001_init.sql', '2026-07-06T00:00:00.123Z')`,
	`INSERT INTO blobs(name, tag, digest, built_at, pushed_at) VALUES ('demo', 'latest', 'aaa', '2026-07-06T03:00:00+03:00', '2026-07-06T01:02:03.456Z')`,
	`INSERT INTO blobs(name, tag, digest, built_at, pushed_at) VALUES ('demo', 'latest', 'bbb', '', '2026-07-06T01:02:03.000000001Z')`,
	`INSERT INTO tokens(token_sha256, scope, label, created_at) VALUES ('t1', 'push', '', '2026-07-06T04:05:06Z')`,
}

func TestOpenCanonicalizesLegacyTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range legacyStoreSchema {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	checks := []struct{ query, want string }{
		{`SELECT applied_at FROM schema_migrations WHERE name = '0001_init.sql'`, "2026-07-06T00:00:00.123000000Z"},
		{`SELECT built_at FROM blobs WHERE digest = 'aaa'`, "2026-07-06T00:00:00.000000000Z"},
		{`SELECT pushed_at FROM blobs WHERE digest = 'aaa'`, "2026-07-06T01:02:03.456000000Z"},
		{`SELECT built_at FROM blobs WHERE digest = 'bbb'`, ""},
		{`SELECT pushed_at FROM blobs WHERE digest = 'bbb'`, "2026-07-06T01:02:03.000000001Z"},
		{`SELECT created_at FROM tokens WHERE token_sha256 = 't1'`, "2026-07-06T04:05:06.000000000Z"},
	}
	for _, c := range checks {
		var got string
		if err := db.sql.QueryRow(c.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.query, err)
		}
		if got != c.want {
			t.Errorf("%s = %q, want %q", c.query, got, c.want)
		}
	}

	var legacy int
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE instr(sql, '%fZ''') > 0`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != 0 {
		t.Fatalf("%d schema entries keep millisecond defaults", legacy)
	}
	assertDefaultsCanonical(t, db)
}

func TestFreshDBDefaultsAreCanonical(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	assertDefaultsCanonical(t, db)
}

func assertDefaultsCanonical(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.sql.Exec(`INSERT INTO tokens(token_sha256, scope) VALUES ('fresh', 'pull')`); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordPush("fresh", "v1", "ccc", "2026-07-06T03:00:00+03:00"); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordPush("fresh", "v1", "ccc", "2026-07-06T03:00:00+03:00"); err != nil { // ON CONFLICT path
		t.Fatal(err)
	}
	values := map[string]string{}
	for name, query := range map[string]string{
		"tokens.created_at":            `SELECT created_at FROM tokens WHERE token_sha256 = 'fresh'`,
		"blobs.pushed_at":              `SELECT pushed_at FROM blobs WHERE digest = 'ccc'`,
		"blobs.built_at":               `SELECT built_at FROM blobs WHERE digest = 'ccc'`,
		"schema_migrations.applied_at": `SELECT applied_at FROM schema_migrations ORDER BY name DESC LIMIT 1`,
	} {
		var v string
		if err := db.sql.QueryRow(query).Scan(&v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		values[name] = v
	}
	for name, v := range values {
		if _, err := store.ParseTime(v); err != nil {
			t.Errorf("%s = %q is not canonical: %v", name, v, err)
		}
	}
	if got := values["blobs.built_at"]; got != "2026-07-06T00:00:00.000000000Z" {
		t.Errorf("built_at = %q, want UTC canonical", got)
	}
	var schema string
	if err := db.sql.QueryRow(`SELECT group_concat(sql, ' ') FROM sqlite_schema WHERE type = 'table'`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(schema, "%fZ'") {
		t.Errorf("schema keeps millisecond defaults: %s", schema)
	}
}
