-- Tags are external annotations on an immutable iteration: one agent marks
-- another agent's iterations as handled without touching the iteration row.
-- They live in their own table so nothing that writes `iterations` can be
-- confused with a tag write. ON DELETE CASCADE (foreign_keys is ON, see
-- store.Open) retires a deleted iteration's tags in both places iterations are
-- removed: retention pruning and agent deletion.
CREATE TABLE iteration_tags (
    iteration_id TEXT NOT NULL REFERENCES iterations(id) ON DELETE CASCADE,
    tag          TEXT NOT NULL,
    PRIMARY KEY (iteration_id, tag)
);
CREATE INDEX idx_iteration_tags_tag ON iteration_tags(tag);
