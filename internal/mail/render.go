package mail

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// defaultBatch is the RenderBatch size when the caller gives none.
const defaultBatch = 100

// RenderBatch is the prompt drain of agent: in one transaction it reads the
// oldest open letters, requests first and notices after them, adds one to the
// attempts of each letter it returns, and stops at n letters. A letter whose
// render would be its sixth is marked dead with reason attempts instead, and
// a letter whose data does not decode is marked dead with reason decode_error;
// neither counts toward n. Letters held back by n are not touched.
func (m *Mail) RenderBatch(agent string, n int) ([]Letter, error) {
	return m.drain(agentPrincipal(agent), "", n)
}

// RenderIteration is RenderBatch for the prompt of iteration: a rendered
// instant letter a hook has not delivered becomes read in iteration.
func (m *Mail) RenderIteration(agent, iteration string, n int) ([]Letter, error) {
	return m.drain(agentPrincipal(agent), iteration, n)
}

// DrainBatch is RenderBatch for any principal: the plugin host drains the
// inbox of plugin:<name> with the same attempt and dead-letter rules.
func (m *Mail) DrainBatch(principal string, n int) ([]Letter, error) {
	return m.drain(principal, "", n)
}

func (m *Mail) drain(principal, iteration string, n int) ([]Letter, error) {
	if n <= 0 {
		n = defaultBatch
	}
	now := formatNow(m.now())
	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT `+letterColumns+` FROM mail WHERE recipient = ? AND `+openSQL+`
		ORDER BY needs_reply DESC, created_at, seq`, principal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Letter{}
	var dead []Letter
	for len(out) < n && rows.Next() {
		l, err := scanLetter(rows)
		switch {
		case errors.Is(err, errDecode):
			l.DeadReason = DeadDecodeError
			dead = append(dead, l)
		case err != nil:
			return nil, err
		case l.Attempts+1 > maxAttempts:
			l.DeadReason = DeadAttempts
			dead = append(dead, l)
		default:
			l.Attempts++
			out = append(out, l)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Close the cursor before writing on the same transaction.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var w written
	for i, l := range out {
		if _, err := tx.Exec(`UPDATE mail SET attempts = attempts + 1 WHERE id = ?`, l.ID); err != nil {
			return nil, fmt.Errorf("render: count attempt of %s: %w", l.ID, err)
		}
		if !l.Instant || l.InstantState == InstantDelivered {
			continue
		}
		if _, err := tx.Exec(`UPDATE mail SET instant_state = ?, instant_iteration = ?, instant_at = ? WHERE id = ?`,
			InstantRead, iteration, now, l.ID); err != nil {
			return nil, fmt.Errorf("render: read %s: %w", l.ID, err)
		}
		out[i].InstantState, out[i].InstantIteration, out[i].InstantAt = InstantRead, iteration, now
		w.hints = append(w.hints, out[i])
	}
	for _, l := range dead {
		if err := markDead(tx, l.ID, l.DeadReason, now); err != nil {
			return nil, fmt.Errorf("render: dead-letter %s: %w", l.ID, err)
		}
		w.audits = append(w.audits, letterAudit("mail_dead", l, map[string]any{"reason": l.DeadReason}))
		if l.Instant {
			w.hints = append(w.hints, l)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	m.emit(w)
	return out, nil
}

// CloseRead closes the notices among ids that are still open letters to
// agent, with close note read. The runner calls it when the iteration that
// rendered ids finishes successfully. Requests stay open.
func (m *Mail) CloseRead(agent string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	now := formatNow(m.now())
	args := []any{now, now, CloseNoteRead, agentPrincipal(agent)}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := m.db.Exec(`UPDATE mail SET answered_at = ?, read_at = CASE WHEN read_at = '' THEN ? ELSE read_at END,
		close_note = ? WHERE recipient = ? AND needs_reply = 0 AND `+openSQL+`
		AND id IN (`+inPlaceholders(len(ids))+`)`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// OneLine is s with every run of line breaks and other control characters
// replaced by one space, trimmed: a subject is one line in the prompt.
func OneLine(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
	})
	return strings.TrimSpace(strings.Join(fields, " "))
}
