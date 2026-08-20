-- Agent-authored status message ("what I'm doing now"), daemon-written via
-- agent.Store.SetStatus. Distinct from the computed live state (running/idle/
-- stopped/error) removed in 0011 — this is free-text narration, like CONTEXT.md.
ALTER TABLE agents ADD COLUMN status_message TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN status_updated TEXT NOT NULL DEFAULT '';
