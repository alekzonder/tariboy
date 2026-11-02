-- 0076_mail_instant.sql -- instant delivery of a letter into a running
-- iteration, at the boundary of a tool call.
-- instant_state: waiting | delivered | queued | stopped | read | not_delivered,
-- '' on a letter without the flag. instant_iteration and instant_at are the
-- iteration and the time of the delivery or of the prompt render.
ALTER TABLE mail ADD COLUMN instant INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mail ADD COLUMN instant_state TEXT NOT NULL DEFAULT '';
ALTER TABLE mail ADD COLUMN instant_iteration TEXT NOT NULL DEFAULT '';
ALTER TABLE mail ADD COLUMN instant_at TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_mail_instant ON mail(recipient, instant_state) WHERE instant = 1;

-- The agents that may send this agent an instant letter: NULL is not set (the
-- lead of the agent's group may), a JSON array of agent principals is set.
ALTER TABLE agents ADD COLUMN mail_instant_senders TEXT;
