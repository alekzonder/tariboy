package storesvc

import (
	"path/filepath"
	"testing"
)

func TestDBRecordAndHistory(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.RecordPush("demo", "latest", "aaa", "2026-07-06T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordPush("demo", "latest", "bbb", "2026-07-06T01:00:00Z"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.TagsFor("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("history len = %d, want 2 (immutable digests, movable tag)", len(rows))
	}
	if rows[0].Digest != "bbb" {
		t.Fatalf("newest first: got %s, want bbb", rows[0].Digest)
	}
	// Re-pushing an existing digest is idempotent (no duplicate row).
	if err := db.RecordPush("demo", "latest", "bbb", "2026-07-06T02:00:00Z"); err != nil {
		t.Fatal(err)
	}
	rows, _ = db.TagsFor("demo")
	if len(rows) != 2 {
		t.Fatalf("re-push of same digest must not add a row; got %d", len(rows))
	}
}
