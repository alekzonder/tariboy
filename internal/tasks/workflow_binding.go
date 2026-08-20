package tasks

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const queueWorkflowSelect = `
SELECT qw.queue_prefix, qw.workflow_version_id, w.name, w.version,
       qw.revision, qw.bound_by, qw.bound_at
FROM task_queue_workflows qw
JOIN task_workflow_versions w ON w.id = qw.workflow_version_id`

func (s *Service) ActivateQueueWorkflow(
	ctx context.Context,
	actor Actor,
	queue string,
	versionID int64,
	revision int64,
	idempotencyKey string,
) (QueueWorkflowBinding, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return QueueWorkflowBinding{}, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return QueueWorkflowBinding{}, err
	}
	defer tx.Rollback()
	if replayed, ok, err := readTaskIdempotency[QueueWorkflowBinding](
		ctx, tx, actor.Principal, "activate_queue_workflow", idempotencyKey,
	); err != nil {
		return QueueWorkflowBinding{}, err
	} else if ok {
		return replayed, nil
	}
	if err := requireQueueExists(ctx, tx, queue); err != nil {
		return QueueWorkflowBinding{}, err
	}
	workflow, err := workflowVersionByID(ctx, tx, versionID)
	if err != nil {
		return QueueWorkflowBinding{}, err
	}
	if workflow.State != "published" {
		return QueueWorkflowBinding{}, domainError(
			http.StatusConflict, "workflow_not_published", "only published workflow versions can be activated",
		)
	}
	missing, err := missingWorkflowPools(ctx, tx, queue, workflow.Definition)
	if err != nil {
		return QueueWorkflowBinding{}, err
	}
	if len(missing) != 0 {
		return QueueWorkflowBinding{}, &Error{
			Status: http.StatusConflict, Code: "workflow_pool_empty",
			Msg:  "every logical workflow pool must have at least one agent",
			Data: map[string]any{"missing_pools": missing},
		}
	}

	current, found, err := queueWorkflowByPrefix(ctx, tx, queue)
	if err != nil {
		return QueueWorkflowBinding{}, err
	}
	now := s.now()
	nextRevision := int64(1)
	if found {
		if revision <= 0 || revision != current.Revision {
			return QueueWorkflowBinding{}, bindingRevisionConflict(current)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE task_queue_workflows
			SET workflow_version_id = ?, bound_by = ?, bound_at = ?, revision = revision + 1
			WHERE queue_prefix = ? AND revision = ?`,
			workflow.ID, actor.Principal, now, queue, revision)
		if err != nil {
			return QueueWorkflowBinding{}, err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return QueueWorkflowBinding{}, err
		} else if affected != 1 {
			fresh, _, loadErr := queueWorkflowByPrefix(ctx, tx, queue)
			if loadErr != nil {
				return QueueWorkflowBinding{}, loadErr
			}
			return QueueWorkflowBinding{}, bindingRevisionConflict(fresh)
		}
		nextRevision = current.Revision + 1
	} else {
		if revision != 0 {
			return QueueWorkflowBinding{}, bindingRevisionConflict(QueueWorkflowBinding{Queue: queue})
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_queue_workflows(
				queue_prefix, workflow_version_id, bound_by, bound_at, revision
			) VALUES (?, ?, ?, ?, 1)`, queue, workflow.ID, actor.Principal, now); err != nil {
			return QueueWorkflowBinding{}, err
		}
	}
	binding := QueueWorkflowBinding{
		Queue: queue, WorkflowVersionID: workflow.ID, WorkflowName: workflow.Name,
		WorkflowVersion: workflow.Version, Revision: nextRevision,
		BoundBy: actor.Principal, BoundAt: now,
	}
	if _, err := appendQueueEventTx(ctx, tx, Queue{Prefix: queue, Revision: nextRevision},
		"task.queue_workflow_activated", actor, map[string]any{
			"workflow_version_id": workflow.ID,
			"workflow":            workflow.Name + "@" + workflowVersionString(workflow.Version),
		}, now); err != nil {
		return QueueWorkflowBinding{}, err
	}
	if err := writeTaskIdempotency(
		ctx, tx, actor.Principal, "activate_queue_workflow", idempotencyKey, binding, now,
	); err != nil {
		return QueueWorkflowBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return QueueWorkflowBinding{}, err
	}
	s.signal()
	return binding, nil
}

func (s *Service) GetQueueWorkflow(ctx context.Context, actor Actor, queue string) (QueueWorkflowBinding, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return QueueWorkflowBinding{}, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	if err := requireQueueExists(ctx, s.db, queue); err != nil {
		return QueueWorkflowBinding{}, err
	}
	binding, found, err := queueWorkflowByPrefix(ctx, s.db, queue)
	if err != nil {
		return QueueWorkflowBinding{}, err
	}
	if !found {
		return QueueWorkflowBinding{}, &Error{
			Status: http.StatusNotFound, Code: "queue_workflow_not_found",
			Msg: "queue has no active workflow", Data: map[string]any{"queue": queue},
		}
	}
	return binding, nil
}

func (s *Service) RebindAgentPool(
	ctx context.Context,
	actor Actor,
	queue string,
	poolName string,
	agents []string,
	revision int64,
	idempotencyKey string,
) (AgentPool, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return AgentPool{}, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	poolName = strings.TrimSpace(poolName)
	if poolName == "" {
		return AgentPool{}, domainError(http.StatusBadRequest, "missing_pool", "agent pool name is required")
	}
	agents = normalizePoolAgents(agents)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentPool{}, err
	}
	defer tx.Rollback()
	if replayed, ok, err := readTaskIdempotency[AgentPool](
		ctx, tx, actor.Principal, "rebind_agent_pool", idempotencyKey,
	); err != nil {
		return AgentPool{}, err
	} else if ok {
		return replayed, nil
	}
	if err := requireQueueExists(ctx, tx, queue); err != nil {
		return AgentPool{}, err
	}
	if err := requireAgentsExist(ctx, tx, agents); err != nil {
		return AgentPool{}, err
	}
	if len(agents) == 0 {
		workflow, active, err := activeWorkflowForQueue(ctx, tx, queue)
		if err != nil {
			return AgentPool{}, err
		}
		if active {
			if _, referenced := referencedWorkflowPools(workflow.Definition)[poolName]; referenced {
				return AgentPool{}, &Error{
					Status: http.StatusConflict, Code: "workflow_pool_empty",
					Msg:  "an active workflow pool must have at least one agent",
					Data: map[string]any{"pool": poolName},
				}
			}
		}
	}

	current, found, err := agentPoolByName(ctx, tx, queue, poolName)
	if err != nil {
		return AgentPool{}, err
	}
	now := s.now()
	var pool AgentPool
	if found {
		if revision <= 0 || revision != current.Revision {
			return AgentPool{}, poolRevisionConflict(current)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE task_agent_pools SET revision = revision + 1, updated_at = ?
			WHERE id = ? AND revision = ?`, now, current.ID, revision)
		if err != nil {
			return AgentPool{}, err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return AgentPool{}, err
		} else if affected != 1 {
			fresh, _, loadErr := agentPoolByName(ctx, tx, queue, poolName)
			if loadErr != nil {
				return AgentPool{}, loadErr
			}
			return AgentPool{}, poolRevisionConflict(fresh)
		}
		pool = current
		pool.Revision++
		pool.UpdatedAt = now
		pool.Agents = agents
	} else {
		if revision != 0 {
			return AgentPool{}, poolRevisionConflict(AgentPool{Queue: queue, Name: poolName})
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO task_agent_pools(queue_prefix, name, revision, created_at, updated_at)
			VALUES (?, ?, 1, ?, ?)`, queue, poolName, now, now)
		if err != nil {
			return AgentPool{}, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return AgentPool{}, err
		}
		pool = AgentPool{
			ID: id, Queue: queue, Name: poolName, Agents: agents,
			Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_agent_pool_members WHERE pool_id = ?`, pool.ID); err != nil {
		return AgentPool{}, err
	}
	for position, agent := range agents {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_agent_pool_members(pool_id, agent, position)
			VALUES (?, ?, ?)`, pool.ID, agent, position); err != nil {
			return AgentPool{}, err
		}
	}
	if _, err := appendQueueEventTx(ctx, tx, Queue{Prefix: queue, Revision: pool.Revision},
		"task.agent_pool_rebound", actor, map[string]any{
			"pool": pool.Name, "agents": pool.Agents,
		}, now); err != nil {
		return AgentPool{}, err
	}
	if err := writeTaskIdempotency(
		ctx, tx, actor.Principal, "rebind_agent_pool", idempotencyKey, pool, now,
	); err != nil {
		return AgentPool{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentPool{}, err
	}
	s.signal()
	return pool, nil
}

func (s *Service) GetAgentPool(ctx context.Context, actor Actor, queue, pool string) (AgentPool, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return AgentPool{}, err
	}
	item, found, err := agentPoolByName(ctx, s.db, strings.ToUpper(strings.TrimSpace(queue)), strings.TrimSpace(pool))
	if err != nil {
		return AgentPool{}, err
	}
	if !found {
		return AgentPool{}, domainError(http.StatusNotFound, "workflow_pool_not_found", "workflow agent pool not found")
	}
	return item, nil
}

func (s *Service) ListAgentPools(ctx context.Context, actor Actor, queue string) ([]AgentPool, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return nil, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM task_agent_pools WHERE queue_prefix=? ORDER BY name`, queue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]AgentPool, 0, len(names))
	for _, name := range names {
		item, _, err := agentPoolByName(ctx, s.db, queue, name)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func initializeWorkflowTaskTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
	now string,
) error {
	var initial WorkflowStatus
	found := false
	for _, status := range workflow.Definition.Statuses {
		if status.ID == workflow.Definition.InitialStatus {
			initial, found = status, true
			break
		}
	}
	if !found {
		return domainError(http.StatusConflict, "workflow_invalid", "workflow initial status is missing")
	}
	_, err := materializeStatusTx(ctx, tx, task, workflow, initial, 1, now)
	if _, persisted := persistedWorkflowDomain(err); persisted {
		return nil
	}
	return err
}

func requireQueueExists(ctx context.Context, q queryer, queue string) error {
	var exists int
	if err := q.QueryRowContext(ctx, `SELECT 1 FROM task_queues WHERE prefix = ?`, queue).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return domainError(http.StatusNotFound, "queue_not_found", "queue not found")
		}
		return err
	}
	return nil
}

func requireAgentsExist(ctx context.Context, q queryer, agents []string) error {
	for _, agent := range agents {
		var exists int
		if err := q.QueryRowContext(ctx, `SELECT 1 FROM agents WHERE name = ?`, agent).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return &Error{
					Status: http.StatusBadRequest, Code: "agent_not_found",
					Msg: "agent pool member does not exist", Data: map[string]any{"agent": agent},
				}
			}
			return err
		}
	}
	return nil
}

func queueWorkflowByPrefix(ctx context.Context, q queryer, queue string) (QueueWorkflowBinding, bool, error) {
	var binding QueueWorkflowBinding
	err := q.QueryRowContext(ctx, queueWorkflowSelect+` WHERE qw.queue_prefix = ?`, queue).Scan(
		&binding.Queue, &binding.WorkflowVersionID, &binding.WorkflowName, &binding.WorkflowVersion,
		&binding.Revision, &binding.BoundBy, &binding.BoundAt,
	)
	if err == sql.ErrNoRows {
		return QueueWorkflowBinding{}, false, nil
	}
	return binding, err == nil, err
}

func activeWorkflowForQueue(ctx context.Context, q queryer, queue string) (WorkflowVersion, bool, error) {
	var versionID int64
	err := q.QueryRowContext(ctx, `
		SELECT workflow_version_id FROM task_queue_workflows WHERE queue_prefix = ?`, queue).Scan(&versionID)
	if err == sql.ErrNoRows {
		return WorkflowVersion{}, false, nil
	}
	if err != nil {
		return WorkflowVersion{}, false, err
	}
	workflow, err := workflowVersionByID(ctx, q, versionID)
	if err != nil {
		return WorkflowVersion{}, false, err
	}
	if workflow.State != "published" {
		return WorkflowVersion{}, false, domainError(
			http.StatusConflict, "workflow_not_published", "active queue workflow is not published",
		)
	}
	return workflow, true, nil
}

func agentPoolByName(ctx context.Context, q queryer, queue string, name string) (AgentPool, bool, error) {
	var pool AgentPool
	err := q.QueryRowContext(ctx, `
		SELECT id, queue_prefix, name, revision, created_at, updated_at
		FROM task_agent_pools WHERE queue_prefix = ? AND name = ?`, queue, name).Scan(
		&pool.ID, &pool.Queue, &pool.Name, &pool.Revision, &pool.CreatedAt, &pool.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return AgentPool{}, false, nil
	}
	if err != nil {
		return AgentPool{}, false, err
	}
	rows, err := q.QueryContext(ctx, `
		SELECT agent FROM task_agent_pool_members
		WHERE pool_id = ? ORDER BY position`, pool.ID)
	if err != nil {
		return AgentPool{}, false, err
	}
	defer rows.Close()
	pool.Agents = []string{}
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return AgentPool{}, false, err
		}
		pool.Agents = append(pool.Agents, agent)
	}
	if err := rows.Err(); err != nil {
		return AgentPool{}, false, err
	}
	return pool, true, nil
}

func missingWorkflowPools(ctx context.Context, q queryer, queue string, definition WorkflowDefinition) ([]string, error) {
	missing := make([]string, 0)
	for pool := range referencedWorkflowPools(definition) {
		binding, found, err := agentPoolByName(ctx, q, queue, pool)
		if err != nil {
			return nil, err
		}
		if !found || len(binding.Agents) == 0 {
			missing = append(missing, pool)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func referencedWorkflowPools(definition WorkflowDefinition) map[string]struct{} {
	pools := make(map[string]struct{})
	for _, status := range definition.Statuses {
		for _, requirement := range status.Requirements {
			pools[requirement.Pool] = struct{}{}
		}
	}
	if definition.Questions.RouteTo != "" {
		pools[definition.Questions.RouteTo] = struct{}{}
	}
	return pools
}

func normalizePoolAgents(agents []string) []string {
	return normalizeOwners(agents)
}

func bindingRevisionConflict(current QueueWorkflowBinding) error {
	return &Error{
		Status: http.StatusConflict, Code: "revision_conflict",
		Msg:  "queue workflow binding was changed by another actor",
		Data: map[string]any{"current_revision": current.Revision, "current": current},
	}
}

func poolRevisionConflict(current AgentPool) error {
	return &Error{
		Status: http.StatusConflict, Code: "revision_conflict",
		Msg:  "agent pool was changed by another actor",
		Data: map[string]any{"current_revision": current.Revision, "current": current},
	}
}

func workflowVersionString(version int) string {
	return strconv.Itoa(version)
}
