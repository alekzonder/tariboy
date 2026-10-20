package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCanonicalTimestampsMigration(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "canonical-timestamps.db")
	legacy := openBeforeMigration(t, path, canonicalTimestampsMigration)
	if _, err := legacy.DB.Exec(`
		INSERT INTO events(id, ts, kind) VALUES
			(1, '2026-10-19T06:33:58.123Z', 'millis'),
			(2, '2026-10-19T06:33:58.00000001Z', 'nano'),
			(3, '2026-10-19T09:33:58.5+03:00', 'offset'),
			(4, 'now', 'garbage');
		INSERT INTO ai_requests(id, ts) VALUES ('r1', '2026-10-19T06:33:58Z');
		INSERT INTO iterations(id, agent, trigger, status, started_at, ended_at, timeout_deadline)
			VALUES ('i1', 'a', 'manual', 'done', '2026-10-19T06:33:58.000000011Z', '', NULL);`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	want := map[int]string{
		1: "2026-10-19T06:33:58.123000000Z",
		2: "2026-10-19T06:33:58.000000010Z",
		3: "2026-10-19T06:33:58.500000000Z",
		4: "now", // not a time: left unchanged
	}
	for id, ts := range want {
		var got string
		if err := s.DB.QueryRow(`SELECT ts FROM events WHERE id = ?`, id).Scan(&got); err != nil || got != ts {
			t.Fatalf("event %d ts = %q, %v; want %q", id, got, err, ts)
		}
	}
	var ts, started, ended string
	var deadline sql.NullString
	if err := s.DB.QueryRow(`SELECT ts FROM ai_requests WHERE id = 'r1'`).Scan(&ts); err != nil || ts != "2026-10-19T06:33:58.000000000Z" {
		t.Fatalf("ai_requests ts = %q, %v", ts, err)
	}
	if err := s.DB.QueryRow(`SELECT started_at, ended_at, timeout_deadline FROM iterations WHERE id = 'i1'`).Scan(&started, &ended, &deadline); err != nil ||
		started != "2026-10-19T06:33:58.000000011Z" || ended != "" || deadline.Valid {
		t.Fatalf("iteration times = %q %q %v, %v", started, ended, deadline, err)
	}

	// SQL defaults write the canonical format too.
	if _, err := s.DB.Exec(`INSERT INTO channels(name, kind) VALUES ('c', 'topic')`); err != nil {
		t.Fatal(err)
	}
	var created string
	if err := s.DB.QueryRow(`SELECT created_at FROM channels WHERE name = 'c'`).Scan(&created); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTime(created); err != nil {
		t.Fatalf("default created_at = %q is not canonical", created)
	}
	var check string
	if err := s.DB.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatalf("integrity_check = %q, %v", check, err)
	}
}

func TestCanonicalizeColumnCrossesChunkBoundaries(t *testing.T) {
	// Sequential: swaps the package-level canonicalizeChunk.
	defer func(chunk int) { canonicalizeChunk = chunk }(canonicalizeChunk)
	canonicalizeChunk = 2
	s := open(t)
	if _, err := s.DB.Exec(`INSERT INTO events(id, ts, kind) VALUES
		(1, '2026-10-19T06:33:58Z', 'a'), (2, '', 'b'), (3, 'now', 'c'),
		(4, '2026-10-19T06:33:59.1Z', 'd'), (5, '2026-10-19T06:34:00.000000000Z', 'e')`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := CanonicalizeColumn(tx, "events", "ts"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	want := map[int]string{
		1: "2026-10-19T06:33:58.000000000Z", 2: "", 3: "now",
		4: "2026-10-19T06:33:59.100000000Z", 5: "2026-10-19T06:34:00.000000000Z",
	}
	for id, ts := range want {
		var got string
		if err := s.DB.QueryRow(`SELECT ts FROM events WHERE id = ?`, id).Scan(&got); err != nil || got != ts {
			t.Fatalf("event %d ts = %q, %v; want %q", id, got, err, ts)
		}
	}
}

func TestSchemaDefaultsAreCanonical(t *testing.T) {
	t.Parallel()
	s := open(t)
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE instr(sql, '%fZ''') > 0`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("schema objects with a millisecond default = %d, %v; want 0", n, err)
	}
	var applied string
	if err := s.DB.QueryRow(`SELECT applied_at FROM schema_migrations ORDER BY name DESC LIMIT 1`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTime(applied); err != nil {
		t.Fatalf("applied_at = %q is not canonical", applied)
	}
}
