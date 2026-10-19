package storesvc

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/alekzonder/tariboy/internal/store"
)

const canonicalTimestampsMigration = "0002_canonical_timestamps.sql"

// 0001 created these defaults with millisecond precision.
const (
	legacyDefaultSuffix    = "%fZ','now')"
	canonicalDefaultSuffix = "%f000000Z','now')"
)

// timeColumns lists every column of the Store-service database that holds a
// time in store.TimeLayout.
var timeColumns = []struct{ table, column string }{
	{"blobs", "built_at"},
	{"blobs", "pushed_at"},
	{"schema_migrations", "applied_at"},
	{"tokens", "created_at"},
}

// canonicalizeTimestamps rewrites every time column to store.TimeLayout and
// changes the millisecond SQL defaults to store.SQLNow, in the migration's
// transaction. It mirrors the main store's 0066 migration. A value that is
// not a time stays as it is and is logged.
func canonicalizeTimestamps(tx *sql.Tx) error {
	for _, c := range timeColumns {
		if err := canonicalizeColumn(tx, c.table, c.column); err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.column, err)
		}
	}
	return canonicalizeDefaults(tx)
}

func canonicalizeColumn(tx *sql.Tx, table, column string) error {
	rows, err := tx.Query(fmt.Sprintf(`SELECT rowid, %q FROM %q WHERE %q IS NOT NULL AND %q <> ''`, column, table, column, column))
	if err != nil {
		return err
	}
	type change struct {
		rowid int64
		value string
	}
	var changes []change
	bad := 0
	for rows.Next() {
		var rowid int64
		var value string
		if err := rows.Scan(&rowid, &value); err != nil {
			rows.Close()
			return err
		}
		if _, err := store.ParseTime(value); err == nil {
			continue
		}
		normalized, ok := store.NormalizeTimeValue(value)
		if !ok {
			bad++
			continue
		}
		changes = append(changes, change{rowid, normalized})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if bad > 0 {
		slog.Warn("stored values are not times; left unchanged", "table", table, "column", column, "rows", bad)
	}
	if len(changes) == 0 {
		return nil
	}
	update, err := tx.Prepare(fmt.Sprintf(`UPDATE %q SET %q = ? WHERE rowid = ?`, table, column))
	if err != nil {
		return err
	}
	defer update.Close()
	for _, c := range changes {
		if _, err := update.Exec(c.value, c.rowid); err != nil {
			return err
		}
	}
	return nil
}

// canonicalizeDefaults edits the DEFAULT text in sqlite_schema, the procedure
// SQLite documents for changes that leave the stored rows as they are, so no
// table needs a rebuild. integrity_check guards the edited schema.
func canonicalizeDefaults(tx *sql.Tx) error {
	var version int
	if err := tx.QueryRow(`PRAGMA schema_version`).Scan(&version); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA writable_schema = ON`); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE sqlite_schema SET sql = replace(sql, ?, ?) WHERE type = 'table' AND instr(sql, ?) > 0`,
		legacyDefaultSuffix, canonicalDefaultSuffix, legacyDefaultSuffix)
	if err == nil {
		_, err = tx.Exec(fmt.Sprintf(`PRAGMA schema_version = %d`, version+1))
	}
	if _, offErr := tx.Exec(`PRAGMA writable_schema = OFF`); err == nil {
		err = offErr
	}
	if err != nil {
		return err
	}
	var check string
	if err := tx.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return fmt.Errorf("integrity check after default change: %s", check)
	}
	return nil
}
