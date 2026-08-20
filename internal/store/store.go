// Package store owns the daemon database. The daemon is the single writer.
package store

import (
	"database/sql"
	"net/url"

	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

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
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

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
