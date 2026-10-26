package mail

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// target is one resolved recipient. cap is the inbox cap of an agent
// recipient (zero or less: none).
type target struct {
	principal string
	cap       int
}

// Send validates the sender and the recipients, expands groups and writes one
// letter per recipient in one transaction. A letter to a system:* or queue:*
// principal is stored closed; a letter to an agent whose open inbox is at its
// cap is stored dead with reason queue_limit. With an idempotency key, a
// recipient that already holds a letter with that key gets that letter back
// and nothing new. After the commit Send wakes the recipients of open letters
// and fires one hint and one audit event per written row.
func (m *Mail) Send(l Letter) ([]Letter, error) {
	return m.SendGuarded(l, nil)
}

// ErrGuardDenied is what a SendGuarded guard returns to drop the send without
// an error worth logging.
var ErrGuardDenied = errors.New("mail send guard denied")

// SendGuarded is Send with guard run first inside the write transaction: an
// error from guard writes nothing and is returned as is. The scheduler uses it
// so a schedule cancelled during its tick sends nothing.
func (m *Mail) SendGuarded(l Letter, guard func(*sql.Tx) error) ([]Letter, error) {
	now := m.now()
	l, err := normalize(l, now)
	if err != nil {
		return nil, err
	}
	if err := m.validateSender(l.From); err != nil {
		return nil, err
	}
	targets, err := m.resolve(l.From, l.To)
	if err != nil {
		return nil, err
	}
	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if guard != nil {
		if err := guard(tx); err != nil {
			return nil, err
		}
	}
	out, w, err := m.writeTx(tx, l, targets, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	m.emit(w)
	return out, nil
}

// ValidateRecipient checks that to (a comma list) names principals a letter
// from from can be sent to, without writing anything.
func (m *Mail) ValidateRecipient(from, to string) error {
	_, err := m.resolve(from, to)
	return err
}

// normalize fills the defaults of an outgoing letter and checks its fields.
func normalize(l Letter, now time.Time) (Letter, error) {
	l.From, l.To = strings.TrimSpace(l.From), strings.TrimSpace(l.To)
	if l.From == "" {
		return Letter{}, errInvalid("sender is required")
	}
	if l.To == "" {
		return Letter{}, errInvalid("recipient is required")
	}
	if l.Kind == "" {
		l.Kind = KindMessage
	}
	if l.Data == nil {
		l.Data = map[string]any{}
	}
	if l.Deadline != "" {
		deadline, err := normalizeDeadline(l.Deadline, now)
		if err != nil {
			return Letter{}, err
		}
		l.Deadline = deadline
	}
	l.ID, l.Seq, l.CreatedAt = "", 0, ""
	l.ReadAt, l.AnsweredAt, l.AnswerID, l.CloseNote = "", "", "", ""
	l.Attempts, l.DeadAt, l.DeadReason = 0, "", ""
	return l, nil
}

// normalizeDeadline turns a Go duration or an RFC 3339 time into the stored
// absolute time.
func normalizeDeadline(deadline string, now time.Time) (string, error) {
	if d, err := time.ParseDuration(deadline); err == nil {
		if d <= 0 {
			return "", errInvalid("deadline %q must be in the future", deadline)
		}
		return store.FormatTime(now.Add(d)), nil
	}
	t, err := store.ParseInputTime(deadline)
	if err != nil {
		return "", errInvalid("deadline %q is neither a duration such as 5m nor an RFC 3339 time", deadline)
	}
	return store.FormatTime(t), nil
}

// validateSender accepts an existing agent, the customer, an installed plugin
// or a system component.
func (m *Mail) validateSender(from string) error {
	ok, err := m.knownPrincipal(from)
	if err != nil {
		return err
	}
	if !ok {
		return errInvalid("unknown sender %q", from)
	}
	return nil
}

// knownPrincipal reports whether p is an agent, user, plugin or system
// principal the directory knows.
func (m *Mail) knownPrincipal(p string) (bool, error) {
	kind, name, ok := strings.Cut(p, ":")
	if !ok || name == "" {
		return false, nil
	}
	switch kind {
	case "agent":
		return m.dir.AgentExists(name)
	case "user":
		return name == m.dir.CustomerLogin(), nil
	case "plugin":
		return m.dir.PluginInstalled(name)
	case "system":
		return true, nil
	}
	return false, nil
}

// resolve splits the comma list to, validates each principal, expands groups
// without the sender and drops repeats, keeping the first position.
func (m *Mail) resolve(from, to string) ([]target, error) {
	var principals []string
	for _, p := range strings.Split(to, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		expanded, err := m.expand(from, p)
		if err != nil {
			return nil, err
		}
		principals = append(principals, expanded...)
	}
	if len(principals) == 0 {
		return nil, errInvalid("recipient is required")
	}
	seen := map[string]bool{}
	var out []target
	for _, p := range principals {
		if seen[p] {
			continue
		}
		seen[p] = true
		t := target{principal: p}
		if name, ok := strings.CutPrefix(p, "agent:"); ok {
			limit, err := m.dir.InboxCap(name)
			if err != nil {
				return nil, err
			}
			t.cap = limit
		}
		out = append(out, t)
	}
	return out, nil
}

// expand validates one recipient principal and returns the principals it
// addresses: itself, or the members of a group except the sender.
func (m *Mail) expand(from, p string) ([]string, error) {
	kind, name, _ := strings.Cut(p, ":")
	switch {
	case name == "":
		return nil, errUnknownRecipient(p)
	case kind == "queue":
		ok, err := m.dir.QueueExists(name)
		if err != nil || !ok {
			return nil, firstErr(err, errUnknownRecipient(p))
		}
		return []string{p}, nil
	case kind == "group":
		return m.expandGroup(from, p, name)
	}
	ok, err := m.knownPrincipal(p)
	if err != nil || !ok {
		return nil, firstErr(err, errUnknownRecipient(p))
	}
	return []string{p}, nil
}

func (m *Mail) expandGroup(from, p, group string) ([]string, error) {
	members, exists, err := m.dir.GroupMembers(group)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errUnknownRecipient(p)
	}
	var out []string
	for _, member := range members {
		if principal := agentPrincipal(member); principal != from {
			out = append(out, principal)
		}
	}
	if len(out) == 0 {
		return nil, newError(http.StatusBadRequest, "empty_group", "group %q has no member other than the sender", group)
	}
	return out, nil
}

func firstErr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

// writeTx writes l to every target inside tx and returns the letters and what
// the hooks announce after the commit.
func (m *Mail) writeTx(tx *sql.Tx, l Letter, targets []target, now time.Time) ([]Letter, written, error) {
	var out []Letter
	var w written
	created := store.FormatTime(now)
	deadlines := m.deadlineSeam()
	for _, t := range targets {
		if l.IdempotencyKey != "" {
			existing, found, err := byIdempotency(tx, l.IdempotencyKey, t.principal)
			if err != nil {
				return nil, written{}, err
			}
			if found {
				out = append(out, existing)
				continue
			}
		}
		row := l
		row.To, row.CreatedAt = t.principal, created
		if err := closeOnArrival(tx, &row, t); err != nil {
			return nil, written{}, err
		}
		row, err := insertLetter(tx, row, now)
		if err != nil {
			return nil, written{}, err
		}
		if deadlines != nil && row.Open() && row.NeedsReply && row.Deadline != "" {
			if err := deadlines.ArmTx(tx, row); err != nil {
				return nil, written{}, err
			}
		}
		out = append(out, row)
		w.letters = append(w.letters, row)
		w.audits = append(w.audits, letterAudit("mail_sent", row, nil))
		if row.DeadAt != "" {
			w.audits = append(w.audits, letterAudit("mail_dead", row, map[string]any{"reason": row.DeadReason}))
		}
	}
	return out, w, nil
}

// closeOnArrival stores a letter nobody reads closed and a letter over the
// agent's inbox cap dead.
func closeOnArrival(tx *sql.Tx, row *Letter, t target) error {
	if strings.HasPrefix(t.principal, "system:") || strings.HasPrefix(t.principal, "queue:") {
		row.AnsweredAt = row.CreatedAt
		return nil
	}
	if t.cap <= 0 {
		return nil
	}
	open, err := openCount(tx, t.principal)
	if err != nil {
		return err
	}
	if open >= t.cap {
		row.DeadAt, row.DeadReason, row.AnsweredAt = row.CreatedAt, DeadQueueLimit, row.CreatedAt
	}
	return nil
}
