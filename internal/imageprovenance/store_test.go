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
	record := Record{Ref: "demo:latest", Digest: "abc", SourceCWD: source, BuiltAt: time.Unix(1, 0).UTC().Format(time.RFC3339)}
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
