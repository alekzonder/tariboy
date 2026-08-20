ALTER TABLE task_assignments
ADD COLUMN lease_iteration TEXT NOT NULL DEFAULT '';
