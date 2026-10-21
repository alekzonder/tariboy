package store

import (
	"strings"
	"testing"
)

// TestSchemaMigrationsDefaultIsSQLNow guards the copy of SQLNow in package
// migrate, which cannot import store.
func TestSchemaMigrationsDefaultIsSQLNow(t *testing.T) {
	t.Parallel()
	s := open(t)
	var ddl string
	if err := s.DB.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'schema_migrations'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "DEFAULT ("+SQLNow+")") {
		t.Fatalf("schema_migrations DDL %q does not default applied_at to SQLNow", ddl)
	}
}
