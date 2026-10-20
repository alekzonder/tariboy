// Package storetest gives tests a migrated daemon database without running
// every migration again for each test. The first call in a test binary opens a
// database with store.Open, so the real migration code builds the template;
// later calls copy the template file.
package storetest

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/store"
)

var (
	templateOnce  sync.Once
	templateBytes []byte
	templateErr   error
)

func template() ([]byte, error) {
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "storetest-")
		if err != nil {
			templateErr = err
			return
		}
		defer os.RemoveAll(dir)
		s, err := store.Open(filepath.Join(dir, "template.db"))
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

// Seed writes a migrated database to path, which must not exist yet. Use it
// when the code under test opens the database itself, such as a daemon.
func Seed(t testing.TB, path string) {
	t.Helper()
	b, err := template()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// Open returns a migrated database in a new temporary directory. It is closed
// when the test ends.
func Open(t testing.TB) *store.Store {
	t.Helper()
	return OpenAt(t, filepath.Join(t.TempDir(), "tariboyd.db"))
}

// OpenAt seeds path and opens it. It is closed when the test ends.
func OpenAt(t testing.TB, path string) *store.Store {
	t.Helper()
	Seed(t, path)
	s, err := store.Open(path)
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
