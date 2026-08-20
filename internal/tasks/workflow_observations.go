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
	"time"
)

const (
	ObservationRecordOnly        = "record_only"
	ObservationWakeCurrent       = "wake_current"
	ObservationCreateRequirement = "create_requirement"
	ObservationHoldAssignment    = "hold_assignment"
	observationRequirementPrefix = "__observation:"
)

type QueueWorkflowTrigger struct {
	ID                    int64  `json:"id"`
	Queue                 string `json:"queue"`
	Pattern               string `json:"pattern"`
	CorrelationKey        string `json:"correlation_key,omitempty"`
	Action                string `json:"action"`
	Enabled               bool   `json:"enabled"`
	CreatedBy             string `json:"created_by"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
	CreatedAfterSequence  int64  `json:"-"`
	ActivationSequenceSet bool   `json:"-"`
}

type CreateQueueWorkflowTriggerInput struct {
	Pattern        string `json:"pattern"`
	CorrelationKey string `json:"correlation_key,omitempty"`
	Action         string `json:"action"`
}

type CreateWorkflowSubscriptionInput struct {
	TaskRevision       int64  `json:"task_revision"`
	AssignmentRevision int64  `json:"assignment_revision"`
	Pattern            string `json:"pattern"`
	CorrelationKey     string `json:"correlation_key,omitempty"`
	Reaction           string `json:"reaction"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type CancelWorkflowSubscriptionInput struct {
	TaskRevision       int64  `json:"task_revision"`
	AssignmentRevision int64  `json:"assignment_revision"`
	IdempotencyKey     string `json:"idempotency_key"`
}

// ApplyWorkflowObservationInput is the daemon-side, post-bus-commit ingress
// contract. EventID must be the immutable bus message id.
type ApplyWorkflowObservationInput struct {
	EventID        string         `json:"event_id"`
	EventAt        string         `json:"event_at,omitempty"`
	Channel        string         `json:"channel"`
	Kind           string         `json:"kind"`
	CorrelationKey string         `json:"correlation_key,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	Sequence       int64          `json:"-"`
}

// ReconcileWorkflowObservations durably replays committed bus rows after the
// last successfully ingested monotonic sequence. Apply is idempotent by message
// id, so a crash between Apply and cursor advancement is harmless.
func (s *Service) ReconcileWorkflowObservations(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	// Repair messages produced by a pre-sequence binary during a rolling update.
	// The statement is one serialized SQLite write and preserves legacy row order.
	if err := repairWorkflowMessageSequences(ctx, s.db); err != nil {
		return 0, err
	}
	var lastSequence int64
	if err := s.db.QueryRowContext(ctx, `SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton=1`).Scan(&lastSequence); err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT s.sequence,m.id,m.channel,m.ts,m.source,m.type,m.subject,m.text,m.data,m.kind,m.correlation_id,m.in_reply_to,m.reply_to FROM task_workflow_message_sequence s JOIN messages m ON m.id=s.message_id WHERE s.sequence>? ORDER BY s.sequence LIMIT ?`, lastSequence, limit)
	if err != nil {
		return 0, err
	}
	type ingressMessage struct {
		sequence                           int64
		id, channel, ts, source, kind, typ string
		correlation, inReplyTo, replyTo    sql.NullString
		subject, text, data                sql.NullString
	}
	messages := []ingressMessage{}
	for rows.Next() {
		var m ingressMessage
		if err := rows.Scan(&m.sequence, &m.id, &m.channel, &m.ts, &m.source, &m.typ, &m.subject, &m.text, &m.data, &m.kind, &m.correlation, &m.inReplyTo, &m.replyTo); err != nil {
			rows.Close()
			return 0, err
		}
		messages = append(messages, m)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(messages) == 0 {
		return 0, nil
	}
	advanced, err := s.advanceWorkflowIngressIfNoTargets(ctx, messages[len(messages)-1].sequence)
	if err != nil {
		return 0, err
	}
	if advanced {
		return len(messages), nil
	}
	processed := 0
	var lastProcessed int64
	for _, m := range messages {
		payload := map[string]any{"source": m.source, "text": m.text.String, "kind": m.kind, "in_reply_to": m.inReplyTo.String, "reply_to": m.replyTo.String}
		if m.subject.Valid && m.subject.String != "" {
			var subject map[string]any
			if err := json.Unmarshal([]byte(m.subject.String), &subject); err != nil {
				return processed, err
			}
			payload["subject"] = subject
		}
		if m.data.Valid && m.data.String != "" {
			var data map[string]any
			if err := json.Unmarshal([]byte(m.data.String), &data); err != nil {
				return processed, err
			}
			payload["data"] = data
		}
		if _, err := s.ApplyWorkflowObservation(ctx, ApplyWorkflowObservationInput{EventID: m.id, EventAt: m.ts, Channel: m.channel, Kind: m.typ, CorrelationKey: m.correlation.String, Payload: payload, Sequence: m.sequence}); err != nil {
			return processed, err
		}
		lastProcessed = m.sequence
		processed++
	}
	if lastProcessed != 0 {
		if _, err := s.db.ExecContext(ctx, `UPDATE task_workflow_ingress_state SET last_message_sequence=CASE WHEN last_message_sequence<? THEN ? ELSE last_message_sequence END WHERE singleton=1`, lastProcessed, lastProcessed); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func reserveWorkflowIngressWriter(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE task_workflow_ingress_state SET singleton=singleton WHERE singleton=1`)
	return err
}

type workflowSequenceExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func repairWorkflowMessageSequences(ctx context.Context, execer workflowSequenceExecer) error {
	_, err := execer.ExecContext(ctx, `INSERT OR IGNORE INTO task_workflow_message_sequence(message_id) SELECT m.id FROM messages m LEFT JOIN task_workflow_message_sequence s ON s.message_id=m.id WHERE s.message_id IS NULL ORDER BY m.rowid`)
	return err
}

func workflowIngressMaxSequence(ctx context.Context, tx *sql.Tx) (int64, error) {
	var sequence int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM task_workflow_message_sequence`).Scan(&sequence)
	return sequence, err
}

// advanceWorkflowIngressIfNoTargets makes the zero-target check and cursor
// advance one serialized SQLite write. Target creation/cancellation on another
// daemon must land wholly before the count or wholly after the cursor move.
func (s *Service) advanceWorkflowIngressIfNoTargets(ctx context.Context, last int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// BEGIN is deferred in SQLite. Take the writer reservation before reading
	// target state so another connection cannot commit a target in the gap.
	if err := reserveWorkflowIngressWriter(ctx, tx); err != nil {
		return false, err
	}
	var targets int
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM task_workflow_subscriptions WHERE state='active') + (SELECT COUNT(*) FROM task_queue_workflow_triggers WHERE enabled=1)`).Scan(&targets); err != nil {
		return false, err
	}
	if s.workflowIngressAfterTargetCount != nil {
		s.workflowIngressAfterTargetCount()
	}
	if targets != 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_workflow_ingress_state SET last_message_sequence=CASE WHEN last_message_sequence<? THEN ? ELSE last_message_sequence END WHERE singleton=1`, last, last); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) applyQueueWorkflowTriggers(ctx context.Context, in ApplyWorkflowObservationInput) (bool, error) {
	source, _ := in.Payload["source"].(string)
	if !strings.HasPrefix(source, "plugin:") {
		return false, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,queue_prefix,pattern,correlation_key,action,created_by,created_at,created_after_sequence,activation_sequence_set FROM task_queue_workflow_triggers WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return false, err
	}
	type trigger struct {
		id, createdAfterSequence                                  int64
		activationSequenceSet                                     bool
		queue, pattern, correlation, action, createdBy, createdAt string
	}
	triggers := []trigger{}
	for rows.Next() {
		var trigger trigger
		if err := rows.Scan(&trigger.id, &trigger.queue, &trigger.pattern, &trigger.correlation, &trigger.action, &trigger.createdBy, &trigger.createdAt, &trigger.createdAfterSequence, &trigger.activationSequenceSet); err != nil {
			rows.Close()
			return false, err
		}
		triggers = append(triggers, trigger)
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	matched := false
	for _, trigger := range triggers {
		if !workflowTargetEligible(in.Sequence, trigger.createdAfterSequence, trigger.activationSequenceSet, in.EventAt, trigger.createdAt) {
			continue
		}
		if !channelPatternMatches(trigger.pattern, in.Channel) || (trigger.correlation != "" && trigger.correlation != in.CorrelationKey) {
			continue
		}
		if trigger.action != "create_task" {
			return false, domainError(http.StatusConflict, "invalid_trigger_action", "persisted workflow trigger action is unsupported")
		}
		matched = true
		title, _ := in.Payload["text"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			title = "External event: " + in.Kind
		}
		description, err := json.Marshal(map[string]any{"event_id": in.EventID, "channel": in.Channel, "correlation_key": in.CorrelationKey, "payload": in.Payload})
		if err != nil {
			return false, err
		}
		actor := Actor{Principal: trigger.createdBy, IsCustomer: true}
		if _, err := s.CreateTask(ctx, actor, CreateTaskInput{Queue: trigger.queue, Title: title, Description: string(description), IdempotencyKey: "workflow-trigger:" + strconv.FormatInt(trigger.id, 10) + ":" + in.EventID}); err != nil {
			return false, err
		}
	}
	return matched, nil
}

func workflowTargetEligible(eventSequence, createdAfterSequence int64, activationSequenceSet bool, eventAt, createdAt string) bool {
	if eventSequence > 0 && activationSequenceSet {
		return eventSequence > createdAfterSequence
	}
	return timestampNotBefore(eventAt, createdAt)
}

func (s *Service) CreateQueueWorkflowTrigger(ctx context.Context, actor Actor, queue string, in CreateQueueWorkflowTriggerInput) (QueueWorkflowTrigger, error) {
	if err := requireOperator(actor); err != nil {
		return QueueWorkflowTrigger{}, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	in.Pattern, in.CorrelationKey, in.Action = strings.TrimSpace(in.Pattern), strings.TrimSpace(in.CorrelationKey), strings.TrimSpace(in.Action)
	if !validRuntimeChannelPattern(in.Pattern) {
		return QueueWorkflowTrigger{}, domainError(http.StatusBadRequest, "invalid_channel_pattern", "trigger channel pattern is invalid")
	}
	switch strings.SplitN(in.Pattern, ":", 2)[0] {
	case "agent", "group", "user", "system":
		return QueueWorkflowTrigger{}, domainError(http.StatusBadRequest, "invalid_channel_pattern", "task-creating triggers require an external channel namespace")
	}
	if in.Action == "" {
		return QueueWorkflowTrigger{}, domainError(http.StatusBadRequest, "invalid_trigger_action", "trigger action is required")
	}
	if in.Action != "create_task" {
		return QueueWorkflowTrigger{}, domainError(http.StatusBadRequest, "invalid_trigger_action", "trigger action must be create_task")
	}
	if _, err := s.GetQueue(ctx, actor, queue); err != nil {
		return QueueWorkflowTrigger{}, err
	}
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return QueueWorkflowTrigger{}, err
	}
	defer tx.Rollback()
	if err := reserveWorkflowIngressWriter(ctx, tx); err != nil {
		return QueueWorkflowTrigger{}, err
	}
	if s.workflowActivationAfterWriterReservation != nil {
		s.workflowActivationAfterWriterReservation()
	}
	if err := repairWorkflowMessageSequences(ctx, tx); err != nil {
		return QueueWorkflowTrigger{}, err
	}
	createdAfterSequence, err := workflowIngressMaxSequence(ctx, tx)
	if err != nil {
		return QueueWorkflowTrigger{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO task_queue_workflow_triggers(queue_prefix, pattern, correlation_key, action, enabled, created_by, created_at, updated_at, created_after_sequence, activation_sequence_set) VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, 1)`, queue, in.Pattern, in.CorrelationKey, in.Action, actor.Principal, now, now, createdAfterSequence)
	if err != nil {
		return QueueWorkflowTrigger{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return QueueWorkflowTrigger{}, err
	}
	s.workflowIngressEnabled.Store(true)
	return QueueWorkflowTrigger{ID: id, Queue: queue, Pattern: in.Pattern, CorrelationKey: in.CorrelationKey, Action: in.Action, Enabled: true, CreatedBy: actor.Principal, CreatedAt: now, UpdatedAt: now, CreatedAfterSequence: createdAfterSequence, ActivationSequenceSet: true}, nil
}

func (s *Service) ListQueueWorkflowTriggers(ctx context.Context, actor Actor, queue string) ([]QueueWorkflowTrigger, error) {
	if err := requireOperator(actor); err != nil {
		return nil, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	rows, err := s.db.QueryContext(ctx, `SELECT id, pattern, correlation_key, action, enabled, created_by, created_at, updated_at, created_after_sequence, activation_sequence_set FROM task_queue_workflow_triggers WHERE queue_prefix=? ORDER BY id`, queue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QueueWorkflowTrigger{}
	for rows.Next() {
		var x QueueWorkflowTrigger
		x.Queue = queue
		if err := rows.Scan(&x.ID, &x.Pattern, &x.CorrelationKey, &x.Action, &x.Enabled, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt, &x.CreatedAfterSequence, &x.ActivationSequenceSet); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) DeleteQueueWorkflowTrigger(ctx context.Context, actor Actor, queue string, id int64) error {
	if err := requireOperator(actor); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM task_queue_workflow_triggers WHERE id=? AND queue_prefix=?`, id, strings.ToUpper(strings.TrimSpace(queue)))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return domainError(http.StatusNotFound, "workflow_trigger_not_found", "workflow trigger not found")
	}
	s.refreshWorkflowIngressEnabled(ctx)
	return nil
}

func (s *Service) CreateWorkflowSubscription(ctx context.Context, actor Actor, assignmentID string, in CreateWorkflowSubscriptionInput) (WorkflowSubscription, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return WorkflowSubscription{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	defer tx.Rollback()
	if err := reserveWorkflowIngressWriter(ctx, tx); err != nil {
		return WorkflowSubscription{}, err
	}
	if s.workflowActivationAfterWriterReservation != nil {
		s.workflowActivationAfterWriterReservation()
	}
	if err := repairWorkflowMessageSequences(ctx, tx); err != nil {
		return WorkflowSubscription{}, err
	}
	if replay, ok, err := readTaskIdempotency[WorkflowSubscription](ctx, tx, actor.Principal, "create_workflow_subscription", in.IdempotencyKey); err != nil || ok {
		return replay, err
	}
	if err := requireAssignmentMutationRevisions(in.TaskRevision, in.AssignmentRevision); err != nil {
		return WorkflowSubscription{}, err
	}
	current, err := assignmentContextByID(ctx, tx, id)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := requireRuntimeRevisions(current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return WorkflowSubscription{}, err
	}
	reaction := strings.TrimSpace(in.Reaction)
	if reaction == "" {
		reaction = ObservationRecordOnly
	}
	if !workflowAllowsReaction(current.Workflow.Definition, reaction) {
		return WorkflowSubscription{}, domainError(http.StatusForbidden, "workflow_reaction_not_allowed", "observation reaction is not declared by the workflow")
	}
	pattern := strings.TrimSpace(in.Pattern)
	allowed, err := resolvedAllowedChannels(ctx, tx, current)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	if !channelRequestAllowed(pattern, allowed) {
		return WorkflowSubscription{}, domainError(http.StatusForbidden, "channel_not_allowed", "channel subscription is outside the assignment policy")
	}
	now := s.now()
	createdAfterSequence, err := workflowIngressMaxSequence(ctx, tx)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO task_workflow_subscriptions(task_id, assignment_id, pattern, correlation_key, reaction, state, created_by, created_at, created_after_sequence, activation_sequence_set) VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, 1)`, current.Task.ID, id, pattern, strings.TrimSpace(in.CorrelationKey), reaction, actor.Principal, now, createdAfterSequence)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	sid, _ := res.LastInsertId()
	if err := bumpObservationMutationRevisions(ctx, tx, current, in.TaskRevision, in.AssignmentRevision, now); err != nil {
		return WorkflowSubscription{}, err
	}
	sub := WorkflowSubscription{ID: sid, TaskKey: current.Task.Key, AssignmentID: id, Pattern: pattern, CorrelationKey: strings.TrimSpace(in.CorrelationKey), Reaction: reaction, State: "active", CreatedBy: actor.Principal, CreatedAt: now, CreatedAfterSequence: createdAfterSequence, ActivationSequenceSet: true}
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.subscription_created", actor, map[string]any{"subscription_id": sid, "assignment_id": id, "pattern": pattern, "reaction": reaction}, now); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := writeTaskIdempotency(ctx, tx, actor.Principal, "create_workflow_subscription", in.IdempotencyKey, sub, now); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkflowSubscription{}, err
	}
	s.workflowIngressEnabled.Store(true)
	s.signal()
	return sub, nil
}

func (s *Service) ListWorkflowSubscriptions(ctx context.Context, actor Actor, assignmentID string) ([]WorkflowSubscription, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return nil, err
	}
	current, err := assignmentContextByID(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if actor.IsCustomer {
		access, err := taskAccess(ctx, s.db, actor, current.Task.ID)
		if err != nil {
			return nil, err
		}
		if access == "" {
			return nil, notFound(current.Task.Key)
		}
	} else {
		if err := requireWorkflowAgent(actor); err != nil {
			return nil, err
		}
		if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
			return nil, err
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, pattern, correlation_key, reaction, state, created_by, created_at, cancelled_at, created_after_sequence, activation_sequence_set FROM task_workflow_subscriptions WHERE task_id=? AND assignment_id=? ORDER BY id`, current.Task.ID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowSubscription{}
	for rows.Next() {
		var x WorkflowSubscription
		x.TaskKey, x.AssignmentID = current.Task.Key, id
		if err := rows.Scan(&x.ID, &x.Pattern, &x.CorrelationKey, &x.Reaction, &x.State, &x.CreatedBy, &x.CreatedAt, &x.CancelledAt, &x.CreatedAfterSequence, &x.ActivationSequenceSet); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) ListTaskWorkflowSubscriptions(ctx context.Context, actor Actor, taskKey, assignmentID string) ([]WorkflowSubscription, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return nil, err
	}
	task, err := taskByKey(s.db, strings.TrimSpace(taskKey))
	if err != nil {
		return nil, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return nil, err
	}
	current, err := assignmentContextByID(ctx, s.db, id)
	if err != nil || current.Task.ID != task.ID {
		if err == nil || ErrorCode(err) == "assignment_not_found" {
			return nil, domainError(http.StatusNotFound, "workflow_assignment_not_found", "workflow assignment not found for task")
		}
		return nil, err
	}
	return s.ListWorkflowSubscriptions(ctx, actor, assignmentID)
}

func (s *Service) CancelWorkflowSubscription(ctx context.Context, actor Actor, assignmentID string, subscriptionID int64, in CancelWorkflowSubscriptionInput) (WorkflowSubscription, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return WorkflowSubscription{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	defer tx.Rollback()
	if replay, ok, err := readTaskIdempotency[WorkflowSubscription](ctx, tx, actor.Principal, "cancel_workflow_subscription", in.IdempotencyKey); err != nil || ok {
		return replay, err
	}
	current, err := assignmentContextByID(ctx, tx, id)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := requireRuntimeRevisions(current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return WorkflowSubscription{}, err
	}
	now := s.now()
	res, err := tx.ExecContext(ctx, `UPDATE task_workflow_subscriptions SET state='cancelled', cancelled_at=? WHERE id=? AND task_id=? AND assignment_id=? AND state='active'`, now, subscriptionID, current.Task.ID, id)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return WorkflowSubscription{}, domainError(http.StatusNotFound, "workflow_subscription_not_found", "active workflow subscription not found")
	}
	if err := bumpObservationMutationRevisions(ctx, tx, current, in.TaskRevision, in.AssignmentRevision, now); err != nil {
		return WorkflowSubscription{}, err
	}
	sub, err := workflowSubscriptionByID(ctx, tx, current.Task.Key, subscriptionID)
	if err != nil {
		return WorkflowSubscription{}, err
	}
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.subscription_cancelled", actor, map[string]any{"subscription_id": subscriptionID, "assignment_id": id}, now); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := writeTaskIdempotency(ctx, tx, actor.Principal, "cancel_workflow_subscription", in.IdempotencyKey, sub, now); err != nil {
		return WorkflowSubscription{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkflowSubscription{}, err
	}
	s.refreshWorkflowIngressEnabled(ctx)
	s.signal()
	return sub, nil
}

func (s *Service) ApplyWorkflowObservation(ctx context.Context, in ApplyWorkflowObservationInput) ([]WorkflowObservation, error) {
	in.EventID, in.EventAt, in.Channel, in.Kind, in.CorrelationKey = strings.TrimSpace(in.EventID), strings.TrimSpace(in.EventAt), strings.TrimSpace(in.Channel), strings.TrimSpace(in.Kind), strings.TrimSpace(in.CorrelationKey)
	if in.EventID == "" || in.Channel == "" {
		return nil, domainError(http.StatusBadRequest, "invalid_observation", "event id and channel are required")
	}
	if in.EventAt == "" {
		in.EventAt = s.now()
	}
	if _, err := time.Parse(time.RFC3339Nano, in.EventAt); err != nil {
		return nil, domainError(http.StatusBadRequest, "invalid_observation", "event_at must be RFC3339")
	}
	triggered, err := s.applyQueueWorkflowTriggers(ctx, in)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	const actor = "system:workflow-observation"
	if replay, ok, err := readTaskIdempotency[[]WorkflowObservation](ctx, tx, actor, "apply_workflow_observation", in.EventID); err != nil || ok {
		return replay, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.task_id, s.assignment_id, s.pattern, s.correlation_key, s.reaction, s.created_at, s.created_after_sequence, s.activation_sequence_set, t.task_key, t.queue_prefix, COALESCE(t.workflow_version_id,0), COALESCE(t.workflow_status,'') FROM task_workflow_subscriptions s JOIN tasks t ON t.id=s.task_id WHERE s.state='active' ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	type matched struct {
		sid, tid, aid, createdAfterSequence                       int64
		pattern, correlation, reaction, createdAt, taskKey, queue string
		workflowID                                                int64
		activationSequenceSet                                     bool
		status                                                    string
	}
	var matches []matched
	for rows.Next() {
		var m matched
		var aid sql.NullInt64
		if err := rows.Scan(&m.sid, &m.tid, &aid, &m.pattern, &m.correlation, &m.reaction, &m.createdAt, &m.createdAfterSequence, &m.activationSequenceSet, &m.taskKey, &m.queue, &m.workflowID, &m.status); err != nil {
			rows.Close()
			return nil, err
		}
		m.aid = aid.Int64
		if workflowTargetEligible(in.Sequence, m.createdAfterSequence, m.activationSequenceSet, in.EventAt, m.createdAt) && channelPatternMatches(m.pattern, in.Channel) && (m.correlation == "" || m.correlation == in.CorrelationKey) {
			matches = append(matches, m)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(matches) == 0 && !triggered {
		return []WorkflowObservation{}, nil
	}
	now := s.now()
	payload := cloneMap(in.Payload)
	payload["event_id"] = in.EventID
	payload["channel"] = in.Channel
	payload["correlation_key"] = in.CorrelationKey
	raw, _ := json.Marshal(payload)
	observations := make([]WorkflowObservation, 0, len(matches))
	touched := map[int64]bool{}
	for _, m := range matches {
		result, err := tx.ExecContext(ctx, `INSERT INTO task_observations(task_id,subscription_id,assignment_id,kind,payload,observed_at) VALUES(?,?,?,?,?,?)`, m.tid, m.sid, nullInt64(m.aid), in.Kind, string(raw), now)
		if err != nil {
			return nil, err
		}
		oid, _ := result.LastInsertId()
		obs := WorkflowObservation{ID: oid, TaskKey: m.taskKey, SubscriptionID: m.sid, AssignmentID: m.aid, Kind: in.Kind, Payload: cloneMap(payload), ObservedAt: now}
		observations = append(observations, obs)
		if _, err := appendEventTx(ctx, tx, Task{ID: m.tid, Key: m.taskKey, Queue: m.queue}, "observation.appended", Actor{Principal: actor}, map[string]any{
			"observation_id": oid, "subscription_id": m.sid, "assignment_id": m.aid,
			"event_id": in.EventID, "channel": in.Channel,
		}, now); err != nil {
			return nil, err
		}
		reaction := m.reaction
		current, loadErr := assignmentContextByID(ctx, tx, m.aid)
		leaseExpired := true
		if loadErr == nil {
			if deadline, parseErr := time.Parse(time.RFC3339Nano, current.LeaseExpiresAt); parseErr == nil {
				leaseExpired = !deadline.After(s.clock().UTC())
			}
		}
		if loadErr != nil || current.State != AssignmentLeased || leaseExpired || current.Task.WorkflowStatus != m.status {
			reaction = ObservationRecordOnly
		}
		if reaction != ObservationRecordOnly && !workflowAllowsReaction(current.Workflow.Definition, reaction) {
			reaction = ObservationRecordOnly
		}
		switch reaction {
		case ObservationWakeCurrent:
			err = enqueueWorkflowRuntimeNotificationTx(ctx, tx, current.Task, m.aid, current.RequirementID, current.Pool, strings.TrimPrefix(current.LeaseOwner, "agent:"), "workflow.assignment_resumed", "", now)
		case ObservationHoldAssignment:
			_, err = tx.ExecContext(ctx, `INSERT INTO task_workflow_holds(task_id,assignment_id,requirement_execution_id,scope,reason,created_at) VALUES(?,?,?,'assignment',?,?)`, m.tid, m.aid, current.RequirementExecutionID, "observation:"+in.EventID, now)
			if err == nil {
				err = enqueueWorkflowRuntimeNotificationTx(ctx, tx, current.Task, m.aid, current.RequirementID, current.Pool, strings.TrimPrefix(current.LeaseOwner, "agent:"), "workflow.observation_ready", "", now)
			}
		case ObservationCreateRequirement:
			err = createObservationRequirementTx(ctx, tx, current, m.sid, in.EventID, now)
		}
		if err != nil {
			return nil, err
		}
		touched[m.tid] = true
	}
	for tid := range touched {
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET workflow_revision=workflow_revision+1,updated_at=? WHERE id=?`, now, tid); err != nil {
			return nil, err
		}
	}
	if err := writeTaskIdempotency(ctx, tx, actor, "apply_workflow_observation", in.EventID, observations, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.signal()
	return observations, nil
}

func resolvedAllowedChannels(ctx context.Context, q queryer, current assignmentContext) ([]string, error) {
	out := []string{}
	for _, template := range current.Workflow.Definition.Permissions.Channels.Subscribe {
		resolved := template
		for strings.Contains(resolved, "${") {
			start := strings.Index(resolved, "${")
			endRel := strings.Index(resolved[start:], "}")
			if endRel < 0 {
				return nil, domainError(http.StatusConflict, "workflow_invalid", "unterminated channel template")
			}
			end := start + endRel
			field := resolved[start+2 : end]
			value, err := channelTemplateValue(ctx, q, current, field)
			if err != nil {
				if ErrorCode(err) == "channel_not_allowed" {
					resolved = ""
					break
				}
				return nil, err
			}
			if !channelSegmentRE.MatchString(value) {
				return nil, domainError(http.StatusConflict, "channel_template_invalid", "channel template produced an unsafe segment")
			}
			resolved = resolved[:start] + value + resolved[end+1:]
		}
		if resolved != "" {
			out = append(out, resolved)
		}
	}
	sort.Strings(out)
	return out, nil
}
func channelTemplateValue(ctx context.Context, q queryer, current assignmentContext, field string) (string, error) {
	switch field {
	case "task.key":
		return strings.ToLower(current.Task.Key), nil
	case "task.queue":
		return strings.ToLower(current.Task.Queue), nil
	case "task.priority":
		return strings.ToLower(string(current.Task.Priority)), nil
	case "task.group":
		return current.Task.Group, nil
	case "task.customer":
		return strings.TrimPrefix(current.Task.Customer, "user:"), nil
	}
	const p = "task.artifacts."
	if !strings.HasPrefix(field, p) {
		return "", domainError(http.StatusConflict, "workflow_invalid", "unsupported channel template")
	}
	var value string
	err := q.QueryRowContext(ctx, `SELECT content FROM task_artifacts WHERE task_id=? AND name=? ORDER BY id DESC LIMIT 1`, current.Task.ID, strings.TrimPrefix(field, p)).Scan(&value)
	if err == sql.ErrNoRows {
		return "", domainError(http.StatusForbidden, "channel_not_allowed", "required channel artifact is unavailable")
	}
	return strings.TrimSpace(value), err
}
func channelRequestAllowed(request string, allowed []string) bool {
	if !validRuntimeChannelPattern(request) {
		return false
	}
	for _, a := range allowed {
		if strings.Contains(request, "*") {
			if request == a && strings.HasSuffix(a, ":*") {
				return true
			}
			continue
		}
		if channelPatternMatches(a, request) {
			return true
		}
	}
	return false
}
func validRuntimeChannelPattern(pattern string) bool {
	parts := strings.Split(strings.TrimSpace(pattern), ":")
	if len(parts) < 2 {
		return false
	}
	for i, p := range parts {
		if p == "*" {
			if len(parts) != 2 || i != 1 {
				return false
			}
			continue
		}
		if !channelSegmentRE.MatchString(p) {
			return false
		}
	}
	return true
}
func channelPatternMatches(pattern, channel string) bool {
	if !validRuntimeChannelPattern(pattern) || !validRuntimeChannelPattern(channel) || strings.Contains(channel, "*") {
		return false
	}
	if strings.HasSuffix(pattern, ":*") {
		return strings.HasPrefix(channel, strings.TrimSuffix(pattern, "*")) && len(strings.Split(channel, ":")) == 2
	}
	return pattern == channel
}
func workflowAllowsReaction(def WorkflowDefinition, reaction string) bool {
	if !validObservationReaction(reaction) {
		return false
	}
	if !containsString(def.Observations.AllowedReactions, reaction) {
		return false
	}
	if len(def.Permissions.Channels.Reactions) > 0 && !containsString(def.Permissions.Channels.Reactions, reaction) {
		return false
	}
	return true
}
func bumpObservationMutationRevisions(ctx context.Context, tx *sql.Tx, current assignmentContext, tr, ar int64, now string) error {
	r, e := tx.ExecContext(ctx, `UPDATE task_assignments SET revision=revision+1,updated_at=? WHERE id=? AND revision=?`, now, current.ID, ar)
	if e != nil {
		return e
	}
	if n, _ := r.RowsAffected(); n != 1 {
		return domainError(http.StatusConflict, "revision_conflict", "assignment changed concurrently")
	}
	r, e = tx.ExecContext(ctx, `UPDATE tasks SET workflow_revision=workflow_revision+1,updated_at=? WHERE id=? AND workflow_revision=?`, now, current.Task.ID, tr)
	if e != nil {
		return e
	}
	if n, _ := r.RowsAffected(); n != 1 {
		return domainError(http.StatusConflict, "revision_conflict", "task changed concurrently")
	}
	return nil
}
func workflowSubscriptionByID(ctx context.Context, q queryer, taskKey string, id int64) (WorkflowSubscription, error) {
	var x WorkflowSubscription
	var aid sql.NullInt64
	x.TaskKey = taskKey
	err := q.QueryRowContext(ctx, `SELECT id,assignment_id,pattern,correlation_key,reaction,state,created_by,created_at,cancelled_at,created_after_sequence,activation_sequence_set FROM task_workflow_subscriptions WHERE id=?`, id).Scan(&x.ID, &aid, &x.Pattern, &x.CorrelationKey, &x.Reaction, &x.State, &x.CreatedBy, &x.CreatedAt, &x.CancelledAt, &x.CreatedAfterSequence, &x.ActivationSequenceSet)
	x.AssignmentID = aid.Int64
	return x, err
}
func createObservationRequirementTx(ctx context.Context, tx *sql.Tx, current assignmentContext, sid int64, eventID, now string) error {
	snapshot, _ := json.Marshal(current.PoolSnapshot)
	digest := sha256.Sum256([]byte(eventID))
	requirementID := observationRequirementPrefix + strconv.FormatInt(sid, 10) + ":" + hex.EncodeToString(digest[:16])
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO task_requirement_executions(status_execution_id,requirement_id,pool_id,dispatch,optional,pool_snapshot,inputs,produces,outcomes,state,created_at) SELECT ?,?,id,'claim_one',1,?,'[]','[]','["acknowledged"]','pending',? FROM task_agent_pools WHERE queue_prefix=? AND name=?`, current.StatusExecutionID, requirementID, string(snapshot), now, current.Task.Queue, current.Pool)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}
	var rid int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM task_requirement_executions WHERE status_execution_id=? AND requirement_id=?`, current.StatusExecutionID, requirementID).Scan(&rid); err != nil {
		return err
	}
	assignmentResult, err := tx.ExecContext(ctx, `INSERT INTO task_assignments(requirement_execution_id,agent,attempt,state,revision,created_at,updated_at) VALUES(?,NULL,1,'claimable',1,?,?)`, rid, now, now)
	if err != nil {
		return err
	}
	assignmentID, _ := assignmentResult.LastInsertId()
	for _, agent := range current.PoolSnapshot {
		if err := enqueueWorkflowRuntimeNotificationTx(ctx, tx, current.Task, assignmentID, requirementID, current.Pool, agent, "workflow.assignment_ready", "", now); err != nil {
			return err
		}
	}
	return nil
}
func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
func cloneMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func timestampNotBefore(eventAt, createdAt string) bool {
	eventTime, eventErr := time.Parse(time.RFC3339Nano, eventAt)
	createdTime, createdErr := time.Parse(time.RFC3339Nano, createdAt)
	return eventErr == nil && createdErr == nil && !eventTime.Before(createdTime)
}
func requireOperator(actor Actor) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if !actor.IsCustomer {
		return domainError(http.StatusForbidden, "forbidden", "operator access required")
	}
	return nil
}
