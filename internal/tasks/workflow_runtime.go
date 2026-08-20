package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAssignmentLease    = 30 * time.Minute
	defaultAssignmentAttempts = 3
)

type ClaimAssignmentInput struct {
	TaskRevision       int64  `json:"task_revision"`
	AssignmentRevision int64  `json:"assignment_revision"`
	IdempotencyKey     string `json:"idempotency_key"`
	IterationID        string `json:"-"`
}

type ReleaseAssignmentInput struct {
	TaskRevision       int64  `json:"task_revision"`
	AssignmentRevision int64  `json:"assignment_revision"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type CompleteAssignmentInput struct {
	TaskRevision       int64  `json:"task_revision"`
	AssignmentRevision int64  `json:"assignment_revision"`
	Outcome            string `json:"outcome"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type assignmentMutationReplay struct {
	Assignment Assignment  `json:"assignment"`
	Packet     *WorkPacket `json:"work_packet,omitempty"`
	ErrorCode  string      `json:"error_code,omitempty"`
	ErrorMsg   string      `json:"error_message,omitempty"`
}

type assignmentContext struct {
	Assignment
	Task                   Task
	StatusExecutionID      int64
	StatusExecutionState   string
	RequirementID          string
	Pool                   string
	RequirementState       string
	Dispatch               string
	PoolSnapshot           []string
	AllowedOutcomes        []string
	Workflow               WorkflowVersion
	RequirementExecutionID int64
}

// NextWork returns active work visible to an agent without mutating workflow
// state. claim_one requirements use one transactionally materialized ownerless
// assignment as the shared CAS token; require_all assignments remain bound to
// the immutable pool snapshot captured at status materialization.
func (s *Service) NextWork(
	ctx context.Context,
	actor Actor,
	queue string,
	limit int,
) ([]Assignment, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return nil, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.requirement_execution_id, a.agent, a.attempt, a.state,
		       a.lease_owner, a.lease_iteration, a.lease_expires_at, a.revision, a.outcome,
		       a.created_at, a.updated_at, a.completed_at,
		       re.dispatch, re.pool_snapshot, se.status_id, w.definition
		FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		JOIN tasks t ON t.id = se.task_id
		JOIN task_workflow_versions w ON w.id = se.workflow_version_id
		WHERE (se.state = 'active' OR re.requirement_id GLOB '__question:*')
		  AND re.state = 'pending'
		  AND a.state IN ('claimable', 'leased')
		  AND NOT EXISTS (
		    SELECT 1 FROM task_workflow_holds h
		    WHERE h.released_at = ''
		      AND ((h.scope = 'assignment' AND h.assignment_id = a.id)
		        OR (h.scope = 'requirement' AND h.requirement_execution_id = re.id))
		  )
		  AND (? = '' OR t.queue_prefix = ?)
		ORDER BY t.queue_prefix, t.priority, t.position, t.task_key,
		         se.sequence, re.id, a.attempt, a.id`, queue, queue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agent := strings.TrimPrefix(actor.Principal, "agent:")
	now := s.clock().UTC()
	out := make([]Assignment, 0, limit)
	for rows.Next() {
		var assignment Assignment
		var assignmentAgent sql.NullString
		var dispatch, rawSnapshot, statusID, rawDefinition string
		if err := rows.Scan(
			&assignment.ID, &assignment.RequirementExecutionID, &assignmentAgent,
			&assignment.Attempt, &assignment.State, &assignment.LeaseOwner, &assignment.LeaseIteration,
			&assignment.LeaseExpiresAt, &assignment.Revision, &assignment.Outcome,
			&assignment.CreatedAt, &assignment.UpdatedAt, &assignment.CompletedAt,
			&dispatch, &rawSnapshot, &statusID, &rawDefinition,
		); err != nil {
			return nil, err
		}
		var definition WorkflowDefinition
		if err := json.Unmarshal([]byte(rawDefinition), &definition); err != nil {
			return nil, err
		}
		status, found := workflowStatusByID(definition, statusID)
		if !found || status.Terminal {
			continue
		}
		assignment.Agent = assignmentAgent.String
		var snapshot []string
		if err := json.Unmarshal([]byte(rawSnapshot), &snapshot); err != nil {
			return nil, err
		}
		visible := false
		switch assignment.State {
		case AssignmentLeased:
			deadline, parseErr := time.Parse(time.RFC3339Nano, assignment.LeaseExpiresAt)
			visible = parseErr == nil && deadline.After(now) && assignment.LeaseOwner == actor.Principal
		case AssignmentClaimable:
			if dispatch == DispatchRequireAll {
				visible = assignment.Agent == agent
			} else {
				visible = containsString(snapshot, agent)
			}
		}
		if visible {
			out = append(out, assignment)
			if len(out) == limit {
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) ClaimAssignment(
	ctx context.Context,
	actor Actor,
	assignmentID string,
	in ClaimAssignmentInput,
) (Assignment, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return Assignment{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return Assignment{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return Assignment{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Assignment{}, err
	}
	defer tx.Rollback()
	if replay, ok, err := readAssignmentReplay(ctx, tx, actor, "claim_assignment", in.IdempotencyKey); err != nil {
		return Assignment{}, err
	} else if ok {
		return replay.Assignment, replay.err()
	}
	if err := requireAssignmentMutationRevisions(in.TaskRevision, in.AssignmentRevision); err != nil {
		return Assignment{}, err
	}
	current, err := assignmentContextByID(ctx, tx, id)
	if err != nil {
		return Assignment{}, err
	}
	if err := authorizeClaim(actor, current); err != nil {
		return Assignment{}, err
	}
	if current.State != AssignmentClaimable {
		return Assignment{}, domainError(http.StatusConflict, "assignment_already_claimed", "assignment is no longer claimable")
	}
	if err := requireRuntimeRevisions(current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return Assignment{}, err
	}
	if err := workflowAssignmentHeld(ctx, tx, current.ID, current.RequirementExecutionID); err != nil {
		return Assignment{}, err
	}
	leaseDuration, err := workflowLeaseDuration(current.Workflow.Definition)
	if err != nil {
		return Assignment{}, err
	}
	nowTime := s.clock().UTC()
	now, deadline := nowTime.Format(time.RFC3339Nano), nowTime.Add(leaseDuration).Format(time.RFC3339Nano)
	agent := strings.TrimPrefix(actor.Principal, "agent:")
	result, err := tx.ExecContext(ctx, `
		UPDATE task_assignments
		SET agent = ?, state = 'leased', lease_owner = ?, lease_iteration = ?, lease_expires_at = ?,
		    revision = revision + 1, updated_at = ?
		WHERE id = ? AND state = 'claimable' AND revision = ?
		  AND EXISTS (
			SELECT 1
			FROM task_requirement_executions re
			JOIN task_status_executions se ON se.id = re.status_execution_id
			JOIN tasks t ON t.id = se.task_id
			WHERE re.id = task_assignments.requirement_execution_id
			  AND re.state = 'pending'
			  AND (se.state = 'active' OR re.requirement_id GLOB '__question:*')
			  AND t.workflow_revision = ?
			  AND (re.dispatch = 'require_all' OR NOT EXISTS (
				SELECT 1 FROM task_assignments other
				WHERE other.requirement_execution_id = re.id
				  AND other.id <> task_assignments.id
				  AND other.state IN ('leased', 'completed')
			  ))
		  )`,
		agent, actor.Principal, strings.TrimSpace(in.IterationID), deadline, now, id, in.AssignmentRevision, in.TaskRevision)
	if err != nil {
		return Assignment{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Assignment{}, domainError(http.StatusConflict, "assignment_already_claimed", "assignment was claimed concurrently")
	}
	claimed, err := assignmentByID(ctx, tx, id)
	if err != nil {
		return Assignment{}, err
	}
	current.Task.Access = "write"
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.assignment_claimed", actor,
		map[string]any{"assignment_id": id, "attempt": claimed.Attempt}, now); err != nil {
		return Assignment{}, err
	}
	replay := assignmentMutationReplay{Assignment: claimed}
	if strings.TrimSpace(in.IterationID) != "" {
		refreshed, packetErr := assignmentContextByID(ctx, tx, id)
		if packetErr != nil {
			return Assignment{}, packetErr
		}
		packet, packetErr := buildWorkPacket(ctx, tx, actor, refreshed)
		if packetErr != nil {
			return Assignment{}, packetErr
		}
		replay.Packet = &packet
	}
	if err := writeAssignmentReplay(ctx, tx, actor, "claim_assignment", in.IdempotencyKey,
		replay, now); err != nil {
		return Assignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Assignment{}, err
	}
	s.signal()
	return claimed, nil
}

func (s *Service) ReleaseAssignment(
	ctx context.Context,
	actor Actor,
	assignmentID string,
	in ReleaseAssignmentInput,
) (Assignment, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return Assignment{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return Assignment{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return Assignment{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Assignment{}, err
	}
	defer tx.Rollback()
	if replay, ok, err := readAssignmentReplay(ctx, tx, actor, "release_assignment", in.IdempotencyKey); err != nil {
		return Assignment{}, err
	} else if ok {
		return replay.Assignment, replay.err()
	}
	if err := requireAssignmentMutationRevisions(in.TaskRevision, in.AssignmentRevision); err != nil {
		return Assignment{}, err
	}
	current, err := assignmentContextByID(ctx, tx, id)
	if err != nil {
		return Assignment{}, err
	}
	if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
		return Assignment{}, err
	}
	if err := requireRuntimeRevisions(current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return Assignment{}, err
	}
	if err := workflowAssignmentHeld(ctx, tx, current.ID, current.RequirementExecutionID); err != nil {
		return Assignment{}, err
	}
	now := s.now()
	result, err := tx.ExecContext(ctx, `
		UPDATE task_assignments
		SET state = 'released', lease_iteration = '', revision = revision + 1, updated_at = ?
		WHERE id = ? AND state = 'leased' AND lease_owner = ?
		  AND lease_expires_at = ? AND revision = ?
		  AND EXISTS (
			SELECT 1 FROM task_requirement_executions re
			JOIN task_status_executions se ON se.id = re.status_execution_id
			JOIN tasks t ON t.id = se.task_id
			WHERE re.id = task_assignments.requirement_execution_id
			  AND re.state = 'pending'
			  AND (se.state = 'active' OR re.requirement_id GLOB '__question:*')
			  AND t.workflow_revision = ?
		  )`, now, id, actor.Principal, current.LeaseExpiresAt,
		in.AssignmentRevision, in.TaskRevision)
	if err != nil {
		return Assignment{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Assignment{}, domainError(http.StatusConflict, "assignment_lease_lost", "assignment lease changed concurrently")
	}
	released, err := assignmentByID(ctx, tx, id)
	if err != nil {
		return Assignment{}, err
	}
	workflowErr := resumeOrExhaustAssignmentTx(ctx, tx, current, now, actor)
	var committedErr error
	if workflowErr != nil {
		domain, persisted := persistedWorkflowDomain(workflowErr)
		if !persisted {
			return Assignment{}, workflowErr
		}
		committedErr = domain
	}
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.assignment_released", actor,
		map[string]any{"assignment_id": id, "attempt": released.Attempt}, now); err != nil {
		return Assignment{}, err
	}
	replay := assignmentMutationReplay{Assignment: released}
	if committedErr != nil {
		replay.ErrorCode, replay.ErrorMsg = ErrorCode(committedErr), committedErr.Error()
	}
	if err := writeAssignmentReplay(ctx, tx, actor, "release_assignment", in.IdempotencyKey, replay, now); err != nil {
		return Assignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Assignment{}, err
	}
	s.signal()
	return released, committedErr
}

func (s *Service) CompleteAssignment(
	ctx context.Context,
	actor Actor,
	assignmentID string,
	in CompleteAssignmentInput,
) (Assignment, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return Assignment{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return Assignment{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return Assignment{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Assignment{}, err
	}
	defer tx.Rollback()
	if replay, ok, err := readAssignmentReplay(ctx, tx, actor, "complete_assignment", in.IdempotencyKey); err != nil {
		return Assignment{}, err
	} else if ok {
		return replay.Assignment, replay.err()
	}
	if err := requireAssignmentMutationRevisions(in.TaskRevision, in.AssignmentRevision); err != nil {
		return Assignment{}, err
	}
	current, err := assignmentContextByID(ctx, tx, id)
	if err != nil {
		return Assignment{}, err
	}
	if current.State == AssignmentCompleted {
		return Assignment{}, domainError(http.StatusConflict, "assignment_already_completed", "assignment is already completed")
	}
	if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
		return Assignment{}, err
	}
	if err := requireRuntimeRevisions(current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return Assignment{}, err
	}
	if err := workflowAssignmentHeld(ctx, tx, current.ID, current.RequirementExecutionID); err != nil {
		return Assignment{}, err
	}
	outcome := strings.TrimSpace(in.Outcome)
	if outcome == "" && len(current.AllowedOutcomes) == 1 {
		outcome = current.AllowedOutcomes[0]
	}
	if !containsString(current.AllowedOutcomes, outcome) {
		return Assignment{}, &Error{Status: http.StatusBadRequest, Code: "invalid_outcome",
			Msg: "outcome is not allowed for this assignment", Data: map[string]any{"allowed_outcomes": current.AllowedOutcomes}}
	}
	if err := requireAssignmentOutputsTx(ctx, tx, current); err != nil {
		return Assignment{}, err
	}
	now := s.now()
	result, err := tx.ExecContext(ctx, `
		UPDATE task_assignments
		SET state = 'completed', lease_iteration = '', outcome = ?, revision = revision + 1,
		    updated_at = ?, completed_at = ?
		WHERE id = ? AND state = 'leased' AND lease_owner = ?
		  AND lease_expires_at = ? AND revision = ?
		  AND EXISTS (
			SELECT 1 FROM task_requirement_executions re
			JOIN task_status_executions se ON se.id = re.status_execution_id
			JOIN tasks t ON t.id = se.task_id
			WHERE re.id = task_assignments.requirement_execution_id
			  AND re.state = 'pending' AND se.state = 'active'
			  AND t.workflow_revision = ?
		  )`, outcome, now, now, id, actor.Principal, current.LeaseExpiresAt,
		in.AssignmentRevision, in.TaskRevision)
	if err != nil {
		return Assignment{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Assignment{}, domainError(http.StatusConflict, "assignment_lease_lost", "assignment lease changed concurrently")
	}
	completed, err := assignmentByID(ctx, tx, id)
	if err != nil {
		return Assignment{}, err
	}
	if err := completeRequirementIfReadyTx(ctx, tx, current, now); err != nil {
		return Assignment{}, err
	}
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.assignment_completed", actor,
		map[string]any{"assignment_id": id, "attempt": completed.Attempt, "outcome": outcome}, now); err != nil {
		return Assignment{}, err
	}
	workflowErr := evaluateExecutionTx(ctx, tx, current.Task, current.Workflow,
		current.StatusExecutionID, actor, now)
	var committedErr error
	if workflowErr != nil {
		domain, persisted := persistedWorkflowDomain(workflowErr)
		if !persisted {
			return Assignment{}, workflowErr
		}
		committedErr = domain
	}
	replay := assignmentMutationReplay{Assignment: completed}
	if committedErr != nil {
		replay.ErrorCode, replay.ErrorMsg = ErrorCode(committedErr), committedErr.Error()
	}
	if err := writeAssignmentReplay(ctx, tx, actor, "complete_assignment", in.IdempotencyKey, replay, now); err != nil {
		return Assignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Assignment{}, err
	}
	s.signal()
	return completed, committedErr
}

func requireAssignmentOutputsTx(ctx context.Context, tx *sql.Tx, current assignmentContext) error {
	var rawProduces string
	if err := tx.QueryRowContext(ctx, `
		SELECT produces FROM task_requirement_executions
		WHERE id = ? AND status_execution_id = ?`,
		current.RequirementExecutionID, current.StatusExecutionID).Scan(&rawProduces); err != nil {
		return err
	}
	var produces []string
	if err := json.Unmarshal([]byte(rawProduces), &produces); err != nil {
		return err
	}
	missing := make([]string, 0)
	for _, name := range produces {
		var exists bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM task_artifacts
				WHERE task_id = ? AND assignment_id = ? AND name = ?
			)`, current.Task.ID, current.ID, name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return &Error{Status: http.StatusConflict, Code: "missing_artifact",
			Msg: "assignment is missing required output artifacts", Data: map[string]any{"missing": missing}}
	}
	return nil
}

// ExpireLeases is a system reconciliation operation. Each expiry uses a
// conditional state/revision/deadline update so the same implementation stays
// correct if the store later permits more than one writer connection.
func (s *Service) ExpireLeases(ctx context.Context, nowTime time.Time) (int, error) {
	nowTime = nowTime.UTC()
	now := nowTime.Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT a.id, a.lease_expires_at FROM task_assignments a
		WHERE a.state = 'leased' AND a.lease_expires_at <> ''
		  AND NOT EXISTS (
		    SELECT 1 FROM task_workflow_holds h
		    WHERE h.released_at = ''
		      AND ((h.scope = 'assignment' AND h.assignment_id = a.id)
		        OR (h.scope = 'requirement' AND h.requirement_execution_id = a.requirement_execution_id))
		  )
		ORDER BY a.id`)
	if err != nil {
		return 0, err
	}
	type dueLease struct {
		id       int64
		deadline string
	}
	due := make([]dueLease, 0)
	for rows.Next() {
		var candidate dueLease
		if err := rows.Scan(&candidate.id, &candidate.deadline); err != nil {
			rows.Close()
			return 0, err
		}
		deadline, parseErr := time.Parse(time.RFC3339Nano, candidate.deadline)
		if parseErr == nil && !deadline.After(nowTime) {
			due = append(due, candidate)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	expired := 0
	actor := Actor{Principal: "system:workflow"}
	expiredContexts := make([]assignmentContext, 0, len(due))
	for _, candidate := range due {
		current, err := assignmentContextByID(ctx, tx, candidate.id)
		if err != nil {
			return 0, err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE task_assignments
			SET state = 'expired', lease_iteration = '', revision = revision + 1, updated_at = ?
			WHERE id = ? AND state = 'leased' AND revision = ? AND lease_expires_at = ?
			  AND EXISTS (
				SELECT 1 FROM task_requirement_executions re
				JOIN task_status_executions se ON se.id = re.status_execution_id
				WHERE re.id = task_assignments.requirement_execution_id
				  AND re.state = 'pending'
				  AND (se.state = 'active' OR re.requirement_id GLOB '__question:*')
			  )`, now, candidate.id, current.Revision, candidate.deadline)
		if err != nil {
			return 0, err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			continue
		}
		expired++
		expiredContexts = append(expiredContexts, current)
		if _, err := appendEventTx(ctx, tx, current.Task, "workflow.assignment_expired", actor,
			map[string]any{"assignment_id": candidate.id, "attempt": current.Attempt}, now); err != nil {
			return 0, err
		}
	}
	var firstWorkflowErr error
	for _, current := range expiredContexts {
		var executionState string
		if err := tx.QueryRowContext(ctx, `
			SELECT state FROM task_status_executions WHERE id = ?`,
			current.StatusExecutionID).Scan(&executionState); err != nil {
			return 0, err
		}
		if executionState != "active" && !strings.HasPrefix(current.RequirementID, questionRequirementPrefix) {
			continue
		}
		if err := resumeOrExhaustAssignmentTx(ctx, tx, current, now, actor); err != nil {
			domain, persisted := persistedWorkflowDomain(err)
			if !persisted {
				return 0, err
			}
			if firstWorkflowErr == nil {
				firstWorkflowErr = domain
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if expired > 0 {
		s.signal()
	}
	return expired, firstWorkflowErr
}

func completeRequirementIfReadyTx(ctx context.Context, tx *sql.Tx, current assignmentContext, now string) error {
	ready := current.Dispatch == DispatchClaimOne
	if current.Dispatch == DispatchRequireAll {
		ready = true
		for _, agent := range current.PoolSnapshot {
			var completed bool
			if err := tx.QueryRowContext(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM task_assignments
					WHERE requirement_execution_id = ? AND agent = ? AND state = 'completed'
				)`, current.RequirementExecutionID, agent).Scan(&completed); err != nil {
				return err
			}
			if !completed {
				ready = false
				break
			}
		}
	}
	if !ready {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE task_requirement_executions
		SET state = 'completed', completed_at = ?
		WHERE id = ? AND state = 'pending'`, now, current.RequirementExecutionID)
	return err
}

func resumeOrExhaustAssignmentTx(
	ctx context.Context,
	tx *sql.Tx,
	current assignmentContext,
	now string,
	actor Actor,
) error {
	maxAttempts := current.Workflow.Definition.Retries.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultAssignmentAttempts
	}
	if current.Attempt >= maxAttempts {
		if strings.HasPrefix(current.RequirementID, questionRequirementPrefix) {
			return exhaustWorkflowQuestionAssignmentTx(ctx, tx, current, actor, now)
		}
		destination := strings.TrimSpace(current.Workflow.Definition.Retries.OnExhausted)
		if destination == "" {
			return freezeExecutionTx(ctx, tx, current.Task, current.Workflow,
				current.StatusExecutionID, "workflow_retry_exhausted", actor, now)
		}
		return transitionTaskTx(ctx, tx, current.Task, current.Workflow,
			current.StatusExecutionID, destination, actor, now, true)
	}
	available, err := assignmentBudgetAvailableTx(ctx, tx, current.Task, current.Workflow)
	if err != nil {
		return err
	}
	if !available {
		return exhaustAssignmentBudgetTx(
			ctx, tx, current.Task, current.Workflow, current.StatusExecutionID,
			current.Task.WorkflowStatus, actor, now,
		)
	}
	var assignmentAgent any = current.Agent
	if current.Dispatch == DispatchClaimOne {
		assignmentAgent = nil
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_assignments(
			requirement_execution_id, agent, attempt, state, revision, created_at, updated_at
		) VALUES (?, ?, ?, 'claimable', 1, ?, ?)`,
		current.RequirementExecutionID, assignmentAgent, current.Attempt+1, now, now)
	if err != nil {
		return err
	}
	newID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	recipients := []string{current.Agent}
	if current.Dispatch == DispatchClaimOne {
		recipients = current.PoolSnapshot
	}
	for _, recipient := range recipients {
		if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, current.Task, newID,
			current.RequirementID, current.Pool, recipient, "workflow.assignment_resumed", "", now); err != nil {
			return err
		}
	}
	return nil
}

func assignmentContextByID(ctx context.Context, q queryer, id int64) (assignmentContext, error) {
	var current assignmentContext
	var assignmentAgent sql.NullString
	var rawSnapshot, rawOutcomes, rawDefinition string
	err := q.QueryRowContext(ctx, `
		SELECT a.id, a.requirement_execution_id, a.agent, a.attempt, a.state,
		       a.lease_owner, a.lease_iteration, a.lease_expires_at, a.revision, a.outcome,
		       a.created_at, a.updated_at, a.completed_at,
		       re.requirement_id, p.name, re.state, re.dispatch, re.pool_snapshot, re.outcomes,
		       se.id, se.state,
		       t.id, t.task_key, t.queue_prefix, t.position, t.priority,
		       t.title, t.description, t.status, t.author, t.customer, t.group_name,
		       t.assignee, t.manual_block_reason,
		       EXISTS (
		         SELECT 1
		         FROM task_relations relation
		         JOIN tasks blocker ON blocker.id = relation.source_id
		         WHERE relation.target_id = t.id AND relation.type = 'blocks'
		           AND blocker.status NOT IN ('done', 'cancelled')
		       ) OR t.manual_block_reason <> '',
		       COALESCE(t.workflow_version_id, 0), COALESCE(t.workflow_status, ''),
		       COALESCE(t.workflow_revision, 0), t.revision, t.created_at, t.updated_at,
		       t.completed_at,
		       w.id, w.name, w.version, w.state, w.definition,
		       w.created_at, w.updated_at, w.published_at
		FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_agent_pools p ON p.id = re.pool_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		JOIN tasks t ON t.id = se.task_id
		JOIN task_workflow_versions w ON w.id = se.workflow_version_id
		WHERE a.id = ?`, id).Scan(
		&current.ID, &current.RequirementExecutionID, &assignmentAgent, &current.Attempt,
		&current.State, &current.LeaseOwner, &current.LeaseIteration, &current.LeaseExpiresAt, &current.Revision,
		&current.Outcome, &current.CreatedAt, &current.UpdatedAt, &current.CompletedAt,
		&current.RequirementID, &current.Pool, &current.RequirementState, &current.Dispatch,
		&rawSnapshot, &rawOutcomes, &current.StatusExecutionID, &current.StatusExecutionState,
		&current.Task.ID, &current.Task.Key, &current.Task.Queue, &current.Task.Position,
		&current.Task.Priority, &current.Task.Title, &current.Task.Description, &current.Task.Status,
		&current.Task.Author, &current.Task.Customer, &current.Task.Group, &current.Task.Assignee,
		&current.Task.ManualBlockReason, &current.Task.Blocked, &current.Task.WorkflowVersionID,
		&current.Task.WorkflowStatus, &current.Task.WorkflowRevision, &current.Task.Revision,
		&current.Task.CreatedAt, &current.Task.UpdatedAt, &current.Task.CompletedAt,
		&current.Workflow.ID, &current.Workflow.Name, &current.Workflow.Version,
		&current.Workflow.State, &rawDefinition, &current.Workflow.CreatedAt,
		&current.Workflow.UpdatedAt, &current.Workflow.PublishedAt,
	)
	if err == sql.ErrNoRows {
		return assignmentContext{}, domainError(http.StatusNotFound, "assignment_not_found", "assignment not found")
	}
	if err != nil {
		return assignmentContext{}, err
	}
	current.Agent = assignmentAgent.String
	if err := json.Unmarshal([]byte(rawSnapshot), &current.PoolSnapshot); err != nil {
		return assignmentContext{}, err
	}
	if err := json.Unmarshal([]byte(rawOutcomes), &current.AllowedOutcomes); err != nil {
		return assignmentContext{}, err
	}
	if err := json.Unmarshal([]byte(rawDefinition), &current.Workflow.Definition); err != nil {
		return assignmentContext{}, err
	}
	current.Workflow.Definition = normalizeWorkflowDefinition(current.Workflow.Definition)
	current.Task.WorkflowVersion = current.Workflow.Name + "@" + workflowVersionString(current.Workflow.Version)
	return current, nil
}

func assignmentByID(ctx context.Context, q queryer, id int64) (Assignment, error) {
	var assignment Assignment
	var assignmentAgent sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT id, requirement_execution_id, agent, attempt, state, lease_owner,
		       lease_iteration, lease_expires_at, revision, outcome, created_at, updated_at, completed_at
		FROM task_assignments WHERE id = ?`, id).Scan(
		&assignment.ID, &assignment.RequirementExecutionID, &assignmentAgent,
		&assignment.Attempt, &assignment.State, &assignment.LeaseOwner, &assignment.LeaseIteration,
		&assignment.LeaseExpiresAt, &assignment.Revision, &assignment.Outcome,
		&assignment.CreatedAt, &assignment.UpdatedAt, &assignment.CompletedAt,
	)
	if err == sql.ErrNoRows {
		return Assignment{}, domainError(http.StatusNotFound, "assignment_not_found", "assignment not found")
	}
	assignment.Agent = assignmentAgent.String
	return assignment, err
}

func authorizeClaim(actor Actor, current assignmentContext) error {
	agent := strings.TrimPrefix(actor.Principal, "agent:")
	if current.Dispatch == DispatchRequireAll {
		if current.Agent != agent {
			return domainError(http.StatusForbidden, "assignment_not_eligible", "assignment belongs to another pool member")
		}
		return nil
	}
	if !containsString(current.PoolSnapshot, agent) {
		return domainError(http.StatusForbidden, "assignment_not_eligible", "agent is not in the frozen assignment pool")
	}
	return nil
}

func requireOwnedActiveLease(actor Actor, current assignmentContext, now time.Time) error {
	if current.State == AssignmentCompleted {
		return domainError(http.StatusConflict, "assignment_already_completed", "assignment is already completed")
	}
	if current.State != AssignmentLeased {
		return domainError(http.StatusConflict, "assignment_not_leased", "assignment is not leased")
	}
	if current.LeaseOwner != actor.Principal {
		return domainError(http.StatusForbidden, "assignment_not_owned", "assignment lease belongs to another actor")
	}
	deadline, err := time.Parse(time.RFC3339Nano, current.LeaseExpiresAt)
	if err != nil || !deadline.After(now) {
		return domainError(http.StatusConflict, "assignment_lease_expired", "assignment lease has expired")
	}
	return nil
}

func requireRuntimeRevisions(current assignmentContext, taskRevision, assignmentRevision int64) error {
	if taskRevision != current.Task.WorkflowRevision {
		return &Error{Status: http.StatusConflict, Code: "revision_conflict",
			Msg: "workflow task revision is stale", Data: map[string]any{"current_revision": current.Task.WorkflowRevision}}
	}
	if assignmentRevision != current.Revision {
		return &Error{Status: http.StatusConflict, Code: "revision_conflict",
			Msg: "assignment revision is stale", Data: map[string]any{"current_revision": current.Revision, "current": current.Assignment}}
	}
	return nil
}

func requireIdempotencyKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return domainError(http.StatusBadRequest, "missing_idempotency_key", "idempotency key is required")
	}
	return nil
}

func requireAssignmentMutationRevisions(taskRevision, assignmentRevision int64) error {
	if taskRevision <= 0 || assignmentRevision <= 0 {
		return domainError(http.StatusConflict, "revision_conflict", "task and assignment revisions are required")
	}
	return nil
}

func requireWorkflowAgent(actor Actor) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if actor.IsCustomer || !strings.HasPrefix(actor.Principal, "agent:") || strings.TrimPrefix(actor.Principal, "agent:") == "" {
		return domainError(http.StatusForbidden, "forbidden", "only an agent can mutate workflow assignments")
	}
	return nil
}

func parseAssignmentID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, domainError(http.StatusBadRequest, "invalid_assignment_id", "assignment id must be a positive integer")
	}
	return id, nil
}

func workflowLeaseDuration(definition WorkflowDefinition) (time.Duration, error) {
	raw := strings.TrimSpace(definition.Timeouts.Assignment)
	if raw == "" {
		return defaultAssignmentLease, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, domainError(http.StatusConflict, "workflow_invalid", "workflow assignment timeout is invalid")
	}
	return duration, nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func enqueueWorkflowRuntimeNotificationTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	assignmentID any,
	requirement string,
	pool string,
	agent string,
	kind string,
	errorCode string,
	now string,
) error {
	payload := map[string]any{
		"task_key": task.Key, "queue": task.Queue, "requirement": requirement,
		"pool": pool, "agent": agent,
	}
	if errorCode != "" {
		payload["error_code"] = errorCode
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO task_workflow_outbox(
			wake_id, task_id, assignment_id, kind, payload, next_attempt_at
		) VALUES (?, ?, ?, ?, ?, ?)`, newID("tw"), task.ID, assignmentID, kind, string(raw), now)
	return err
}

func readAssignmentReplay(
	ctx context.Context,
	tx *sql.Tx,
	actor Actor,
	action string,
	key string,
) (assignmentMutationReplay, bool, error) {
	return readTaskIdempotency[assignmentMutationReplay](ctx, tx, actor.Principal, action, key)
}

func writeAssignmentReplay(
	ctx context.Context,
	tx *sql.Tx,
	actor Actor,
	action string,
	key string,
	replay assignmentMutationReplay,
	now string,
) error {
	return writeTaskIdempotency(ctx, tx, actor.Principal, action, key, replay, now)
}

func (r assignmentMutationReplay) err() error {
	if r.ErrorCode == "" {
		return nil
	}
	return domainError(http.StatusConflict, r.ErrorCode, r.ErrorMsg)
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
