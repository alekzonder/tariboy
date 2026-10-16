package store

import (
	"path/filepath"
	"testing"
	"time"
)

// A read through the read pool must not wait for the single writer
// connection: the API polls must keep answering while a write runs.
func TestReadPoolDoesNotWaitForTheWriter(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ConfigSet("k", "v1"); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin() // holds the only writer connection
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE daemon_config SET value = 'v2' WHERE key = 'k'`); err != nil {
		t.Fatal(err)
	}

	done := make(chan string, 1)
	go func() {
		var v string
		_ = s.Reader().QueryRow(`SELECT value FROM daemon_config WHERE key = 'k'`).Scan(&v)
		done <- v
	}()
	select {
	case v := <-done:
		if v != "v1" {
			t.Fatalf("read = %q, want the committed v1", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read pool waited for the writer connection")
	}
}

// The read pool is read-only, so a handler cannot write through it by mistake.
func TestReadPoolRejectsWrites(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Reader().Exec(`INSERT INTO daemon_config(key, value) VALUES ('a', 'b')`); err == nil {
		t.Fatal("write through the read pool succeeded")
	}
}

// A Store built around one handle, as tests do, reads through that handle.
func TestReaderFallsBackToDB(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if bare := (&Store{DB: s.DB}); bare.Reader() != s.DB {
		t.Fatal("Reader of a bare Store is not its DB")
	}
}
