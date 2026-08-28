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
