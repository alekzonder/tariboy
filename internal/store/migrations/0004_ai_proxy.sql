-- AI proxy (spec §9): per-request usage metadata (bodies live in files),
-- a configurable pricing table, and budgets.

CREATE TABLE ai_requests (
    id                 TEXT PRIMARY KEY,
    ts                 TEXT NOT NULL,
    agent              TEXT NOT NULL DEFAULT '',
    iteration          TEXT NOT NULL DEFAULT '',
    image_name         TEXT NOT NULL DEFAULT '',
    image_tag          TEXT NOT NULL DEFAULT '',
    image_digest       TEXT NOT NULL DEFAULT '',
    provider           TEXT NOT NULL DEFAULT '',
    model              TEXT NOT NULL DEFAULT '',
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cost_usd           REAL NOT NULL DEFAULT 0,
    latency_ms         INTEGER NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT '',
    upstream_status    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_ai_requests_agent ON ai_requests(agent, ts);
CREATE INDEX idx_ai_requests_iteration ON ai_requests(iteration);
CREATE INDEX idx_ai_requests_ts ON ai_requests(ts);

CREATE TABLE ai_pricing (
    model                TEXT PRIMARY KEY,
    input_per_mtok       REAL NOT NULL DEFAULT 0,
    output_per_mtok      REAL NOT NULL DEFAULT 0,
    cache_write_per_mtok REAL NOT NULL DEFAULT 0,
    cache_read_per_mtok  REAL NOT NULL DEFAULT 0
);

CREATE TABLE budgets (
    scope      TEXT PRIMARY KEY,   -- agent:<name> | group:<g> | global
    limit_usd  REAL NOT NULL DEFAULT 0,
    period_s   INTEGER NOT NULL DEFAULT 86400,
    mode       TEXT NOT NULL DEFAULT 'warn',  -- warn | block
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
