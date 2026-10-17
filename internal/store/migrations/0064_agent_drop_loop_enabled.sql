-- The agent has one on/off switch: enabled. An agent that was on with its loop
-- off ran no iterations; keep it from starting to iterate (and spend AI budget)
-- after the upgrade by turning it off. A halted agent stays on so its error
-- stays visible, and the terminal-only bare image never iterates anyway.
UPDATE agents SET enabled = 0
WHERE enabled = 1 AND loop_enabled = 0 AND error_reason = '' AND image_ref <> 'bare:latest';
ALTER TABLE agents DROP COLUMN loop_enabled;
