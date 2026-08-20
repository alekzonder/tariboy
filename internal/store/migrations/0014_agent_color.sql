-- User-managed per-agent accent color, a hex string like "#4f8cff" used to tint
-- the agent header in the UI. Empty means "no color set". Owned by the dedicated
-- SetColor setter and NOT touched by Store.Update, mirroring alias/notes (0013).
ALTER TABLE agents ADD COLUMN color TEXT NOT NULL DEFAULT '';
