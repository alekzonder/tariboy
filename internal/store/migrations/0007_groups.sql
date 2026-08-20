-- Groups (spec §4): a first-class collaboration unit. Channel names and the
-- shared-dir path are DERIVED from the group name, not stored. settings is a
-- reserved JSON blob for future group options.
CREATE TABLE groups (
    name       TEXT PRIMARY KEY,
    lead       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    settings   TEXT NOT NULL DEFAULT '{}'
);

-- Membership lives on the agent row (nullable-empty = no group). "group" is a
-- SQL keyword, so it is always quoted.
ALTER TABLE agents ADD COLUMN "group" TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_agents_group ON agents("group");
