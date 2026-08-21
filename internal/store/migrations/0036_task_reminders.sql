-- One reminder generation per agent. Rows are written only after the normal
-- inbox publication succeeds, so a restart does not repeat the same work.
CREATE TABLE task_reminders (
    agent       TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    activity_at TEXT NOT NULL,
    sent_at     TEXT NOT NULL
);
