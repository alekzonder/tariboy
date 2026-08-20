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
	_, err := s.DB.Exec(`INSERT INTO image_provenance(ref,digest,source_cwd,built_at) VALUES(?,?,?,?)
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
	_, err := s.DB.Exec(`DELETE FROM image_provenance WHERE ref=?`, ref)
	return err
}
