-- 0080_store_ref.sql -- the branch, tag or commit a Git Store's managed clone
-- is checked out at; '' follows the remote's default branch.
ALTER TABLE image_stores ADD COLUMN ref TEXT NOT NULL DEFAULT '';
