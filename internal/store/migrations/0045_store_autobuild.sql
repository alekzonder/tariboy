ALTER TABLE image_stores ADD COLUMN auto_interval_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE image_stores ADD COLUMN auto_images TEXT NOT NULL DEFAULT '';
ALTER TABLE image_stores ADD COLUMN auto_last_run_at INTEGER NOT NULL DEFAULT 0;
