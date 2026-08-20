CREATE TABLE image_source_snapshots (
  image_ref TEXT PRIMARY KEY,
  image_digest TEXT NOT NULL,
  source_name TEXT NOT NULL,
  source_digest TEXT NOT NULL,
  relative_dir TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX image_source_snapshots_digest_idx
  ON image_source_snapshots(source_digest);
