// Package tasknotify sends native Task notification outbox rows as letters
// from the principal that caused them.
package tasknotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tidy"
)

// Sender is the slice of the mail service the publisher needs.
type Sender interface {
	Send(mail.Letter) ([]mail.Letter, error)
}

// SenderTasks sends a notification whose causing principal is not known or
// can no longer send, such as a deleted agent.
const SenderTasks = "system:tasks"

type Publisher struct {
	db    *sql.DB
	mail  Sender
	clock func() time.Time
	log   *slog.Logger
}

func New(db *sql.DB, sender Sender, clock func() time.Time, log *slog.Logger) *Publisher {
	if clock == nil {
		clock = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Publisher{db: db, mail: sender, clock: clock, log: log}
}

type outboxRow struct {
	id      string
	channel string
	typ     string
	subject string
	text    string
	data    string
	attempt int
}

func (p *Publisher) Flush(ctx context.Context) error {
	now := p.clock().UTC()
	rows, err := p.db.QueryContext(ctx, `
		SELECT notification_id, channel, message_type, subject, text, data, attempts
		FROM task_notification_outbox
		WHERE published_at = '' AND next_attempt_at <= ?
		ORDER BY event_sequence, notification_id
		LIMIT 100`, store.FormatTime(now))
	if err != nil {
		return err
	}
	var pending []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.id, &row.channel, &row.typ, &row.subject,
			&row.text, &row.data, &row.attempt); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, row := range pending {
		data := map[string]any{}
		tidy.Log(p.log, "notification data is not valid JSON", json.Unmarshal([]byte(row.data), &data), "row", row.id)
		letter := notificationLetter(row, data)
		letters, publishErr := p.mail.Send(letter)
		if mail.ErrorCode(publishErr) == "unknown_sender" {
			letter.From = SenderTasks
			letters, publishErr = p.mail.Send(letter)
		}
		if mail.ErrorCode(publishErr) == "unknown_recipient" {
			// The recipient is gone (an agent deleted after the enqueue): a
			// retry cannot succeed, so the row is closed as failed.
			if _, err := p.db.ExecContext(ctx, `
				UPDATE task_notification_outbox
				SET attempts = attempts + 1, published_at = ?, last_error = ?
				WHERE notification_id = ? AND published_at = ''`,
				store.FormatTime(now), publishErr.Error(), row.id); err != nil {
				return err
			}
			p.log.Warn("task notification dropped", "notification", row.id, "to", row.channel, "err", publishErr)
			continue
		}
		if publishErr != nil {
			attempts := row.attempt + 1
			delay := time.Second * time.Duration(1<<min(attempts, 8))
			next := store.FormatTime(now.Add(delay))
			if _, err := p.db.ExecContext(ctx, `
				UPDATE task_notification_outbox
				SET attempts = ?, next_attempt_at = ?, last_error = ?
				WHERE notification_id = ? AND published_at = ''`,
				attempts, next, publishErr.Error(), row.id); err != nil {
				return err
			}
			p.log.Warn("task notification send failed",
				"notification", row.id, "attempts", attempts, "err", publishErr)
			continue
		}
		if _, err := p.db.ExecContext(ctx, `
			UPDATE task_notification_outbox
			SET attempts = attempts + 1, published_message = ?, published_at = ?,
			    last_error = ''
			WHERE notification_id = ? AND published_at = ''`,
			letters[0].ID, store.FormatTime(now), row.id); err != nil {
			return err
		}
	}
	return nil
}

// notificationLetter is the letter of one outbox row: from the causing
// principal to the notified one. A task.question is a request, answered on
// the task; every other notification is a notice.
func notificationLetter(row outboxRow, data map[string]any) mail.Letter {
	from, _ := data["from"].(string)
	if from == "" {
		from = SenderTasks
	}
	key, _ := data["task_key"].(string)
	if title, _ := data["task_title"].(string); title != "" {
		key += ": " + title
	}
	return mail.Letter{
		IdempotencyKey: row.id,
		From:           from,
		To:             row.channel,
		Kind:           row.typ,
		Subject:        notificationSubject(row.typ, key),
		Body:           row.text,
		Data:           data,
		NeedsReply:     row.typ == mail.KindTaskQuestion,
	}
}

func notificationSubject(kind, key string) string {
	switch kind {
	case "task.assigned":
		return "Assigned " + key
	case mail.KindTaskQuestion:
		return "Question on " + key
	case "task.answered":
		return "Answer on " + key
	case "task.triage":
		return "Triage " + key
	}
	return kind + " " + key
}

func (p *Publisher) Run(ctx context.Context) {
	tidy.Log(p.log, "task notification flush failed", p.Flush(ctx))
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Flush(ctx); err != nil {
				p.log.Warn("task notification outbox flush failed", "err", err)
			}
		}
	}
}
