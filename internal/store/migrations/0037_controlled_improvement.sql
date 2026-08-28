ALTER TABLE image_source_snapshots ADD COLUMN repository_id TEXT NOT NULL DEFAULT '';
ALTER TABLE image_source_snapshots ADD COLUMN git_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE image_source_snapshots ADD COLUMN lock_digest TEXT NOT NULL DEFAULT '';

CREATE INDEX image_source_snapshots_image_digest_idx
  ON image_source_snapshots(image_digest);

CREATE TABLE judge_subjects (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES judge_runs(id) ON DELETE CASCADE,
  subject_type TEXT NOT NULL CHECK (subject_type IN ('task','iteration')),
  external_id TEXT NOT NULL,
  sequence INTEGER NOT NULL CHECK (sequence >= 0),
  snapshot_hash TEXT NOT NULL,
  snapshot_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(run_id, subject_type, external_id),
  UNIQUE(run_id, sequence)
);

CREATE TABLE judge_subject_targets (
  subject_id TEXT NOT NULL REFERENCES judge_subjects(id) ON DELETE CASCADE,
  target_id TEXT NOT NULL REFERENCES judge_targets(id) ON DELETE CASCADE,
  PRIMARY KEY(subject_id, target_id),
  UNIQUE(target_id)
);

CREATE TABLE improvement_proposals (
  id TEXT PRIMARY KEY,
  judge_run_id TEXT NOT NULL,
  summary_id TEXT NOT NULL DEFAULT '',
  creator_agent TEXT NOT NULL,
  creator_iteration TEXT NOT NULL,
  document_json TEXT NOT NULL,
  revision_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  branch TEXT NOT NULL DEFAULT '',
  pull_request_url TEXT NOT NULL DEFAULT '',
  head_commit TEXT NOT NULL DEFAULT '',
  merged_commit TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX improvement_proposals_judge_run_idx
  ON improvement_proposals(judge_run_id, created_at);

CREATE TABLE improvement_approvals (
  id TEXT PRIMARY KEY,
  proposal_id TEXT NOT NULL REFERENCES improvement_proposals(id) ON DELETE RESTRICT,
  phase TEXT NOT NULL CHECK (phase IN ('plan','rollout')),
  object_hash TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('approve','reject')),
  actor TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TRIGGER improvement_approvals_no_update
BEFORE UPDATE ON improvement_approvals
BEGIN
  SELECT RAISE(ABORT, 'improvement approvals are append-only');
END;

CREATE TRIGGER improvement_approvals_no_delete
BEFORE DELETE ON improvement_approvals
BEGIN
  SELECT RAISE(ABORT, 'improvement approvals are append-only');
END;
