-- 0075_assessments.sql -- immutable assessments of an iteration or a task.
-- An assessment references its iteration, task and agents by value, without a
-- foreign key: it is historical evidence of quality and outlives iteration
-- retention and agent removal, which is why it snapshots the image and the
-- workflow. Only its axes and violations cascade from it.
CREATE TABLE assessments (
    seq                    INTEGER PRIMARY KEY AUTOINCREMENT,
    id                     TEXT NOT NULL UNIQUE,              -- as-<ts>-<n>, the public handle
    created_at             TEXT NOT NULL,
    assessor_agent         TEXT NOT NULL DEFAULT '',          -- from the agent socket
    assessor_image_ref     TEXT NOT NULL DEFAULT '',          -- from the assessor's iteration
    assessor_image_version TEXT NOT NULL DEFAULT '',
    assessor_image_digest  TEXT NOT NULL DEFAULT '',
    rubric_version         TEXT NOT NULL DEFAULT '',
    iteration_id           TEXT NOT NULL DEFAULT '',          -- subject iteration, no FK
    task_key               TEXT NOT NULL DEFAULT '',          -- subject task, no FK
    subject_agent          TEXT NOT NULL DEFAULT '',          -- snapshots of the iteration row
    image_ref              TEXT NOT NULL DEFAULT '',
    image_version          TEXT NOT NULL DEFAULT '',
    image_digest           TEXT NOT NULL DEFAULT '',
    workflow_digest        TEXT NOT NULL DEFAULT '',          -- snapshots of the task
    workflow_version       TEXT NOT NULL DEFAULT '',
    summary                TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_assessments_iteration ON assessments(iteration_id);
CREATE INDEX idx_assessments_task      ON assessments(task_key);
CREATE INDEX idx_assessments_subject   ON assessments(subject_agent, created_at);
CREATE INDEX idx_assessments_assessor  ON assessments(assessor_agent, created_at);
CREATE INDEX idx_assessments_image     ON assessments(image_ref, image_version);
CREATE INDEX idx_assessments_created   ON assessments(created_at);

-- Exactly three axes per assessment; the service writes all three.
CREATE TABLE assessment_axes (
    assessment_seq INTEGER NOT NULL REFERENCES assessments(seq) ON DELETE CASCADE,
    axis           TEXT NOT NULL CHECK (axis IN ('image_process', 'role', 'task_fit')),
    verdict        TEXT NOT NULL CHECK (verdict IN ('scored', 'not_applicable')),
    score          INTEGER CHECK (score BETWEEN 0 AND 100),
    note           TEXT NOT NULL DEFAULT '',
    CHECK ((verdict = 'scored') = (score IS NOT NULL)),
    UNIQUE (assessment_seq, axis)
);

-- Zero or more violations per assessment. The evidence locator columns that
-- apply depend on evidence_kind; the referenced rows are not checked.
CREATE TABLE assessment_violations (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    assessment_seq          INTEGER NOT NULL REFERENCES assessments(seq) ON DELETE CASCADE,
    axis                    TEXT NOT NULL CHECK (axis IN ('image_process', 'role', 'task_fit')),
    description             TEXT NOT NULL,
    quote                   TEXT NOT NULL,
    evidence_kind           TEXT NOT NULL CHECK (evidence_kind IN ('transcript', 'task_event', 'script_run')),
    evidence_iteration_id   TEXT NOT NULL DEFAULT '',          -- transcript
    evidence_locator        TEXT NOT NULL DEFAULT '',          -- transcript
    evidence_task_key       TEXT NOT NULL DEFAULT '',          -- task_event
    evidence_task_event_seq INTEGER NOT NULL DEFAULT 0,        -- task_event
    evidence_script_run_id  TEXT NOT NULL DEFAULT ''           -- script_run
);
CREATE INDEX idx_assessment_violations_assessment ON assessment_violations(assessment_seq, id);
