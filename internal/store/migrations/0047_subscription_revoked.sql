-- 0047_subscription_revoked.sql -- leaving a chat stops future delivery without
-- erasing outstanding work. Pending joins deliveries to subscriptions, so
-- deleting the subscription row would make already-created, unacked deliveries
-- vanish from the agent queue. A revoked subscription is skipped by the fan-out
-- and still owns its existing deliveries.
ALTER TABLE subscriptions ADD COLUMN revoked_at TEXT NOT NULL DEFAULT '';
