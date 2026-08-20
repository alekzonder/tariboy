-- Idle-autostop restart boundary (idle-autostop epic, lpq.8).
-- `idle_reset_rowid` records the newest iteration rowid seen at the last
-- Start/restart. IdleStreak counts only iterations with rowid greater than this
-- boundary, so a Start grants a fresh max_idle_iterations budget instead of
-- re-tripping on the historical idle streak. 0 (the default) means "no boundary
-- yet" — every iteration counts, matching the pre-boundary behaviour on first run.
-- SQL-internal: read by IdleStreak and written by StartResetIdle only; it does
-- not round-trip through the Agent struct.
ALTER TABLE agents ADD COLUMN idle_reset_rowid INTEGER NOT NULL DEFAULT 0;
