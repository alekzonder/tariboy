package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

func appendEventTx(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	kind string,
	actor Actor,
	payload map[string]any,
	now string,
) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_events(
			event_id, task_id, queue_prefix, kind, actor, task_revision, payload, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		newID("te"), task.ID, task.Queue, kind, actor.Principal, task.Revision, string(raw), now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func appendQueueEventTx(
	ctx context.Context,
	tx *sql.Tx,
	queue Queue,
	kind string,
	actor Actor,
	payload map[string]any,
	now string,
) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_events(
			event_id, task_id, queue_prefix, kind, actor, task_revision, payload, created_at
		) VALUES (?, NULL, ?, ?, ?, ?, ?, ?)`,
		newID("te"), queue.Prefix, kind, actor.Principal, queue.Revision, string(raw), now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func principalChannel(principal string) string {
	switch {
	case strings.HasPrefix(principal, "agent:"):
		return principal + ":inbox"
	case strings.HasPrefix(principal, "user:"):
		return principal
	default:
		return ""
	}
}

func enqueueNotificationTx(
	ctx context.Context,
	tx *sql.Tx,
	sequence int64,
	principal string,
	typ string,
	task Task,
	text string,
	now string,
) error {
	channel := principalChannel(principal)
	if channel == "" {
		return nil
	}
	notificationID := newID("tn")
	subject, _ := json.Marshal(map[string]any{
		"notification_id": notificationID,
		"task_key":        task.Key,
		"event_sequence":  sequence,
	})
	data, _ := json.Marshal(map[string]any{
		"task_key":  task.Key,
		"queue":     task.Queue,
		"principal": principal,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_notification_outbox(
			notification_id, event_sequence, channel, message_type, subject, text,
			data, next_attempt_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		notificationID, sequence, channel, typ, string(subject), text, string(data), now); err != nil {
		return err
	}
	if strings.HasPrefix(principal, "user:") {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO task_notification_state(customer_principal, notification_id)
			VALUES (?, ?)`, principal, notificationID)
		return err
	}
	return nil
}
