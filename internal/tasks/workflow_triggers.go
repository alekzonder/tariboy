package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/store"
)

var principalSegmentRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

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

// CreateQueueWorkflowTriggerInput creates a trigger of a queue. Pattern is a
// sender glob (plugin:*, plugin:telegram) a letter to queue:<prefix> must
// match; CorrelationKey, when set, must equal the letter's
// data.correlation_key.
type CreateQueueWorkflowTriggerInput struct {
	Pattern        string `json:"pattern"`
	CorrelationKey string `json:"correlation_key,omitempty"`
	Action         string `json:"action"`
}

// ApplyWorkflowObservationInput is one letter to a queue: EventID is the
// letter id and Sequence its mail.seq.
type ApplyWorkflowObservationInput struct {
	EventID        string         `json:"event_id"`
	EventAt        string         `json:"event_at,omitempty"`
	Sender         string         `json:"sender"`
	Queue          string         `json:"queue"`
	Kind           string         `json:"kind"`
	Subject        string         `json:"subject,omitempty"`
	Body           string         `json:"body,omitempty"`
	CorrelationKey string         `json:"correlation_key,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
	Sequence       int64          `json:"-"`
}

// ReconcileWorkflowObservations replays committed letters to queue:*
// principals after the last ingested mail.seq. Apply is idempotent by letter
// id, so a crash between Apply and cursor advancement is harmless.
func (s *WorkflowEngine) ReconcileWorkflowObservations(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	var lastSequence int64
	if err := s.db.QueryRowContext(ctx, `SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton=1`).Scan(&lastSequence); err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq,id,sender,recipient,created_at,kind,subject,body,data FROM mail
		WHERE seq>? AND recipient GLOB 'queue:*' ORDER BY seq LIMIT ?`, lastSequence, limit)
	if err != nil {
		return 0, err
	}
	letters := []ApplyWorkflowObservationInput{}
	for rows.Next() {
		var in ApplyWorkflowObservationInput
		var recipient, data string
		if err := rows.Scan(&in.Sequence, &in.EventID, &in.Sender, &recipient, &in.EventAt, &in.Kind, &in.Subject, &in.Body, &data); err != nil {
			rows.Close()
			return 0, err
		}
		in.Queue = strings.TrimPrefix(recipient, "queue:")
		// A letter whose data does not decode still creates its task.
		if json.Unmarshal([]byte(data), &in.Data) != nil {
			in.Data = map[string]any{}
		}
		in.CorrelationKey, _ = in.Data["correlation_key"].(string)
		letters = append(letters, in)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(letters) == 0 {
		return 0, nil
	}
	advanced, err := s.advanceWorkflowIngressIfNoTargets(ctx, letters[len(letters)-1].Sequence)
	if err != nil {
		return 0, err
	}
	if advanced {
		return len(letters), nil
	}
	processed := 0
	var lastProcessed int64
	for _, in := range letters {
		if err := s.ApplyWorkflowObservation(ctx, in); err != nil {
			return processed, err
		}
		lastProcessed = in.Sequence
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

// WorkflowLetterRetainedSQL holds a mail row workflow ingress has not consumed
// yet: a letter to a queue past the ingress cursor. Retention keeps such rows.
const WorkflowLetterRetainedSQL = `(mail.recipient GLOB 'queue:*' AND mail.seq >
	(SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton = 1))`

func workflowIngressMaxSequence(ctx context.Context, tx *sql.Tx) (int64, error) {
	var sequence int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM mail`).Scan(&sequence)
	return sequence, err
}

// advanceWorkflowIngressIfNoTargets makes the zero-target check and cursor
// advance one serialized SQLite write. Trigger creation on another daemon must
// land wholly before the count or wholly after the cursor move.
func (s *WorkflowEngine) advanceWorkflowIngressIfNoTargets(ctx context.Context, last int64) (bool, error) {
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
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_queue_workflow_triggers WHERE enabled=1`).Scan(&targets); err != nil {
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

func (s *WorkflowEngine) applyQueueWorkflowTriggers(ctx context.Context, in ApplyWorkflowObservationInput) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,queue_prefix,pattern,correlation_key,action,created_by,created_at,created_after_sequence,activation_sequence_set FROM task_queue_workflow_triggers WHERE enabled=1 AND queue_prefix=? ORDER BY id`, in.Queue)
	if err != nil {
		return err
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
			return err
		}
		triggers = append(triggers, trigger)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, trigger := range triggers {
		if !workflowTargetEligible(in.Sequence, trigger.createdAfterSequence, trigger.activationSequenceSet, in.EventAt, trigger.createdAt) {
			continue
		}
		if !senderPatternMatches(trigger.pattern, in.Sender) || (trigger.correlation != "" && trigger.correlation != in.CorrelationKey) {
			continue
		}
		if trigger.action != "create_task" {
			return newError(http.StatusConflict, "invalid_trigger_action", "persisted workflow trigger action is unsupported")
		}
		title := strings.TrimSpace(in.Subject)
		if title == "" {
			title = strings.TrimSpace(in.Body)
		}
		if title == "" {
			title = "External event: " + in.Kind
		}
		description, err := json.Marshal(map[string]any{"letter_id": in.EventID, "sender": in.Sender, "kind": in.Kind,
			"correlation_key": in.CorrelationKey, "body": in.Body, "data": in.Data})
		if err != nil {
			return err
		}
		actor := Actor{Principal: trigger.createdBy, IsCustomer: true}
		if _, err := s.taskSvc.CreateTask(ctx, actor, CreateTaskInput{Queue: trigger.queue, Title: title, Description: string(description), IdempotencyKey: "workflow-trigger:" + strconv.FormatInt(trigger.id, 10) + ":" + in.EventID}); err != nil {
			return err
		}
	}
	return nil
}

func workflowTargetEligible(eventSequence, createdAfterSequence int64, activationSequenceSet bool, eventAt, createdAt string) bool {
	if eventSequence > 0 && activationSequenceSet {
		return eventSequence > createdAfterSequence
	}
	return timestampNotBefore(eventAt, createdAt)
}

func (s *WorkflowEngine) CreateQueueWorkflowTrigger(ctx context.Context, actor Actor, queue string, in CreateQueueWorkflowTriggerInput) (QueueWorkflowTrigger, error) {
	if err := requireOperator(actor); err != nil {
		return QueueWorkflowTrigger{}, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	in.Pattern, in.CorrelationKey, in.Action = strings.TrimSpace(in.Pattern), strings.TrimSpace(in.CorrelationKey), strings.TrimSpace(in.Action)
	if !validSenderPattern(in.Pattern) {
		return QueueWorkflowTrigger{}, newError(http.StatusBadRequest, "invalid_sender_pattern",
			"trigger pattern must be a sender glob such as plugin:* or plugin:telegram")
	}
	if in.Action == "" {
		return QueueWorkflowTrigger{}, newError(http.StatusBadRequest, "invalid_trigger_action", "trigger action is required")
	}
	if in.Action != "create_task" {
		return QueueWorkflowTrigger{}, newError(http.StatusBadRequest, "invalid_trigger_action", "trigger action must be create_task")
	}
	if _, err := s.admin.GetQueue(ctx, actor, queue); err != nil {
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

func (s *WorkflowEngine) ListQueueWorkflowTriggers(ctx context.Context, actor Actor, queue string) ([]QueueWorkflowTrigger, error) {
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

func (s *WorkflowEngine) DeleteQueueWorkflowTrigger(ctx context.Context, actor Actor, queue string, id int64) error {
	if err := requireOperator(actor); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM task_queue_workflow_triggers WHERE id=? AND queue_prefix=?`, id, strings.ToUpper(strings.TrimSpace(queue)))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return notFound("workflow_trigger_not_found", "workflow trigger not found")
	}
	s.refreshWorkflowIngressEnabled(ctx)
	return nil
}

// ApplyWorkflowObservation turns one letter to a queue into tasks for every
// matching trigger of that queue. Trigger task creation is idempotent per
// trigger and letter id, so replaying a letter creates nothing new.
func (s *WorkflowEngine) ApplyWorkflowObservation(ctx context.Context, in ApplyWorkflowObservationInput) error {
	in.EventID, in.EventAt, in.Sender, in.Queue, in.Kind, in.CorrelationKey = strings.TrimSpace(in.EventID), strings.TrimSpace(in.EventAt), strings.TrimSpace(in.Sender), strings.TrimSpace(in.Queue), strings.TrimSpace(in.Kind), strings.TrimSpace(in.CorrelationKey)
	if in.EventID == "" || in.Sender == "" || in.Queue == "" {
		return newError(http.StatusBadRequest, "invalid_observation", "letter id, sender and queue are required")
	}
	if in.EventAt == "" {
		in.EventAt = s.now()
	}
	eventAt, err := store.ParseInputTime(in.EventAt)
	if err != nil {
		return newError(http.StatusBadRequest, "invalid_observation", "event_at must be an RFC 3339 time")
	}
	in.EventAt = store.FormatTime(eventAt)
	return s.applyQueueWorkflowTriggers(ctx, in)
}

// validSenderPattern accepts kind:name or kind:* for a principal kind that
// can send mail.
func validSenderPattern(pattern string) bool {
	kind, name, ok := strings.Cut(strings.TrimSpace(pattern), ":")
	if !ok {
		return false
	}
	switch kind {
	case "agent", "user", "plugin", "system":
	default:
		return false
	}
	return name == "*" || principalSegmentRE.MatchString(name)
}

func senderPatternMatches(pattern, sender string) bool {
	if !validSenderPattern(pattern) {
		return false
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(sender, prefix) && len(sender) > len(prefix)
	}
	return pattern == sender
}

func timestampNotBefore(eventAt, createdAt string) bool {
	eventTime, eventErr := store.ParseTime(eventAt)
	createdTime, createdErr := store.ParseTime(createdAt)
	return eventErr == nil && createdErr == nil && !eventTime.Before(createdTime)
}

func requireOperator(actor Actor) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if !actor.IsCustomer {
		return newError(http.StatusForbidden, "forbidden", "operator access required")
	}
	return nil
}

func (s *WorkflowEngine) WorkflowIngressEnabled() bool { return s.workflowIngressEnabled.Load() }

func (s *WorkflowEngine) refreshWorkflowIngressEnabled(ctx context.Context) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_queue_workflow_triggers WHERE enabled=1`).Scan(&count); err == nil {
		s.workflowIngressEnabled.Store(count > 0)
	}
}
