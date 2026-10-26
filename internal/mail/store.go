package mail

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// dbtx is the subset of *sql.DB and *sql.Tx the helpers use.
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

const letterColumns = `seq, id, sender, recipient, subject, body, kind, data, needs_reply, in_reply_to,
	deadline, COALESCE(idempotency_key, ''), produced_in_iteration, created_at, read_at, answered_at,
	answer_id, close_note, attempts, dead_at, dead_reason`

// openSQL is the inbox condition: a letter is open until it is closed or dead.
const openSQL = `answered_at = '' AND dead_at = ''`

// errDecode marks a row whose data column is not a JSON object.
var errDecode = errors.New("mail data does not decode")

type scanner interface{ Scan(dest ...any) error }

// scanLetter reads one row of letterColumns. A row whose data does not decode
// is returned with errDecode wrapped, and with every other field set.
func scanLetter(row scanner) (Letter, error) {
	var l Letter
	var data string
	var needsReply int
	if err := row.Scan(&l.Seq, &l.ID, &l.From, &l.To, &l.Subject, &l.Body, &l.Kind, &data, &needsReply,
		&l.InReplyTo, &l.Deadline, &l.IdempotencyKey, &l.ProducedInIteration, &l.CreatedAt, &l.ReadAt,
		&l.AnsweredAt, &l.AnswerID, &l.CloseNote, &l.Attempts, &l.DeadAt, &l.DeadReason); err != nil {
		return Letter{}, err
	}
	l.NeedsReply = needsReply != 0
	if err := json.Unmarshal([]byte(data), &l.Data); err != nil {
		l.Data = map[string]any{}
		return l, fmt.Errorf("%w: letter %s: %w", errDecode, l.ID, err)
	}
	if l.Data == nil {
		l.Data = map[string]any{}
	}
	return l, nil
}

func scanLetters(rows *sql.Rows) ([]Letter, error) {
	defer rows.Close()
	out := []Letter{}
	for rows.Next() {
		l, err := scanLetter(rows)
		if err != nil && !errors.Is(err, errDecode) {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// getLetter reads one letter by id; errNotFound when it does not exist.
func getLetter(x dbtx, id string) (Letter, error) {
	l, err := scanLetter(x.QueryRow(`SELECT `+letterColumns+` FROM mail WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Letter{}, errNotFound(id)
	}
	if err != nil && !errors.Is(err, errDecode) {
		return Letter{}, err
	}
	return l, nil
}

// byIdempotency returns the letter already written for (key, recipient).
func byIdempotency(x dbtx, key, recipient string) (Letter, bool, error) {
	l, err := scanLetter(x.QueryRow(`SELECT `+letterColumns+` FROM mail
		WHERE idempotency_key = ? AND recipient = ?`, key, recipient))
	if errors.Is(err, sql.ErrNoRows) {
		return Letter{}, false, nil
	}
	if err != nil && !errors.Is(err, errDecode) {
		return Letter{}, false, err
	}
	return l, true, nil
}

// letterID is the public handle m-<UTC yyyymmddThhmmssZ>-<seq, 6 digits>.
func letterID(now time.Time, seq int64) string {
	return fmt.Sprintf("m-%s-%06d", now.UTC().Format("20060102T150405Z"), seq)
}

// nextSeq is the seq the next inserted row gets. AUTOINCREMENT never reuses a
// value, so sqlite_sequence holds the highest seq ever written.
func nextSeq(x dbtx) (int64, error) {
	var seq int64
	err := x.QueryRow(`SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'mail'), 0) + 1`).Scan(&seq)
	return seq, err
}

// insertLetter assigns the seq and the id and writes l. l.CreatedAt must be set.
func insertLetter(x dbtx, l Letter, now time.Time) (Letter, error) {
	seq, err := nextSeq(x)
	if err != nil {
		return Letter{}, err
	}
	l.Seq, l.ID = seq, letterID(now, seq)
	if l.Data == nil {
		l.Data = map[string]any{}
	}
	data, err := json.Marshal(l.Data)
	if err != nil {
		return Letter{}, errInvalid("letter data: %v", err)
	}
	var key any
	if l.IdempotencyKey != "" {
		key = l.IdempotencyKey
	}
	_, err = x.Exec(`INSERT INTO mail(seq, id, sender, recipient, subject, body, kind, data, needs_reply,
		in_reply_to, deadline, idempotency_key, produced_in_iteration, created_at, read_at, answered_at,
		answer_id, close_note, attempts, dead_at, dead_reason)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.Seq, l.ID, l.From, l.To, l.Subject, l.Body, l.Kind, string(data), boolInt(l.NeedsReply),
		l.InReplyTo, l.Deadline, key, l.ProducedInIteration, l.CreatedAt, l.ReadAt, l.AnsweredAt,
		l.AnswerID, l.CloseNote, l.Attempts, l.DeadAt, l.DeadReason)
	if err != nil {
		return Letter{}, err
	}
	return l, nil
}

func openCount(x dbtx, recipient string) (int, error) {
	var n int
	err := x.QueryRow(`SELECT COUNT(*) FROM mail WHERE recipient = ? AND `+openSQL, recipient).Scan(&n)
	return n, err
}

// markDead closes l as dead with reason, in the caller's transaction.
func markDead(x dbtx, id, reason, at string) error {
	_, err := x.Exec(`UPDATE mail SET dead_at = ?, dead_reason = ?, answered_at = ? WHERE id = ?`,
		at, reason, at, id)
	return err
}

// inPlaceholders returns "?,?,..." for n values.
func inPlaceholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// OpenCount is the number of open letters to agent (a name or agent:<name>).
func (m *Mail) OpenCount(agent string) (int, error) {
	return openCount(m.db, agentPrincipal(agent))
}

// OpenAgentTotal is the number of open letters to every agent (the queue
// depth gauge).
func (m *Mail) OpenAgentTotal() (int, error) {
	var n int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM mail WHERE recipient GLOB 'agent:*' AND ` + openSQL).Scan(&n)
	return n, err
}

// List returns one mailbox view of as. The inbox is oldest first; sent,
// archive and all are newest first. before is the opaque cursor a full page
// returns as next ("<created_at>/<seq>" of its last letter): the next page
// continues after that letter in the view's order, or "" after the last
// page. A bare created_at is still accepted and lists the letters created
// before it.
func (m *Mail) List(box, as string, limit int, before string) ([]Letter, string, error) {
	if limit <= 0 {
		limit = 100
	}
	var where, order string
	switch box {
	case BoxInbox:
		where, order = `recipient = ? AND `+openSQL, `created_at, seq`
	case BoxSent:
		where, order = `sender = ?`, `created_at DESC, seq DESC`
	case BoxArchive:
		where, order = `recipient = ? AND NOT (`+openSQL+`)`, `created_at DESC, seq DESC`
	case BoxAll:
		where, order = `recipient = ?`, `created_at DESC, seq DESC`
	default:
		return nil, "", errInvalid("box must be inbox, sent or archive, got %q", box)
	}
	args := []any{as}
	if before != "" {
		cond, condArgs, err := cursorWhere(before, box == BoxInbox)
		if err != nil {
			return nil, "", err
		}
		where += ` AND ` + cond
		args = append(args, condArgs...)
	}
	args = append(args, limit)
	rows, err := m.db.Query(`SELECT `+letterColumns+` FROM mail WHERE `+where+` ORDER BY `+order+` LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	letters, err := scanLetters(rows)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(letters) == limit {
		last := letters[len(letters)-1]
		next = fmt.Sprintf("%s/%d", last.CreatedAt, last.Seq)
	}
	return letters, next, nil
}

// cursorWhere is the condition of the page after cursor: the letters after
// (created_at, seq) in ascending order, or before it in descending order.
func cursorWhere(cursor string, ascending bool) (string, []any, error) {
	at, seqText, ok := strings.Cut(cursor, "/")
	if !ok {
		return `created_at < ?`, []any{cursor}, nil
	}
	seq, err := strconv.ParseInt(seqText, 10, 64)
	if err != nil || at == "" {
		return "", nil, errInvalid("before %q is not a cursor from next_before", cursor)
	}
	op := "<"
	if ascending {
		op = ">"
	}
	return `(created_at ` + op + ` ? OR (created_at = ? AND seq ` + op + ` ?))`, []any{at, at, seq}, nil
}

// Show returns one letter and its reply, if any. as must be its sender or its
// recipient.
func (m *Mail) Show(as, id string) (Letter, *Letter, error) {
	l, err := getLetter(m.db, id)
	if err != nil {
		return Letter{}, nil, err
	}
	if as != l.From && as != l.To {
		return Letter{}, nil, newError(http.StatusForbidden, "forbidden", "%s is neither the sender nor the recipient of letter %s", as, id)
	}
	rows, err := m.db.Query(`SELECT `+letterColumns+` FROM mail WHERE in_reply_to = ?
		ORDER BY id = ? DESC, created_at, seq LIMIT 1`, id, l.AnswerID)
	if err != nil {
		return Letter{}, nil, err
	}
	replies, err := scanLetters(rows)
	if err != nil {
		return Letter{}, nil, err
	}
	if len(replies) == 0 {
		return l, nil, nil
	}
	return l, &replies[0], nil
}

// Unread counts the open letters of as.
func (m *Mail) Unread(as string) (MailUnread, error) {
	u := MailUnread{As: as, BySender: []SenderCount{}}
	if err := m.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(read_at = ''), 0) FROM mail
		WHERE recipient = ? AND `+openSQL, as).Scan(&u.Open, &u.Unread); err != nil {
		return MailUnread{}, err
	}
	rows, err := m.db.Query(`SELECT sender, kind, COUNT(*) FROM mail WHERE recipient = ? AND `+openSQL+`
		GROUP BY sender, kind ORDER BY sender, kind`, as)
	if err != nil {
		return MailUnread{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var c SenderCount
		if err := rows.Scan(&c.Sender, &c.Kind, &c.Count); err != nil {
			return MailUnread{}, err
		}
		u.BySender = append(u.BySender, c)
	}
	return u, rows.Err()
}

// Awaiting returns the requests agent sent that carry a deadline and are not
// answered yet, oldest first.
func (m *Mail) Awaiting(agent string) ([]Letter, error) {
	rows, err := m.db.Query(`SELECT `+letterColumns+` FROM mail
		WHERE sender = ? AND needs_reply = 1 AND deadline <> '' AND `+openSQL+`
		ORDER BY created_at, seq`, agentPrincipal(agent))
	if err != nil {
		return nil, err
	}
	return scanLetters(rows)
}

// formatNow is the canonical stored form of t.
func formatNow(t time.Time) string { return store.FormatTime(t) }
