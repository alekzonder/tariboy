-- Agent Goals add per-agent selection state and task release fields. Rebuild
-- tasks because SQLite cannot alter its status CHECK constraint in place.
PRAGMA defer_foreign_keys = ON;

CREATE TABLE tasks_new (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    task_key            TEXT NOT NULL UNIQUE,
    queue_prefix        TEXT NOT NULL REFERENCES task_queues(prefix) ON DELETE RESTRICT,
    parent_id           INTEGER REFERENCES tasks(id) ON DELETE RESTRICT,
    position            INTEGER NOT NULL DEFAULT 0,
    priority            TEXT NOT NULL DEFAULT 'P2'
                        CHECK (priority IN ('P0', 'P1', 'P2', 'P3')),
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'in_progress', 'wait_customer', 'done', 'cancelled')),
    pull_request        TEXT NOT NULL DEFAULT '',
    author              TEXT NOT NULL,
    customer            TEXT NOT NULL,
    group_name          TEXT NOT NULL DEFAULT '',
    assignee            TEXT NOT NULL DEFAULT '',
    manual_block_reason TEXT NOT NULL DEFAULT '',
    workflow_version_id INTEGER REFERENCES task_workflow_versions(id) ON DELETE RESTRICT,
    workflow_status     TEXT,
    workflow_revision   INTEGER,
    revision            INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    completed_at        TEXT NOT NULL DEFAULT ''
);

INSERT INTO tasks_new (
    id, task_key, queue_prefix, parent_id, position, priority, title,
    description, status, pull_request, author, customer, group_name, assignee,
    manual_block_reason, workflow_version_id, workflow_status, workflow_revision,
    revision, created_at, updated_at, completed_at
)
SELECT id, task_key, queue_prefix, parent_id, position, priority, title,
       description, status, '', author, customer, group_name, assignee,
       manual_block_reason, workflow_version_id, workflow_status, workflow_revision,
       revision, created_at, updated_at, completed_at
FROM tasks;

DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;

CREATE INDEX idx_tasks_parent
    ON tasks(parent_id, priority, position, task_key);
CREATE INDEX idx_tasks_queue
    ON tasks(queue_prefix, parent_id, priority, position, task_key);
CREATE INDEX idx_tasks_author ON tasks(author);
CREATE INDEX idx_tasks_assignee ON tasks(assignee);
CREATE INDEX idx_tasks_group ON tasks(group_name);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_workflow_version
    ON tasks(workflow_version_id, workflow_status);

ALTER TABLE agents ADD COLUMN goal_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agents ADD COLUMN goal_wait_customer_timeout_s INTEGER NOT NULL DEFAULT 300 CHECK(goal_wait_customer_timeout_s > 0);
ALTER TABLE agents ADD COLUMN current_goal_task_key TEXT NOT NULL DEFAULT '';
