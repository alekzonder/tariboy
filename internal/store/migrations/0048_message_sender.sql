-- 0048_message_sender.sql -- the principal that sent a message, resolved once
-- at write time instead of parsing data.from on every chat read. Chat summaries
-- and one chat's history filter by (channel, sender); computing the sender from
-- the body forced them to walk every agent inbox and parse each row, holding
-- the daemon's single database connection long enough to queue other requests.
-- The backfill is bus.fromSQL, the same resolution the chat queries used.
ALTER TABLE messages ADD COLUMN sender TEXT NOT NULL DEFAULT '';
UPDATE messages SET sender = COALESCE(
	NULLIF(json_extract(CASE WHEN json_valid(data) THEN data END, '$.from'), ''),
	CASE WHEN source GLOB 'agent:*' OR source GLOB 'user:*' THEN source END,
	CASE WHEN COALESCE(produced_by_agent, '') <> '' THEN 'agent:' || produced_by_agent END,
	'system');
CREATE INDEX idx_messages_channel_sender ON messages(channel, sender, ts);
