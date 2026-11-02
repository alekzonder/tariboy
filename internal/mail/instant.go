package mail

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// Instant delivery states (Letter.InstantState).
const (
	// InstantWaiting: the recipient's iteration runs and waits for a tool call.
	InstantWaiting = "waiting"
	// InstantDelivered: a hook showed the letter to the running iteration.
	InstantDelivered = "delivered"
	// InstantQueued: no iteration ran; the next one reads the letter.
	InstantQueued = "queued"
	// InstantStopped: the agent does not start iterations on its own.
	InstantStopped = "stopped"
	// InstantRead: the letter was rendered into an iteration prompt.
	InstantRead = "read"
	// InstantNotDelivered: the letter went dead before delivery.
	InstantNotDelivered = "not_delivered"
)

// instantPlugin is the one plugin that may send instant letters.
const instantPlugin = "plugin:telegram"

// InstantDirectory is the optional part of a Directory that answers who may
// send an agent an instant letter. Without it no agent may.
type InstantDirectory interface {
	// InstantSenders is the agent's mail_instant_senders; set is false when
	// the setting is not set.
	InstantSenders(agent string) (senders []string, set bool, err error)
	// GroupLead is the lead agent of the agent's group, or "".
	GroupLead(agent string) (string, error)
}

// InstantRuntime is the loop state an instant letter needs. Without it an
// instant letter is queued and marks nothing.
type InstantRuntime interface {
	// InstantState is the agent's running iteration, or "", and whether the
	// agent starts iterations on its own.
	InstantState(agent string) (iteration string, iterates bool)
	// MarkInstant creates the flag file of a running iteration.
	MarkInstant(agent, iteration string)
}

// SetInstantRuntime wires the loop state of instant letters.
func (m *Mail) SetInstantRuntime(rt InstantRuntime) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.instant = rt
}

func (m *Mail) instantRuntime() InstantRuntime {
	m.hookMu.RLock()
	defer m.hookMu.RUnlock()
	return m.instant
}

func errInstantNotAllowed() error {
	return newError(http.StatusForbidden, "instant_not_allowed", "instant delivery is not allowed from this sender")
}

// checkInstant refuses an instant letter from a sender that may not set the
// flag, or to a recipient that is not an agent. The customer and the Telegram
// plugin may always set it; an agent only when every open recipient allows it.
func (m *Mail) checkInstant(from string, targets []target) error {
	isAgent := strings.HasPrefix(from, "agent:")
	if !isAgent && from != "user:"+m.dir.CustomerLogin() && from != instantPlugin {
		return errInstantNotAllowed()
	}
	for _, t := range targets {
		if !strings.HasPrefix(t.principal, "agent:") {
			return newError(http.StatusBadRequest, "instant_recipient",
				"instant delivery is only for agent recipients; %s is not an agent", t.principal)
		}
	}
	if !isAgent {
		return nil
	}
	dir, ok := m.dir.(InstantDirectory)
	if !ok {
		return errInstantNotAllowed()
	}
	for _, t := range targets {
		if t.closed {
			continue
		}
		allowed, err := instantAllowed(dir, from, strings.TrimPrefix(t.principal, "agent:"))
		if err != nil {
			return err
		}
		if !allowed {
			return errInstantNotAllowed()
		}
	}
	return nil
}

// instantAllowed reports whether agent allows from: its mail_instant_senders
// when set, otherwise the lead of its group unless that is the agent itself.
func instantAllowed(dir InstantDirectory, from, agent string) (bool, error) {
	senders, set, err := dir.InstantSenders(agent)
	if err != nil || set {
		return slices.Contains(senders, from), err
	}
	lead, err := dir.GroupLead(agent)
	return lead != "" && lead != agent && agentPrincipal(lead) == from, err
}

// instantTargets sets the state an instant letter starts in for each target.
func (m *Mail) instantTargets(targets []target) {
	for i := range targets {
		targets[i].instantState, targets[i].iteration = m.arrivalState(targets[i].principal)
	}
}

// arrivalState is the state of an instant letter written to principal now,
// and the running iteration when it waits for one.
func (m *Mail) arrivalState(principal string) (string, string) {
	rt := m.instantRuntime()
	if rt == nil {
		return InstantQueued, ""
	}
	iteration, iterates := rt.InstantState(strings.TrimPrefix(principal, "agent:"))
	switch {
	case iteration != "":
		return InstantWaiting, iteration
	case iterates:
		return InstantQueued, ""
	}
	return InstantStopped, ""
}

// TakeInstant is the tool-call drain of agent in iteration: in one
// transaction it reads the open instant letters a hook has not shown and no
// prompt has rendered, requests first, adds one to their attempts and marks
// them delivered in iteration. A letter whose take would be its sixth attempt
// is marked dead with reason attempts instead.
func (m *Mail) TakeInstant(agent, iteration string) ([]Letter, error) {
	principal := agentPrincipal(agent)
	now := formatNow(m.now())
	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT `+letterColumns+` FROM mail WHERE recipient = ? AND instant = 1
		AND instant_state IN (?, ?, ?) AND `+openSQL+` ORDER BY needs_reply DESC, created_at, seq`,
		principal, InstantWaiting, InstantQueued, InstantStopped)
	if err != nil {
		return nil, err
	}
	letters, err := scanLetters(rows)
	if err != nil {
		return nil, err
	}
	out := []Letter{}
	var w written
	name := strings.TrimPrefix(principal, "agent:")
	for _, l := range letters {
		if l.Attempts+1 > maxAttempts {
			if err := markDead(tx, l.ID, DeadAttempts, now); err != nil {
				return nil, fmt.Errorf("instant: dead-letter %s: %w", l.ID, err)
			}
			w.hints = append(w.hints, l)
			w.audits = append(w.audits, letterAudit("mail_dead", l, map[string]any{"reason": DeadAttempts}))
			continue
		}
		if _, err := tx.Exec(`UPDATE mail SET attempts = attempts + 1, instant_state = ?, instant_iteration = ?,
			instant_at = ? WHERE id = ?`, InstantDelivered, iteration, now, l.ID); err != nil {
			return nil, fmt.Errorf("instant: deliver %s: %w", l.ID, err)
		}
		l.Attempts++
		l.InstantState, l.InstantIteration, l.InstantAt = InstantDelivered, iteration, now
		out = append(out, l)
		w.hints = append(w.hints, l)
		w.audits = append(w.audits, auditEvent{agent: name, kind: "mail_instant_shown",
			data: map[string]any{"id": l.ID, "from": l.From, "to": l.To, "kind": l.Kind, "iteration": iteration}})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	m.emit(w)
	return out, nil
}

// InstantDeliveredIDs is the open letters to agent a hook delivered in
// iteration.
func (m *Mail) InstantDeliveredIDs(agent, iteration string) ([]string, error) {
	rows, err := m.db.Query(`SELECT id FROM mail WHERE recipient = ? AND instant = 1 AND instant_state = ?
		AND instant_iteration = ? AND `+openSQL+` ORDER BY created_at, seq`,
		agentPrincipal(agent), InstantDelivered, iteration)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
