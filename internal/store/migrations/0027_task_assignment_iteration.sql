-- tariboy: skip_if_column=task_assignments.lease_iteration
ALTER TABLE task_assignments
ADD COLUMN lease_iteration TEXT NOT NULL DEFAULT '';
