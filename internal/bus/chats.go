package bus

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Chat is a conversation as an entity: an identity, a kind, a title and exactly
// one transport channel it owns. Everything durable about delivery stays in
// messages/subscriptions/deliveries — a chat only decides who takes part and
// which channel carries it.
type Chat struct {
	ID          string
	Kind        string
	Title       string
	Channel     string
	LegacyAgent string
	CreatedAt   string
	ArchivedAt  string
}

// Participant is one principal in a chat: a user (membership plus a read mark)
// or an agent (membership that materializes a subscription to the chat channel,
// which is what keeps the wake path untouched).
type Participant struct {
	ChatID    string
	Principal string
	Role      string
	JoinedAt  string
	ReadTS    string
	Muted     bool
}

// The kinds reconciliation provisions per agent, in the order a chat list shows
// them.
const (
	ChatKindDirect  = "direct"
	ChatKindTasks   = "tasks"
	ChatKindService = "service"
)

// legacyReadKey is the pre-chats read mark: one JSON object in daemon_config
// mapping an agent name to the newest timestamp the customer had seen. It is
// read once per reconciliation and folded into chat_participants.read_ts.
const legacyReadKey = "chat_read_v1"

// ChatChannelFor is the one transport channel a chat owns. It is deliberately
// never an agent inbox: the bus suppresses an author's own delivery only on
// NON-inbox channels, so an inbox-backed chat would echo an agent's own reply
// back into its own queue.
func ChatChannelFor(id string) string { return "chat:" + id }

// ChatIDFromChannel is ChatChannelFor's inverse; ok is false for a channel no
// chat can own.
func ChatIDFromChannel(channel string) (string, bool) {
	id := strings.TrimPrefix(channel, "chat:")
	if id == channel || id == "" {
		return "", false
	}
	return id, true
}

// The per-agent chat ids. Functions rather than formatting at every call site,
// so the id scheme has exactly one definition.
func ChatIDDirect(agent string) string  { return "dm:" + agent }
func ChatIDTasks(agent string) string   { return "tasks:" + agent }
func ChatIDService(agent string) string { return "service:" + agent }

const chatColumns = `id, kind, title, channel, legacy_agent, created_at, archived_at`

func scanChat(row interface{ Scan(...any) error }) (Chat, error) {
	var c Chat
	err := row.Scan(&c.ID, &c.Kind, &c.Title, &c.Channel, &c.LegacyAgent, &c.CreatedAt, &c.ArchivedAt)
	return c, err
}

// CreateChat inserts a chat, rejecting an id that is not a well-formed set of
// channel segments and an id whose channel is already owned.
func (b *Bus) CreateChat(c Chat) (Chat, error) {
	c.ID = strings.TrimSpace(c.ID)
	if c.ID == "" {
		return Chat{}, fmt.Errorf("chat id is required")
	}
	c.Channel = ChatChannelFor(c.ID)
	if !ValidChannel(c.Channel) {
		return Chat{}, fmt.Errorf("invalid chat id %q", c.ID)
	}
	if c.Kind == "" {
		c.Kind = ChatKindDirect
	}
	if _, err := b.db.Exec(`INSERT INTO chats(id, kind, title, channel, legacy_agent)
		VALUES (?, ?, ?, ?, ?)`, c.ID, c.Kind, c.Title, c.Channel, c.LegacyAgent); err != nil {
		return Chat{}, err
	}
	return b.GetChat(c.ID)
}

func (b *Bus) GetChat(id string) (Chat, error) {
	c, err := scanChat(b.db.QueryRow(`SELECT `+chatColumns+` FROM chats WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return Chat{}, ErrNotFound
	}
	return c, err
}

// ChatByChannel resolves the chat that owns channel. ok is false for every
// channel outside the chat namespace, which is how the reply router falls
// through to the existing inbox and origin rules.
func (b *Bus) ChatByChannel(channel string) (Chat, bool, error) {
	c, err := scanChat(b.db.QueryRow(`SELECT `+chatColumns+` FROM chats WHERE channel = ?`, channel))
	if err == sql.ErrNoRows {
		return Chat{}, false, nil
	}
	if err != nil {
		return Chat{}, false, err
	}
	return c, true, nil
}

func (b *Bus) ListChats() ([]Chat, error) {
	rows, err := b.db.Query(`SELECT ` + chatColumns + ` FROM chats ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chat
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (b *Bus) Participants(chatID string) ([]Participant, error) {
	rows, err := b.db.Query(`SELECT chat_id, principal, role, joined_at, read_ts, muted
		FROM chat_participants WHERE chat_id = ? ORDER BY principal`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Participant
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.ChatID, &p.Principal, &p.Role, &p.JoinedAt, &p.ReadTS, &p.Muted); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// customerLoginKey and defaultCustomerLogin mirror the daemon's customer
// identity so a chat write on any path can resolve the one participant that is
// not an agent without threading the login through every caller. The daemon
// owns the value; the bus only reads it.
const customerLoginKey = "customer_login"
const defaultCustomerLogin = "customer"

// CustomerPrincipal is the principal of the customer this daemon serves.
func (b *Bus) CustomerPrincipal() (string, error) {
	var login string
	err := b.db.QueryRow(`SELECT value FROM daemon_config WHERE key = ?`, customerLoginKey).Scan(&login)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if login = strings.TrimSpace(login); login == "" {
		login = defaultCustomerLogin
	}
	return "user:" + strings.TrimPrefix(login, "user:"), nil
}

// EnsureAgentChats provisions one agent's chats, for the paths that create or
// first touch an agent between two daemon startups.
func (b *Bus) EnsureAgentChats(agent string) error {
	customer, err := b.CustomerPrincipal()
	if err != nil {
		return err
	}
	return b.ReconcileChats([]string{agent}, customer)
}

// ReconcileChats provisions the per-agent chats and their participants, and
// carries the pre-chats read marks into the participant rows. Safe to re-run:
// every write is keyed by (chat id, principal), so an agent persisted before
// this migration gets its chats on the next daemon start.
func (b *Bus) ReconcileChats(agents []string, customer string) error {
	legacy, err := b.legacyReadMarks()
	if err != nil {
		return err
	}
	for _, a := range agents {
		seeds := []struct{ id, kind, title, customerRole string }{
			{ChatIDDirect(a), ChatKindDirect, a, "member"},
			{ChatIDTasks(a), ChatKindTasks, a + " tasks", "member"},
			{ChatIDService(a), ChatKindService, a + " service", "observer"},
		}
		for _, seed := range seeds {
			if err := b.upsertChat(Chat{
				ID: seed.id, Kind: seed.kind, Title: seed.title,
				Channel: ChatChannelFor(seed.id), LegacyAgent: legacyFor(seed.kind, a),
			}); err != nil {
				return fmt.Errorf("reconcile chat %q: %w", seed.id, err)
			}
			if err := b.upsertParticipant(seed.id, "agent:"+a, "member"); err != nil {
				return fmt.Errorf("reconcile participant of %q: %w", seed.id, err)
			}
			if err := b.upsertParticipant(seed.id, customer, seed.customerRole); err != nil {
				return fmt.Errorf("reconcile customer of %q: %w", seed.id, err)
			}
		}
		// The legacy mark was per agent and belonged to the customer's view of
		// the merged inboxes, which is exactly the personal chat's history.
		if ts := legacy[a]; ts != "" {
			if err := b.SetReadTS(ChatIDDirect(a), customer, ts, false); err != nil {
				return fmt.Errorf("carry read mark of %q: %w", a, err)
			}
		}
	}
	return nil
}

// legacyFor marks only the personal chat as carrying pre-migration history:
// that history is the two merged inboxes, which is what chatRowsSQL reads.
func legacyFor(kind, agent string) string {
	if kind == ChatKindDirect {
		return agent
	}
	return ""
}

func (b *Bus) legacyReadMarks() (map[string]string, error) {
	var value string
	err := b.db.QueryRow(`SELECT value FROM daemon_config WHERE key = ?`, legacyReadKey).Scan(&value)
	if err == sql.ErrNoRows {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	marks := map[string]string{}
	if err := json.Unmarshal([]byte(value), &marks); err != nil {
		// Unreadable optional UI state must not fail daemon startup; the
		// customer simply sees the history as unread again.
		return map[string]string{}, nil
	}
	return marks, nil
}

// upsertChat creates the chat or refreshes the fields reconciliation owns. It
// never touches created_at or archived_at, so an archived chat stays archived.
func (b *Bus) upsertChat(c Chat) error {
	_, err := b.db.Exec(`INSERT INTO chats(id, kind, title, channel, legacy_agent)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET kind = excluded.kind, channel = excluded.channel,
			legacy_agent = excluded.legacy_agent`,
		c.ID, c.Kind, c.Title, c.Channel, c.LegacyAgent)
	return err
}

// upsertParticipant adds the principal or leaves an existing row alone: role,
// read mark and mute belong to whoever set them last, not to reconciliation.
func (b *Bus) upsertParticipant(chatID, principal, role string) error {
	_, err := b.db.Exec(`INSERT INTO chat_participants(chat_id, principal, role)
		VALUES (?, ?, ?) ON CONFLICT(chat_id, principal) DO NOTHING`, chatID, principal, role)
	return err
}
