package commands

import (
	"errors"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

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

// personalChat resolves the customer conversation with one agent. The
// per-agent chats are provisioned at daemon startup and on agent creation; an
// agent first touched between the two is provisioned here, so the route keeps
// working instead of answering an empty feed.
func personalChat(b *bus.Bus, agent string) (bus.Chat, error) {
	if agent == "" {
		return bus.Chat{}, api.UserError{Code: "missing_agent", Msg: "agent is required"}
	}
	chat, err := b.GetChat(bus.ChatIDDirect(agent))
	if err == nil || !errors.Is(err, bus.ErrNotFound) {
		return chat, err
	}
	if err := b.EnsureAgentChats(agent); err != nil {
		return bus.Chat{}, err
	}
	return b.GetChat(bus.ChatIDDirect(agent))
}

// resolveChat takes either a chat id or an agent name in the one argument the
// chat endpoints carry. The sidebar knows agent names and nothing else; the
// chat list knows ids. An existing chat wins, so an agent whose name happens to
// read like an id can never be shadowed by one.
func resolveChat(b *bus.Bus, value string) (bus.Chat, error) {
	if value == "" {
		return bus.Chat{}, api.UserError{Code: "missing_agent", Msg: "agent is required"}
	}
	chat, err := b.GetChat(value)
	if err == nil {
		return chat, nil
	}
	if !errors.Is(err, bus.ErrNotFound) {
		return bus.Chat{}, err
	}
	return personalChat(b, value)
}

// participantReadTS is one participant read mark, empty when that principal has
// read nothing in the chat.
func participantReadTS(b *bus.Bus, chatID, principal string) (string, error) {
	parts, err := b.Participants(chatID)
	if err != nil {
		return "", err
	}
	for _, p := range parts {
		if p.Principal == principal {
			return p.ReadTS, nil
		}
	}
	return "", nil
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
			customer := customerPrincipal(c)
			summaries, err := b.ChatList(customer, chatTypes(p))
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(summaries))
			for _, summary := range summaries {
				rows = append(rows, map[string]any{
					"id": summary.ID, "kind": summary.Kind, "title": summary.Title,
					"channel": summary.Channel,
					"agent":   summary.Agent, "last_ts": summary.LastTS, "last_from": summary.LastFrom,
					"last_type": summary.LastType, "last_text": summary.LastText,
					"unread": summary.Unread, "read_ts": summary.ReadTS,
				})
			}
			return map[string]any{"customer": customer, "chats": rows, "count": len(rows)}, nil
		},
	}
}

func chatMessages() registry.Command {
	return registry.Command{
		Path:    "chat.messages",
		Summary: "Read one chat, oldest first, by chat id or agent name",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "chat id or agent name"},
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
			chat, err := resolveChat(b, str(p, "agent"))
			if err != nil {
				return nil, err
			}
			msgs, err := b.ChatFeed(chat, customer, chatTypes(p), limit, str(p, "before"))
			if err != nil {
				return nil, err
			}
			// The read mark rides along so the reader can separate what it has
			// already seen from what arrived since, before opening the chat
			// moves the mark forward.
			readTS, err := participantReadTS(b, chat.ID, customer)
			if err != nil {
				return nil, err
			}
			rows := messageViews(msgs)
			for i, msg := range msgs {
				rows[i]["channel"] = msg.Channel
				rows[i]["from"] = bus.MessageFrom(msg)
			}
			return map[string]any{"customer": customer, "agent": bus.ChatAgent(chat),
				"chat": chat.ID, "channel": chat.Channel, "kind": chat.Kind, "title": chat.Title,
				"messages": rows, "count": len(rows), "read_ts": readTS}, nil
		},
	}
}

func chatRead() registry.Command {
	return registry.Command{
		Path:    "chat.read",
		Summary: "Mark a chat read up to a message timestamp, by chat id or agent name",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "chat id or agent name"},
			{Name: "ts", Flag: "ts", Type: registry.String, Required: true, Help: "timestamp of the newest message shown"},
			{Name: "exact", Flag: "exact", Type: registry.Bool, Help: "set the mark to ts even when that moves it backwards (mark unread)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/chats/{agent}/read"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			agent, ts := str(p, "agent"), strings.TrimSpace(str(p, "ts"))
			if agent == "" || ts == "" {
				return nil, api.UserError{Code: "missing_ts", Msg: "agent and ts are required"}
			}
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			chat, err := resolveChat(b, agent)
			if err != nil {
				return nil, err
			}
			// The mark only moves forward: a late or duplicated request from a
			// second window must not resurrect messages already seen. Marking a
			// chat unread is the one deliberate exception, and says so.
			exact, _ := boolParam(p, "exact")
			customer := customerPrincipal(c)
			if err := b.SetReadTS(chat.ID, customer, ts, exact); err != nil {
				return nil, err
			}
			readTS, err := participantReadTS(b, chat.ID, customer)
			if err != nil {
				return nil, err
			}
			return map[string]any{"agent": bus.ChatAgent(chat), "chat": chat.ID, "read_ts": readTS}, nil
		},
	}
}

// chatUnanswered is the answering obligation as a queue: the chat messages one
// principal has not replied to yet. It is not the delivery queue — processing a
// delivery satisfies the transport, a reply satisfies the conversation.
func chatUnanswered() registry.Command {
	return registry.Command{
		Path:    "chat.unanswered",
		Summary: "List the chat messages a principal has not answered yet",
		Args: []registry.Arg{
			{Name: "chat", Type: registry.String, Required: true, Help: "chat id"},
			{Name: "principal", Flag: "principal", Type: registry.String, Required: true,
				Help: "user:<login> or agent:<name>"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/chats/{chat}/unanswered"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			chat, principal := str(p, "chat"), str(p, "principal")
			if chat == "" || principal == "" {
				return nil, api.UserError{Code: "missing_principal", Msg: "chat and principal are required"}
			}
			msgs, err := b.Unanswered(chat, principal)
			if err != nil {
				return nil, err
			}
			rows := messageViews(msgs)
			for i, msg := range msgs {
				rows[i]["channel"] = msg.Channel
				rows[i]["from"] = bus.MessageFrom(msg)
			}
			return map[string]any{"chat": chat, "principal": principal,
				"messages": rows, "count": len(rows)}, nil
		},
	}
}

func chatParticipants() registry.Command {
	return registry.Command{
		Path:    "chat.participants",
		Summary: "List the principals taking part in a chat",
		Args: []registry.Arg{
			{Name: "chat", Type: registry.String, Required: true, Help: "chat id"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/chats/{chat}/participants"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			chat := str(p, "chat")
			parts, err := b.Participants(chat)
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(parts))
			for _, part := range parts {
				rows = append(rows, map[string]any{"principal": part.Principal, "role": part.Role,
					"joined_at": part.JoinedAt, "read_ts": part.ReadTS, "muted": part.Muted})
			}
			return map[string]any{"chat": chat, "participants": rows, "count": len(rows)}, nil
		},
	}
}
