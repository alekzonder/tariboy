// Package store owns the daemon database. The daemon is the single writer.
package store

import (
	"database/sql"
	"net/url"

	_ "modernc.org/sqlite"
)

// Store holds the single writer connection (DB) and a small pool of read-only
// connections (Reader). WAL mode lets those readers run while a write is open.
type Store struct {
	DB   *sql.DB
	read *sql.DB
}

// readPoolSize is the number of read-only connections for API reads.
const readPoolSize = 4

func Open(path string) (*Store, error) {
	dsn := "file:" + url.PathEscape(path) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single writer; serialize access to the file
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	read, err := sql.Open("sqlite", "file:"+url.PathEscape(path)+"?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)")
	if err != nil {
		db.Close()
		return nil, err
	}
	read.SetMaxOpenConns(readPoolSize)
	s.read = read
	return s, nil
}

// Reader is the read-only pool for queries that never write, such as the API
// list reads the UI polls. A Store built without Open reads through DB.
func (s *Store) Reader() *sql.DB {
	if s.read != nil {
		return s.read
	}
	return s.DB
}

func (s *Store) Close() error {
	if s.read != nil {
		s.read.Close()
	}
	return s.DB.Close()
}

func (s *Store) ConfigGet(key string) (string, bool, error) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM daemon_config WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) ConfigSet(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO daemon_config(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) AddEvent(agent, kind, dataJSON string) error {
	if dataJSON == "" {
		dataJSON = "{}"
	}
	_, err := s.DB.Exec(`INSERT INTO events(agent, kind, data) VALUES (?, ?, ?)`, agent, kind, dataJSON)
	return err
}
