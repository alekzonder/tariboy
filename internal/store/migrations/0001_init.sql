CREATE TABLE daemon_config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE events (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    ts    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    agent TEXT    NOT NULL DEFAULT '',
    kind  TEXT    NOT NULL,
    data  TEXT    NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_events_agent_ts ON events(agent, ts);
