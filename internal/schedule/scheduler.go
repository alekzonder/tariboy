package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/tidy"
)

// Sender is the slice of the mail service the scheduler needs: a send that
// writes only while guard, run in the same transaction, allows it.
type Sender interface {
	SendGuarded(mail.Letter, func(*sql.Tx) error) ([]mail.Letter, error)
}

// SenderSchedule is the sender of every letter a schedule fires.
const SenderSchedule = "system:schedule"

type Scheduler struct {
	store *Store
	pub   Sender
	log   *slog.Logger
	clock func() time.Time
	after func(time.Duration) <-chan time.Time
}

func NewScheduler(st *Store, pub Sender, log *slog.Logger,
	clock func() time.Time, after func(time.Duration) <-chan time.Time) *Scheduler {
	if clock == nil {
		clock = time.Now
	}
	if after == nil {
		after = time.After
	}
	return &Scheduler{store: st, pub: pub, log: log, clock: clock, after: after}
}

// Run fires due schedules once per second until ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.after(time.Second):
			if _, err := s.fireDue(s.clock()); err != nil {
				s.log.Error("scheduler tick", "err", err)
			}
		}
	}
}

// fireDue sends the letter of every schedule due at/before now and
// advances/disables each.
func (s *Scheduler) fireDue(now time.Time) (int, error) {
	due, err := s.store.DueBefore(now)
	if err != nil {
		return 0, err
	}
	fired := 0
	for _, sch := range due {
		if _, err := s.pub.SendGuarded(renderTemplate(sch), scheduleGuard(sch)); err != nil {
			if !errors.Is(err, mail.ErrGuardDenied) {
				s.log.Error("schedule send", "id", sch.ID, "err", err)
			}
			continue
		}
		fired++
		s.advance(&sch, now)
		if err := s.store.MarkFired(sch); err != nil {
			s.log.Error("schedule mark fired", "id", sch.ID, "err", err)
		}
	}
	return fired, nil
}

// scheduleGuard lets the letter through only while the schedule is still
// enabled at the firing it was read for, so a cancel racing the tick wins.
func scheduleGuard(sch Schedule) func(*sql.Tx) error {
	return func(tx *sql.Tx) error {
		var live int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM schedules WHERE id=? AND enabled=1 AND next_fire_at=?`, sch.ID, sch.NextFireAt).Scan(&live); err != nil {
			return err
		}
		if live == 0 {
			return mail.ErrGuardDenied
		}
		return nil
	}
}

// advance recomputes a cron schedule's next firing; a oneshot is disabled.
func (s *Scheduler) advance(sch *Schedule, now time.Time) {
	if sch.Kind == "oneshot" {
		sch.Enabled = false
		return
	}
	if next, ok := NextAfter(sch.Spec, now); ok {
		sch.NextFireAt = next
		sch.Enabled = true
	} else {
		sch.Enabled = false
	}
}

// renderTemplate builds the letter of one firing from the schedule's message
// template {kind, subject, text, data, notice}. The older template field type
// names the kind when kind is absent, and an object subject is kept in data.
func renderTemplate(sch Schedule) mail.Letter {
	var tpl struct {
		Kind    string          `json:"kind"`
		Type    string          `json:"type"`
		Subject json.RawMessage `json:"subject"`
		Text    string          `json:"text"`
		Data    map[string]any  `json:"data"`
		Notice  bool            `json:"notice"`
	}
	tidy.Log(nil, "schedule message template is not valid JSON; using defaults", json.Unmarshal([]byte(sch.MessageTemplate), &tpl), "schedule", sch.ID)
	kind := tpl.Kind
	if kind == "" {
		kind = tpl.Type
	}
	if kind == "" {
		kind = "schedule.fire"
	}
	if tpl.Data == nil {
		tpl.Data = map[string]any{}
	}
	tpl.Data["schedule_id"] = sch.ID
	subject := ""
	if len(tpl.Subject) > 0 && json.Unmarshal(tpl.Subject, &subject) != nil {
		var obj map[string]any
		if json.Unmarshal(tpl.Subject, &obj) == nil && obj != nil {
			tpl.Data["subject"] = obj
		}
	}
	if subject == "" {
		subject = fmt.Sprintf("Schedule %s", sch.ID)
	}
	return mail.Letter{
		From: SenderSchedule, To: sch.Recipient, Subject: subject, Body: tpl.Text, Kind: kind,
		Data: tpl.Data, NeedsReply: !tpl.Notice,
		IdempotencyKey: "schedule:" + sch.ID + ":" + sch.NextFireAt,
	}
}
