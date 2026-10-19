-- Queue environment: a row with secret = 0 is a plain env var whose value the
-- API returns; every earlier row stays a secret.
ALTER TABLE task_queue_secrets ADD COLUMN secret INTEGER NOT NULL DEFAULT 1;
