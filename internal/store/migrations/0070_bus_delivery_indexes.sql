-- 0070_bus_delivery_indexes.sql -- two bus reads stop growing with history.
--
-- idx_deliveries_pending holds only deliveries still waiting to be processed,
-- so the queue-depth gauge and the per-agent pending reads scan the backlog,
-- not every delivery ever made.
CREATE INDEX idx_deliveries_pending ON deliveries(subscription_id)
	WHERE acked_at IS NULL AND dlq = 0;

-- id_second / id_seq are the message-id counter of a channel: the second of
-- its last id and that id's sequence number. A NULL id_second means the
-- counter is not seeded yet; the bus seeds it once from the stored ids of that
-- second.
ALTER TABLE channels ADD COLUMN id_second TEXT;
ALTER TABLE channels ADD COLUMN id_seq INTEGER NOT NULL DEFAULT 0;
