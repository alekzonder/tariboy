-- 0002_canonical_timestamps.sql -- one stored time format (store.TimeLayout).
-- The work runs in Go (canonicalizeTimestamps in timefmt_migrate.go): it
-- rewrites every time column to fixed-width UTC with nine fractional digits
-- and changes the millisecond SQL defaults to the same format. SQLite cannot
-- parse every RFC 3339 variant into that layout, so this file is empty.
SELECT 1;
