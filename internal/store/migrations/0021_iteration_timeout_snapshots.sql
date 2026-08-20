-- Running-iteration timeout extension: per-iteration snapshots are intentionally
-- nullable so rows created by older daemon versions remain readable without a
-- backfill. New running iterations are initialized by the runner immediately
-- before the shim is spawned.
ALTER TABLE iterations ADD COLUMN timeout_period_s INTEGER;
ALTER TABLE iterations ADD COLUMN timeout_deadline TEXT;
ALTER TABLE iterations ADD COLUMN hard_timeout_deadline TEXT;
ALTER TABLE iterations ADD COLUMN timeout_extensions INTEGER NOT NULL DEFAULT 0;
ALTER TABLE iterations ADD COLUMN timeout_triggered_at TEXT;
