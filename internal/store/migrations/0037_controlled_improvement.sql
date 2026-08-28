ALTER TABLE image_source_snapshots ADD COLUMN repository_id TEXT NOT NULL DEFAULT '';
ALTER TABLE image_source_snapshots ADD COLUMN git_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE image_source_snapshots ADD COLUMN lock_digest TEXT NOT NULL DEFAULT '';

CREATE INDEX image_source_snapshots_image_digest_idx
  ON image_source_snapshots(image_digest);
