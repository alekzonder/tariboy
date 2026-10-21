package store

import (
	"database/sql"
	"embed"

	"github.com/alekzonder/tariboy/internal/migrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func (s *Store) migrate() error {
	return migrate.Run(s.DB, migrationsFS, "migrations", map[string]migrate.Step{
		"canonical_timestamps": canonicalizeTimestamps,
	})
}

func migrationColumnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// SchemaVersion returns the number of applied migrations.
func (s *Store) SchemaVersion() (int, error) {
	var n int
	err := s.Reader().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n)
	return n, err
}
