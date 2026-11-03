-- 0077_assessments_latest_index.sql -- indexes for the latest-record
-- subquery of assessment.IsLatest. The subquery looks for a later record of
-- the same object: an iteration (iteration_id) or, with an empty
-- iteration_id, a task (task_key). Its condition starts with
-- b.created_at >= a.created_at, so SQLite searches a range of created_at
-- inside the object instead of reading every record of it.
-- The new indexes start with the columns of idx_assessments_iteration and
-- idx_assessments_task and serve the same equality searches, so those two
-- are dropped. Only indexes change; no row changes.
CREATE INDEX idx_assessments_iteration_created      ON assessments(iteration_id, created_at);
CREATE INDEX idx_assessments_task_iteration_created ON assessments(task_key, iteration_id, created_at);
DROP INDEX idx_assessments_iteration;
DROP INDEX idx_assessments_task;
