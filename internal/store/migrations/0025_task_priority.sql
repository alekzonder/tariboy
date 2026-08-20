-- Rebuild tasks because SQLite cannot alter the legacy
-- UNIQUE(queue_prefix, parent_id, position) constraint in place. Positions are
-- now local to a priority bucket.
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
                        CHECK (status IN ('open', 'in_progress', 'done', 'cancelled')),
    author              TEXT NOT NULL,
    customer            TEXT NOT NULL,
    group_name          TEXT NOT NULL DEFAULT '',
    assignee            TEXT NOT NULL DEFAULT '',
    manual_block_reason TEXT NOT NULL DEFAULT '',
    revision            INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    completed_at        TEXT NOT NULL DEFAULT ''
);

INSERT INTO tasks_new (
    id, task_key, queue_prefix, parent_id, position, priority, title,
    description, status, author, customer, group_name, assignee,
    manual_block_reason, revision, created_at, updated_at, completed_at
)
SELECT id, task_key, queue_prefix, parent_id, position, 'P2', title,
       description, status, author, customer, group_name, assignee,
       manual_block_reason, revision, created_at, updated_at, completed_at
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
