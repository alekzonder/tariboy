-- Phase R (channels/messages/plugins design §4.2): a request's deadline reuses
-- the schedule mechanism. Tag the one-shot timeout schedule with the request's
-- correlation id so a reply that lands first can cancel exactly that entry
-- (CancelByCorrelation) without scanning message templates.
ALTER TABLE schedules ADD COLUMN correlation_id TEXT;
CREATE INDEX idx_schedules_correlation ON schedules(correlation_id);
