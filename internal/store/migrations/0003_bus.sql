-- Channel bus (spec §6): channels, messages, subscriptions, deliveries,
-- plus schedules and scripts (agent-owned bus overlays).

CREATE TABLE channels (
    name       TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE messages (
    id                    TEXT PRIMARY KEY,
    channel               TEXT NOT NULL,
    ts                    TEXT NOT NULL,
    source                TEXT NOT NULL DEFAULT '',
    type                  TEXT NOT NULL DEFAULT '',
    subject               TEXT NOT NULL DEFAULT '{}',
    text                  TEXT,
    data                  TEXT,
    produced_by_agent     TEXT,
    produced_in_iteration TEXT,
    produced_by_plugin    TEXT
);
CREATE INDEX idx_messages_channel ON messages(channel, ts, id);

CREATE TABLE subscriptions (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,
    channel     TEXT NOT NULL,
    matcher     TEXT NOT NULL DEFAULT '{}',
    type_filter TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (agent, channel, matcher)
);
CREATE INDEX idx_subscriptions_agent ON subscriptions(agent);
CREATE INDEX idx_subscriptions_channel ON subscriptions(channel);

CREATE TABLE deliveries (
    subscription_id TEXT NOT NULL,
    message_id      TEXT NOT NULL,
    delivered_at    TEXT,
    acked_at        TEXT,
    attempts        INTEGER NOT NULL DEFAULT 0,
    dlq             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (subscription_id, message_id)
);
CREATE INDEX idx_deliveries_message ON deliveries(message_id);

CREATE TABLE schedules (
    id               TEXT PRIMARY KEY,
    agent            TEXT NOT NULL,
    kind             TEXT NOT NULL,        -- oneshot | cron
    spec             TEXT NOT NULL,        -- RFC3339 (oneshot) | cron expr
    channel          TEXT NOT NULL,        -- target channel (default: own inbox)
    message_template TEXT NOT NULL DEFAULT '{}',
    next_fire_at     TEXT NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_schedules_due ON schedules(enabled, next_fire_at);

CREATE TABLE scripts (
    id         TEXT PRIMARY KEY,
    agent      TEXT NOT NULL,
    name       TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (agent, name)
);

ALTER TABLE agents ADD COLUMN messages_batch     INTEGER NOT NULL DEFAULT 10;
ALTER TABLE agents ADD COLUMN messages_max_queue INTEGER NOT NULL DEFAULT 1000;
