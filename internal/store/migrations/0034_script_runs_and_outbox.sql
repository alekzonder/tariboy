-- Split agent-owned local script definitions from their execution attempts.
-- The unrelated schedules table deliberately remains untouched.

CREATE TABLE scripts_new (
    id               TEXT PRIMARY KEY,
    agent            TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,
    command          TEXT NOT NULL,
    mode             TEXT NOT NULL CHECK(mode IN ('once', 'every')),
    interval_seconds INTEGER,
    quiet_exit       INTEGER CHECK(quiet_exit IS NULL OR (quiet_exit BETWEEN 0 AND 255)),
    state            TEXT NOT NULL CHECK(state IN ('active', 'completed', 'cancelled')),
    created_at       TEXT NOT NULL,
    next_run_at      TEXT,
    CHECK(
        (mode = 'once' AND interval_seconds IS NULL AND quiet_exit IS NULL) OR
        (mode = 'every' AND interval_seconds > 0)
    )
);

INSERT INTO scripts_new(
    id, agent, name, description, command, mode, interval_seconds,
    quiet_exit, state, created_at, next_run_at
)
SELECT
    id,
    agent,
    name,
    description,
    command,
    mode,
    interval_seconds,
    NULL,
    CASE
        WHEN status = 'canceled' THEN 'cancelled'
        WHEN mode = 'every' AND status IN ('pending', 'running', 'waiting') THEN 'active'
        WHEN mode = 'once' AND status = 'pending' THEN 'active'
        ELSE 'completed'
    END,
    created_at,
    CASE
        WHEN mode = 'every' AND status = 'waiting' THEN next_run_at
        WHEN mode = 'every' AND status = 'running' THEN strftime('%Y-%m-%dT%H:%M:%SZ','now','+' || interval_seconds || ' seconds')
        ELSE NULL
    END
FROM scripts;

CREATE TABLE script_runs (
    id          TEXT PRIMARY KEY,
    script_id   TEXT NOT NULL REFERENCES scripts_new(id) ON DELETE CASCADE,
    agent       TEXT NOT NULL,
    status      TEXT NOT NULL CHECK(status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled', 'timed_out', 'interrupted')),
    cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK(cancel_requested IN (0, 1)),
    pid         INTEGER,
    exit_code   INTEGER,
    created_at  TEXT NOT NULL,
    started_at  TEXT,
    finished_at TEXT,
    log_path    TEXT NOT NULL DEFAULT ''
);

INSERT INTO script_runs(
    id, script_id, agent, status, pid, exit_code, created_at,
    started_at, finished_at, log_path
)
SELECT
    'srun-' || id || '-000000001',
    id,
    agent,
    CASE
        WHEN status = 'pending' THEN 'pending'
        WHEN status = 'running' THEN 'interrupted'
        WHEN status = 'canceled' THEN 'cancelled'
        WHEN status IN ('done', 'waiting') AND COALESCE(last_exit, 0) = 0 THEN 'succeeded'
        ELSE 'failed'
    END,
    NULL,
    last_exit,
    created_at,
    last_started_at,
    CASE
        WHEN status = 'pending' THEN NULL
        WHEN status = 'running' THEN strftime('%Y-%m-%dT%H:%M:%SZ','now')
        ELSE COALESCE(last_finished_at, last_started_at, created_at)
    END,
    log_path
FROM scripts;

DROP TABLE scripts;
ALTER TABLE scripts_new RENAME TO scripts;

CREATE INDEX idx_scripts_agent_created
    ON scripts(agent, created_at DESC, id DESC);
CREATE INDEX idx_scripts_due
    ON scripts(state, next_run_at);
CREATE INDEX idx_script_runs_script_created
    ON script_runs(script_id, created_at DESC, id DESC);
CREATE INDEX idx_script_runs_agent_created
    ON script_runs(agent, created_at DESC, id DESC);
CREATE UNIQUE INDEX idx_script_runs_one_active
    ON script_runs(script_id)
    WHERE status IN ('pending', 'running');

CREATE TABLE script_result_outbox (
    idempotency_key TEXT PRIMARY KEY,
    script_id       TEXT NOT NULL,
    run_id          TEXT NOT NULL UNIQUE,
    agent           TEXT NOT NULL,
    payload         TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0 CHECK(attempts >= 0),
    next_attempt_at TEXT NOT NULL,
    published_at    TEXT NOT NULL DEFAULT '',
    message_id      TEXT NOT NULL DEFAULT '',
    last_error      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_script_result_outbox_due
    ON script_result_outbox(published_at, next_attempt_at, idempotency_key);

-- A command that was running during the upgrade has an unknown outcome, just
-- like a running command found during an ordinary daemon restart. Preserve its
-- required terminal notification even though startup recovery will no longer
-- see it as running after this migration.
INSERT INTO script_result_outbox(
    idempotency_key, script_id, run_id, agent, payload, next_attempt_at
)
SELECT
    'script-result:' || r.id,
    s.id,
    r.id,
    s.agent,
    json_object(
        'script_id', s.id,
        'run_id', r.id,
        'name', s.name,
        'mode', s.mode,
        'status', 'interrupted',
        'log_path', r.log_path
    ),
    r.finished_at
FROM script_runs r
JOIN scripts s ON s.id = r.script_id
WHERE r.status = 'interrupted';
