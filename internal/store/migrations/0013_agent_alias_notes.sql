-- User-managed per-agent free-form fields. alias is a friendly display name;
-- notes is scratch text edited from the Overview page. Both are owned by
-- dedicated setters (SetAlias/SetNotes) and NOT touched by Store.Update,
-- mirroring error_reason (0010) and status_message (0012).
ALTER TABLE agents ADD COLUMN alias TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN notes TEXT NOT NULL DEFAULT '';
