-- Phase P (channels/messages/plugins design §2, §3.1, §5.1): threading &
-- async axis on messages, explicit-processed bookkeeping on deliveries, and
-- provider params/watch/locked on subscriptions. Columns only — the values
-- flow through the bus structs end-to-end but change no routing behaviour yet.

-- messages: threading / async axis (ported from the old MESSAGE_SCHEMA).
ALTER TABLE messages ADD COLUMN kind           TEXT NOT NULL DEFAULT 'event';
ALTER TABLE messages ADD COLUMN correlation_id TEXT;
ALTER TABLE messages ADD COLUMN in_reply_to    TEXT;
ALTER TABLE messages ADD COLUMN reply_to       TEXT;
ALTER TABLE messages ADD COLUMN deadline       TEXT;

-- deliveries: explicit-processed bookkeeping (replaces auto-ack, §3.1).
ALTER TABLE deliveries ADD COLUMN processed_at TEXT;
ALTER TABLE deliveries ADD COLUMN result       TEXT;

-- subscriptions: provider params + dedup watch fingerprint + system lock (§5.1).
ALTER TABLE subscriptions ADD COLUMN params TEXT;
ALTER TABLE subscriptions ADD COLUMN watch  TEXT;
ALTER TABLE subscriptions ADD COLUMN locked INTEGER NOT NULL DEFAULT 0;
