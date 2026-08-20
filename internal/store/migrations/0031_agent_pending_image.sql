ALTER TABLE agents ADD COLUMN pending_image_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN pending_image_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN pending_image_error TEXT NOT NULL DEFAULT '';
