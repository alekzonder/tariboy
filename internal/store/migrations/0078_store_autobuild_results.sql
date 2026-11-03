-- 0078_store_autobuild_results.sql -- the last automatic build result of
-- each selected Store image, as a JSON array of
-- {image, selector, at, image_version, error}.
ALTER TABLE image_stores ADD COLUMN auto_results TEXT NOT NULL DEFAULT '[]';
