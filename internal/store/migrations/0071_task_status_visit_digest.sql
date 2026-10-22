-- 0071_task_status_visit_digest.sql -- each status visit records the workflow
-- image digest the task was pinned to when it entered the status. A workflow
-- upgrade moves the task to a new digest and restarts it at the initial
-- status; the visits of the old version stay as history.
ALTER TABLE task_status_visits ADD COLUMN workflow_digest TEXT NOT NULL DEFAULT '';
UPDATE task_status_visits SET workflow_digest = COALESCE(
	(SELECT workflow_digest FROM tasks WHERE tasks.id = task_status_visits.task_id), '');
