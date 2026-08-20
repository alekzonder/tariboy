-- Native Tariboy Tasks live in the daemon's existing SQLite database.

CREATE TABLE task_queues (
    prefix            TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    responsible_agent TEXT NOT NULL DEFAULT '',
    next_number       INTEGER NOT NULL DEFAULT 1 CHECK (next_number > 0),
    revision          INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);

CREATE TABLE task_queue_owners (
    queue_prefix TEXT NOT NULL REFERENCES task_queues(prefix) ON DELETE RESTRICT,
    agent        TEXT NOT NULL,
    PRIMARY KEY (queue_prefix, agent)
);
CREATE INDEX idx_task_queue_owners_agent
    ON task_queue_owners(agent, queue_prefix);

CREATE TABLE tasks (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    task_key            TEXT NOT NULL UNIQUE,
    queue_prefix        TEXT NOT NULL REFERENCES task_queues(prefix) ON DELETE RESTRICT,
    parent_id           INTEGER REFERENCES tasks(id) ON DELETE RESTRICT,
    position            INTEGER NOT NULL DEFAULT 0,
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
    completed_at        TEXT NOT NULL DEFAULT '',
    UNIQUE (queue_prefix, parent_id, position)
);
CREATE INDEX idx_tasks_parent ON tasks(parent_id, position, id);
CREATE INDEX idx_tasks_queue ON tasks(queue_prefix, position, id);
CREATE INDEX idx_tasks_author ON tasks(author);
CREATE INDEX idx_tasks_assignee ON tasks(assignee);
CREATE INDEX idx_tasks_group ON tasks(group_name);
CREATE INDEX idx_tasks_status ON tasks(status);

CREATE TABLE task_relations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id   INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    target_id   INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    type        TEXT NOT NULL CHECK (type IN ('blocks', 'related')),
    created_by  TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    CHECK (source_id <> target_id),
    UNIQUE (source_id, target_id, type)
);
CREATE INDEX idx_task_relations_target
    ON task_relations(target_id, type, source_id);

CREATE TABLE task_comments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    author      TEXT NOT NULL,
    body        TEXT NOT NULL,
    revision    INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX idx_task_comments_task ON task_comments(task_id, id);

CREATE TABLE task_waiting_for (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id               INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    expected_principal    TEXT NOT NULL,
    requesting_principal  TEXT NOT NULL,
    requesting_comment_id INTEGER NOT NULL REFERENCES task_comments(id) ON DELETE RESTRICT,
    requested_at          TEXT NOT NULL,
    resolving_comment_id  INTEGER REFERENCES task_comments(id) ON DELETE RESTRICT,
    resolved_at           TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX idx_task_waiting_open
    ON task_waiting_for(task_id, expected_principal)
    WHERE resolved_at = '';
CREATE INDEX idx_task_waiting_principal
    ON task_waiting_for(expected_principal, resolved_at, task_id);

CREATE TABLE task_events (
    sequence      INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id      TEXT NOT NULL UNIQUE,
    task_id       INTEGER REFERENCES tasks(id) ON DELETE RESTRICT,
    queue_prefix  TEXT NOT NULL,
    kind          TEXT NOT NULL,
    actor         TEXT NOT NULL,
    task_revision INTEGER NOT NULL DEFAULT 0,
    payload       TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL
);
CREATE INDEX idx_task_events_task ON task_events(task_id, sequence);
CREATE INDEX idx_task_events_queue ON task_events(queue_prefix, sequence);

CREATE TABLE task_notification_outbox (
    notification_id    TEXT PRIMARY KEY,
    event_sequence     INTEGER NOT NULL REFERENCES task_events(sequence) ON DELETE RESTRICT,
    channel            TEXT NOT NULL,
    message_type       TEXT NOT NULL,
    subject            TEXT NOT NULL DEFAULT '{}',
    text               TEXT NOT NULL DEFAULT '',
    data               TEXT NOT NULL DEFAULT '{}',
    attempts           INTEGER NOT NULL DEFAULT 0,
    next_attempt_at    TEXT NOT NULL,
    published_message  TEXT NOT NULL DEFAULT '',
    published_at       TEXT NOT NULL DEFAULT '',
    last_error         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_task_outbox_due
    ON task_notification_outbox(published_at, next_attempt_at, event_sequence);

CREATE TABLE task_notification_state (
    customer_principal TEXT NOT NULL,
    notification_id    TEXT NOT NULL REFERENCES task_notification_outbox(notification_id) ON DELETE RESTRICT,
    read_at            TEXT NOT NULL DEFAULT '',
    dismissed_at       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (customer_principal, notification_id)
);

CREATE TABLE task_idempotency (
    actor           TEXT NOT NULL,
    action          TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    response        TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    PRIMARY KEY (actor, action, idempotency_key)
);

-- Existing Bus messages gain a stable producer key so an outbox retry after a
-- crash cannot create a second immutable message or duplicate deliveries.
ALTER TABLE messages ADD COLUMN idempotency_key TEXT;
CREATE UNIQUE INDEX idx_messages_idempotency
    ON messages(idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
