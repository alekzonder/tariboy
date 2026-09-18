package commands

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// chatReadKey holds the customer's per-agent read marks as one JSON object
// mapping an agent name to the timestamp of the newest message it has shown.
// One daemon has one customer, so a second dimension would buy nothing; several
// customers on one daemon would key this per customer instead.
//
// Known ceiling: the update is a read-modify-write, not a transaction, so two
// windows marking different chats read at the same instant can lose one mark.
// It costs an unread count that the next open restores, which is why this is
// one config value rather than a table with its own locking.
const chatReadKey = "chat_read_v1"

// customerPrincipal is the channel the customer receives on and the identity
// operator-published messages speak with.
func customerPrincipal(c *registry.Ctx) string {
	login := tasks.DefaultCustomerLogin
	if c.Tasks != nil {
		if configured := strings.TrimSpace(c.Tasks.CustomerLogin()); configured != "" {
			login = configured
		}
	}
	return "user:" + strings.TrimPrefix(login, "user:")
}

// chatTypes resolves the requested message-type filter, falling back to the
// default conversation types when the caller sends none.
func chatTypes(p registry.Params) []string {
	if requested := splitCommaList(str(p, "types")); len(requested) > 0 {
		return requested
	}
	return bus.DefaultChatTypes
}

func chatReadMarks(c *registry.Ctx) (map[string]string, error) {
	marks := map[string]string{}
	value, ok, err := c.Store.ConfigGet(chatReadKey)
	if err != nil || !ok {
		return marks, err
	}
	if err := json.Unmarshal([]byte(value), &marks); err != nil {
		// Unreadable optional UI state must not hide the conversations
		// themselves; the customer simply sees everything as unread again.
		return map[string]string{}, nil
	}
	return marks, nil
}

func chatLs() registry.Command {
	return registry.Command{
		Path:    "chat.ls",
		Summary: "List the customer's chats, most recently active first",
		Args: []registry.Arg{
			{Name: "types", Flag: "types", Type: registry.String, Help: "comma-separated message type globs"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/chats"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			marks, err := chatReadMarks(c)
			if err != nil {
				return nil, err
			}
			customer := customerPrincipal(c)
			summaries, err := b.ChatSummaries(customer, chatTypes(p), marks)
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(summaries))
			for _, summary := range summaries {
				rows = append(rows, map[string]any{
					"agent": summary.Agent, "last_ts": summary.LastTS, "last_from": summary.LastFrom,
					"last_type": summary.LastType, "last_text": summary.LastText,
					"unread": summary.Unread, "read_ts": marks[summary.Agent],
				})
			}
			return map[string]any{"customer": customer, "chats": rows, "count": len(rows)}, nil
		},
	}
}

func chatMessages() registry.Command {
	return registry.Command{
		Path:    "chat.messages",
		Summary: "Read the conversation with one agent, oldest first",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "types", Flag: "types", Type: registry.String, Help: "comma-separated message type globs"},
			{Name: "limit", Flag: "limit", Type: registry.String, Help: "max messages (default 200)"},
			{Name: "before", Flag: "before", Type: registry.String, Help: "page older: only messages before this timestamp"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/chats/{agent}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			limit := 0
			if raw := str(p, "limit"); raw != "" {
				if parsed, convErr := strconv.Atoi(raw); convErr == nil && parsed > 0 {
					limit = parsed
				}
			}
			customer := customerPrincipal(c)
			msgs, err := b.ChatMessages(customer, str(p, "agent"), chatTypes(p), limit, str(p, "before"))
			if err != nil {
				return nil, err
			}
			// The read mark rides along so the reader can separate what it has
			// already seen from what arrived since, before opening the chat
			// moves the mark forward.
			marks, err := chatReadMarks(c)
			if err != nil {
				return nil, err
			}
			rows := messageViews(msgs)
			for i, msg := range msgs {
				rows[i]["channel"] = msg.Channel
				rows[i]["from"] = bus.MessageFrom(msg)
			}
			return map[string]any{"customer": customer, "agent": str(p, "agent"),
				"messages": rows, "count": len(rows), "read_ts": marks[str(p, "agent")]}, nil
		},
	}
}

func chatRead() registry.Command {
	return registry.Command{
		Path:    "chat.read",
		Summary: "Mark the chat with one agent read up to a message timestamp",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "ts", Flag: "ts", Type: registry.String, Required: true, Help: "timestamp of the newest message shown"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/chats/{agent}/read"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			agent, ts := str(p, "agent"), strings.TrimSpace(str(p, "ts"))
			if agent == "" || ts == "" {
				return nil, api.UserError{Code: "missing_ts", Msg: "agent and ts are required"}
			}
			marks, err := chatReadMarks(c)
			if err != nil {
				return nil, err
			}
			// The mark only moves forward: a late or duplicated request from a
			// second window must not resurrect messages already seen.
			if marks[agent] < ts {
				marks[agent] = ts
			}
			raw, err := json.Marshal(marks)
			if err != nil {
				return nil, err
			}
			if err := c.Store.ConfigSet(chatReadKey, string(raw)); err != nil {
				return nil, err
			}
			return map[string]any{"agent": agent, "read_ts": marks[agent]}, nil
		},
	}
}
