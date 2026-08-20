package tasks

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ListWorkflowAssignments is the operator projection of the immutable
// assignment history for one managed task. Agent callers intentionally use
// NextWork/GetWorkPacket instead and cannot enumerate sibling assignments.
func (s *Service) ListWorkflowAssignments(ctx context.Context, actor Actor, taskKey string) ([]Assignment, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return nil, err
	}
	task, err := taskByKey(s.db, strings.TrimSpace(taskKey))
	if err != nil {
		return nil, err
	}
	if task.WorkflowVersionID == 0 {
		return nil, domainError(http.StatusConflict, "workflow_not_managed", "task is not managed by a workflow")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id
		FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		WHERE se.task_id = ? ORDER BY se.sequence, re.id, a.attempt, a.id`, task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Assignment{}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		item, err := assignmentByID(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ListWorkPackets returns the same assignment-scoped projections agents see,
// but only to the daemon customer for operator inspection.
func (s *Service) ListWorkPackets(ctx context.Context, actor Actor, taskKey string) ([]WorkPacket, error) {
	assignments, err := s.ListWorkflowAssignments(ctx, actor, taskKey)
	if err != nil {
		return nil, err
	}
	items := make([]WorkPacket, 0, len(assignments))
	for _, assignment := range assignments {
		packet, err := s.GetWorkPacket(ctx, actor, strconv.FormatInt(assignment.ID, 10))
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}
		items = append(items, packet)
	}
	return items, nil
}

func (s *Service) GetWorkflowExecution(ctx context.Context, actor Actor, taskKey string) (WorkflowExecutionView, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return WorkflowExecutionView{}, err
	}
	detail, err := s.GetTask(ctx, actor, strings.TrimSpace(taskKey))
	if err != nil {
		return WorkflowExecutionView{}, err
	}
	if detail.Task.WorkflowVersionID == 0 {
		return WorkflowExecutionView{}, domainError(http.StatusConflict, "workflow_not_managed", "task is not managed by a workflow")
	}
	workflow, err := workflowVersionByID(ctx, s.db, detail.Task.WorkflowVersionID)
	if err != nil {
		return WorkflowExecutionView{}, err
	}
	view := WorkflowExecutionView{Task: detail.Task, Workflow: workflow,
		Statuses: []StatusExecution{}, Requirements: []RequirementExecution{},
		Assignments: []Assignment{}, Holds: []WorkflowHold{}, Observations: []WorkflowObservation{}}
	rows, err := s.db.QueryContext(ctx, `SELECT id,status_id,sequence,state,transition_to,task_revision,created_at,completed_at FROM task_status_executions WHERE task_id=? ORDER BY sequence,id`, detail.Task.ID)
	if err != nil {
		return WorkflowExecutionView{}, err
	}
	statusIDs := []int64{}
	for rows.Next() {
		var x StatusExecution
		x.TaskKey = detail.Task.Key
		x.WorkflowVersionID = workflow.ID
		if err := rows.Scan(&x.ID, &x.Status, &x.Sequence, &x.State, &x.TransitionTo, &x.TaskRevision, &x.CreatedAt, &x.CompletedAt); err != nil {
			rows.Close()
			return WorkflowExecutionView{}, err
		}
		view.Statuses = append(view.Statuses, x)
		statusIDs = append(statusIDs, x.ID)
	}
	if err := rows.Close(); err != nil {
		return WorkflowExecutionView{}, err
	}
	for _, sid := range statusIDs {
		rr, err := s.db.QueryContext(ctx, `SELECT id,requirement_id,dispatch,optional,pool_snapshot,inputs,produces,outcomes,state,created_at,completed_at,COALESCE((SELECT name FROM task_agent_pools WHERE id=pool_id),'') FROM task_requirement_executions WHERE status_execution_id=? ORDER BY id`, sid)
		if err != nil {
			return WorkflowExecutionView{}, err
		}
		for rr.Next() {
			var x RequirementExecution
			var snapshot, inputs, produces, outcomes string
			x.StatusExecutionID = sid
			if err := rr.Scan(&x.ID, &x.RequirementID, &x.Dispatch, &x.Optional, &snapshot, &inputs, &produces, &outcomes, &x.State, &x.CreatedAt, &x.CompletedAt, &x.Pool); err != nil {
				rr.Close()
				return WorkflowExecutionView{}, err
			}
			for _, pair := range []struct {
				raw string
				dst *[]string
			}{{snapshot, &x.PoolSnapshot}, {inputs, &x.Inputs}, {produces, &x.Produces}, {outcomes, &x.Outcomes}} {
				if err := json.Unmarshal([]byte(pair.raw), pair.dst); err != nil {
					rr.Close()
					return WorkflowExecutionView{}, err
				}
			}
			view.Requirements = append(view.Requirements, x)
		}
		if err := rr.Close(); err != nil {
			return WorkflowExecutionView{}, err
		}
	}
	view.Assignments, err = s.ListWorkflowAssignments(ctx, actor, detail.Task.Key)
	if err != nil {
		return WorkflowExecutionView{}, err
	}
	holds, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(assignment_id,0),COALESCE(requirement_execution_id,0),COALESCE(question_id,0),scope,reason,created_at,released_at FROM task_workflow_holds WHERE task_id=? ORDER BY id`, detail.Task.ID)
	if err != nil {
		return WorkflowExecutionView{}, err
	}
	for holds.Next() {
		var x WorkflowHold
		x.TaskKey = detail.Task.Key
		if err := holds.Scan(&x.ID, &x.AssignmentID, &x.RequirementExecutionID, &x.QuestionID, &x.Scope, &x.Reason, &x.CreatedAt, &x.ReleasedAt); err != nil {
			holds.Close()
			return WorkflowExecutionView{}, err
		}
		view.Holds = append(view.Holds, x)
	}
	if err := holds.Close(); err != nil {
		return WorkflowExecutionView{}, err
	}
	obs, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(subscription_id,0),COALESCE(assignment_id,0),kind,payload,observed_at FROM task_observations WHERE task_id=? ORDER BY id`, detail.Task.ID)
	if err != nil {
		return WorkflowExecutionView{}, err
	}
	for obs.Next() {
		var x WorkflowObservation
		var raw string
		x.TaskKey = detail.Task.Key
		if err := obs.Scan(&x.ID, &x.SubscriptionID, &x.AssignmentID, &x.Kind, &raw, &x.ObservedAt); err != nil {
			obs.Close()
			return WorkflowExecutionView{}, err
		}
		if err := json.Unmarshal([]byte(raw), &x.Payload); err != nil {
			obs.Close()
			return WorkflowExecutionView{}, err
		}
		view.Observations = append(view.Observations, x)
	}
	if err := obs.Close(); err != nil {
		return WorkflowExecutionView{}, err
	}
	return view, nil
}

// GetWorkPacket returns the least-context projection for one assignment.
func (s *Service) GetWorkPacket(ctx context.Context, actor Actor, assignmentID string) (WorkPacket, error) {
	if err := validateActor(actor); err != nil {
		return WorkPacket{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return WorkPacket{}, err
	}
	current, err := assignmentContextByID(ctx, s.db, id)
	if err != nil {
		return WorkPacket{}, err
	}
	if strings.HasPrefix(current.RequirementID, questionRequirementPrefix) && current.State != AssignmentClaimable && current.State != AssignmentLeased {
		return WorkPacket{}, domainError(http.StatusConflict, "assignment_not_active", "assignment is not active")
	}
	if actor.IsCustomer {
		access, err := taskAccess(ctx, s.db, actor, current.Task.ID)
		if err != nil {
			return WorkPacket{}, err
		}
		if access == "" {
			return WorkPacket{}, notFound(current.Task.Key)
		}
	} else {
		switch current.State {
		case AssignmentClaimable:
			if err := authorizeClaim(actor, current); err != nil {
				return WorkPacket{}, err
			}
		case AssignmentLeased:
			if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
				return WorkPacket{}, err
			}
		case AssignmentCompleted:
			if current.LeaseOwner != actor.Principal && current.Agent != strings.TrimPrefix(actor.Principal, "agent:") {
				return WorkPacket{}, domainError(http.StatusForbidden, "assignment_not_owned", "assignment belongs to another actor")
			}
		default:
			return WorkPacket{}, domainError(http.StatusConflict, "assignment_not_active", "assignment is not active")
		}
	}
	return buildWorkPacket(ctx, s.db, actor, current)
}

func buildWorkPacket(ctx context.Context, q queryer, _ Actor, current assignmentContext) (WorkPacket, error) {
	if strings.HasPrefix(current.RequirementID, questionRequirementPrefix) {
		questionID, err := strconv.ParseInt(strings.TrimPrefix(current.RequirementID, questionRequirementPrefix), 10, 64)
		if err != nil {
			return WorkPacket{}, domainError(http.StatusConflict, "workflow_invalid", "question assignment has an invalid requirement id")
		}
		question, err := workflowQuestionByID(ctx, q, questionID)
		if err != nil {
			return WorkPacket{}, err
		}
		question.TaskKey = current.Task.Key
		inputs, err := questionAttachmentArtifacts(ctx, q, current.Task.Key, question.ArtifactAttachments)
		if err != nil {
			return WorkPacket{}, err
		}
		actions := []string{}
		if current.State == AssignmentClaimable {
			actions = []string{"claim"}
		} else if current.State == AssignmentLeased {
			actions = []string{"answer", "release"}
		}
		return WorkPacket{
			TaskKey: current.Task.Key, TaskRevision: current.Task.WorkflowRevision,
			Goal: "Answer a workflow question", Status: current.Task.WorkflowStatus,
			StatusInstructions: "Answer the bounded question using its context and attachments.",
			Assignment:         current.Assignment,
			Requirement:        WorkflowRequirement{ID: current.RequirementID, Pool: current.Pool, Dispatch: DispatchClaimOne},
			Inputs:             inputs, AllowedActions: actions, Questions: []WorkflowQuestion{question},
		}, nil
	}
	if strings.HasPrefix(current.RequirementID, observationRequirementPrefix) {
		parts := strings.Split(strings.TrimPrefix(current.RequirementID, observationRequirementPrefix), ":")
		if len(parts) != 2 {
			return WorkPacket{}, domainError(http.StatusConflict, "workflow_invalid", "observation assignment has an invalid requirement id")
		}
		subscriptionID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return WorkPacket{}, domainError(http.StatusConflict, "workflow_invalid", "observation assignment has an invalid requirement id")
		}
		observations, err := observationsForSubscription(ctx, q, current.Task.Key, current.Task.ID, subscriptionID, parts[1])
		if err != nil {
			return WorkPacket{}, err
		}
		actions := []string{}
		if current.State == AssignmentClaimable {
			actions = []string{"claim"}
		} else if current.State == AssignmentLeased {
			actions = []string{"complete", "release"}
		}
		return WorkPacket{
			TaskKey: current.Task.Key, TaskRevision: current.Task.WorkflowRevision,
			Goal: "Handle a correlated workflow observation", Status: current.Task.WorkflowStatus,
			StatusInstructions: "Inspect the scoped observation and acknowledge the declared follow-up requirement.",
			Assignment:         current.Assignment,
			Requirement: WorkflowRequirement{ID: current.RequirementID, Pool: current.Pool,
				Dispatch: DispatchClaimOne, Outcomes: []string{"acknowledged"}, Optional: true},
			AllowedOutcomes: []string{"acknowledged"}, AllowedActions: actions, Observations: observations,
		}, nil
	}
	status, ok := workflowStatusByID(current.Workflow.Definition, current.Task.WorkflowStatus)
	if !ok {
		return WorkPacket{}, domainError(http.StatusConflict, "workflow_invalid", "task status is missing from the pinned workflow")
	}
	requirement, ok := workflowRequirementByID(current.Workflow.Definition, current.Task.WorkflowStatus, current.RequirementID)
	if !ok {
		return WorkPacket{}, domainError(http.StatusConflict, "workflow_invalid", "assignment requirement is missing from the pinned workflow")
	}
	inputs, err := visiblePacketArtifacts(ctx, q, current.Task.ID, requirement.Inputs)
	if err != nil {
		return WorkPacket{}, err
	}
	questions, err := packetQuestions(ctx, q, current.Task.Key, current.Task.ID, current.ID, current.RequirementExecutionID)
	if err != nil {
		return WorkPacket{}, err
	}
	holds, err := packetHolds(ctx, q, current.Task.Key, current.Task.ID, current.ID, current.RequirementExecutionID)
	if err != nil {
		return WorkPacket{}, err
	}
	observations, err := packetObservations(ctx, q, current.Task.Key, current.Task.ID, current.ID)
	if err != nil {
		return WorkPacket{}, err
	}
	subscriptions, err := packetSubscriptions(ctx, q, current.Task.Key, current.Task.ID, current.ID)
	if err != nil {
		return WorkPacket{}, err
	}
	goal := strings.TrimSpace(current.Task.Description)
	if goal == "" {
		goal = current.Task.Title
	}
	actions := []string{}
	switch current.State {
	case AssignmentClaimable:
		actions = []string{"claim"}
	case AssignmentLeased:
		actions = []string{"artifacts.add", "complete", "release", "ask", "channels.subscribe", "channels.unsubscribe"}
	}
	return WorkPacket{
		TaskKey: current.Task.Key, TaskRevision: current.Task.WorkflowRevision,
		Goal: goal, Status: current.Task.WorkflowStatus, StatusInstructions: status.Instructions,
		Assignment: current.Assignment, Requirement: requirement, Inputs: inputs,
		AllowedOutcomes: append([]string(nil), requirement.Outcomes...), AllowedActions: actions,
		AllowedTools:                append([]string(nil), current.Workflow.Definition.Permissions.Tools...),
		AllowedChannelSubscriptions: append([]string(nil), current.Workflow.Definition.Permissions.Channels.Subscribe...),
		Questions:                   questions, Holds: holds, Observations: observations, Subscriptions: subscriptions,
	}, nil
}

func observationsForSubscription(ctx context.Context, q queryer, taskKey string, taskID, subscriptionID int64, eventHash string) ([]WorkflowObservation, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, assignment_id, kind, payload, observed_at FROM task_observations WHERE task_id=? AND subscription_id=? ORDER BY id`, taskID, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowObservation{}
	for rows.Next() {
		var observation WorkflowObservation
		var assignmentID sql.NullInt64
		var payload string
		if err := rows.Scan(&observation.ID, &assignmentID, &observation.Kind, &payload, &observation.ObservedAt); err != nil {
			return nil, err
		}
		observation.TaskKey, observation.SubscriptionID, observation.AssignmentID = taskKey, subscriptionID, assignmentID.Int64
		if err := json.Unmarshal([]byte(payload), &observation.Payload); err != nil {
			return nil, err
		}
		eventID, _ := observation.Payload["event_id"].(string)
		digest := sha256.Sum256([]byte(eventID))
		if hex.EncodeToString(digest[:16]) != eventHash {
			continue
		}
		out = append(out, observation)
	}
	return out, rows.Err()
}

func questionAttachmentArtifacts(ctx context.Context, q queryer, taskKey string, ids []int64) ([]Artifact, error) {
	artifacts := make([]Artifact, 0, len(ids))
	for _, id := range ids {
		artifact, err := artifactByID(ctx, q, id)
		if err != nil || artifact.TaskKey != taskKey {
			return nil, domainError(http.StatusConflict, "workflow_question_attachment_missing", "question attachment is no longer available")
		}
		artifact.Metadata = nil
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func packetHolds(ctx context.Context, q queryer, taskKey string, taskID, assignmentID, requirementID int64) ([]WorkflowHold, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, assignment_id, requirement_execution_id, question_id, scope, reason, created_at, released_at
		FROM task_workflow_holds
		WHERE task_id = ? AND released_at = ''
		  AND (assignment_id = ? OR requirement_execution_id = ?)
		ORDER BY id`, taskID, assignmentID, requirementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	holds := []WorkflowHold{}
	for rows.Next() {
		var hold WorkflowHold
		var aid, rid, qid sql.NullInt64
		if err := rows.Scan(&hold.ID, &aid, &rid, &qid, &hold.Scope, &hold.Reason, &hold.CreatedAt, &hold.ReleasedAt); err != nil {
			return nil, err
		}
		hold.TaskKey, hold.AssignmentID, hold.RequirementExecutionID, hold.QuestionID = taskKey, aid.Int64, rid.Int64, qid.Int64
		holds = append(holds, hold)
	}
	return holds, rows.Err()
}

func visiblePacketArtifacts(ctx context.Context, q queryer, taskID int64, names []string) ([]Artifact, error) {
	wanted := append([]string(nil), names...)
	sort.Strings(wanted)
	artifacts := make([]Artifact, 0, len(wanted))
	seen := map[string]bool{}
	for _, name := range wanted {
		if seen[name] {
			continue
		}
		seen[name] = true
		var id int64
		err := q.QueryRowContext(ctx, `
			SELECT id FROM task_artifacts
			WHERE task_id = ? AND name = ? ORDER BY revision DESC, id DESC LIMIT 1`, taskID, name).Scan(&id)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		artifact, err := artifactByID(ctx, q, id)
		if err != nil {
			return nil, err
		}
		// Metadata is deliberately excluded: it may contain delivery tokens,
		// credentials, or transport-specific correlation state.
		artifact.Metadata = nil
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func packetQuestions(ctx context.Context, q queryer, taskKey string, taskID, assignmentID, requirementID int64) ([]WorkflowQuestion, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, assignment_id, requirement_execution_id, question, context,
		       blocking_scope, anchor, options, suggested_answer, artifact_attachments,
		       state, deadline_at, answer,
		       answered_by, created_at, answered_at
		FROM task_workflow_questions
		WHERE task_id = ? AND (? = 1
		  OR (blocking_scope = 'requirement' AND requirement_execution_id = ?)
		  OR (blocking_scope IN ('none', 'assignment') AND assignment_id = ?))
		ORDER BY id`, taskID, assignmentID == 0 && requirementID == 0, requirementID, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	questions := []WorkflowQuestion{}
	for rows.Next() {
		var question WorkflowQuestion
		var scopedAssignment, scopedRequirement sql.NullInt64
		var rawOptions, rawAttachments string
		if err := rows.Scan(&question.ID, &scopedAssignment, &scopedRequirement,
			&question.Question, &question.Context, &question.BlockingScope, &question.Anchor,
			&rawOptions, &question.SuggestedAnswer, &rawAttachments, &question.State, &question.DeadlineAt,
			&question.Answer, &question.AnsweredBy,
			&question.CreatedAt, &question.AnsweredAt); err != nil {
			return nil, err
		}
		question.TaskKey = taskKey
		question.AssignmentID = scopedAssignment.Int64
		question.RequirementExecutionID = scopedRequirement.Int64
		if err := json.Unmarshal([]byte(rawOptions), &question.Options); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(rawAttachments), &question.ArtifactAttachments); err != nil {
			return nil, err
		}
		questions = append(questions, question)
	}
	return questions, rows.Err()
}

func packetObservations(ctx context.Context, q queryer, taskKey string, taskID, assignmentID int64) ([]WorkflowObservation, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, subscription_id, assignment_id, kind, observed_at
		FROM task_observations
		WHERE task_id = ? AND (assignment_id IS NULL OR assignment_id = ?)
		ORDER BY id`, taskID, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	observations := []WorkflowObservation{}
	for rows.Next() {
		var observation WorkflowObservation
		var subscriptionID, observedAssignment sql.NullInt64
		if err := rows.Scan(&observation.ID, &subscriptionID, &observedAssignment,
			&observation.Kind, &observation.ObservedAt); err != nil {
			return nil, err
		}
		observation.SubscriptionID = subscriptionID.Int64
		observation.AssignmentID = observedAssignment.Int64
		observation.TaskKey = taskKey
		// Raw channel payload is intentionally not part of the least-context packet.
		observation.Payload = nil
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

func packetSubscriptions(ctx context.Context, q queryer, taskKey string, taskID, assignmentID int64) ([]WorkflowSubscription, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, assignment_id, pattern, reaction, state, created_by, created_at, cancelled_at, created_after_sequence, activation_sequence_set
		FROM task_workflow_subscriptions
		WHERE task_id = ? AND (assignment_id IS NULL OR assignment_id = ?)
		ORDER BY id`, taskID, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subscriptions := []WorkflowSubscription{}
	for rows.Next() {
		var subscription WorkflowSubscription
		var scopedAssignment sql.NullInt64
		if err := rows.Scan(&subscription.ID, &scopedAssignment, &subscription.Pattern,
			&subscription.Reaction, &subscription.State, &subscription.CreatedBy,
			&subscription.CreatedAt, &subscription.CancelledAt, &subscription.CreatedAfterSequence, &subscription.ActivationSequenceSet); err != nil {
			return nil, err
		}
		subscription.AssignmentID = scopedAssignment.Int64
		subscription.TaskKey = taskKey
		// Correlation keys are delivery metadata and must not be exposed.
		subscription.CorrelationKey = ""
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, rows.Err()
}
