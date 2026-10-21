package bus

import (
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/store"
)

// messageColumns is the message projection scanMessage expects, in its order;
// messageColumnsM is the same projection qualified by the alias m.
const (
	messageColumns = `id, channel, ts, source, type, subject, text, data, ` +
		`produced_by_agent, produced_in_iteration, produced_by_plugin, ` +
		`kind, correlation_id, in_reply_to, reply_to, deadline`
	messageColumnsM = `m.id, m.channel, m.ts, m.source, m.type, m.subject, m.text, m.data, ` +
		`m.produced_by_agent, m.produced_in_iteration, m.produced_by_plugin, ` +
		`m.kind, m.correlation_id, m.in_reply_to, m.reply_to, m.deadline`
)

// ChatFeed returns one chat's messages, oldest first: the chat channel plus,
// for a chat that carries pre-migration history, the legacy two-inbox
// projection. Messages are never rewritten onto the new channel, so the old
// conversation stays readable exactly where it was written.
//
// before is a message TIMESTAMP, not an id, and pages backwards: ids embed
// their channel, so two channels merged into one feed do not sort by id.
func (b *Bus) ChatFeed(chat Chat, viewer string, types []string, limit int, before string) ([]Message, error) {
	if limit <= 0 {
		limit = 200
	}
	filter, filterArgs := typeFilterSQL(types)
	args := []any{chat.Channel}
	args = append(args, filterArgs...)
	page := ""
	if before != "" {
		// A client may echo the cursor with any RFC 3339 offset or width;
		// the stored ts is canonical, so the text comparison needs it too.
		if v, ok := store.NormalizeTimeValue(before); ok {
			before = v
		}
		page = " AND ts < ?"
		args = append(args, before)
	}
	// Newest page first so before can walk back; reversed to chronological
	// order once both sources are merged.
	rows, err := b.db.Query(`SELECT `+messageColumns+` FROM messages
		WHERE channel = ? AND `+filter+page+`
		ORDER BY ts DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if chat.LegacyAgent != "" {
		// The legacy projection is already tested and returns chronological
		// order; both sides are capped at limit before the merge, so a page
		// never scans more than two pages worth of rows.
		legacy, err := b.ChatMessages(viewer, chat.LegacyAgent, types, limit, before)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, legacy...)
		sort.SliceStable(msgs, func(i, j int) bool {
			if msgs[i].TS != msgs[j].TS {
				return msgs[i].TS > msgs[j].TS
			}
			return msgs[i].ID > msgs[j].ID
		})
		if len(msgs) > limit {
			msgs = msgs[:limit]
		}
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// ChatList returns every chat principal takes part in, most recently active
// first, with the last message and the unread count measured against that
// participant own read mark. A chat with no messages yet is still listed: the
// chat exists as an entity, not as a side effect of traffic.
func (b *Bus) ChatList(principal string, types []string) ([]ChatSummary, error) {
	rows, err := b.read.Query(`SELECT `+chatColumns+`, p.read_ts
		FROM chats AS c JOIN chat_participants AS p ON p.chat_id = c.id
		WHERE p.principal = ? AND c.archived_at = ''
		ORDER BY c.id`, principal)
	if err != nil {
		return nil, err
	}
	type entry struct {
		chat   Chat
		readTS string
	}
	var entries []entry
	func() {
		defer rows.Close()
		for rows.Next() {
			var e entry
			if err = rows.Scan(&e.chat.ID, &e.chat.Kind, &e.chat.Title, &e.chat.Channel,
				&e.chat.LegacyAgent, &e.chat.CreatedAt, &e.chat.ArchivedAt, &e.readTS); err != nil {
				return
			}
			entries = append(entries, e)
		}
		err = rows.Err()
	}()
	if err != nil {
		return nil, err
	}

	// The legacy per-agent summaries are one query for all chats that carry
	// pre-migration history, keyed by the read mark of the chat that owns it.
	reads := map[string]string{}
	for _, e := range entries {
		if e.chat.LegacyAgent != "" {
			reads[e.chat.LegacyAgent] = e.readTS
		}
	}
	legacy := map[string]ChatSummary{}
	if len(reads) > 0 {
		summaries, err := b.ChatSummaries(principal, types, reads)
		if err != nil {
			return nil, err
		}
		for _, s := range summaries {
			legacy[s.Agent] = s
		}
	}

	summaries, err := b.chatChannelSummaries(principal, types)
	if err != nil {
		return nil, err
	}
	participants, err := b.chatParticipants(principal)
	if err != nil {
		return nil, err
	}
	out := make([]ChatSummary, 0, len(entries))
	for _, e := range entries {
		summary := summaries[e.chat.ID]
		if old, ok := legacy[e.chat.LegacyAgent]; ok && e.chat.LegacyAgent != "" {
			summary.Unread += old.Unread
			if old.LastTS > summary.LastTS {
				summary.LastTS, summary.LastType = old.LastTS, old.LastType
				summary.LastText, summary.LastFrom = old.LastText, old.LastFrom
			}
		}
		summary.ID, summary.Kind, summary.Title = e.chat.ID, e.chat.Kind, e.chat.Title
		summary.Channel, summary.ReadTS = e.chat.Channel, e.readTS
		summary.Agent = ChatAgent(e.chat)
		summary.Participants = participants[e.chat.ID]
		if summary.Participants == nil {
			summary.Participants = []string{}
		}
		out = append(out, summary)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastTS > out[j].LastTS })
	return out, nil
}

// chatChannelSummaries aggregates the own channel of every unarchived chat of
// principal in one query: its last message and how many messages from other
// participants arrived after principal read mark. A sender that is not a
// participant, such as a system:* notice addressed to the agent, is shown in
// the feed but never counts as unread. A chat with no messages is absent.
func (b *Bus) chatChannelSummaries(principal string, types []string) (map[string]ChatSummary, error) {
	filter, filterArgs := typeFilterSQL(types)
	// The bare columns beside max(m.ts) are SQLite documented "row that holds
	// the maximum" behaviour, so each chat last message comes back in one pass.
	rows, err := b.read.Query(`
		SELECT p.chat_id, max(m.ts), m.type, m.text, m.sender,
		       COALESCE(sum(CASE WHEN m.sender <> p.principal AND m.ts > p.read_ts AND m.sender IN (
		           SELECT principal FROM chat_participants WHERE chat_id = p.chat_id) THEN 1 ELSE 0 END), 0)
		FROM chat_participants AS p
		JOIN chats AS c ON c.id = p.chat_id
		JOIN messages AS m ON m.channel = c.channel
		WHERE p.principal = ? AND c.archived_at = '' AND `+filter+`
		GROUP BY p.chat_id`,
		append([]any{principal}, filterArgs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ChatSummary{}
	for rows.Next() {
		var id string
		var s ChatSummary
		if err := rows.Scan(&id, &s.LastTS, &s.LastType, &s.LastText, &s.LastFrom, &s.Unread); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

// chatParticipants lists the participants of every chat principal is in, in
// the order Participants returns them.
func (b *Bus) chatParticipants(principal string) (map[string][]string, error) {
	rows, err := b.read.Query(`SELECT chat_id, principal FROM chat_participants
		WHERE chat_id IN (SELECT chat_id FROM chat_participants WHERE principal = ?)
		ORDER BY chat_id, principal`, principal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var chatID, p string
		if err := rows.Scan(&chatID, &p); err != nil {
			return nil, err
		}
		out[chatID] = append(out[chatID], p)
	}
	return out, rows.Err()
}

// ChatAgent is the agent a per-agent chat belongs to, or "" for a chat that is
// not about one agent (a group or an ad-hoc multi-agent chat).
func ChatAgent(chat Chat) string {
	switch chat.Kind {
	case ChatKindDirect, ChatKindTasks, ChatKindService:
		if _, suffix, ok := strings.Cut(chat.ID, ":"); ok {
			return suffix
		}
	}
	return ""
}

// SetReadTS moves a participant read mark. It only moves forward: a late or
// duplicated request from a second window must not resurrect messages already
// seen. exact sets the mark even when that moves it backwards, which is how
// marking a chat unread is expressed.
func (b *Bus) SetReadTS(chatID, principal, ts string, exact bool) error {
	if v, ok := store.NormalizeTimeValue(ts); ok {
		ts = v
	}
	query := `UPDATE chat_participants SET read_ts = ? WHERE chat_id = ? AND principal = ? AND read_ts < ?`
	args := []any{ts, chatID, principal, ts}
	if exact {
		query = `UPDATE chat_participants SET read_ts = ? WHERE chat_id = ? AND principal = ?`
		args = args[:3]
	}
	_, err := b.db.Exec(query, args...)
	return err
}
