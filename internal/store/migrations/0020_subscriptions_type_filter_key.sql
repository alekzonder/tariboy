-- Phase R review finding F2: include type_filter in the subscriptions dedup key.
-- The R2 compose subscribe object form lets a config declare two subscribes to the
-- same channel+matcher differing only by their type globs. The old table-level
-- UNIQUE(agent, channel, matcher) (migration 0003) collapsed the second onto the
-- first, silently dropping its type filter. Widen the key to include type_filter so
-- both take effect.
--
-- SQLite can't drop a table-level constraint in place, so rebuild the table. There
-- are no inbound foreign keys to subscriptions (deliveries.subscription_id has no
-- REFERENCES clause), so the drop/rename is safe. NULL type_filter values remain
-- distinct in the UNIQUE index; the code-level dedup (SELECT ... type_filter IS ?)
-- is the real guard for the bare/NULL case, with this constraint the DB backstop
-- for distinct non-null filters.
CREATE TABLE subscriptions_new (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,
    channel     TEXT NOT NULL,
    matcher     TEXT NOT NULL DEFAULT '{}',
    type_filter TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    params      TEXT,
    watch       TEXT,
    locked      INTEGER NOT NULL DEFAULT 0,
    UNIQUE (agent, channel, matcher, type_filter)
);
INSERT INTO subscriptions_new (id, agent, channel, matcher, type_filter, created_at, params, watch, locked)
    SELECT id, agent, channel, matcher, type_filter, created_at, params, watch, locked FROM subscriptions;
DROP TABLE subscriptions;
ALTER TABLE subscriptions_new RENAME TO subscriptions;
CREATE INDEX idx_subscriptions_agent ON subscriptions(agent);
CREATE INDEX idx_subscriptions_channel ON subscriptions(channel);
