CREATE TABLE agents (
    name           TEXT PRIMARY KEY,
    image_ref      TEXT    NOT NULL,
    image_digest   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    state          TEXT    NOT NULL DEFAULT 'stopped',
    cwd            TEXT    NOT NULL DEFAULT '',
    harness_type   TEXT    NOT NULL DEFAULT 'claude',
    model          TEXT    NOT NULL DEFAULT '',
    effort         TEXT    NOT NULL DEFAULT '',
    interactive    INTEGER NOT NULL DEFAULT 0,
    loop_enabled   INTEGER NOT NULL DEFAULT 1,
    interval_s     INTEGER NOT NULL DEFAULT 0,
    timeout_s      INTEGER NOT NULL DEFAULT 0,
    hard_timeout_s INTEGER NOT NULL DEFAULT 0,
    on_timeout     TEXT    NOT NULL DEFAULT 'restart',
    on_error       TEXT    NOT NULL DEFAULT 'restart',
    user_prompt    TEXT    NOT NULL DEFAULT '',
    env            TEXT    NOT NULL DEFAULT '{}',
    plugins        TEXT    NOT NULL DEFAULT '[]'
);

CREATE TABLE iterations (
    id          TEXT PRIMARY KEY,
    agent       TEXT    NOT NULL,
    trigger     TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    started_at  TEXT    NOT NULL,
    ended_at    TEXT    NOT NULL DEFAULT '',
    exit_code   INTEGER,
    done_flag   INTEGER NOT NULL DEFAULT 0,
    prompt_path TEXT    NOT NULL DEFAULT '',
    cpu_ms      INTEGER,
    mem_peak_kb INTEGER
);
CREATE INDEX idx_iterations_agent ON iterations(agent, started_at);

CREATE TABLE secrets (
    agent TEXT NOT NULL,
    key   TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (agent, key)
);
