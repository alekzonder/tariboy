package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/schedule"
)

func requireBus(c *registry.Ctx) (*bus.Bus, error) {
	if c.Bus == nil {
		return nil, api.UserError{Code: "bus_unavailable", Msg: "channel bus is not available"}
	}
	return c.Bus, nil
}

func channelLs() registry.Command {
	return registry.WithResult[ChannelList](registry.Command{
		Path:    "channel.ls",
		Summary: "List channels",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/channels"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			chans, err := b.Channels()
			if err != nil {
				return nil, err
			}
			rows := make([]ChannelRow, 0, len(chans))
			for _, ch := range chans {
				rows = append(rows, ChannelRow{Name: ch.Name, Kind: ch.Kind})
			}
			return ChannelList{Channels: rows, Count: len(rows)}, nil
		},
	})
}

func agentSubscriptions() registry.Command {
	return registry.WithResult[SubscriptionChannelList](registry.Command{
		Path:    "agent.subscriptions",
		Summary: "List the channels one agent is subscribed to",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/subscriptions"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			subs, err := b.ListSubscriptions(p.String("name"))
			if err != nil {
				return nil, err
			}
			// Distinct channels (an agent can hold several subscriptions on one
			// channel with different matchers); the UI lists channels, not subs.
			seen := map[string]bool{}
			rows := make([]SubscriptionChannel, 0, len(subs))
			for _, s := range subs {
				if seen[s.Channel] {
					continue
				}
				seen[s.Channel] = true
				rows = append(rows, SubscriptionChannel{
					Name:      s.Channel,
					Kind:      bus.ChannelKind(s.Channel),
					Protected: bus.IsProtectedSubscription(p.String("name"), s.Channel),
				})
			}
			return SubscriptionChannelList{Channels: rows, Count: len(rows)}, nil
		},
	})
}

func agentSubscribe() registry.Command {
	return registry.WithResult[SubscribeResult](registry.Command{
		Path:    "agent.subscribe",
		Summary: "Subscribe an agent to a channel (operator)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "channel", Type: registry.String, Required: true, Help: "channel name"},
			{Name: "type", Flag: "type", Type: registry.String, Help: "comma-separated message type globs"},
			{Name: "matcher", Flag: "matcher", Type: registry.String, Help: "content matcher as JSON (dotted paths -> globs)", Schema: jsonObjectOrString},
			{Name: "params", Flag: "params", Type: registry.String, Help: "provider params as JSON", Schema: jsonObjectOrString},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/subscriptions"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, channel := p.String("name"), p.String("channel")
			if _, err := agent.NewStore(c.Store).Get(name); err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "agent not found"}
			}
			if !bus.ValidChannel(channel) {
				return nil, api.UserError{Code: "bad_channel", Msg: "channel is empty or malformed"}
			}
			// Optional filter/provider inputs (spec §5.3). Each arrives either as a
			// decoded JSON object (compose/REST body) or a JSON string (CLI flag);
			// coerceMatcher / coerceParams accept both. A parameterized sub routes
			// through SubscribeParams so the daemon validates params against the
			// channel's provider schema at this apply time.
			matcher, err := coerceMatcher(p.Params["matcher"])
			if err != nil {
				return nil, api.UserError{Code: "bad_matcher", Msg: err.Error()}
			}
			params, err := coerceParams(p.Params["params"])
			if err != nil {
				return nil, api.UserError{Code: "bad_params", Msg: err.Error()}
			}
			typeFilter := splitCommaList(p.String("type"))
			// The bus is idempotent and returns the existing row for a repeated
			// subscribe. Read the active rows first so the response can say
			// whether this call added or resumed one; compose reports an
			// unchanged subscription as converged.
			before, err := b.ListSubscriptions(name)
			if err != nil {
				return nil, err
			}
			sub, err := b.SubscribeParams(name, channel, matcher, typeFilter, params)
			if err != nil {
				return nil, err
			}
			changed := true
			for _, s := range before {
				if s.ID == sub.ID {
					changed = false
					break
				}
			}
			return SubscribeResult{Name: name, Channel: channel, Kind: bus.ChannelKind(channel), ID: sub.ID,
				Changed: changed, Watch: sub.Watch, Params: sub.Params}, nil
		},
	})
}

// coerceMatcher turns an optional matcher argument into a bus.Matcher. It
// accepts nil (no matcher), a JSON object (map[string]any from a decoded REST
// body), or a JSON string (the CLI --matcher flag). Values are stringified so
// numeric/bool globs survive. Returns nil for an absent/empty matcher.
// jsonObjectOrString is the schema of an argument the CLI sends as JSON text
// and an HTTP client as a decoded object.
var jsonObjectOrString = map[string]any{"oneOf": []any{
	map[string]any{"type": "string"}, map[string]any{"type": "object"},
}}

func coerceMatcher(v any) (bus.Matcher, error) {
	m, err := coerceStringMap(v, "matcher")
	if err != nil || len(m) == 0 {
		return nil, err
	}
	out := bus.Matcher{}
	maps.Copy(out, m)
	return out, nil
}

// coerceParams turns an optional params argument into a map[string]any. It
// accepts nil, a JSON object, or a JSON string (the CLI --params flag). Returns
// nil for absent/empty params so the plain-subscribe path is taken.
func coerceParams(v any) (map[string]any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		if len(t) == 0 {
			return nil, nil
		}
		return t, nil
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, nil
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(t), &out); err != nil {
			return nil, fmt.Errorf("params is not valid JSON: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("params must be a JSON object")
	}
}

// coerceStringMap decodes a JSON object or JSON-string argument into a
// string-valued map (values stringified). Used by coerceMatcher.
func coerceStringMap(v any, what string) (map[string]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		out := map[string]string{}
		for k, val := range t {
			out[k] = fmt.Sprint(val)
		}
		return out, nil
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, nil
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(t), &raw); err != nil {
			return nil, fmt.Errorf("%s is not valid JSON: %w", what, err)
		}
		out := map[string]string{}
		for k, val := range raw {
			out[k] = fmt.Sprint(val)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be a JSON object", what)
	}
}

// splitCommaList splits a comma-separated flag value into trimmed, non-empty
// items. An empty input yields nil.
func splitCommaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func agentUnsubscribe() registry.Command {
	return registry.WithResult[UnsubscribeResult](registry.Command{
		Path:    "agent.unsubscribe",
		Summary: "Unsubscribe an agent from a channel (operator)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "channel", Type: registry.String, Required: true, Help: "channel name"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/agents/{name}/subscriptions"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, channel := p.String("name"), p.String("channel")
			if channel == "" {
				return nil, api.UserError{Code: "bad_channel", Msg: "channel is required"}
			}
			if bus.IsProtectedSubscription(name, channel) {
				return nil, api.UserError{Code: "protected_subscription", Msg: "system/group subscriptions are managed automatically"}
			}
			n, err := b.UnsubscribeChannel(name, channel)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "no such subscription"}
			}
			return UnsubscribeResult{Name: name, Channel: channel, Removed: n}, nil
		},
	})
}

func channelInspect() registry.Command {
	return registry.WithResult[ChannelInspectView](registry.Command{
		Path:    "channel.inspect",
		Summary: "Show a channel's kind and message count",
		Args:    []registry.Arg{{Name: "channel", Type: registry.String, Required: true, Help: "channel name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/channels/{channel}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			ch, count, err := b.InspectChannel(p.String("channel"))
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "channel not found"}
			}
			return ChannelInspectView{Name: ch.Name, Kind: ch.Kind, Messages: count}, nil
		},
	})
}

func channelTail() registry.Command {
	return registry.WithResult[ChannelTail](registry.Command{
		Path:    "channel.tail",
		Summary: "Print recent messages on a channel (-f to follow)",
		Args: []registry.Arg{
			{Name: "channel", Type: registry.String, Required: true, Help: "channel name"},
			{Name: "since", Flag: "since", Type: registry.String, Help: "return messages after this id"},
			{Name: "follow", Flag: "follow", Short: "f", Type: registry.Bool, Help: "poll for new messages"},
		},
		HTTP:       &registry.HTTPRoute{Method: "GET", Path: "/api/channels/{channel}/messages"},
		FollowFlag: "follow",
		Follow:     followChannelTail,
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			var msgs []bus.Message
			if since := p.String("since"); since != "" {
				msgs, err = b.MessagesSince(p.String("channel"), since, 200)
			} else {
				msgs, err = b.Tail(p.String("channel"), 50)
			}
			if err != nil {
				return nil, err
			}
			return ChannelTail{Messages: messageViews(msgs), Count: len(msgs)}, nil
		},
	})
}

func messageSend() registry.Command {
	return registry.WithResult[MessageSendResult](registry.Command{
		Path:    "message.send",
		Summary: "Publish a message to a channel (operator)",
		Args: []registry.Arg{
			{Name: "channel", Flag: "channel", Short: "c", Type: registry.String, Required: true, Help: "channel name"},
			{Name: "type", Flag: "type", Type: registry.String, Help: "message type"},
			{Name: "subject", Flag: "subject", Type: registry.String, Help: "comma-separated k=v subject"},
			{Name: "text", Flag: "text", Type: registry.String, Help: "message text"},
			{Name: "data", Flag: "data", Type: registry.String, Help: "JSON data payload"},
			{Name: "reply_to", Flag: "reply-to", Type: registry.String, Help: "channel an agent reply should land on"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/messages"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			// An operator publish is the customer speaking. Attributing it to the
			// customer principal — rather than the opaque "operator" — is what
			// lets the agent's own inbox be read back as one side of a chat.
			msg := bus.Message{Channel: p.String("channel"), Type: p.String("type"), Text: p.String("text"),
				Source: customerPrincipal(c), ReplyTo: p.String("reply_to")}
			if sub := p.String("subject"); sub != "" {
				msg.Subject = kvToAny(parseKV(sub))
			}
			if d := p.String("data"); d != "" {
				parsed, derr := decodeJSONObject(d)
				if derr != nil {
					return nil, api.UserError{Code: "bad_data", Msg: "data is not a JSON object"}
				}
				msg.Data = parsed
			}
			if msg.Channel == "" {
				return nil, api.UserError{Code: "missing_channel", Msg: "channel is required"}
			}
			out, err := b.Publish(msg)
			if err != nil {
				return nil, err
			}
			return MessageSendResult{ID: out.ID, Channel: out.Channel, Sent: true}, nil
		},
	})
}

func agentInbox() registry.Command {
	return registry.WithResult[InboxList](registry.Command{
		Path:    "agent.inbox.ls",
		Summary: "List an agent's inbox (operator), newest first",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "status", Flag: "status", Type: registry.String, Help: "pending|processed|dlq|all (default all)"},
			{Name: "limit", Flag: "limit", Type: registry.String, Help: "max rows (default 100)"},
			{Name: "before", Flag: "before", Type: registry.String, Help: "page the archive: only ids before this cursor"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/inbox"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			status := p.String("status")
			if status == "" {
				status = "all"
			}
			limit := 0
			if ls := p.String("limit"); ls != "" {
				if n, aerr := strconv.Atoi(ls); aerr == nil && n > 0 {
					limit = n
				}
			}
			items, err := b.Inbox(p.String("name"), status, limit, p.String("before"))
			if err != nil {
				return nil, api.UserError{Code: "bad_status", Msg: err.Error()}
			}
			rows := inboxItemViews(items)
			return InboxList{Messages: rows, Count: len(rows)}, nil
		},
	})
}

func agentInboxProcessed() registry.Command {
	return registry.WithResult[InboxProcessedResult](registry.Command{
		Path:    "agent.inbox.processed",
		Summary: "Mark an inbox message processed (operator ack)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "message id"},
			{Name: "result", Flag: "result", Type: registry.String, Required: true, Help: "handling result (mandatory)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/inbox/{id}/processed"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			result := p.String("result")
			if strings.TrimSpace(result) == "" {
				return nil, api.UserError{Code: "missing_result", Msg: "result is required"}
			}
			// Attribute the ack to the operator so the audit trail distinguishes
			// human vs agent processing (§8.2).
			item, err := b.MarkProcessed(p.String("name"), p.String("id"), "operator: "+result)
			if err != nil {
				if errors.Is(err, bus.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "no such message in this agent's inbox"}
				}
				return nil, err
			}
			return InboxProcessedResult{ID: item.ID, ProcessedAt: item.ProcessedAt, Result: item.Result}, nil
		},
	})
}

func agentInboxClear() registry.Command {
	return registry.WithResult[InboxClearResult](registry.Command{
		Path:    "agent.inbox.clear",
		Summary: "Physically clear an agent's pending inbox queue",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/inbox/clear"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			result, err := b.ClearPending(p.String("name"))
			if errors.Is(err, bus.ErrNotFound) {
				return nil, api.UserError{Code: "not_found", Msg: "agent not found"}
			}
			if err != nil {
				return nil, err
			}
			return InboxClearResult{DeletedDeliveries: result.DeletedDeliveries, DeletedMessages: result.DeletedMessages}, nil
		},
	})
}

func agentInboxReply() registry.Command {
	return registry.WithResult[InboxReplyResult](registry.Command{
		Path:    "agent.inbox.reply",
		Summary: "Reply to an inbox message (operator); auto-processes the row",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "message id"},
			{Name: "text", Flag: "text", Type: registry.String, Help: "reply text"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/inbox/{id}/reply"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, id := p.String("name"), p.String("id")
			// Publish the reply as the operator. bus.Reply auto-processes the
			// actor's own delivery, but "operator" holds none, so we mark the
			// agent's row processed explicitly with operator attribution (§8.2).
			reply, err := b.Reply("operator", id, p.String("text"), nil, "")
			if err != nil {
				if errors.Is(err, bus.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "no such message: " + id}
				}
				return nil, err
			}
			if _, perr := b.MarkProcessed(name, id, "operator replied: "+reply.ID); perr != nil {
				if errors.Is(perr, bus.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "message is not in this agent's inbox"}
				}
				return nil, perr
			}
			return InboxReplyResult{ID: reply.ID, Channel: reply.Channel,
				InReplyTo: reply.InReplyTo, CorrelationID: reply.CorrelationID, Replied: true}, nil
		},
	})
}

func agentInboxRequeue() registry.Command {
	return registry.WithResult[InboxRequeueResult](registry.Command{
		Path:    "agent.inbox.requeue",
		Summary: "Requeue a DLQ'd inbox message (operator)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "message id"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/inbox/{id}/requeue"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, id := p.String("name"), p.String("id")
			if err := b.Requeue(name, id); err != nil {
				if errors.Is(err, bus.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "no such message in this agent's inbox"}
				}
				return nil, err
			}
			return InboxRequeueResult{ID: id, Requeued: true}, nil
		},
	})
}

func channelWatches() registry.Command {
	return registry.WithResult[ChannelWatchList](registry.Command{
		Path:    "channel.watches",
		Summary: "List a channel's distinct watches (watch, params, subscribers)",
		Args:    []registry.Arg{{Name: "channel", Type: registry.String, Required: true, Help: "channel name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/channels/{channel}/watches"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			watches, err := b.WatchList(p.String("channel"))
			if err != nil {
				return nil, err
			}
			rows := make([]ChannelWatch, 0, len(watches))
			for _, w := range watches {
				rows = append(rows, ChannelWatch{Watch: w.Watch, Params: w.Params, Subscribers: w.Subscribers})
			}
			return ChannelWatchList{Watches: rows, Count: len(rows)}, nil
		},
	})
}

func scheduleLs() registry.Command {
	return registry.Command{
		Path:    "schedule.ls",
		Summary: "List an agent's schedules (read-only)",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/schedules"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			list, err := schedule.NewStore(c.Store, nil).List(p.String("name"))
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(list))
			for _, s := range list {
				rows = append(rows, map[string]any{"id": s.ID, "kind": s.Kind, "spec": s.Spec,
					"channel": s.Channel, "next_fire_at": s.NextFireAt, "enabled": s.Enabled})
			}
			return map[string]any{"schedules": rows, "count": len(rows)}, nil
		},
	}
}

func messageViews(msgs []bus.Message) []ChannelMessage {
	rows := make([]ChannelMessage, 0, len(msgs))
	for _, m := range msgs {
		rows = append(rows, ChannelMessage{ID: m.ID, TS: m.TS, Type: m.Type, Source: m.Source, Text: m.Text,
			ProducedByAgent: m.ProducedByAgent, ProducedInIteration: m.ProducedInIteration,
			MessageThreading: messageThreading(m)})
	}
	return rows
}

// chatMessageViews renders chat messages: the channel view plus the channel
// each message travelled on and the principal it speaks for.
func chatMessageViews(msgs []bus.Message) []ChatMessage {
	views := messageViews(msgs)
	rows := make([]ChatMessage, 0, len(msgs))
	for i, msg := range msgs {
		rows = append(rows, ChatMessage{ChannelMessage: views[i], Channel: msg.Channel, From: bus.MessageFrom(msg)})
	}
	return rows
}

// inboxItemViews renders per-agent inbox items: immutable message fields plus
// threading and aggregated per-agent delivery state (§8.1). Optional fields are
// omitted when empty so a plain pending row stays terse.
func inboxItemViews(items []bus.InboxItem) []InboxMessage {
	rows := make([]InboxMessage, 0, len(items))
	for _, it := range items {
		row := InboxMessage{
			ID: it.ID, Channel: it.Channel, TS: it.TS, Source: it.Source,
			Type: it.Type, Text: it.Text, Attempts: it.Attempts, DLQ: it.DLQ,
			ProducedByAgent: it.ProducedByAgent, ProducedInIteration: it.ProducedInIteration,
			MessageThreading: messageThreading(it.Message), DeliveredAt: it.DeliveredAt,
		}
		if it.ProcessedAt != "" {
			row.ProcessedAt = it.ProcessedAt
			row.Result = &it.Result
		}
		rows = append(rows, row)
	}
	return rows
}

func kvToAny(m map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func decodeJSONObject(s string) (map[string]any, error) {
	var out map[string]any
	if err := jsonUnmarshalCmd([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}
