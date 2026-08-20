-- Configurable native task workflows. Workflow definitions are immutable
-- versions; runtime rows preserve every status entry and assignment attempt.

CREATE TABLE task_workflow_versions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    version      INTEGER NOT NULL CHECK (version > 0),
    definition   TEXT NOT NULL,
    state        TEXT NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'published')),
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    published_at TEXT NOT NULL DEFAULT '',
    UNIQUE (name, version)
);

ALTER TABLE tasks ADD COLUMN workflow_version_id INTEGER
    REFERENCES task_workflow_versions(id) ON DELETE RESTRICT;
ALTER TABLE tasks ADD COLUMN workflow_status TEXT;
ALTER TABLE tasks ADD COLUMN workflow_revision INTEGER;
CREATE INDEX idx_tasks_workflow_version
    ON tasks(workflow_version_id, workflow_status);

-- A queue has at most one active workflow binding. Creation snapshots this
-- version onto the task, so replacing the binding cannot change live work.
CREATE TABLE task_queue_workflows (
    queue_prefix        TEXT PRIMARY KEY REFERENCES task_queues(prefix) ON DELETE RESTRICT,
    workflow_version_id INTEGER NOT NULL REFERENCES task_workflow_versions(id) ON DELETE RESTRICT,
    bound_by            TEXT NOT NULL,
    bound_at            TEXT NOT NULL,
    revision            INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE TABLE task_agent_pools (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    queue_prefix TEXT NOT NULL REFERENCES task_queues(prefix) ON DELETE RESTRICT,
    name         TEXT NOT NULL,
    revision     INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE (queue_prefix, name)
);

CREATE TABLE task_agent_pool_members (
    pool_id  INTEGER NOT NULL REFERENCES task_agent_pools(id) ON DELETE RESTRICT,
    agent    TEXT NOT NULL REFERENCES agents(name) ON DELETE RESTRICT,
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (pool_id, agent),
    UNIQUE (pool_id, position)
);
CREATE INDEX idx_task_agent_pool_members_agent
    ON task_agent_pool_members(agent, pool_id);

CREATE TABLE task_status_executions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id             INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    workflow_version_id INTEGER NOT NULL REFERENCES task_workflow_versions(id) ON DELETE RESTRICT,
    status_id           TEXT NOT NULL,
    sequence            INTEGER NOT NULL CHECK (sequence > 0),
    state               TEXT NOT NULL DEFAULT 'active'
                        CHECK (state IN ('active', 'transitioned', 'frozen')),
    transition_to       TEXT NOT NULL DEFAULT '',
    task_revision       INTEGER NOT NULL CHECK (task_revision > 0),
    created_at          TEXT NOT NULL,
    completed_at        TEXT NOT NULL DEFAULT '',
    UNIQUE (task_id, sequence)
);
CREATE INDEX idx_task_status_executions_active
    ON task_status_executions(task_id, state, sequence);

CREATE TABLE task_requirement_executions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    status_execution_id INTEGER NOT NULL REFERENCES task_status_executions(id) ON DELETE RESTRICT,
    requirement_id      TEXT NOT NULL,
    pool_id             INTEGER REFERENCES task_agent_pools(id) ON DELETE RESTRICT,
    dispatch            TEXT NOT NULL CHECK (dispatch IN ('claim_one', 'require_all')),
    optional            INTEGER NOT NULL DEFAULT 0 CHECK (optional IN (0, 1)),
    pool_snapshot       TEXT NOT NULL DEFAULT '[]',
    inputs              TEXT NOT NULL DEFAULT '[]',
    produces            TEXT NOT NULL DEFAULT '[]',
    outcomes            TEXT NOT NULL DEFAULT '[]',
    state               TEXT NOT NULL DEFAULT 'pending'
                        CHECK (state IN ('pending', 'completed', 'frozen')),
    created_at          TEXT NOT NULL,
    completed_at        TEXT NOT NULL DEFAULT '',
    UNIQUE (status_execution_id, requirement_id)
);
CREATE INDEX idx_task_requirement_executions_active
    ON task_requirement_executions(status_execution_id, state, id);

CREATE TABLE task_assignments (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    requirement_execution_id INTEGER NOT NULL REFERENCES task_requirement_executions(id) ON DELETE RESTRICT,
    agent                    TEXT REFERENCES agents(name) ON DELETE RESTRICT,
    attempt                  INTEGER NOT NULL CHECK (attempt > 0),
    state                    TEXT NOT NULL DEFAULT 'claimable'
                             CHECK (state IN ('claimable', 'leased', 'completed', 'released', 'expired', 'failed')),
    lease_owner              TEXT NOT NULL DEFAULT '',
    lease_expires_at         TEXT NOT NULL DEFAULT '',
    revision                 INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    outcome                  TEXT NOT NULL DEFAULT '',
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL,
    completed_at             TEXT NOT NULL DEFAULT '',
    UNIQUE (requirement_execution_id, agent, attempt)
);
CREATE INDEX idx_task_assignments_active
    ON task_assignments(requirement_execution_id, state, agent)
    WHERE state IN ('claimable', 'leased');
CREATE UNIQUE INDEX idx_task_assignments_ownerless_attempt
    ON task_assignments(requirement_execution_id, attempt)
    WHERE agent IS NULL;
CREATE INDEX idx_task_assignments_lease_expiry
    ON task_assignments(lease_expires_at, id)
    WHERE state = 'leased' AND lease_expires_at <> '';

CREATE TABLE task_artifacts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id       INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    assignment_id INTEGER REFERENCES task_assignments(id) ON DELETE RESTRICT,
    name          TEXT NOT NULL,
    type          TEXT NOT NULL CHECK (type IN ('markdown', 'json', 'file', 'commit', 'url')),
    content       TEXT NOT NULL DEFAULT '',
    metadata      TEXT NOT NULL DEFAULT '{}',
    revision      INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by    TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX idx_task_artifacts_task
    ON task_artifacts(task_id, name, id);

CREATE TABLE task_workflow_questions (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id                INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    assignment_id          INTEGER REFERENCES task_assignments(id) ON DELETE RESTRICT,
    requirement_execution_id INTEGER REFERENCES task_requirement_executions(id) ON DELETE RESTRICT,
    question               TEXT NOT NULL,
    context                TEXT NOT NULL,
    blocking_scope         TEXT NOT NULL CHECK (blocking_scope IN ('none', 'assignment', 'requirement')),
    anchor                 TEXT NOT NULL DEFAULT '',
    options                TEXT NOT NULL DEFAULT '[]',
    suggested_answer       TEXT NOT NULL DEFAULT '',
    artifact_attachments   TEXT NOT NULL DEFAULT '[]',
    state                  TEXT NOT NULL DEFAULT 'open'
                           CHECK (state IN ('open', 'answered', 'timed_out', 'exhausted')),
    deadline_at            TEXT NOT NULL DEFAULT '',
    answer                 TEXT NOT NULL DEFAULT '',
    answered_by            TEXT NOT NULL DEFAULT '',
    created_at             TEXT NOT NULL,
    answered_at            TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_task_workflow_questions_open
    ON task_workflow_questions(task_id, created_at, id)
    WHERE state = 'open';

CREATE TABLE task_workflow_holds (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id                INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    assignment_id          INTEGER REFERENCES task_assignments(id) ON DELETE RESTRICT,
    requirement_execution_id INTEGER REFERENCES task_requirement_executions(id) ON DELETE RESTRICT,
    question_id            INTEGER REFERENCES task_workflow_questions(id) ON DELETE RESTRICT,
    scope                  TEXT NOT NULL CHECK (scope IN ('none', 'assignment', 'requirement')),
    reason                 TEXT NOT NULL DEFAULT '',
    created_at             TEXT NOT NULL,
    released_at            TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_task_workflow_holds_open
    ON task_workflow_holds(task_id, scope, id)
    WHERE released_at = '';

CREATE TABLE task_workflow_subscriptions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id       INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    assignment_id INTEGER REFERENCES task_assignments(id) ON DELETE RESTRICT,
    pattern       TEXT NOT NULL,
    correlation_key TEXT NOT NULL DEFAULT '',
    reaction      TEXT NOT NULL DEFAULT 'record_only',
    state         TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'cancelled')),
    created_by    TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    cancelled_at  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_task_workflow_subscriptions_correlation
    ON task_workflow_subscriptions(correlation_key, state, id);

CREATE TABLE task_observations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id         INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    subscription_id INTEGER REFERENCES task_workflow_subscriptions(id) ON DELETE RESTRICT,
    assignment_id   INTEGER REFERENCES task_assignments(id) ON DELETE RESTRICT,
    kind            TEXT NOT NULL,
    payload         TEXT NOT NULL DEFAULT '{}',
    observed_at     TEXT NOT NULL
);
CREATE INDEX idx_task_observations_task
    ON task_observations(task_id, observed_at, id);

CREATE TABLE task_queue_workflow_triggers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    queue_prefix TEXT NOT NULL REFERENCES task_queues(prefix) ON DELETE RESTRICT,
    pattern      TEXT NOT NULL,
    correlation_key TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_by   TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX idx_task_queue_workflow_triggers_correlation
    ON task_queue_workflow_triggers(queue_prefix, correlation_key, enabled, id);

CREATE TABLE task_workflow_outbox (
    wake_id          TEXT PRIMARY KEY,
    task_id          INTEGER REFERENCES tasks(id) ON DELETE RESTRICT,
    assignment_id    INTEGER REFERENCES task_assignments(id) ON DELETE RESTRICT,
    kind             TEXT NOT NULL,
    payload          TEXT NOT NULL DEFAULT '{}',
    attempts         INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at  TEXT NOT NULL,
    published_at     TEXT NOT NULL DEFAULT '',
    last_error       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_task_workflow_outbox_due
    ON task_workflow_outbox(published_at, next_attempt_at, wake_id);

-- Durable cursor for replaying committed bus messages into workflow
-- observations. The daemon hook is only a latency optimization; restart
-- recovery advances this cursor only after idempotent task ingestion succeeds.
CREATE TABLE task_workflow_ingress_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    last_message_sequence INTEGER NOT NULL DEFAULT 0 CHECK (last_message_sequence >= 0)
);
INSERT INTO task_workflow_ingress_state(singleton) VALUES (1);

-- SQLite serializes writers. This AUTOINCREMENT row is inserted in the same
-- transaction as each bus message and therefore gives workflow ingress a
-- durable commit-order cursor independent of timestamps and channel-shaped ids.
CREATE TABLE task_workflow_message_sequence (
    sequence   INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id TEXT NOT NULL UNIQUE REFERENCES messages(id) ON DELETE CASCADE
);
INSERT INTO task_workflow_message_sequence(message_id)
SELECT id FROM messages ORDER BY rowid;
