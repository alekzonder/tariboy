package storetest_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

func migrationCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("../migrations")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func TestOpenGivesIndependentMigratedDatabases(t *testing.T) {
	a := storetest.Open(t)
	b := storetest.Open(t)
	for _, s := range []*store.Store{a, b} {
		n, err := s.SchemaVersion()
		if err != nil {
			t.Fatal(err)
		}
		if n != migrationCount(t) {
			t.Fatalf("schema version = %d, want %d", n, migrationCount(t))
		}
	}
	if err := a.ConfigSet("k", "a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := b.ConfigGet("k"); err != nil || ok {
		t.Fatalf("second database sees the first one's write: ok=%v err=%v", ok, err)
	}
}

func TestSeedWritesAMigratedFileBeforeOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tariboyd.db")
	storetest.Seed(t, path)

	// Read the seeded file without store.Open, which would migrate it.
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != migrationCount(t) {
		t.Fatalf("seeded migrations = %d, want %d", n, migrationCount(t))
	}
}
