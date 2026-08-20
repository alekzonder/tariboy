-- External plugins (spec §7/§12): installed plugins survive a daemon restart so
-- the daemon re-launches every enabled one. Bodies (exec, logs) live in files
-- under <base>/plugins/<name>/; this table is the metadata + lifecycle state.

CREATE TABLE plugins (
    name             TEXT PRIMARY KEY,
    version          TEXT NOT NULL DEFAULT '',
    types            TEXT NOT NULL DEFAULT '[]',   -- json array
    protocol_version INTEGER NOT NULL DEFAULT 0,
    exec             TEXT NOT NULL DEFAULT '',
    source_path      TEXT NOT NULL DEFAULT '',
    channels         TEXT NOT NULL DEFAULT '{}',   -- json {publish:[],subscribe:[]}
    enabled          INTEGER NOT NULL DEFAULT 1,
    installed_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    state            TEXT NOT NULL DEFAULT 'installed', -- installed|running|unhealthy|stopped
    health           TEXT NOT NULL DEFAULT '{}'
);
