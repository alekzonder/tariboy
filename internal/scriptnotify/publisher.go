// Package scriptnotify sends durable local-script result intents to the
// agent's mailbox, as letters from system:scripts.
package scriptnotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
)

// Sender is the slice of the mail service the publisher needs.
type Sender interface {
	Send(mail.Letter) ([]mail.Letter, error)
}

// SenderScripts is the sender of every script result letter.
const SenderScripts = "system:scripts"

type Publisher struct {
	db    *sql.DB
	mail  Sender
	clock func() time.Time
	log   *slog.Logger
	wake  chan struct{}
}

type outboxRow struct {
	idempotencyKey string
	runID          string
	agent          string
	payload        string
	attempts       int
}

func New(db *sql.DB, sender Sender, clock func() time.Time, log *slog.Logger) *Publisher {
	if clock == nil {
		clock = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Publisher{db: db, mail: sender, clock: clock, log: log, wake: make(chan struct{}, 1)}
}

func (p *Publisher) Wake() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *Publisher) Flush(ctx context.Context) error {
	now := p.clock().UTC()
	rows, err := p.db.QueryContext(ctx, `
		SELECT idempotency_key,run_id,agent,payload,attempts
		FROM script_result_outbox
		WHERE published_at='' AND next_attempt_at<=?
		ORDER BY next_attempt_at,idempotency_key
		LIMIT 100`, store.FormatTime(now))
	if err != nil {
		return err
	}
	var pending []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.idempotencyKey, &row.runID, &row.agent, &row.payload, &row.attempts); err != nil {
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
		if err := json.Unmarshal([]byte(row.payload), &data); err != nil {
			return fmt.Errorf("decode script result %s: %w", row.runID, err)
		}
		// A request: the agent reports what it did with the result.
		letters, publishErr := p.mail.Send(mail.Letter{
			IdempotencyKey: row.idempotencyKey,
			From:           SenderScripts,
			To:             "agent:" + row.agent,
			Subject:        resultSubject(data),
			Kind:           "script.result",
			Body:           resultText(data),
			Data:           data,
			NeedsReply:     true,
		})
		if publishErr != nil {
			attempts := row.attempts + 1
			delay := time.Second * time.Duration(1<<min(attempts, 8))
			next := store.FormatTime(now.Add(delay))
			if _, err := p.db.ExecContext(ctx, `UPDATE script_result_outbox SET attempts=?,next_attempt_at=?,last_error=? WHERE idempotency_key=? AND published_at=''`, attempts, next, publishErr.Error(), row.idempotencyKey); err != nil {
				return err
			}
			p.log.Warn("script result send failed", "run", row.runID, "attempts", attempts, "err", publishErr)
			continue
		}
		if _, err := p.db.ExecContext(ctx, `UPDATE script_result_outbox SET attempts=attempts+1,published_at=?,message_id=?,last_error='' WHERE idempotency_key=? AND published_at=''`, store.FormatTime(now), letters[0].ID, row.idempotencyKey); err != nil {
			return err
		}
	}
	return nil
}

func resultSubject(data map[string]any) string {
	name, _ := data["name"].(string)
	status, _ := data["status"].(string)
	return fmt.Sprintf("Script %s %s", name, status)
}

func resultText(data map[string]any) string {
	name, _ := data["name"].(string)
	status, _ := data["status"].(string)
	runID, _ := data["run_id"].(string)
	logPath, _ := data["log_path"].(string)
	if exit, ok := data["exit_code"].(float64); ok {
		return fmt.Sprintf("Script %q run %s finished %s with exit code %d. Log path: %s", name, runID, status, int(exit), logPath)
	}
	return fmt.Sprintf("Script %q run %s finished %s. Log path: %s", name, runID, status, logPath)
}

func (p *Publisher) Run(ctx context.Context) {
	if err := p.Flush(ctx); err != nil {
		p.log.Warn("script result outbox initial flush failed", "err", err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-ticker.C:
		}
		if err := p.Flush(ctx); err != nil {
			p.log.Warn("script result outbox flush failed", "err", err)
		}
	}
}
