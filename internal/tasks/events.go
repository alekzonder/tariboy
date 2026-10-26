package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
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

// notificationRecipient is the mail recipient of a notification to
// principal: an agent or the customer. Anyone else gets none.
func notificationRecipient(principal string) string {
	if strings.HasPrefix(principal, "agent:") || strings.HasPrefix(principal, "user:") {
		return principal
	}
	return ""
}

func enqueueNotificationTx(
	ctx context.Context,
	tx *sql.Tx,
	sequence int64,
	principal string,
	from string,
	typ string,
	task Task,
	text string,
	now string,
) error {
	recipient := notificationRecipient(principal)
	if recipient == "" {
		return nil
	}
	notificationID := newID("tn")
	subject, _ := json.Marshal(map[string]any{
		"notification_id": notificationID,
		"task_key":        task.Key,
		"event_sequence":  sequence,
	})
	// from is the principal that caused the notification; the letter is sent
	// from it.
	data, _ := json.Marshal(map[string]any{
		"task_key":        task.Key,
		"queue":           task.Queue,
		"principal":       principal,
		"from":            from,
		"notification_id": notificationID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_notification_outbox(
			notification_id, event_sequence, channel, message_type, subject, text,
			data, next_attempt_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		notificationID, sequence, recipient, typ, string(subject), text, string(data), now); err != nil {
		return err
	}
	return nil
}

// closeQuestionLettersTx closes the task.question letters askers sent
// recipient about task, once their questions are answered on the task, and
// marks the question notifications not sent yet as sent, so an answered
// question never arrives as an open letter.
func closeQuestionLettersTx(ctx context.Context, tx *sql.Tx, task Task, recipient string, askers []string, note, now string) error {
	if len(askers) == 0 {
		return nil
	}
	at, err := store.ParseTime(now)
	if err != nil {
		at = time.Now()
	}
	if _, err := mail.CloseTaskQuestionsTx(tx, recipient, task.Key, askers, note, at); err != nil {
		return err
	}
	for _, asker := range askers {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_notification_outbox SET published_at = ?, last_error = 'answered before delivery'
			WHERE published_at = '' AND message_type = 'task.question' AND channel = ?
			AND json_extract(data, '$.task_key') = ? AND json_extract(data, '$.from') = ?`,
			now, recipient, task.Key, asker); err != nil {
			return err
		}
	}
	return nil
}
