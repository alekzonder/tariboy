-- 0073_operations.sql -- the daemon's background operations: one row per
-- image or workflow build. The full log of an operation is the file
-- <base-dir>/operations/<id>.log; the row keeps its status and result.
CREATE TABLE operations (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          TEXT NOT NULL,                     -- image.build | workflow.build
    title         TEXT NOT NULL,
    status        TEXT NOT NULL,                     -- running | succeeded | failed | interrupted
    initiator     TEXT NOT NULL DEFAULT '',          -- ui | cli | auto | ''
    params        TEXT NOT NULL DEFAULT '{}',        -- JSON object of string values
    result        TEXT NOT NULL DEFAULT '',          -- JSON result of a succeeded operation
    error_code    TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    finished_at   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_operations_status ON operations(status, finished_at);
