package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// This package cannot import storetest, which imports it, so it keeps its own
// copy of the template: the first fresh database in a test binary runs every
// migration through Open, and later ones copy that file.
var (
	templateOnce  sync.Once
	templateBytes []byte
	templateErr   error
)

func migratedTemplate() ([]byte, error) {
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "store-template-")
		if err != nil {
			templateErr = err
			return
		}
		defer os.RemoveAll(dir)
		s, err := Open(filepath.Join(dir, "template.db"))
		if err != nil {
			templateErr = fmt.Errorf("build template database: %w", err)
			return
		}
		out := filepath.Join(dir, "out.db")
		err = s.BackupTo(out)
		if closeErr := s.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			templateErr = fmt.Errorf("copy template database: %w", err)
			return
		}
		templateBytes, templateErr = os.ReadFile(out)
	})
	return templateBytes, templateErr
}

// openFresh returns a migrated database copied from the template into a new
// temporary directory. It is closed when the test ends.
func openFresh(t testing.TB) *Store {
	t.Helper()
	b, err := migratedTemplate()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// A test database is thrown away, so commits need not wait for fsync.
	if _, err := s.DB.Exec(`PRAGMA synchronous = OFF`); err != nil {
		t.Fatal(err)
	}
	return s
}
