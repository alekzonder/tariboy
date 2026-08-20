CREATE TABLE judge_runs (
    id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    creator_iteration TEXT NOT NULL DEFAULT '',
    original_request TEXT NOT NULL,
    spec_json TEXT NOT NULL,
    judge_group TEXT NOT NULL,
    lead_agent TEXT NOT NULL,
    judge_agents_json TEXT NOT NULL,
    summary_agent TEXT NOT NULL,
    judges_per_iteration INTEGER NOT NULL CHECK (judges_per_iteration > 0),
    max_attempts INTEGER NOT NULL DEFAULT 1 CHECK (max_attempts > 0),
    status TEXT NOT NULL CHECK (status IN ('snapshotting','running','summarizing','completed','partial','cancelled')),
    targets_total INTEGER NOT NULL DEFAULT 0,
    targets_ready INTEGER NOT NULL DEFAULT 0,
    assignments_total INTEGER NOT NULL DEFAULT 0,
    assignments_completed INTEGER NOT NULL DEFAULT 0,
    manifest_hash TEXT NOT NULL DEFAULT '',
    current_summary_version INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_judge_runs_created_at ON judge_runs(created_at);

CREATE TABLE judge_targets (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES judge_runs(id) ON DELETE CASCADE,
    target_iteration TEXT NOT NULL,
    target_agent TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    bundle_path TEXT NOT NULL DEFAULT '',
    bundle_hash TEXT NOT NULL DEFAULT '',
    bundle_bytes INTEGER NOT NULL DEFAULT 0,
    snapshot_status TEXT NOT NULL DEFAULT 'pending' CHECK (snapshot_status IN ('pending','snapshotting','ready','snapshot_failed')),
    target_state TEXT NOT NULL DEFAULT 'pending',
    consensus_verdict TEXT NOT NULL DEFAULT '',
    consensus_score REAL,
    assignments_completed INTEGER NOT NULL DEFAULT 0,
    assignments_failed INTEGER NOT NULL DEFAULT 0,
    assignments_pending INTEGER NOT NULL DEFAULT 0,
    UNIQUE(run_id, target_iteration),
    UNIQUE(run_id, sequence)
);
CREATE INDEX idx_judge_targets_run_sequence ON judge_targets(run_id, sequence);

CREATE TABLE judge_assignments (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES judge_runs(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES judge_targets(id) ON DELETE CASCADE,
    replica_index INTEGER NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','claimed','completed','failed','cancelled')),
    judge_agent TEXT NOT NULL DEFAULT '',
    judge_iteration TEXT NOT NULL DEFAULT '',
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    analysis_id TEXT NOT NULL DEFAULT '',
    UNIQUE(target_id, replica_index)
);
CREATE INDEX idx_judge_assignments_state_lease ON judge_assignments(state, lease_expires_at);

CREATE TABLE judge_submission_attempts (
    id TEXT PRIMARY KEY,
    assignment_id TEXT NOT NULL REFERENCES judge_assignments(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL,
    raw_json TEXT NOT NULL,
    validation_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE(assignment_id, attempt_number)
);

CREATE TABLE judge_analyses (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES judge_runs(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES judge_targets(id) ON DELETE CASCADE,
    assignment_id TEXT NOT NULL REFERENCES judge_assignments(id) ON DELETE CASCADE,
    judge_agent TEXT NOT NULL,
    judge_iteration TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    result_json TEXT NOT NULL,
    raw_submission TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE judge_summaries (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES judge_runs(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    summary_agent TEXT NOT NULL,
    summary_iteration TEXT NOT NULL,
    coverage_json TEXT NOT NULL,
    result_json TEXT NOT NULL,
    raw_submission TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(run_id, version)
);

CREATE TABLE judge_retention_pins (
    run_id TEXT NOT NULL REFERENCES judge_runs(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES judge_targets(id) ON DELETE CASCADE,
    iteration_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(run_id, target_id)
);
CREATE INDEX idx_judge_retention_pins_iteration ON judge_retention_pins(iteration_id);
