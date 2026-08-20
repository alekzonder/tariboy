-- Eval results (spec §7.3/§8): one verdict per (iteration, image version, eval).
-- Keyed by image_digest so results are attributed to the exact image version
-- the iteration ran; the unique index backs the Insert upsert.
CREATE TABLE eval_results (
    id           TEXT PRIMARY KEY,
    iteration    TEXT NOT NULL DEFAULT '',
    agent        TEXT NOT NULL DEFAULT '',
    image_name   TEXT NOT NULL DEFAULT '',
    image_tag    TEXT NOT NULL DEFAULT '',
    image_digest TEXT NOT NULL DEFAULT '',
    eval_name    TEXT NOT NULL DEFAULT '',
    eval_type    TEXT NOT NULL DEFAULT '',
    verdict      TEXT NOT NULL DEFAULT '',   -- pass | fail | error
    score        REAL NOT NULL DEFAULT 0,
    detail       TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_eval_results_iteration ON eval_results(iteration);
CREATE INDEX idx_eval_results_recent ON eval_results(created_at);
CREATE UNIQUE INDEX idx_eval_results_key ON eval_results(iteration, image_digest, eval_name);
