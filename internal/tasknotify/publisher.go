// Package tasknotify publishes native Task notification outbox rows through
// the existing durable channel Bus.
package tasknotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tidy"
)

type MessagePublisher interface {
	Publish(bus.Message) (bus.Message, error)
}

type Publisher struct {
	db    *sql.DB
	bus   MessagePublisher
	clock func() time.Time
	log   *slog.Logger
}

func New(db *sql.DB, messageBus MessagePublisher, clock func() time.Time, log *slog.Logger) *Publisher {
	if clock == nil {
		clock = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Publisher{db: db, bus: messageBus, clock: clock, log: log}
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
		subject := map[string]any{}
		data := map[string]any{}
		tidy.Log(p.log, "notification subject is not valid JSON", json.Unmarshal([]byte(row.subject), &subject), "row", row.id)
		tidy.Log(p.log, "notification data is not valid JSON", json.Unmarshal([]byte(row.data), &data), "row", row.id)
		message, publishErr := p.bus.Publish(bus.Message{
			IdempotencyKey: row.id,
			Channel:        row.channel,
			Source:         "system:tasks",
			Type:           row.typ,
			Subject:        subject,
			Text:           row.text,
			Data:           data,
		})
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
			p.log.Warn("task notification publish failed",
				"notification", row.id, "attempts", attempts, "err", publishErr)
			continue
		}
		if _, err := p.db.ExecContext(ctx, `
			UPDATE task_notification_outbox
			SET attempts = attempts + 1, published_message = ?, published_at = ?,
			    last_error = ''
			WHERE notification_id = ? AND published_at = ''`,
			message.ID, store.FormatTime(now), row.id); err != nil {
			return err
		}
	}
	return nil
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
