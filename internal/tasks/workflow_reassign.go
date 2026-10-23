package tasks

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// ReassignWorkflow hands the current pool status of a workflow task to name, a
// member of that pool the customer chose. Customer only. The old holder's
// questions to the customer are resolved and the visit's stall counters reset;
// the new holder becomes the sticky holder of the pool. Reassigning to the
// current holder changes nothing.
func (s *WorkflowEngine) ReassignWorkflow(ctx context.Context, actor Actor, key, name string) (Task, error) {
	if err := requireWorkflowOperator(actor); err != nil {
		return Task{}, err
	}
	name = strings.TrimPrefix(strings.TrimSpace(name), "agent:")
	if name == "" {
		return Task{}, newError(http.StatusBadRequest, "invalid_agent", "reassign needs the name of an agent")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	task, manifest, err := artifactTaskTx(ctx, tx, actor, key)
	if err != nil {
		return Task{}, err
	}
	status, err := reassignStatusTx(ctx, tx, task, manifest, name)
	if err != nil {
		return Task{}, err
	}
	if task.Assignee == agentPrincipal(name) {
		task.Access = "write"
		return task, nil
	}
	now := s.now()
	if err := guardRevisionTx(ctx, tx, task); err != nil {
		return Task{}, err
	}
	if err := resolveWorkflowWaitsTx(ctx, tx, task, task.Assignee, now); err != nil {
		return Task{}, err
	}
	// The old holder's idle iterations and rejected requests must not pause
	// the task under the new one.
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_status_visits SET idle_iterations = 0, rejected_requests = 0
		WHERE task_id = ? AND left_at = ''`, task.ID); err != nil {
		return Task{}, err
	}
	if err := s.recordHolderTx(ctx, tx, task, status.Owner.Pool, name); err != nil {
		return Task{}, err
	}
	previousAssignee, previousCategory := task.Assignee, task.Status
	state := stateIn(task, status)
	state.Holder = agentPrincipal(name)
	if err := applyState(&task, state); err != nil {
		return Task{}, err
	}
	if _, err := saveTaskStateTx(ctx, tx, &task, previousCategory, now, false); err != nil {
		return Task{}, err
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.reassigned", actor, map[string]any{
		"status": task.WorkflowStatus, "pool": status.Owner.Pool, "from": previousAssignee,
		"to": task.Assignee, "actor": actor.Principal,
	}, now); err != nil {
		return Task{}, err
	}
	if err := recordAssignmentTx(ctx, tx, task, previousAssignee, now); err != nil {
		return Task{}, err
	}
	reassigned, err := taskByID(ctx, tx, task.ID)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	s.signal()
	reassigned.Access = "write"
	return reassigned, nil
}

// reassignStatusTx returns the current status of task when name may take it
// over: an open, unpaused pool status with no pending request, and a member of
// its pool that can work by holderUnavailableCauseSQL.
func reassignStatusTx(ctx context.Context, tx *sql.Tx, task Task, manifest workflowimage.Manifest, name string) (workflowfile.Status, error) {
	status, ok := currentStatus(manifest, task.WorkflowStatus)
	if !ok || status.Terminal || status.Owner.Kind != workflowfile.OwnerPool {
		return workflowfile.Status{}, newError(http.StatusConflict, "invalid_status_owner",
			"only a status owned by an agent pool can be reassigned; status "+task.WorkflowStatus+" is not")
	}
	if err := requireOpenWorkflow(task); err != nil {
		return workflowfile.Status{}, err
	}
	var pending bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM task_transition_requests WHERE task_id = ? AND state = 'pending')`,
		task.ID).Scan(&pending); err != nil {
		return workflowfile.Status{}, err
	}
	if pending {
		return workflowfile.Status{}, newError(http.StatusConflict, "transition_pending",
			"a transition request of "+task.Key+" is still running")
	}
	var cause string
	if err := tx.QueryRowContext(ctx, `
		SELECT `+holderUnavailableCauseSQL+`
		FROM (SELECT ? AS name) x
		LEFT JOIN agents a ON a.name = x.name
		LEFT JOIN task_agent_pools p ON p.queue_prefix = ? AND p.name = ?
		LEFT JOIN task_agent_pool_members m ON m.pool_id = p.id AND m.agent = x.name`,
		agent.IdleStopPrefix, agent.IdleStopPrefix, name, task.Queue, status.Owner.Pool).Scan(&cause); err != nil {
		return workflowfile.Status{}, err
	}
	if cause != "" {
		return workflowfile.Status{}, newError(http.StatusConflict, "agent_unavailable",
			"agent "+name+" cannot take status "+status.ID+" of pool "+status.Owner.Pool+": "+cause).
			WithData(map[string]any{"agent": name, "pool": status.Owner.Pool, "cause": cause})
	}
	return status, nil
}
