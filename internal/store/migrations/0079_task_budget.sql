-- A task's AI spending limit in USD for the task and its subtree; 0 means no
-- limit. The proxy sums ai_requests.cost_usd by task key on every request of
-- a task under a limit, so the index starts with task_id.
ALTER TABLE tasks ADD COLUMN budget_usd REAL NOT NULL DEFAULT 0;
CREATE INDEX idx_ai_requests_task_cost ON ai_requests(task_id, cost_usd);
