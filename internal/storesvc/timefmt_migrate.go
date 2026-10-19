package storesvc

import (
	"database/sql"

	"github.com/alekzonder/tariboy/internal/store"
)

const canonicalTimestampsMigration = "0002_canonical_timestamps.sql"

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
// transaction, with the same steps as the main store's 0066 migration.
func canonicalizeTimestamps(tx *sql.Tx) error {
	for _, c := range timeColumns {
		if err := store.CanonicalizeColumn(tx, c.table, c.column); err != nil {
			return err
		}
	}
	return store.CanonicalizeDefaults(tx)
}
