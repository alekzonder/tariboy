package mail

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Reply writes the answer to letter r.ID from its recipient to its sender:
// subject "Re: <subject>", in_reply_to the letter, a notice unless r.Ask. In
// the same transaction it closes the letter with answer_id and cancels the
// letter's deadline. A reply to a closed letter is recorded and reopens
// nothing; closed reports whether this reply closed the letter. A reply to a
// task.question is refused with answer_on_task: the answer belongs on the task.
// A reply to a sender that no longer exists is stored closed.
func (m *Mail) Reply(r ReplyRequest) (Letter, bool, error) {
	if strings.TrimSpace(r.Text) == "" {
		return Letter{}, false, errInvalid("reply text is required")
	}
	orig, err := getLetter(m.db, r.ID)
	if err != nil {
		return Letter{}, false, err
	}
	if orig.To != r.As {
		return Letter{}, false, errForbidden(r.As, r.ID)
	}
	if orig.Kind == KindTaskQuestion {
		key, _ := orig.Data["task_key"].(string)
		return Letter{}, false, newError(http.StatusConflict, "answer_on_task",
			"letter %s is a task question; answer it on the task: ttasks comment %s \"<answer>\"", r.ID, key)
	}
	reply := Letter{From: r.As, To: orig.From, Subject: replySubject(orig.Subject), Body: r.Text,
		Kind: KindMessage, NeedsReply: r.Ask, InReplyTo: orig.ID, Data: map[string]any{},
		ProducedInIteration: r.Iteration}
	if r.OnBehalf {
		reply.Data["on_behalf"] = "operator"
	}
	now := m.now()
	targets, err := m.resolve(reply.From, reply.To)
	if ErrorCode(err) == "unknown_recipient" {
		// The sender is gone (a deleted agent, an uninstalled plugin): the
		// reply is stored closed so the request can still be answered.
		targets, err = []target{{principal: reply.To, closed: true}}, nil
	}
	if err != nil {
		return Letter{}, false, err
	}
	tx, err := m.db.Begin()
	if err != nil {
		return Letter{}, false, err
	}
	defer tx.Rollback()
	out, w, err := m.writeTx(tx, reply, targets, now)
	if err != nil {
		return Letter{}, false, err
	}
	reply = out[0]
	res, err := tx.Exec(`UPDATE mail SET answered_at = ?, answer_id = ? WHERE id = ? AND `+openSQL,
		reply.CreatedAt, reply.ID, orig.ID)
	if err != nil {
		return Letter{}, false, err
	}
	n, _ := res.RowsAffected()
	closed := n > 0
	if closed {
		if err := m.cancelDeadline(tx, orig.ID); err != nil {
			return Letter{}, false, err
		}
		w.audits = append(w.audits, letterAudit("mail_answered", orig, map[string]any{"answer_id": reply.ID}))
	}
	if err := tx.Commit(); err != nil {
		return Letter{}, false, err
	}
	m.emit(w)
	return reply, closed, nil
}

func replySubject(subject string) string {
	if strings.HasPrefix(subject, "Re:") {
		return subject
	}
	return "Re: " + subject
}

func (m *Mail) cancelDeadline(tx *sql.Tx, id string) error {
	if d := m.deadlineSeam(); d != nil {
		return d.CancelTx(tx, id)
	}
	return nil
}

// Close archives letter id without a reply, with note as its close note. as
// must be its recipient. Closing a closed letter returns it unchanged.
func (m *Mail) Close(as, id, note string) (Letter, error) {
	tx, err := m.db.Begin()
	if err != nil {
		return Letter{}, err
	}
	defer tx.Rollback()
	l, err := recipientLetter(tx, as, id)
	if err != nil {
		return Letter{}, err
	}
	if !l.Open() {
		return l, nil
	}
	l.AnsweredAt, l.CloseNote = formatNow(m.now()), note
	if _, err := tx.Exec(`UPDATE mail SET answered_at = ?, close_note = ? WHERE id = ?`, l.AnsweredAt, note, id); err != nil {
		return Letter{}, err
	}
	if err := m.cancelDeadline(tx, id); err != nil {
		return Letter{}, err
	}
	if err := tx.Commit(); err != nil {
		return Letter{}, err
	}
	m.emit(written{audits: []auditEvent{letterAudit("mail_closed", l, map[string]any{"note": note})}})
	return l, nil
}

// recipientLetter reads letter id for its recipient as.
func recipientLetter(x dbtx, as, id string) (Letter, error) {
	l, err := getLetter(x, id)
	if err != nil {
		return Letter{}, err
	}
	if l.To != as {
		return Letter{}, errForbidden(as, id)
	}
	return l, nil
}

// MarkRead marks letters of as read and closes the open notices among them
// with close note read. read counts the letters read for the first time.
func (m *Mail) MarkRead(as string, ids []string) (closedNotices, read int, err error) {
	now := formatNow(m.now())
	tx, err := m.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	for _, id := range ids {
		l, err := recipientLetter(tx, as, id)
		if err != nil {
			return 0, 0, err
		}
		if l.ReadAt == "" {
			if _, err := tx.Exec(`UPDATE mail SET read_at = ? WHERE id = ?`, now, id); err != nil {
				return 0, 0, err
			}
			read++
		}
		if !l.NeedsReply && l.Open() {
			if _, err := tx.Exec(`UPDATE mail SET answered_at = ?, close_note = ? WHERE id = ?`, now, CloseNoteRead, id); err != nil {
				return 0, 0, err
			}
			closedNotices++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return closedNotices, read, nil
}

// Requeue returns a dead letter of as to the inbox: attempts start over and
// the dead and closed fields are cleared. It is refused with queue_limit while
// the agent's inbox is at its cap. A request's deadline is armed again when
// it is still ahead and cleared when it has passed.
func (m *Mail) Requeue(as, id string) (Letter, error) {
	limit := 0
	if name, ok := strings.CutPrefix(as, "agent:"); ok {
		var err error
		if limit, err = m.dir.InboxCap(name); err != nil {
			return Letter{}, err
		}
	}
	tx, err := m.db.Begin()
	if err != nil {
		return Letter{}, err
	}
	defer tx.Rollback()
	l, err := recipientLetter(tx, as, id)
	if err != nil {
		return Letter{}, err
	}
	if l.DeadAt == "" {
		return Letter{}, errInvalid("letter %s is not dead", id)
	}
	if limit > 0 {
		open, err := openCount(tx, as)
		if err != nil {
			return Letter{}, err
		}
		if open >= limit {
			return Letter{}, newError(http.StatusConflict, DeadQueueLimit,
				"inbox of %s is full (%d/%d open); close letters first", as, open, limit)
		}
	}
	reason := l.DeadReason
	l.DeadAt, l.DeadReason, l.Attempts, l.AnsweredAt, l.AnswerID = "", "", 0, "", ""
	if err := m.requeueDeadlineTx(tx, &l); err != nil {
		return Letter{}, err
	}
	if _, err := tx.Exec(`UPDATE mail SET dead_at = '', dead_reason = '', attempts = 0, answered_at = '',
		answer_id = '', deadline = ? WHERE id = ?`, l.Deadline, id); err != nil {
		return Letter{}, err
	}
	if err := tx.Commit(); err != nil {
		return Letter{}, err
	}
	m.emit(written{letters: []Letter{l}, audits: []auditEvent{letterAudit("mail_requeued", l, map[string]any{"reason": reason})}})
	return l, nil
}

// requeueDeadlineTx cancels the old deadline schedule of a requeued request
// and arms it again when the deadline is still ahead; a passed deadline is
// cleared, as it can no longer fire.
func (m *Mail) requeueDeadlineTx(tx *sql.Tx, l *Letter) error {
	if !l.NeedsReply || l.Deadline == "" {
		return nil
	}
	if err := m.cancelDeadline(tx, l.ID); err != nil {
		return err
	}
	at, err := store.ParseTime(l.Deadline)
	if err != nil || !at.After(m.now()) {
		l.Deadline = ""
		return nil
	}
	if d := m.deadlineSeam(); d != nil {
		return d.ArmTx(tx, *l)
	}
	return nil
}

// ClearInbox deletes the open letters to agent and returns how many.
func (m *Mail) ClearInbox(agent string) (int, error) {
	principal := agentPrincipal(agent)
	res, err := m.db.Exec(`DELETE FROM mail WHERE recipient = ? AND `+openSQL, principal)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	name := strings.TrimPrefix(principal, "agent:")
	m.emit(written{audits: []auditEvent{{agent: name, kind: "mail_cleared", data: map[string]any{"deleted": n}}}})
	return int(n), nil
}

// CloseTaskQuestionsTx closes, inside the caller's transaction, the open
// task.question letters to recipient about taskKey asked by one of askers,
// with note as close note, and returns how many it closed. Native Tasks calls
// it when the questions those askers asked recipient are answered on the task.
// The asker is data.from, or the sender when data has none: the outbox sends
// a question as system:tasks when its asker can no longer send.
func CloseTaskQuestionsTx(tx *sql.Tx, recipient, taskKey string, askers []string, note string, now time.Time) (int, error) {
	if len(askers) == 0 {
		return 0, nil
	}
	args := []any{store.FormatTime(now), note, recipient, KindTaskQuestion, taskKey}
	for _, asker := range askers {
		args = append(args, asker)
	}
	res, err := tx.Exec(`UPDATE mail SET answered_at = ?, close_note = ?
		WHERE recipient = ? AND kind = ? AND CASE WHEN json_valid(data) THEN json_extract(data, '$.task_key') END = ?
		AND COALESCE(NULLIF(CASE WHEN json_valid(data) THEN json_extract(data, '$.from') END, ''), sender)
		IN (`+inPlaceholders(len(askers))+`) AND `+openSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("close task questions of %s: %w", taskKey, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CloseTaskNoticesTx closes, inside the caller's transaction, the open
// letters of kinds to recipient about taskKey created at or before
// createdBefore, with note as close note, and returns how many it closed.
// Native Tasks calls it when an iteration that worked the task already saw
// what those letters announce.
func CloseTaskNoticesTx(tx *sql.Tx, recipient, taskKey string, kinds []string, createdBefore time.Time, note string, now time.Time) (int, error) {
	if len(kinds) == 0 {
		return 0, nil
	}
	args := []any{store.FormatTime(now), note, recipient, taskKey, store.FormatTime(createdBefore)}
	for _, kind := range kinds {
		args = append(args, kind)
	}
	res, err := tx.Exec(`UPDATE mail SET answered_at = ?, close_note = ?
		WHERE recipient = ? AND CASE WHEN json_valid(data) THEN json_extract(data, '$.task_key') END = ?
		AND created_at <= ? AND kind IN (`+inPlaceholders(len(kinds))+`) AND `+openSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("close task notices of %s: %w", taskKey, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
