-- Per-agent Usage tab (epic dev-t-3e1 §2): carry task/epic attribution
-- on each proxied request so usage can be sliced by task/epic. Nullable columns
-- only — untagged requests keep NULL and every existing aggregation is unaffected.

ALTER TABLE ai_requests ADD COLUMN task_id TEXT;
ALTER TABLE ai_requests ADD COLUMN epic_id TEXT;

CREATE INDEX idx_ai_requests_agent_task ON ai_requests(agent, task_id);
