CREATE TABLE image_provenance (
    ref TEXT PRIMARY KEY,
    digest TEXT NOT NULL,
    source_cwd TEXT NOT NULL,
    built_at TEXT NOT NULL
);
