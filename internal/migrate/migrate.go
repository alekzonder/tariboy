// Package migrate applies embedded SQL migrations to a SQLite database. Both
// the daemon store and the Store service database use it.
//
// Each *.sql file in dir runs once, in name order, in its own transaction, and
// its name is recorded in schema_migrations. Flags come from consecutive
// header lines at the very top of a file:
//
//	-- tariboy: foreign_keys=off
//	-- tariboy: skip_if_column=TABLE.COLUMN
//	-- tariboy: step=NAME
//
// foreign_keys=off disables enforcement outside the transaction and checks
// pragma_foreign_key_check before commit. skip_if_column skips the body when
// the column exists and still records the name. step runs steps[NAME] after
// the body in the same transaction. An unknown key or step is an error.
package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Step is Go migration work run in the migration's transaction.
type Step func(tx *sql.Tx) error

// sqlNow is store.SQLNow; store cannot be imported here (store imports this
// package), and a store test checks that the two agree.
const sqlNow = "strftime('%Y-%m-%dT%H:%M:%f000000Z','now')"

const headerPrefix = "-- tariboy:"

type header struct {
	foreignKeysOff bool
	skipTable      string
	skipColumn     string
	step           Step
}

// Run applies every migration in dir of files that schema_migrations does not
// list yet.
func Run(db *sql.DB, files fs.FS, dir string, steps map[string]Step) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (` + sqlNow + `))`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := fs.ReadFile(files, path.Join(dir, name))
		if err != nil {
			return err
		}
		h, err := parseHeader(string(body), steps)
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err := apply(db, name, string(body), h); err != nil {
			return err
		}
	}
	return nil
}

func parseHeader(body string, steps map[string]Step) (header, error) {
	var h header
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, headerPrefix) {
			break
		}
		key, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, headerPrefix)), "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok {
			return header{}, fmt.Errorf("header %q is not key=value", line)
		}
		switch key {
		case "foreign_keys":
			if value != "off" {
				return header{}, fmt.Errorf("header foreign_keys=%q: only off is supported", value)
			}
			h.foreignKeysOff = true
		case "skip_if_column":
			table, column, ok := strings.Cut(value, ".")
			if !ok || table == "" || column == "" {
				return header{}, fmt.Errorf("header skip_if_column=%q is not TABLE.COLUMN", value)
			}
			h.skipTable, h.skipColumn = table, column
		case "step":
			step, ok := steps[value]
			if !ok {
				return header{}, fmt.Errorf("unknown step %q", value)
			}
			h.step = step
		default:
			return header{}, fmt.Errorf("unknown header key %q", key)
		}
	}
	return h, nil
}

func apply(db *sql.DB, name, body string, h header) (resultErr error) {
	// A table rebuild with populated inbound foreign keys cannot drop the
	// parent while enforcement is enabled, even with deferred checks. Disable
	// enforcement outside the transaction, then validate the rebuilt graph
	// before allowing it to commit.
	if h.foreignKeysOff {
		if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("migration %s disable foreign keys: %w", name, err)
		}
		defer func() {
			if _, err := db.Exec(`PRAGMA foreign_keys = ON`); resultErr == nil && err != nil {
				resultErr = fmt.Errorf("migration %s restore foreign keys: %w", name, err)
			}
		}()
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	skipBody := false
	if h.skipTable != "" {
		exists, err := columnExists(tx, h.skipTable, h.skipColumn)
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
	if h.step != nil {
		if err := h.step(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	if h.foreignKeysOff {
		var table string
		err := tx.QueryRow(`SELECT "table" FROM pragma_foreign_key_check LIMIT 1`).Scan(&table)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
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
	return tx.Commit()
}

func columnExists(tx *sql.Tx, table, column string) (bool, error) {
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
