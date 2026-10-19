package imageprovenance

import (
	"path/filepath"
	"testing"
	"time"

	storedb "github.com/alekzonder/tariboy/internal/store"
)

func TestProvenanceRoundTripAndDynamicAvailability(t *testing.T) {
	db, err := storedb.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db.DB}
	source := t.TempDir()
	record := Record{Ref: "demo:latest", Digest: "abc", SourceCWD: source, BuiltAt: storedb.FormatTime(time.Unix(1, 0))}
	if err := s.Upsert(record); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get(record.Ref)
	if err != nil || !ok || got.Ref != record.Ref || got.Digest != record.Digest || got.SourceCWD != record.SourceCWD || got.BuiltAt != record.BuiltAt || !got.SourceAvailable {
		t.Fatalf("Get = %#v, %v, %v", got, ok, err)
	}
	if _, ok, err := s.Get("imported:latest"); err != nil || ok {
		t.Fatalf("missing = %v, %v", ok, err)
	}
}

func TestProvenanceStoresBuiltAtCanonical(t *testing.T) {
	db, err := storedb.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db.DB}
	if err := s.Upsert(Record{Ref: "old:latest", Digest: "abc", BuiltAt: "2026-01-02T05:04:05+03:00"}); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Get("old:latest")
	if err != nil || got.BuiltAt != "2026-01-02T02:04:05.000000000Z" {
		t.Fatalf("built_at = %q, %v; want canonical UTC", got.BuiltAt, err)
	}
}
