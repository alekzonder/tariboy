-- 0069_task_stored_state.sql -- two values every task select used to compute
-- per row are stored on the task and kept current by triggers, so every write
-- path updates them without each statement doing it.
--
-- workflow_owner_kind is the owner kind (pool, customer, script) of the task's
-- current workflow status, read from its pinned image manifest; it is empty
-- for a flexible task, a terminal status, or a status the manifest does not
-- declare. It changes only when the status, the pinned digest, or the
-- manifest changes.
ALTER TABLE tasks ADD COLUMN workflow_owner_kind TEXT NOT NULL DEFAULT '';
UPDATE tasks SET workflow_owner_kind = COALESCE((
	SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
	FROM task_workflow_images wi, json_each(wi.manifest, '$.definition.statuses') s
	WHERE wi.digest = tasks.workflow_digest AND json_extract(s.value, '$.id') = tasks.workflow_status
), '')
WHERE workflow_digest IS NOT NULL;
CREATE TRIGGER tasks_workflow_owner_kind_insert AFTER INSERT ON tasks
WHEN NEW.workflow_digest IS NOT NULL
BEGIN
	UPDATE tasks SET workflow_owner_kind = COALESCE((
		SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
		FROM task_workflow_images wi, json_each(wi.manifest, '$.definition.statuses') s
		WHERE wi.digest = NEW.workflow_digest AND json_extract(s.value, '$.id') = NEW.workflow_status
	), '') WHERE id = NEW.id;
END;
CREATE TRIGGER tasks_workflow_owner_kind_update AFTER UPDATE OF workflow_status, workflow_digest ON tasks
BEGIN
	UPDATE tasks SET workflow_owner_kind = COALESCE((
		SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
		FROM task_workflow_images wi, json_each(wi.manifest, '$.definition.statuses') s
		WHERE wi.digest = NEW.workflow_digest AND json_extract(s.value, '$.id') = NEW.workflow_status
	), '') WHERE id = NEW.id;
END;
-- A published manifest does not change, but one stored after its tasks, or
-- replaced, still gives them the owner kind it declares.
CREATE TRIGGER task_workflow_images_owner_kind_insert AFTER INSERT ON task_workflow_images
BEGIN
	UPDATE tasks SET workflow_owner_kind = COALESCE((
		SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
		FROM json_each(NEW.manifest, '$.definition.statuses') s
		WHERE json_extract(s.value, '$.id') = tasks.workflow_status
	), '') WHERE workflow_digest = NEW.digest;
END;
CREATE TRIGGER task_workflow_images_owner_kind_update AFTER UPDATE OF manifest ON task_workflow_images
BEGIN
	UPDATE tasks SET workflow_owner_kind = COALESCE((
		SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
		FROM json_each(NEW.manifest, '$.definition.statuses') s
		WHERE json_extract(s.value, '$.id') = tasks.workflow_status
	), '') WHERE workflow_digest = NEW.digest;
END;

-- open_blockers counts the task's blocks relations whose source is not done
-- or cancelled. The task is blocked while it is above zero or the task has a
-- manual block reason. Each trigger recounts the affected targets, so the
-- count stays exact whatever order relations and statuses change in.
ALTER TABLE tasks ADD COLUMN open_blockers INTEGER NOT NULL DEFAULT 0;
UPDATE tasks SET open_blockers = (
	SELECT COUNT(*) FROM task_relations r JOIN tasks blocker ON blocker.id = r.source_id
	WHERE r.target_id = tasks.id AND r.type = 'blocks' AND blocker.status NOT IN ('done', 'cancelled')
);
CREATE TRIGGER task_relations_open_blockers_insert AFTER INSERT ON task_relations
WHEN NEW.type = 'blocks'
BEGIN
	UPDATE tasks SET open_blockers = (
		SELECT COUNT(*) FROM task_relations r JOIN tasks blocker ON blocker.id = r.source_id
		WHERE r.target_id = tasks.id AND r.type = 'blocks' AND blocker.status NOT IN ('done', 'cancelled')
	) WHERE id = NEW.target_id;
END;
CREATE TRIGGER task_relations_open_blockers_delete AFTER DELETE ON task_relations
WHEN OLD.type = 'blocks'
BEGIN
	UPDATE tasks SET open_blockers = (
		SELECT COUNT(*) FROM task_relations r JOIN tasks blocker ON blocker.id = r.source_id
		WHERE r.target_id = tasks.id AND r.type = 'blocks' AND blocker.status NOT IN ('done', 'cancelled')
	) WHERE id = OLD.target_id;
END;
CREATE TRIGGER tasks_open_blockers_status AFTER UPDATE OF status ON tasks
WHEN (OLD.status IN ('done', 'cancelled')) <> (NEW.status IN ('done', 'cancelled'))
BEGIN
	UPDATE tasks SET open_blockers = (
		SELECT COUNT(*) FROM task_relations r JOIN tasks blocker ON blocker.id = r.source_id
		WHERE r.target_id = tasks.id AND r.type = 'blocks' AND blocker.status NOT IN ('done', 'cancelled')
	) WHERE id IN (SELECT target_id FROM task_relations WHERE source_id = NEW.id AND type = 'blocks');
END;
