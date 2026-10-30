-- 0074_task_status_visit_owner_pool.sql -- a visit of a customer status
-- records the pool that owns it in the customer's place: the queue env var
-- CUSTOMER_AGENT_POOL as it was when the task entered the status. '' keeps the
-- owner the manifest declares. tasks.workflow_owner_kind is 'pool' for such a
-- visit, so its triggers read the open visit as well as the manifest.
ALTER TABLE task_status_visits ADD COLUMN owner_pool TEXT NOT NULL DEFAULT '';

DROP TRIGGER tasks_workflow_owner_kind_update;
CREATE TRIGGER tasks_workflow_owner_kind_update AFTER UPDATE OF workflow_status, workflow_digest ON tasks
BEGIN
	UPDATE tasks SET workflow_owner_kind = CASE
		WHEN EXISTS (SELECT 1 FROM task_status_visits v WHERE v.task_id = NEW.id AND v.left_at = ''
			AND v.status_id = NEW.workflow_status AND v.owner_pool <> '') THEN 'pool'
		ELSE COALESCE((
			SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
			FROM task_workflow_images wi, json_each(wi.manifest, '$.definition.statuses') s
			WHERE wi.digest = NEW.workflow_digest AND json_extract(s.value, '$.id') = NEW.workflow_status
		), '') END
	WHERE id = NEW.id;
END;
DROP TRIGGER task_workflow_images_owner_kind_insert;
CREATE TRIGGER task_workflow_images_owner_kind_insert AFTER INSERT ON task_workflow_images
BEGIN
	UPDATE tasks SET workflow_owner_kind = CASE
		WHEN EXISTS (SELECT 1 FROM task_status_visits v WHERE v.task_id = tasks.id AND v.left_at = ''
			AND v.status_id = tasks.workflow_status AND v.owner_pool <> '') THEN 'pool'
		ELSE COALESCE((
			SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
			FROM json_each(NEW.manifest, '$.definition.statuses') s
			WHERE json_extract(s.value, '$.id') = tasks.workflow_status
		), '') END
	WHERE workflow_digest = NEW.digest;
END;
DROP TRIGGER task_workflow_images_owner_kind_update;
CREATE TRIGGER task_workflow_images_owner_kind_update AFTER UPDATE OF manifest ON task_workflow_images
BEGIN
	UPDATE tasks SET workflow_owner_kind = CASE
		WHEN EXISTS (SELECT 1 FROM task_status_visits v WHERE v.task_id = tasks.id AND v.left_at = ''
			AND v.status_id = tasks.workflow_status AND v.owner_pool <> '') THEN 'pool'
		ELSE COALESCE((
			SELECT CASE WHEN json_extract(s.value, '$.terminal') THEN '' ELSE json_extract(s.value, '$.owner.kind') END
			FROM json_each(NEW.manifest, '$.definition.statuses') s
			WHERE json_extract(s.value, '$.id') = tasks.workflow_status
		), '') END
	WHERE workflow_digest = NEW.digest;
END;
