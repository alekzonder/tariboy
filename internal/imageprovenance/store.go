package imageprovenance

import (
	"database/sql"
	"os"
)

type Record struct {
	Ref             string `json:"ref"`
	Digest          string `json:"digest"`
	SourceCWD       string `json:"source_cwd"`
	BuiltAt         string `json:"built_at"`
	SourceAvailable bool   `json:"source_available"`
}

type Store struct{ DB *sql.DB }

func (s Store) Upsert(record Record) error {
	return upsert(s.DB, record)
}

func (s Store) UpsertTx(tx *sql.Tx, record Record) error {
	return upsert(tx, record)
}

type executor interface {
	Exec(string, ...any) (sql.Result, error)
}

func upsert(exec executor, record Record) error {
	_, err := exec.Exec(`INSERT INTO image_provenance(ref,digest,source_cwd,built_at) VALUES(?,?,?,?)
		ON CONFLICT(ref) DO UPDATE SET digest=excluded.digest, source_cwd=excluded.source_cwd, built_at=excluded.built_at`, record.Ref, record.Digest, record.SourceCWD, record.BuiltAt)
	return err
}

func (s Store) Get(ref string) (Record, bool, error) {
	var record Record
	err := s.DB.QueryRow(`SELECT ref,digest,source_cwd,built_at FROM image_provenance WHERE ref=?`, ref).Scan(&record.Ref, &record.Digest, &record.SourceCWD, &record.BuiltAt)
	if err == sql.ErrNoRows {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	info, statErr := os.Stat(record.SourceCWD)
	record.SourceAvailable = statErr == nil && info.IsDir()
	return record, true, nil
}

func (s Store) Delete(ref string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM image_provenance WHERE ref=?`, ref); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM image_source_snapshots WHERE image_ref=?`, ref); err != nil {
		return err
	}
	return tx.Commit()
}

// IsCommitted requires the provenance and source-snapshot rows written by the
// ordinary build's single metadata transaction to agree on one generation.
func (s Store) IsCommitted(ref, digest string) (bool, error) {
	var committed bool
	err := s.DB.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM image_provenance p JOIN image_source_snapshots s ON s.image_ref=p.ref AND s.image_digest=p.digest
		WHERE p.ref=? AND p.digest=?)`, ref, digest).Scan(&committed)
	return committed, err
}
