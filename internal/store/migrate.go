package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func (s *Store) migrate() error {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if err := s.applyMigration(name, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(name, body string) (resultErr error) {
	// Task table rebuilds have populated inbound foreign keys.
	// SQLite cannot drop that parent table while enforcement is enabled, even
	// with deferred checks. Disable enforcement outside the transaction, then
	// validate the rebuilt graph before allowing it to commit.
	rebuildTasks := name == "0025_task_priority.sql" || name == "0037_agent_goals.sql"
	if rebuildTasks {
		if _, err := s.DB.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("migration %s disable foreign keys: %w", name, err)
		}
		defer func() {
			if _, err := s.DB.Exec(`PRAGMA foreign_keys = ON`); resultErr == nil && err != nil {
				resultErr = fmt.Errorf("migration %s restore foreign keys: %w", name, err)
			}
		}()
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	skipBody := false
	if name == "0027_task_assignment_iteration.sql" {
		exists, err := migrationColumnExists(tx, "task_assignments", "lease_iteration")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s inspect column: %w", name, err)
		}
		skipBody = exists
	}
	if !skipBody {
		if _, err := tx.Exec(body); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	if rebuildTasks {
		var table string
		err := tx.QueryRow(`SELECT "table" FROM pragma_foreign_key_check LIMIT 1`).Scan(&table)
		if err != nil && err != sql.ErrNoRows {
			tx.Rollback()
			return fmt.Errorf("migration %s foreign key check: %w", name, err)
		}
		if err == nil {
			tx.Rollback()
			return fmt.Errorf("migration %s foreign key check failed for table %s", name, table)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(name) VALUES (?)`, name); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
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
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n)
	return n, err
}
