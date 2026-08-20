-- Scripts are durable execution records. Rebuild because SQLite cannot remove
-- the legacy UNIQUE(agent, name) constraint in place.
CREATE TABLE scripts_new (
    id               TEXT PRIMARY KEY,
    agent            TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    command          TEXT NOT NULL,
    mode             TEXT NOT NULL CHECK(mode IN ('once', 'every')),
    interval_seconds INTEGER,
    status           TEXT NOT NULL,
    pid              INTEGER,
    last_exit        INTEGER,
    created_at       TEXT NOT NULL,
    last_started_at  TEXT,
    last_finished_at TEXT,
    next_run_at      TEXT,
    log_path         TEXT NOT NULL
);
INSERT INTO scripts_new (id, agent, name, command, mode, status, created_at, log_path)
    SELECT id, agent, name, body, 'once', 'done', created_at, '' FROM scripts;
DROP TABLE scripts;
ALTER TABLE scripts_new RENAME TO scripts;
CREATE INDEX idx_scripts_due ON scripts(status, next_run_at);
CREATE INDEX idx_scripts_agent_created ON scripts(agent, created_at DESC, id DESC);
