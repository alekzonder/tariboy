-- 0046_chats.sql -- chats as a first-class entity over the existing bus.
-- messages/subscriptions/deliveries are untouched: a chat owns exactly one
-- transport channel, so Publish -> delivery -> WakeMessage keeps working.
CREATE TABLE chats (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    channel      TEXT NOT NULL UNIQUE,
    legacy_agent TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    archived_at  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE chat_participants (
    chat_id   TEXT NOT NULL,
    principal TEXT NOT NULL,
    role      TEXT NOT NULL DEFAULT 'member',
    joined_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    read_ts   TEXT NOT NULL DEFAULT '',
    muted     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (chat_id, principal)
);
CREATE INDEX idx_chat_participants_principal ON chat_participants(principal);
