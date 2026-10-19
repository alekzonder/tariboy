package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
)

const canonicalTimestampsMigration = "0066_canonical_timestamps.sql"

// Earlier migrations created these defaults with millisecond precision.
const (
	legacyDefaultSuffix    = "%fZ','now')"
	canonicalDefaultSuffix = "%f000000Z','now')"
)

// canonicalizeChunk bounds the rows one backfill read holds in memory.
var canonicalizeChunk = 10000

// canonicalizeTimestamps rewrites every registered time column to TimeLayout
// and changes the millisecond SQL defaults to SQLNow, in the migration's
// transaction. A value that is not a time stays as it is and is logged.
func canonicalizeTimestamps(tx *sql.Tx) error {
	tables := make([]string, 0, len(timeColumns))
	for table := range timeColumns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		for _, column := range timeColumns[table] {
			// A column a later migration adds is written canonically from the start.
			exists, err := migrationColumnExists(tx, table, column)
			if err != nil {
				return err
			}
			if exists {
				if err := CanonicalizeColumn(tx, table, column); err != nil {
					return err
				}
			}
		}
	}
	return CanonicalizeDefaults(tx)
}

// CanonicalizeColumn rewrites every RFC 3339 value of table.column to
// TimeLayout, in chunks of rowids so memory stays bounded on large tables.
// A value that is not a time stays as it is and is logged.
func CanonicalizeColumn(tx *sql.Tx, table, column string) error {
	read := fmt.Sprintf(`SELECT rowid, %q FROM %q WHERE rowid > ? AND %q IS NOT NULL AND %q <> '' ORDER BY rowid LIMIT ?`,
		column, table, column, column)
	update, err := tx.Prepare(fmt.Sprintf(`UPDATE %q SET %q = ? WHERE rowid = ?`, table, column))
	if err != nil {
		return fmt.Errorf("%s.%s: %w", table, column, err)
	}
	defer update.Close()
	type change struct {
		rowid int64
		value string
	}
	var after int64
	bad := 0
	for {
		rows, err := tx.Query(read, after, canonicalizeChunk)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", table, column, err)
		}
		var changes []change
		n := 0
		for rows.Next() {
			var rowid int64
			var value string
			if err := rows.Scan(&rowid, &value); err != nil {
				rows.Close()
				return fmt.Errorf("%s.%s: %w", table, column, err)
			}
			n++
			after = rowid
			if _, err := ParseTime(value); err == nil {
				continue
			}
			normalized, ok := NormalizeTimeValue(value)
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
		for _, c := range changes {
			if _, err := update.Exec(c.value, c.rowid); err != nil {
				return fmt.Errorf("%s.%s: %w", table, column, err)
			}
		}
		if n < canonicalizeChunk {
			break
		}
	}
	if bad > 0 {
		slog.Warn("stored values are not times; left unchanged", "table", table, "column", column, "rows", bad)
	}
	return nil
}

// CanonicalizeDefaults changes every millisecond time default to SQLNow by
// editing the DEFAULT text in sqlite_schema, the procedure SQLite documents
// for changes that leave the stored rows as they are, so no table needs a
// rebuild. Each edited table is then prepared again, which fails the
// transaction if the edited schema does not parse.
func CanonicalizeDefaults(tx *sql.Tx) error {
	var tables []string
	rows, err := tx.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' AND instr(sql, ?) > 0`, legacyDefaultSuffix)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}
	var version int
	if err := tx.QueryRow(`PRAGMA schema_version`).Scan(&version); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA writable_schema = ON`); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE sqlite_schema SET sql = replace(sql, ?, ?) WHERE type = 'table' AND instr(sql, ?) > 0`,
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
	for _, table := range tables {
		rows, err := tx.Query(fmt.Sprintf(`SELECT * FROM %q LIMIT 0`, table))
		if err != nil {
			return fmt.Errorf("default change broke table %s: %w", table, err)
		}
		rows.Close()
	}
	return nil
}
