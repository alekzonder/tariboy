-- Task keys move from a per-queue autoincrement number to a random short
-- suffix, so a task carries the same key on every daemon it is transferred to.
-- Existing keys are rewritten by the daemon (it needs a random generator with
-- retry, which SQL has no safe loop for); every old key stays resolvable here,
-- because keys are already written outside this database in agent contexts,
-- branch names, pull request titles and comment text.

CREATE TABLE task_key_aliases (
    old_key    TEXT PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_task_key_aliases_task ON task_key_aliases(task_id);
