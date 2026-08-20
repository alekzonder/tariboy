CREATE TABLE blobs (
    name       TEXT NOT NULL,
    tag        TEXT NOT NULL,
    digest     TEXT NOT NULL,
    built_at   TEXT NOT NULL DEFAULT '',
    pushed_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (name, tag, digest)
);
CREATE INDEX idx_blobs_name ON blobs(name, tag, pushed_at);

CREATE TABLE tokens (
    token_sha256 TEXT PRIMARY KEY,
    scope        TEXT NOT NULL,
    label        TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
