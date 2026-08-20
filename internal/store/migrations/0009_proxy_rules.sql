-- Proxy policy rules (spec §9): an ordered engine evaluated per proxied AI
-- request. kind ∈ rate-limit | model-policy. allow/deny are JSON arrays of model
-- globs. Rules are evaluated in ascending (priority, id) order.
CREATE TABLE proxy_rules (
    id           TEXT PRIMARY KEY,
    priority     INTEGER NOT NULL DEFAULT 0,
    scope        TEXT NOT NULL DEFAULT 'global',   -- global | agent:<name> | group:<g>
    model_glob   TEXT NOT NULL DEFAULT '',         -- '' = any model
    kind         TEXT NOT NULL DEFAULT '',         -- rate-limit | model-policy
    max_requests INTEGER NOT NULL DEFAULT 0,
    max_tokens   INTEGER NOT NULL DEFAULT 0,
    window_s     INTEGER NOT NULL DEFAULT 0,
    allow        TEXT NOT NULL DEFAULT '[]',
    deny         TEXT NOT NULL DEFAULT '[]',
    route        TEXT NOT NULL DEFAULT '',
    enabled      INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_proxy_rules_order ON proxy_rules(priority, id);
