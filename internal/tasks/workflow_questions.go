package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const questionRequirementPrefix = "__question:"

type AskWorkflowQuestionInput struct {
	TaskRevision        int64    `json:"task_revision"`
	AssignmentRevision  int64    `json:"assignment_revision"`
	Question            string   `json:"question"`
	Context             string   `json:"context"`
	BlockingScope       string   `json:"blocking_scope"`
	Anchor              string   `json:"anchor,omitempty"`
	Options             []string `json:"options,omitempty"`
	SuggestedAnswer     string   `json:"suggested_answer,omitempty"`
	ArtifactAttachments []int64  `json:"artifact_attachments,omitempty"`
	IdempotencyKey      string   `json:"idempotency_key"`
}

type AnswerWorkflowQuestionInput struct {
	TaskRevision       int64  `json:"task_revision"`
	AssignmentRevision int64  `json:"assignment_revision"`
	Answer             string `json:"answer"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type questionMutationReplay struct {
	Question  WorkflowQuestion `json:"question"`
	ErrorCode string           `json:"error_code,omitempty"`
	ErrorMsg  string           `json:"error_message,omitempty"`
}

func (r questionMutationReplay) err() error {
	if r.ErrorCode == "" {
		return nil
	}
	return domainError(http.StatusConflict, r.ErrorCode, r.ErrorMsg)
}

func (s *Service) AskWorkflowQuestion(ctx context.Context, actor Actor, assignmentID string, in AskWorkflowQuestionInput) (WorkflowQuestion, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return WorkflowQuestion{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	in.Question, in.Context, in.Anchor = strings.TrimSpace(in.Question), strings.TrimSpace(in.Context), strings.TrimSpace(in.Anchor)
	in.SuggestedAnswer = strings.TrimSpace(in.SuggestedAnswer)
	if in.Question == "" {
		return WorkflowQuestion{}, domainError(http.StatusBadRequest, "missing_question", "question is required")
	}
	if in.Context == "" {
		return WorkflowQuestion{}, domainError(http.StatusBadRequest, "missing_question_context", "question context is required")
	}
	if in.BlockingScope != HoldNone && in.BlockingScope != HoldAssignment && in.BlockingScope != HoldRequirement {
		return WorkflowQuestion{}, domainError(http.StatusBadRequest, "invalid_blocking_scope", "blocking scope must be none, assignment, or requirement")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	defer tx.Rollback()
	if replay, ok, err := readTaskIdempotency[WorkflowQuestion](ctx, tx, actor.Principal, "ask_workflow_question", in.IdempotencyKey); err != nil {
		return WorkflowQuestion{}, err
	} else if ok {
		return replay, nil
	}
	if err := requireAssignmentMutationRevisions(in.TaskRevision, in.AssignmentRevision); err != nil {
		return WorkflowQuestion{}, err
	}
	current, err := assignmentContextByID(ctx, tx, id)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := requireRuntimeRevisions(current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := validateQuestionAttachments(ctx, tx, current, in.ArtifactAttachments); err != nil {
		return WorkflowQuestion{}, err
	}
	policy := current.Workflow.Definition.Questions
	if strings.TrimSpace(policy.RouteTo) == "" {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "workflow_questions_disabled", "workflow does not route questions")
	}
	if in.BlockingScope != HoldNone && !containsString(policy.AllowedHolds, in.BlockingScope) {
		return WorkflowQuestion{}, domainError(http.StatusForbidden, "workflow_hold_not_allowed", "blocking scope is not allowed by the workflow")
	}
	if policy.MaxOpenPerAssignment > 0 {
		var open int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_workflow_questions WHERE assignment_id = ? AND state = 'open'`, id).Scan(&open); err != nil {
			return WorkflowQuestion{}, err
		}
		if open >= policy.MaxOpenPerAssignment {
			return WorkflowQuestion{}, domainError(http.StatusConflict, "workflow_question_budget_exhausted", "maximum open questions for assignment reached")
		}
	}
	pool, found, err := agentPoolByName(ctx, tx, current.Task.Queue, policy.RouteTo)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if !found || len(pool.Agents) == 0 {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "workflow_question_pool_empty", "question manager pool has no agents")
	}
	options, _ := json.Marshal(trimStrings(in.Options))
	attachments, _ := json.Marshal(in.ArtifactAttachments)
	now := s.now()
	deadline, err := questionDeadline(current.Workflow.Definition.Questions, s.clock().UTC())
	if err != nil {
		return WorkflowQuestion{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO task_workflow_questions(task_id, assignment_id, requirement_execution_id, question, context, blocking_scope, anchor, options, suggested_answer, artifact_attachments, state, deadline_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)`, current.Task.ID, id, current.RequirementExecutionID, in.Question, in.Context, in.BlockingScope, in.Anchor, string(options), in.SuggestedAnswer, string(attachments), deadline, now)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	qid, err := result.LastInsertId()
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if in.BlockingScope != HoldNone {
		var heldAssignment, heldRequirement any
		if in.BlockingScope == HoldAssignment {
			heldAssignment = id
		} else {
			heldRequirement = current.RequirementExecutionID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_workflow_holds(task_id, assignment_id, requirement_execution_id, question_id, scope, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, current.Task.ID, heldAssignment, heldRequirement, qid, in.BlockingScope, in.Question, now); err != nil {
			return WorkflowQuestion{}, err
		}
	}
	snapshot, _ := json.Marshal(sortedUniqueStrings(pool.Agents))
	rr, err := tx.ExecContext(ctx, `INSERT INTO task_requirement_executions(status_execution_id, requirement_id, pool_id, dispatch, optional, pool_snapshot, inputs, produces, outcomes, state, created_at) VALUES (?, ?, ?, 'claim_one', 1, ?, '[]', '[]', '[]', 'pending', ?)`, current.StatusExecutionID, questionRequirementPrefix+strconv.FormatInt(qid, 10), pool.ID, string(snapshot), now)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	rid, _ := rr.LastInsertId()
	ar, err := tx.ExecContext(ctx, `INSERT INTO task_assignments(requirement_execution_id, agent, attempt, state, revision, created_at, updated_at) VALUES (?, NULL, 1, 'claimable', 1, ?, ?)`, rid, now, now)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	managerAssignmentID, _ := ar.LastInsertId()
	for _, manager := range sortedUniqueStrings(pool.Agents) {
		if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, current.Task, managerAssignmentID, questionRequirementPrefix+strconv.FormatInt(qid, 10), pool.Name, manager, "workflow.question_ready", "", now); err != nil {
			return WorkflowQuestion{}, err
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE task_assignments SET revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`, now, id, in.AssignmentRevision)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "revision_conflict", "assignment changed concurrently")
	}
	res, err = tx.ExecContext(ctx, `UPDATE tasks SET workflow_revision = workflow_revision + 1, updated_at = ? WHERE id = ? AND workflow_revision = ?`, now, current.Task.ID, in.TaskRevision)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "revision_conflict", "task changed concurrently")
	}
	question := WorkflowQuestion{ID: qid, TaskKey: current.Task.Key, AssignmentID: id, RequirementExecutionID: current.RequirementExecutionID, Question: in.Question, Context: in.Context, BlockingScope: in.BlockingScope, Anchor: in.Anchor, Options: trimStrings(in.Options), SuggestedAnswer: in.SuggestedAnswer, ArtifactAttachments: append([]int64(nil), in.ArtifactAttachments...), State: "open", DeadlineAt: deadline, CreatedAt: now}
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.question_asked", actor, map[string]any{"question_id": qid, "assignment_id": id, "blocking_scope": in.BlockingScope}, now); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := writeTaskIdempotency(ctx, tx, actor.Principal, "ask_workflow_question", in.IdempotencyKey, question, now); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkflowQuestion{}, err
	}
	s.signal()
	return question, nil
}

func (s *Service) ListWorkflowQuestions(ctx context.Context, actor Actor, taskKey, assignmentID string) ([]WorkflowQuestion, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	task, err := taskByKey(s.db, strings.ToUpper(strings.TrimSpace(taskKey)))
	if err != nil {
		return nil, err
	}
	if actor.IsCustomer {
		access, err := taskAccess(ctx, s.db, actor, task.ID)
		if err != nil {
			return nil, err
		}
		if access == "" {
			return nil, notFound(task.Key)
		}
		return packetQuestions(ctx, s.db, task.Key, task.ID, 0, 0)
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return nil, err
	}
	current, err := assignmentContextByID(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if current.Task.ID != task.ID {
		return nil, notFound(task.Key)
	}
	switch current.State {
	case AssignmentLeased:
		if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
			return nil, err
		}
	case AssignmentClaimable:
		if err := authorizeClaim(actor, current); err != nil {
			return nil, err
		}
	default:
		return nil, domainError(http.StatusConflict, "assignment_not_active", "assignment is not active")
	}
	if strings.HasPrefix(current.RequirementID, questionRequirementPrefix) {
		qid, _ := strconv.ParseInt(strings.TrimPrefix(current.RequirementID, questionRequirementPrefix), 10, 64)
		q, err := workflowQuestionByID(ctx, s.db, qid)
		if err != nil {
			return nil, err
		}
		q.TaskKey = task.Key
		return []WorkflowQuestion{q}, nil
	}
	return packetQuestions(ctx, s.db, task.Key, task.ID, id, current.RequirementExecutionID)
}

func (s *Service) GetWorkflowQuestion(ctx context.Context, actor Actor, taskKey string, questionID int64) (WorkflowQuestion, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return WorkflowQuestion{}, err
	}
	task, err := taskByKey(s.db, strings.TrimSpace(taskKey))
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if access, err := taskAccess(ctx, s.db, actor, task.ID); err != nil {
		return WorkflowQuestion{}, err
	} else if access == "" {
		return WorkflowQuestion{}, notFound(task.Key)
	}
	q, err := workflowQuestionByID(ctx, s.db, questionID)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	var ownerID int64
	if err := s.db.QueryRowContext(ctx, `SELECT task_id FROM task_workflow_questions WHERE id=?`, questionID).Scan(&ownerID); err == sql.ErrNoRows || ownerID != task.ID {
		return WorkflowQuestion{}, domainError(http.StatusNotFound, "workflow_question_not_found", "workflow question not found")
	} else if err != nil {
		return WorkflowQuestion{}, err
	}
	q.TaskKey = task.Key
	return q, nil
}

func (s *Service) ClaimWorkflowQuestion(ctx context.Context, actor Actor, questionID string, in ClaimAssignmentInput) (Assignment, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return Assignment{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return Assignment{}, err
	}
	qid, err := parseQuestionID(questionID)
	if err != nil {
		return Assignment{}, err
	}
	assignment, workflow, err := questionAssignment(ctx, s.db, qid)
	if err != nil {
		return Assignment{}, err
	}
	question, err := workflowQuestionByID(ctx, s.db, qid)
	if err != nil {
		return Assignment{}, err
	}
	if replay, ok, err := s.readQuestionClaimReplay(ctx, actor, in.IdempotencyKey); err != nil {
		return Assignment{}, err
	} else if ok {
		return replay.Assignment, replay.err()
	}
	if question.State == "timed_out" {
		return Assignment{}, domainError(http.StatusConflict, "workflow_question_timeout", "question answer timeout elapsed")
	}
	if question.State == "exhausted" {
		return Assignment{}, domainError(http.StatusConflict, "workflow_question_exhausted", "question answer attempts are exhausted")
	}
	if question.State != "open" {
		return Assignment{}, domainError(http.StatusConflict, "workflow_question_answered", "question is already answered")
	}
	if timedOut(question, workflow.Definition.Questions, s.clock().UTC()) {
		changed, reconcileErr := s.reconcileWorkflowQuestionTimeout(ctx, qid, actor)
		if reconcileErr != nil {
			return Assignment{}, reconcileErr
		}
		if changed {
			s.signal()
		}
		timeoutErr := domainError(http.StatusConflict, "workflow_question_timeout", "question answer timeout elapsed")
		if err := s.writeQuestionClaimReplay(ctx, actor, in.IdempotencyKey, assignmentMutationReplay{Assignment: assignment, ErrorCode: ErrorCode(timeoutErr), ErrorMsg: timeoutErr.Error()}); err != nil {
			return Assignment{}, err
		}
		return Assignment{}, timeoutErr
	}
	return s.ClaimAssignment(ctx, actor, strconv.FormatInt(assignment.ID, 10), in)
}

func (s *Service) readQuestionClaimReplay(ctx context.Context, actor Actor, key string) (assignmentMutationReplay, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return assignmentMutationReplay{}, false, err
	}
	defer tx.Rollback()
	return readAssignmentReplay(ctx, tx, actor, "claim_assignment", key)
}

func (s *Service) writeQuestionClaimReplay(ctx context.Context, actor Actor, key string, replay assignmentMutationReplay) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, ok, err := readAssignmentReplay(ctx, tx, actor, "claim_assignment", key); err != nil {
		return err
	} else if ok {
		return nil
	}
	if err := writeAssignmentReplay(ctx, tx, actor, "claim_assignment", key, replay, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) AnswerWorkflowQuestion(ctx context.Context, actor Actor, questionID string, in AnswerWorkflowQuestionInput) (WorkflowQuestion, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return WorkflowQuestion{}, err
	}
	qid, err := parseQuestionID(questionID)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	defer tx.Rollback()
	if replay, ok, err := readTaskIdempotency[questionMutationReplay](ctx, tx, actor.Principal, "answer_workflow_question", in.IdempotencyKey); err != nil {
		return WorkflowQuestion{}, err
	} else if ok {
		return replay.Question, replay.err()
	}
	answer := strings.TrimSpace(in.Answer)
	if answer == "" {
		return WorkflowQuestion{}, domainError(http.StatusBadRequest, "missing_answer", "answer is required")
	}
	if err := requireAssignmentMutationRevisions(in.TaskRevision, in.AssignmentRevision); err != nil {
		return WorkflowQuestion{}, err
	}
	manager, _, err := questionAssignment(ctx, tx, qid)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	current, err := assignmentContextByID(ctx, tx, manager.ID)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := requireRuntimeRevisions(current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return WorkflowQuestion{}, err
	}
	question, err := workflowQuestionByID(ctx, tx, qid)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if question.State == "timed_out" {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "workflow_question_timeout", "question answer timeout elapsed")
	}
	if question.State == "exhausted" {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "workflow_question_exhausted", "question answer attempts are exhausted")
	}
	if question.State != "open" {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "workflow_question_answered", "question is already answered")
	}
	if timedOut(question, current.Workflow.Definition.Questions, s.clock().UTC()) {
		_ = tx.Rollback()
		changed, reconcileErr := s.reconcileWorkflowQuestionTimeout(ctx, qid, actor)
		if reconcileErr != nil {
			return WorkflowQuestion{}, reconcileErr
		}
		if changed {
			s.signal()
		}
		timeoutErr := domainError(http.StatusConflict, "workflow_question_timeout", "question answer timeout elapsed")
		if err := s.writeQuestionAnswerReplay(ctx, actor, in.IdempotencyKey, questionMutationReplay{Question: question, ErrorCode: ErrorCode(timeoutErr), ErrorMsg: timeoutErr.Error()}); err != nil {
			return WorkflowQuestion{}, err
		}
		return WorkflowQuestion{}, timeoutErr
	}
	now := s.now()
	res, err := tx.ExecContext(ctx, `UPDATE task_workflow_questions SET state = 'answered', answer = ?, answered_by = ?, answered_at = ? WHERE id = ? AND state = 'open'`, answer, actor.Principal, now, qid)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "workflow_question_answered", "question was answered concurrently")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_workflow_holds SET released_at = ? WHERE question_id = ? AND released_at = ''`, now, qid); err != nil {
		return WorkflowQuestion{}, err
	}
	managerResult, err := tx.ExecContext(ctx, `UPDATE task_assignments SET state = 'completed', lease_iteration = '', outcome = 'answered', revision = revision + 1, updated_at = ?, completed_at = ? WHERE id = ? AND state = 'leased' AND lease_owner = ? AND revision = ?`, now, now, manager.ID, actor.Principal, in.AssignmentRevision)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if changed, _ := managerResult.RowsAffected(); changed != 1 {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "assignment_lease_lost", "question assignment changed concurrently")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_requirement_executions SET state = 'completed', completed_at = ? WHERE id = ?`, now, manager.RequirementExecutionID); err != nil {
		return WorkflowQuestion{}, err
	}
	if question.BlockingScope != HoldNone {
		if err := resumeHeldAssignmentsTx(ctx, tx, current.Task, current.Workflow, question, now, s.clock().UTC()); err != nil {
			return WorkflowQuestion{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET workflow_revision = workflow_revision + 1, updated_at = ? WHERE id = ? AND workflow_revision = ?`, now, current.Task.ID, in.TaskRevision)
	if err != nil {
		return WorkflowQuestion{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return WorkflowQuestion{}, domainError(http.StatusConflict, "revision_conflict", "task changed concurrently")
	}
	question.State, question.Answer, question.AnsweredBy, question.AnsweredAt = "answered", answer, actor.Principal, now
	question.TaskKey = current.Task.Key
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.question_answered", actor, map[string]any{"question_id": qid, "assignment_id": question.AssignmentID}, now); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := writeTaskIdempotency(ctx, tx, actor.Principal, "answer_workflow_question", in.IdempotencyKey, questionMutationReplay{Question: question}, now); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkflowQuestion{}, err
	}
	s.signal()
	return question, nil
}

func (s *Service) writeQuestionAnswerReplay(ctx context.Context, actor Actor, key string, replay questionMutationReplay) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, ok, err := readTaskIdempotency[questionMutationReplay](ctx, tx, actor.Principal, "answer_workflow_question", key); err != nil {
		return err
	} else if ok {
		return nil
	}
	if err := writeTaskIdempotency(ctx, tx, actor.Principal, "answer_workflow_question", key, replay, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}

func parseQuestionID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, domainError(http.StatusBadRequest, "invalid_question_id", "question id must be a positive integer")
	}
	return id, nil
}

func workflowQuestionByID(ctx context.Context, q queryer, id int64) (WorkflowQuestion, error) {
	var out WorkflowQuestion
	var assignment, requirement sql.NullInt64
	var options, attachments string
	err := q.QueryRowContext(ctx, `SELECT assignment_id, requirement_execution_id, question, context, blocking_scope, anchor, options, suggested_answer, artifact_attachments, state, deadline_at, answer, answered_by, created_at, answered_at FROM task_workflow_questions WHERE id = ?`, id).Scan(&assignment, &requirement, &out.Question, &out.Context, &out.BlockingScope, &out.Anchor, &options, &out.SuggestedAnswer, &attachments, &out.State, &out.DeadlineAt, &out.Answer, &out.AnsweredBy, &out.CreatedAt, &out.AnsweredAt)
	if err == sql.ErrNoRows {
		return WorkflowQuestion{}, domainError(http.StatusNotFound, "workflow_question_not_found", "workflow question not found")
	}
	if err != nil {
		return WorkflowQuestion{}, err
	}
	out.ID, out.AssignmentID, out.RequirementExecutionID = id, assignment.Int64, requirement.Int64
	if err := json.Unmarshal([]byte(options), &out.Options); err != nil {
		return WorkflowQuestion{}, err
	}
	if err := json.Unmarshal([]byte(attachments), &out.ArtifactAttachments); err != nil {
		return WorkflowQuestion{}, err
	}
	return out, nil
}

func questionAssignment(ctx context.Context, q queryer, questionID int64) (Assignment, WorkflowVersion, error) {
	var id, workflowID int64
	err := q.QueryRowContext(ctx, `SELECT a.id, se.workflow_version_id FROM task_assignments a JOIN task_requirement_executions re ON re.id = a.requirement_execution_id JOIN task_status_executions se ON se.id = re.status_execution_id WHERE re.requirement_id = ? ORDER BY a.id DESC LIMIT 1`, questionRequirementPrefix+strconv.FormatInt(questionID, 10)).Scan(&id, &workflowID)
	if err == sql.ErrNoRows {
		return Assignment{}, WorkflowVersion{}, domainError(http.StatusNotFound, "workflow_question_not_found", "workflow question assignment not found")
	}
	if err != nil {
		return Assignment{}, WorkflowVersion{}, err
	}
	a, err := assignmentByID(ctx, q, id)
	if err != nil {
		return Assignment{}, WorkflowVersion{}, err
	}
	w, err := workflowVersionByID(ctx, q, workflowID)
	return a, w, err
}

func timedOut(question WorkflowQuestion, policy WorkflowQuestionPolicy, now time.Time) bool {
	if question.DeadlineAt != "" {
		deadline, err := time.Parse(time.RFC3339Nano, question.DeadlineAt)
		return err != nil || !deadline.After(now)
	}
	raw := strings.TrimSpace(policy.Timeout)
	if raw == "" {
		return false
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return true
	}
	created, err := time.Parse(time.RFC3339Nano, question.CreatedAt)
	return err != nil || !created.Add(duration).After(now)
}

func questionDeadline(policy WorkflowQuestionPolicy, now time.Time) (string, error) {
	raw := strings.TrimSpace(policy.Timeout)
	if raw == "" {
		return "", nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return "", domainError(http.StatusConflict, "workflow_invalid", "workflow question timeout is invalid")
	}
	return now.Add(duration).Format(time.RFC3339Nano), nil
}

func (s *Service) ReconcileWorkflowQuestions(ctx context.Context) (int, error) {
	now := s.now()
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM task_workflow_questions WHERE state = 'open' AND deadline_at <> '' AND deadline_at <= ? ORDER BY id`, now)
	if err != nil {
		return 0, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	actor := Actor{Principal: "system:workflow"}
	for _, id := range ids {
		changed, err := s.reconcileWorkflowQuestionTimeout(ctx, id, actor)
		if err != nil {
			return count, err
		}
		if changed {
			count++
		}
	}
	if count > 0 {
		s.signal()
	}
	return count, nil
}

func (s *Service) reconcileWorkflowQuestionTimeout(ctx context.Context, questionID int64, actor Actor) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	question, err := workflowQuestionByID(ctx, tx, questionID)
	if err != nil {
		return false, err
	}
	if question.State != "open" {
		return false, nil
	}
	manager, workflow, err := questionAssignment(ctx, tx, questionID)
	if err != nil {
		return false, err
	}
	current, err := assignmentContextByID(ctx, tx, manager.ID)
	if err != nil {
		return false, err
	}
	if !timedOut(question, workflow.Definition.Questions, s.clock().UTC()) {
		return false, nil
	}
	now := s.now()
	result, err := tx.ExecContext(ctx, `UPDATE task_workflow_questions SET state = 'timed_out' WHERE id = ? AND state = 'open'`, questionID)
	if err != nil {
		return false, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_workflow_holds SET released_at = ? WHERE question_id = ? AND released_at = ''`, now, questionID); err != nil {
		return false, err
	}
	if current.StatusExecutionState == "active" {
		freezeErr := freezeExecutionTx(ctx, tx, current.Task, current.Workflow, current.StatusExecutionID, "workflow_question_timeout", actor, now)
		if _, persisted := persistedWorkflowDomain(freezeErr); !persisted {
			return false, freezeErr
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE task_assignments SET state = 'failed', lease_owner = '', lease_iteration = '', lease_expires_at = '', revision = revision + 1, updated_at = ? WHERE id = ? AND state IN ('claimable', 'leased')`, now, manager.ID); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE task_requirement_executions SET state = 'frozen' WHERE id = ? AND state = 'pending'`, manager.RequirementExecutionID); err != nil {
			return false, err
		}
		if _, err := appendEventTx(ctx, tx, current.Task, "workflow.question_timed_out", actor, map[string]any{"question_id": questionID}, now); err != nil {
			return false, err
		}
		for _, recipient := range current.PoolSnapshot {
			if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, current.Task, nil, current.RequirementID, current.Pool, recipient, "workflow.escalated", "workflow_question_timeout", now); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func workflowAssignmentHeld(ctx context.Context, q queryer, assignmentID, requirementID int64) error {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_workflow_holds WHERE released_at = '' AND ((scope = 'assignment' AND assignment_id = ?) OR (scope = 'requirement' AND requirement_execution_id = ?))`, assignmentID, requirementID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return domainError(http.StatusConflict, "workflow_assignment_held", "assignment is blocked by an open workflow question")
	}
	return nil
}

func resumeHeldAssignmentsTx(ctx context.Context, tx *sql.Tx, task Task, workflow WorkflowVersion, question WorkflowQuestion, now string, nowTime time.Time) error {
	query := `SELECT a.id FROM task_assignments a WHERE a.state IN ('claimable', 'leased') AND a.id = ? AND NOT EXISTS (
		SELECT 1 FROM task_workflow_holds h WHERE h.released_at = '' AND ((h.scope = 'assignment' AND h.assignment_id = a.id) OR (h.scope = 'requirement' AND h.requirement_execution_id = a.requirement_execution_id))
	) ORDER BY a.id`
	arg := question.AssignmentID
	if question.BlockingScope == HoldRequirement {
		query = `SELECT a.id FROM task_assignments a WHERE a.state IN ('claimable', 'leased') AND a.requirement_execution_id = ? AND NOT EXISTS (
			SELECT 1 FROM task_workflow_holds h WHERE h.released_at = '' AND ((h.scope = 'assignment' AND h.assignment_id = a.id) OR (h.scope = 'requirement' AND h.requirement_execution_id = a.requirement_execution_id))
		) ORDER BY a.id`
		arg = question.RequirementExecutionID
	}
	rows, err := tx.QueryContext(ctx, query, arg)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	leaseDuration, err := workflowLeaseDuration(workflow.Definition)
	if err != nil {
		return err
	}
	deadline := nowTime.Add(leaseDuration).Format(time.RFC3339Nano)
	for _, id := range ids {
		current, err := assignmentContextByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.State == AssignmentLeased {
			if _, err := tx.ExecContext(ctx, `UPDATE task_assignments SET lease_expires_at = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND state = 'leased'`, deadline, now, id); err != nil {
				return err
			}
		}
		recipients := current.PoolSnapshot
		if current.State == AssignmentLeased {
			recipients = []string{strings.TrimPrefix(current.LeaseOwner, "agent:")}
		} else if current.Dispatch == DispatchRequireAll {
			recipients = []string{current.Agent}
		}
		for _, recipient := range sortedUniqueStrings(recipients) {
			if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, task, id, current.RequirementID, current.Pool, recipient, "workflow.assignment_resumed", "", now); err != nil {
				return err
			}
		}
	}
	return nil
}

func exhaustWorkflowQuestionAssignmentTx(ctx context.Context, tx *sql.Tx, current assignmentContext, actor Actor, now string) error {
	questionID, err := strconv.ParseInt(strings.TrimPrefix(current.RequirementID, questionRequirementPrefix), 10, 64)
	if err != nil || questionID <= 0 {
		return domainError(http.StatusConflict, "workflow_invalid", "question assignment has an invalid requirement id")
	}
	result, err := tx.ExecContext(ctx, `UPDATE task_workflow_questions SET state = 'exhausted' WHERE id = ? AND state = 'open'`, questionID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domainError(http.StatusConflict, "workflow_question_closed", "workflow question is no longer open")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_workflow_holds SET released_at = ? WHERE question_id = ? AND released_at = ''`, now, questionID); err != nil {
		return err
	}
	if current.StatusExecutionState == "active" {
		return freezeExecutionTx(ctx, tx, current.Task, current.Workflow, current.StatusExecutionID, "workflow_question_retry_exhausted", actor, now)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_requirement_executions SET state = 'frozen' WHERE id = ? AND state = 'pending'`, current.RequirementExecutionID); err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.question_exhausted", actor, map[string]any{"question_id": questionID}, now); err != nil {
		return err
	}
	for _, recipient := range current.PoolSnapshot {
		if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, current.Task, nil, current.RequirementID, current.Pool, recipient, "workflow.escalated", "workflow_question_retry_exhausted", now); err != nil {
			return err
		}
	}
	return nil
}

func validateQuestionAttachments(ctx context.Context, q queryer, current assignmentContext, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	requirement, ok := workflowRequirementByID(current.Workflow.Definition, current.Task.WorkflowStatus, current.RequirementID)
	if !ok {
		return domainError(http.StatusConflict, "workflow_invalid", "assignment requirement is missing from the pinned workflow")
	}
	scope := artifactReadScope{assignmentID: current.ID, inputs: map[string]bool{}, outputs: map[string]bool{}}
	for _, name := range requirement.Inputs {
		scope.inputs[name] = true
	}
	for _, name := range requirement.Produces {
		scope.outputs[name] = true
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return domainError(http.StatusBadRequest, "invalid_artifact_attachment", "artifact attachments must contain unique positive ids")
		}
		seen[id] = true
		artifact, err := artifactByID(ctx, q, id)
		if err != nil || artifact.TaskKey != current.Task.Key {
			return domainError(http.StatusBadRequest, "invalid_artifact_attachment", "artifact attachment is not visible to this assignment")
		}
		visible, err := artifactVisibleInScope(ctx, q, current.Task.ID, artifact, scope)
		if err != nil {
			return err
		}
		if !visible {
			return domainError(http.StatusBadRequest, "invalid_artifact_attachment", "artifact attachment is not visible to this assignment")
		}
	}
	return nil
}
