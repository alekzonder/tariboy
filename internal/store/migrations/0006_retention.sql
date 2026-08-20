-- Per-agent retention policy (spec §12): the daemon prunes old iteration dirs
-- and their DB rows beyond this policy. A side-table keyed by agent; the
-- daemon-wide default lives in daemon_config (key retention_default). A missing
-- row means "use the daemon default"; a zero field means "unlimited".

CREATE TABLE retention_policies (
    agent           TEXT PRIMARY KEY,
    keep_iterations INTEGER NOT NULL DEFAULT 0,
    keep_days       INTEGER NOT NULL DEFAULT 0,
    max_bytes       INTEGER NOT NULL DEFAULT 0,
    archive         INTEGER NOT NULL DEFAULT 1
);
