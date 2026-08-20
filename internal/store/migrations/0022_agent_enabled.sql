-- Master on/off switch for the whole agent, above loop_enabled. New agents are
-- created disabled (default 0). For existing rows, preserve current live
-- behavior: an agent that was running (loop_enabled=1) stays enabled; a stopped
-- one stays disabled. loop_enabled remains a nested sub-mechanism flag.
ALTER TABLE agents ADD COLUMN enabled INTEGER NOT NULL DEFAULT 0;
UPDATE agents SET enabled = loop_enabled;
