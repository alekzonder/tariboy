package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// persistedWorkflowError is the transaction boundary marker for a deliberate
// workflow freeze. Callers may commit only this marker; every other error,
// including a typed domain error from failed SQL/CAS work, must roll back.
type persistedWorkflowError struct {
	domain *Error
}

func (e *persistedWorkflowError) Error() string { return e.domain.Error() }

func persistedWorkflowDomain(err error) (*Error, bool) {
	persisted, ok := err.(*persistedWorkflowError)
	if !ok || persisted == nil || persisted.domain == nil {
		return nil, false
	}
	return persisted.domain, true
}

// materializeStatusTx creates a new immutable status execution and freezes the
// current pool membership into each requirement. A require_all requirement
// gets one assignment per frozen member. A claim_one requirement gets one
// ownerless shared CAS assignment announced to every frozen member.
func materializeStatusTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
	status WorkflowStatus,
	sequence int64,
	now string,
) (int64, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_status_executions(
			task_id, workflow_version_id, status_id, sequence, state, task_revision, created_at
		) VALUES (?, ?, ?, ?, 'active', ?, ?)`,
		task.ID, workflow.ID, status.ID, sequence, task.WorkflowRevision, now)
	if err != nil {
		return 0, err
	}
	statusExecutionID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	wouldExceed, err := statusWouldExceedAssignmentBudgetTx(ctx, tx, task, workflow, status)
	if err != nil {
		return 0, err
	}
	if wouldExceed {
		return statusExecutionID, exhaustAssignmentBudgetTx(
			ctx, tx, task, workflow, statusExecutionID, status.ID,
			Actor{Principal: "system:workflow"}, now,
		)
	}
	for _, requirement := range status.Requirements {
		pool, found, err := agentPoolByName(ctx, tx, task.Queue, requirement.Pool)
		if err != nil {
			return 0, err
		}
		if !found || len(pool.Agents) == 0 {
			return 0, &Error{Status: http.StatusConflict, Code: "workflow_pool_empty",
				Msg: "workflow requirement pool has no agents", Data: map[string]any{"pool": requirement.Pool}}
		}
		snapshot := sortedUniqueStrings(pool.Agents)
		rawSnapshot, err := json.Marshal(snapshot)
		if err != nil {
			return 0, err
		}
		rawInputs, err := json.Marshal(requirement.Inputs)
		if err != nil {
			return 0, err
		}
		rawProduces, err := json.Marshal(requirement.Produces)
		if err != nil {
			return 0, err
		}
		rawOutcomes, err := json.Marshal(requirement.Outcomes)
		if err != nil {
			return 0, err
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO task_requirement_executions(
				status_execution_id, requirement_id, pool_id, dispatch, optional,
				pool_snapshot, inputs, produces, outcomes, state, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
			statusExecutionID, requirement.ID, pool.ID, requirement.Dispatch,
			requirement.Optional, string(rawSnapshot), string(rawInputs),
			string(rawProduces), string(rawOutcomes), now)
		if err != nil {
			return 0, err
		}
		requirementExecutionID, err := result.LastInsertId()
		if err != nil {
			return 0, err
		}
		if requirement.Dispatch == DispatchRequireAll {
			for _, agent := range snapshot {
				assignment, err := tx.ExecContext(ctx, `
					INSERT INTO task_assignments(
						requirement_execution_id, agent, attempt, state, revision, created_at, updated_at
					) VALUES (?, ?, 1, 'claimable', 1, ?, ?)`,
					requirementExecutionID, agent, now, now)
				if err != nil {
					return 0, err
				}
				assignmentID, err := assignment.LastInsertId()
				if err != nil {
					return 0, err
				}
				if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, task, assignmentID,
					requirement.ID, requirement.Pool, agent, "workflow.assignment_ready", "", now); err != nil {
					return 0, err
				}
			}
			continue
		}
		assignment, err := tx.ExecContext(ctx, `
			INSERT INTO task_assignments(
				requirement_execution_id, agent, attempt, state, revision, created_at, updated_at
			) VALUES (?, NULL, 1, 'claimable', 1, ?, ?)`,
			requirementExecutionID, now, now)
		if err != nil {
			return 0, err
		}
		assignmentID, err := assignment.LastInsertId()
		if err != nil {
			return 0, err
		}
		for _, agent := range snapshot {
			if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, task, assignmentID,
				requirement.ID, requirement.Pool, agent, "workflow.assignment_ready", "", now); err != nil {
				return 0, err
			}
		}
	}
	return statusExecutionID, nil
}

// evaluateExecutionTx reduces at most one status transition. It waits for all
// non-optional requirements, evaluates every declarative guard, and refuses to
// guess if the result is not exactly one destination.
func evaluateExecutionTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
	statusExecutionID int64,
	actor Actor,
	now string,
) error {
	var statusID, state string
	if err := tx.QueryRowContext(ctx, `
		SELECT status_id, state FROM task_status_executions WHERE id = ?`, statusExecutionID).Scan(&statusID, &state); err != nil {
		return err
	}
	if state != "active" {
		return nil
	}
	status, found := workflowStatusByID(workflow.Definition, statusID)
	if !found {
		return freezeExecutionTx(ctx, tx, task, workflow, statusExecutionID,
			"workflow_status_missing", actor, now)
	}
	if status.Terminal {
		return nil
	}
	var pendingRequired int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_requirement_executions
		WHERE status_execution_id = ? AND optional = 0 AND state <> 'completed'`,
		statusExecutionID).Scan(&pendingRequired); err != nil {
		return err
	}
	if pendingRequired != 0 {
		return nil
	}

	guardState, err := loadGuardState(ctx, tx, task, statusExecutionID)
	if err != nil {
		return err
	}
	matches := make([]WorkflowTransition, 0, 1)
	requirements := make(map[string]WorkflowRequirement, len(status.Requirements))
	artifacts := make(map[string]struct{})
	for _, requirement := range status.Requirements {
		requirements[requirement.ID] = requirement
		for _, name := range append(append([]string(nil), requirement.Inputs...), requirement.Produces...) {
			artifacts[name] = struct{}{}
		}
	}
	for _, transition := range status.Transitions {
		if strings.TrimSpace(transition.When) == "" {
			matches = append(matches, transition)
			continue
		}
		node, parseErr := newGuardParser(transition.When, requirements, artifacts).parse()
		if parseErr != nil {
			return freezeExecutionTx(ctx, tx, task, workflow, statusExecutionID,
				"workflow_guard_invalid", actor, now)
		}
		if evaluateGuard(node, guardState) {
			matches = append(matches, transition)
		}
	}
	if len(matches) == 0 {
		return freezeExecutionTx(ctx, tx, task, workflow, statusExecutionID,
			"workflow_transition_missing", actor, now)
	}
	if len(matches) != 1 {
		return freezeExecutionTx(ctx, tx, task, workflow, statusExecutionID,
			"workflow_transition_ambiguous", actor, now)
	}
	return transitionTaskTx(ctx, tx, task, workflow, statusExecutionID,
		matches[0].To, actor, now, false)
}

// transitionTaskTx closes one execution and creates precisely one new status
// execution. A destination equal to the source status is still a new row.
func transitionTaskTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
	statusExecutionID int64,
	destination string,
	actor Actor,
	now string,
	policyExhausted bool,
) error {
	destination = strings.TrimSpace(destination)
	var currentSequence int64
	if err := tx.QueryRowContext(ctx, `
		SELECT sequence FROM task_status_executions
		WHERE id = ? AND task_id = ? AND state = 'active'`,
		statusExecutionID, task.ID).Scan(&currentSequence); err != nil {
		if err == sql.ErrNoRows {
			return domainError(http.StatusConflict, "workflow_execution_stale", "workflow execution is no longer active")
		}
		return err
	}
	if !policyExhausted && workflow.Definition.Budgets.MaxCycles > 0 &&
		currentSequence >= int64(workflow.Definition.Budgets.MaxCycles) {
		exhausted := strings.TrimSpace(workflow.Definition.Budgets.OnExhausted)
		if exhausted == "" {
			return freezeExecutionTx(ctx, tx, task, workflow, statusExecutionID,
				"workflow_cycle_exhausted", actor, now)
		}
		destination = exhausted
		policyExhausted = true
	}
	nextStatus, found := workflowStatusByID(workflow.Definition, destination)
	if !found {
		code := "workflow_transition_invalid"
		if policyExhausted {
			code = "workflow_exhausted_status_invalid"
		}
		return freezeExecutionTx(ctx, tx, task, workflow, statusExecutionID, code, actor, now)
	}
	wouldExceed, err := statusWouldExceedAssignmentBudgetTx(ctx, tx, task, workflow, nextStatus)
	if err != nil {
		return err
	}
	if wouldExceed {
		return exhaustAssignmentBudgetTx(
			ctx, tx, task, workflow, statusExecutionID, destination, actor, now,
		)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE task_status_executions
		SET state = 'transitioned', transition_to = ?, completed_at = ?
		WHERE id = ? AND task_id = ? AND state = 'active'`,
		destination, now, statusExecutionID, task.ID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domainError(http.StatusConflict, "workflow_execution_stale", "workflow execution changed concurrently")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_assignments
		SET state = 'released', lease_iteration = '', revision = revision + 1, updated_at = ?
		WHERE state IN ('claimable', 'leased')
		  AND requirement_execution_id IN (
			SELECT id FROM task_requirement_executions
			WHERE status_execution_id = ? AND requirement_id NOT GLOB '__question:*'
		  )`, now, statusExecutionID); err != nil {
		return err
	}

	legacyStatus, completedAt := StatusInProgress, ""
	if nextStatus.Terminal {
		legacyStatus, completedAt = StatusDone, now
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET workflow_status = ?, workflow_revision = workflow_revision + 1,
		    status = ?, completed_at = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND workflow_version_id = ? AND workflow_revision = ?`,
		destination, legacyStatus, completedAt, now, task.ID, workflow.ID, task.WorkflowRevision)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domainError(http.StatusConflict, "revision_conflict", "workflow task changed concurrently")
	}
	task.WorkflowStatus = destination
	task.WorkflowRevision++
	task.Status = legacyStatus
	task.CompletedAt = completedAt
	task.Revision++
	task.UpdatedAt = now
	if _, err := materializeStatusTx(ctx, tx, task, workflow, nextStatus, currentSequence+1, now); err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.transitioned", actor,
		map[string]any{"from_execution_id": statusExecutionID, "to": destination,
			"policy_exhausted": policyExhausted}, now); err != nil {
		return err
	}
	return nil
}

func freezeExecutionTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
	statusExecutionID int64,
	code string,
	actor Actor,
	now string,
) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE task_status_executions SET state = 'frozen'
		WHERE id = ? AND task_id = ? AND state = 'active'`, statusExecutionID, task.ID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domainError(http.StatusConflict, "workflow_execution_stale", "workflow execution changed concurrently")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_requirement_executions SET state = 'frozen'
		WHERE status_execution_id = ? AND state = 'pending'`, statusExecutionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_assignments
		SET state = 'failed', lease_owner = '', lease_iteration = '', lease_expires_at = '',
		    revision = revision + 1, updated_at = ?
		WHERE state IN ('claimable', 'leased')
		  AND requirement_execution_id IN (
			SELECT id FROM task_requirement_executions WHERE status_execution_id = ?
		  )`, now, statusExecutionID); err != nil {
		return err
	}

	recipients := make([]string, 0)
	managerPool := strings.TrimSpace(workflow.Definition.Questions.RouteTo)
	if managerPool != "" {
		if pool, found, poolErr := agentPoolByName(ctx, tx, task.Queue, managerPool); poolErr != nil {
			return poolErr
		} else if found {
			recipients = append(recipients, pool.Agents...)
		}
	}
	if len(recipients) == 0 {
		recipients = append(recipients, strings.TrimPrefix(task.Customer, "user:"))
	}
	for _, recipient := range sortedUniqueStrings(recipients) {
		if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, task, nil, "", managerPool, recipient,
			"workflow.escalated", code, now); err != nil {
			return err
		}
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.escalated", actor,
		map[string]any{"execution_id": statusExecutionID, "error_code": code}, now); err != nil {
		return err
	}
	return &persistedWorkflowError{domain: &Error{
		Status: http.StatusConflict, Code: code,
		Msg: "workflow execution was frozen and escalated",
	}}
}

func assignmentBudgetAvailableTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
) (bool, error) {
	limit := workflow.Definition.Budgets.MaxAssignments
	if limit <= 0 {
		return true, nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		WHERE se.task_id = ?`, task.ID).Scan(&count); err != nil {
		return false, err
	}
	return count < limit, nil
}

func exhaustAssignmentBudgetTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
	statusExecutionID int64,
	currentDestination string,
	actor Actor,
	now string,
) error {
	exhausted := strings.TrimSpace(workflow.Definition.Budgets.OnExhausted)
	if exhausted == "" || exhausted == strings.TrimSpace(currentDestination) {
		return freezeExecutionTx(ctx, tx, task, workflow, statusExecutionID,
			"workflow_assignment_budget_exhausted", actor, now)
	}
	return transitionTaskTx(ctx, tx, task, workflow, statusExecutionID,
		exhausted, actor, now, true)
}

func statusWouldExceedAssignmentBudgetTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	workflow WorkflowVersion,
	status WorkflowStatus,
) (bool, error) {
	limit := workflow.Definition.Budgets.MaxAssignments
	if limit <= 0 {
		return false, nil
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		WHERE se.task_id = ?`, task.ID).Scan(&existing); err != nil {
		return false, err
	}
	demand := 0
	for _, requirement := range status.Requirements {
		if requirement.Dispatch == DispatchClaimOne {
			demand++
			continue
		}
		pool, found, err := agentPoolByName(ctx, tx, task.Queue, requirement.Pool)
		if err != nil {
			return false, err
		}
		if !found || len(pool.Agents) == 0 {
			return false, domainError(http.StatusConflict, "workflow_pool_empty",
				"workflow requirement pool has no agents")
		}
		demand += len(sortedUniqueStrings(pool.Agents))
	}
	return existing+demand > limit, nil
}

type workflowGuardState struct {
	task      Task
	outcomes  map[string][]string
	artifacts map[string]bool
}

func loadGuardState(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	statusExecutionID int64,
) (workflowGuardState, error) {
	state := workflowGuardState{task: task, outcomes: map[string][]string{}, artifacts: map[string]bool{}}
	rows, err := tx.QueryContext(ctx, `
		SELECT re.requirement_id, a.outcome
		FROM task_requirement_executions re
		JOIN task_assignments a ON a.requirement_execution_id = re.id
		WHERE re.status_execution_id = ? AND a.state = 'completed'
		ORDER BY re.id, a.agent, a.attempt`, statusExecutionID)
	if err != nil {
		return workflowGuardState{}, err
	}
	for rows.Next() {
		var requirement, outcome string
		if err := rows.Scan(&requirement, &outcome); err != nil {
			rows.Close()
			return workflowGuardState{}, err
		}
		state.outcomes[requirement] = append(state.outcomes[requirement], outcome)
	}
	if err := rows.Close(); err != nil {
		return workflowGuardState{}, err
	}
	if err := rows.Err(); err != nil {
		return workflowGuardState{}, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT name FROM task_artifacts WHERE task_id = ?`, task.ID)
	if err != nil {
		return workflowGuardState{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return workflowGuardState{}, err
		}
		state.artifacts[name] = true
	}
	return state, rows.Err()
}

func evaluateGuard(node guardNode, state workflowGuardState) bool {
	switch typed := node.(type) {
	case guardBinary:
		if typed.op == "&&" {
			return evaluateGuard(typed.left, state) && evaluateGuard(typed.right, state)
		}
		return evaluateGuard(typed.left, state) || evaluateGuard(typed.right, state)
	case guardPredicate:
		switch typed.kind {
		case "artifact_exists":
			return state.artifacts[typed.subject]
		case "requirement_outcome", "any":
			return containsString(state.outcomes[typed.subject], typed.value)
		case "all":
			outcomes := state.outcomes[typed.subject]
			if len(outcomes) == 0 {
				return false
			}
			for _, outcome := range outcomes {
				if outcome != typed.value {
					return false
				}
			}
			return true
		case "task":
			actual := workflowTaskGuardField(state.task, typed.subject)
			if typed.operator == "!=" {
				return actual != typed.value
			}
			return actual == typed.value
		}
	}
	return false
}

func workflowTaskGuardField(task Task, field string) string {
	switch field {
	case "key":
		return task.Key
	case "queue":
		return task.Queue
	case "priority":
		return string(task.Priority)
	case "status":
		return task.Status
	case "author":
		return task.Author
	case "customer":
		return task.Customer
	case "group":
		return task.Group
	case "blocked":
		return strconv.FormatBool(task.Blocked)
	default:
		return ""
	}
}

func workflowStatusByID(definition WorkflowDefinition, id string) (WorkflowStatus, bool) {
	for _, status := range definition.Statuses {
		if status.ID == id {
			return status, true
		}
	}
	return WorkflowStatus{}, false
}
