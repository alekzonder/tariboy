package bus

import (
	"encoding/json"
	"strings"
)

// DefaultChatTypes is the message types a chat shows unless the operator picks
// another set. It is an explicit list rather than "task.*" so an agent's own
// wakes — task.goal, script.result, schedule alarms — neither appear in the
// conversation nor move that agent up a chat list.
var DefaultChatTypes = []string{
	"task.question", "task.answered", "task.assigned", "task.triage",
	"message", "note", "group.request", "chat.*",
}

// MessageFrom returns the principal that sent m: the explicit data.from an
// attributed producer wrote, else a principal-shaped source, else the producing
// agent, else "system" for daemon-owned traffic.
func MessageFrom(m Message) string {
	if from, ok := m.Data["from"].(string); ok && strings.TrimSpace(from) != "" {
		return strings.TrimSpace(from)
	}
	if strings.HasPrefix(m.Source, "agent:") || strings.HasPrefix(m.Source, "user:") {
		return m.Source
	}
	if m.ProducedByAgent != "" {
		return "agent:" + m.ProducedByAgent
	}
	return "system"
}

// fromSQL is MessageFrom evaluated in SQLite, so a chat can be resolved and
// aggregated by the database instead of scanning every message into Go.
const fromSQL = `COALESCE(
	NULLIF(json_extract(CASE WHEN json_valid(data) THEN data END, '$.from'), ''),
	CASE WHEN source GLOB 'agent:*' OR source GLOB 'user:*' THEN source END,
	CASE WHEN COALESCE(produced_by_agent, '') <> '' THEN 'agent:' || produced_by_agent END,
	'system')`

// chatRowsSQL is the chat projection: every message that belongs to a
// conversation between the customer and one agent, with the agent it belongs to
// and the principal that sent it. GLOB (not LIKE) keeps the channel prefix
// case-sensitive so the channel index is still usable.
//
// Known ceiling: resolving the sender needs each row's body, so a summary walks
// every inbox message rather than only an index. That is cheap for one daemon's
// tens of agents; if a host ever accumulates enough history to matter, the
// sender belongs in its own indexed column rather than in data.
const chatRowsSQL = `
	SELECT agent, id, channel, ts, source, type, subject, text, data,
	       produced_by_agent, produced_in_iteration, produced_by_plugin,
	       kind, correlation_id, in_reply_to, reply_to, deadline, sender
	FROM (
		SELECT CASE WHEN channel = ? THEN substr(` + fromSQL + `, 7)
		            ELSE substr(channel, 7, length(channel) - 12) END AS agent,
		       id, channel, ts, source, type, subject, text, data,
		       produced_by_agent, produced_in_iteration, produced_by_plugin,
		       kind, correlation_id, in_reply_to, reply_to, deadline,
		       ` + fromSQL + ` AS sender
		FROM messages
		WHERE channel = ? OR channel GLOB 'agent:*:inbox'
	)
	WHERE ((channel = ? AND sender GLOB 'agent:*')
	    OR (channel <> ? AND sender = ?))`

// ChatSummary is one conversation in the chat list: when it last moved, what
// was said, and how much of it the customer has not read yet.
type ChatSummary struct {
	Agent    string
	LastTS   string
	LastFrom string
	LastType string
	LastText string
	Unread   int
}

// typeFilterSQL renders an OR of GLOB comparisons for types, or an always-true
// condition when no filter was requested.
func typeFilterSQL(types []string) (string, []any) {
	clauses := make([]string, 0, len(types))
	args := make([]any, 0, len(types))
	for _, glob := range types {
		if glob = strings.TrimSpace(glob); glob != "" {
			clauses = append(clauses, "type GLOB ?")
			args = append(args, glob)
		}
	}
	if len(clauses) == 0 {
		return "1", nil
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}

// ChatSummaries returns one row per agent the customer has a conversation with,
// newest conversation first. reads maps an agent name to the timestamp of the
// newest message the customer has seen in that chat; an agent missing from it
// has read nothing. Only messages the agent sent can be unread.
func (b *Bus) ChatSummaries(customer string, types []string, reads map[string]string) ([]ChatSummary, error) {
	readJSON, err := json.Marshal(reads)
	if err != nil {
		return nil, err
	}
	filter, filterArgs := typeFilterSQL(types)
	args := []any{customer, customer, customer, customer, customer}
	args = append(args, filterArgs...)
	args = append(args, string(readJSON))
	// The bare columns beside max(ts) are SQLite's documented "row that holds
	// the maximum" behaviour, so the last message comes back with its timestamp
	// in one pass.
	rows, err := b.db.Query(`
		SELECT chat.agent, max(chat.ts) AS last_ts, chat.type, chat.text, chat.sender,
		       sum(CASE WHEN chat.sender GLOB 'agent:*'
		                 AND chat.ts > COALESCE(read.value, '') THEN 1 ELSE 0 END) AS unread
		FROM (`+chatRowsSQL+` AND `+filter+`) AS chat
		LEFT JOIN json_each(?) AS read ON read.key = chat.agent
		GROUP BY chat.agent
		ORDER BY last_ts DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatSummary
	for rows.Next() {
		var row ChatSummary
		var text *string
		if err := rows.Scan(&row.Agent, &row.LastTS, &row.LastType, &text, &row.LastFrom, &row.Unread); err != nil {
			return nil, err
		}
		if text != nil {
			row.LastText = *text
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ChatMessages returns the conversation with one agent, oldest first: the
// customer's messages in that agent's inbox merged with the agent's messages in
// the customer's own channel. before is a message TIMESTAMP, not an id, and
// pages backwards through older messages: ids embed their channel, so two
// channels merged into one feed do not sort by id at all.
func (b *Bus) ChatMessages(customer, agent string, types []string, limit int, before string) ([]Message, error) {
	if limit <= 0 {
		limit = 200
	}
	filter, filterArgs := typeFilterSQL(types)
	args := []any{customer, customer, customer, customer, customer}
	args = append(args, filterArgs...)
	args = append(args, agent)
	page := ""
	if before != "" {
		page = " AND ts < ?"
		args = append(args, before)
	}
	// Newest page first so `before` can walk back, reversed to chronological
	// order below.
	rows, err := b.db.Query(`
		SELECT id, channel, ts, source, type, subject, text, data,
		       produced_by_agent, produced_in_iteration, produced_by_plugin,
		       kind, correlation_id, in_reply_to, reply_to, deadline
		FROM (`+chatRowsSQL+` AND `+filter+`)
		WHERE agent = ?`+page+`
		ORDER BY ts DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}
