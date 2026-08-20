ALTER TABLE task_queue_workflow_triggers
    ADD COLUMN created_after_sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task_queue_workflow_triggers
    ADD COLUMN activation_sequence_set INTEGER NOT NULL DEFAULT 0 CHECK (activation_sequence_set IN (0, 1));

ALTER TABLE task_workflow_subscriptions
    ADD COLUMN created_after_sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task_workflow_subscriptions
    ADD COLUMN activation_sequence_set INTEGER NOT NULL DEFAULT 0 CHECK (activation_sequence_set IN (0, 1));
