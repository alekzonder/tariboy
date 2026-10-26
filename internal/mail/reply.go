package mail

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

// Reply writes the answer to letter r.ID from its recipient to its sender:
// subject "Re: <subject>", in_reply_to the letter, a notice unless r.Ask. In
// the same transaction it closes the letter with answer_id and cancels the
// letter's deadline. A reply to a closed letter is recorded and reopens
// nothing; closed reports whether this reply closed the letter. A reply to a
// task.question is refused with answer_on_task: the answer belongs on the task.
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
		Kind: KindMessage, NeedsReply: r.Ask, InReplyTo: orig.ID, Data: map[string]any{}}
	if r.OnBehalf {
		reply.Data["on_behalf"] = "operator"
	}
	now := m.now()
	targets, err := m.resolve(reply.From, reply.To)
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
// the dead and closed fields are cleared.
func (m *Mail) Requeue(as, id string) (Letter, error) {
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
	if _, err := tx.Exec(`UPDATE mail SET dead_at = '', dead_reason = '', attempts = 0, answered_at = '',
		answer_id = '' WHERE id = ?`, id); err != nil {
		return Letter{}, err
	}
	if err := tx.Commit(); err != nil {
		return Letter{}, err
	}
	reason := l.DeadReason
	l.DeadAt, l.DeadReason, l.Attempts, l.AnsweredAt, l.AnswerID = "", "", 0, "", ""
	m.emit(written{letters: []Letter{l}, audits: []auditEvent{letterAudit("mail_requeued", l, map[string]any{"reason": reason})}})
	return l, nil
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
// task.question letters to recipient about taskKey with note as close note.
// Native Tasks calls it when the question's wait resolves.
func (m *Mail) CloseTaskQuestionsTx(tx *sql.Tx, recipient, taskKey, note string) (int, error) {
	res, err := tx.Exec(`UPDATE mail SET answered_at = ?, close_note = ?
		WHERE recipient = ? AND kind = ? AND CASE WHEN json_valid(data) THEN json_extract(data, '$.task_key') END = ? AND `+openSQL,
		formatNow(m.now()), note, recipient, KindTaskQuestion, taskKey)
	if err != nil {
		return 0, fmt.Errorf("close task questions of %s: %w", taskKey, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
