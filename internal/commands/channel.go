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
	return registry.Command{
		Path:    "channel.ls",
		Summary: "List channels",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/channels"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			chans, err := b.Channels()
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(chans))
			for _, ch := range chans {
				rows = append(rows, map[string]any{"name": ch.Name, "kind": ch.Kind})
			}
			return map[string]any{"channels": rows, "count": len(rows)}, nil
		},
	}
}

func agentSubscriptions() registry.Command {
	return registry.Command{
		Path:    "agent.subscriptions",
		Summary: "List the channels one agent is subscribed to",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/subscriptions"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			subs, err := b.ListSubscriptions(str(p, "name"))
			if err != nil {
				return nil, err
			}
			// Distinct channels (an agent can hold several subscriptions on one
			// channel with different matchers); the UI lists channels, not subs.
			seen := map[string]bool{}
			rows := make([]map[string]any, 0, len(subs))
			for _, s := range subs {
				if seen[s.Channel] {
					continue
				}
				seen[s.Channel] = true
				rows = append(rows, map[string]any{
					"name":      s.Channel,
					"kind":      bus.ChannelKind(s.Channel),
					"protected": bus.IsProtectedSubscription(str(p, "name"), s.Channel),
				})
			}
			return map[string]any{"channels": rows, "count": len(rows)}, nil
		},
	}
}

func agentSubscribe() registry.Command {
	return registry.Command{
		Path:    "agent.subscribe",
		Summary: "Subscribe an agent to a channel (operator)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "channel", Type: registry.String, Required: true, Help: "channel name"},
			{Name: "type", Flag: "type", Type: registry.String, Help: "comma-separated message type globs"},
			{Name: "matcher", Flag: "matcher", Type: registry.String, Help: "content matcher as JSON (dotted paths -> globs)"},
			{Name: "params", Flag: "params", Type: registry.String, Help: "provider params as JSON"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/subscriptions"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, channel := str(p, "name"), str(p, "channel")
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
			matcher, err := coerceMatcher(p["matcher"])
			if err != nil {
				return nil, api.UserError{Code: "bad_matcher", Msg: err.Error()}
			}
			params, err := coerceParams(p["params"])
			if err != nil {
				return nil, api.UserError{Code: "bad_params", Msg: err.Error()}
			}
			typeFilter := splitCommaList(str(p, "type"))
			sub, err := b.SubscribeParams(name, channel, matcher, typeFilter, params)
			if err != nil {
				return nil, err
			}
			res := map[string]any{"name": name, "channel": channel, "kind": bus.ChannelKind(channel), "id": sub.ID}
			if sub.Watch != "" {
				res["watch"] = sub.Watch
			}
			if len(sub.Params) > 0 {
				res["params"] = sub.Params
			}
			return res, nil
		},
	}
}

// coerceMatcher turns an optional matcher argument into a bus.Matcher. It
// accepts nil (no matcher), a JSON object (map[string]any from a decoded REST
// body), or a JSON string (the CLI --matcher flag). Values are stringified so
// numeric/bool globs survive. Returns nil for an absent/empty matcher.
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
	return registry.Command{
		Path:    "agent.unsubscribe",
		Summary: "Unsubscribe an agent from a channel (operator)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "channel", Type: registry.String, Required: true, Help: "channel name"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/agents/{name}/subscriptions"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, channel := str(p, "name"), str(p, "channel")
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
			return map[string]any{"name": name, "channel": channel, "removed": n}, nil
		},
	}
}

func channelInspect() registry.Command {
	return registry.Command{
		Path:    "channel.inspect",
		Summary: "Show a channel's kind and message count",
		Args:    []registry.Arg{{Name: "channel", Type: registry.String, Required: true, Help: "channel name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/channels/{channel}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			ch, count, err := b.InspectChannel(str(p, "channel"))
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "channel not found"}
			}
			return map[string]any{"name": ch.Name, "kind": ch.Kind, "messages": count}, nil
		},
	}
}

func channelTail() registry.Command {
	return registry.Command{
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
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			var msgs []bus.Message
			if since := str(p, "since"); since != "" {
				msgs, err = b.MessagesSince(str(p, "channel"), since, 200)
			} else {
				msgs, err = b.Tail(str(p, "channel"), 50)
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{"messages": messageViews(msgs), "count": len(msgs)}, nil
		},
	}
}

func messageSend() registry.Command {
	return registry.Command{
		Path:    "message.send",
		Summary: "Publish a message to a channel (operator)",
		Args: []registry.Arg{
			{Name: "channel", Flag: "channel", Short: "c", Type: registry.String, Required: true, Help: "channel name"},
			{Name: "type", Flag: "type", Type: registry.String, Help: "message type"},
			{Name: "subject", Flag: "subject", Type: registry.String, Help: "comma-separated k=v subject"},
			{Name: "text", Flag: "text", Type: registry.String, Help: "message text"},
			{Name: "data", Flag: "data", Type: registry.String, Help: "JSON data payload"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/messages"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			msg := bus.Message{Channel: str(p, "channel"), Type: str(p, "type"), Text: str(p, "text"),
				Source: "operator"}
			if sub := str(p, "subject"); sub != "" {
				msg.Subject = kvToAny(parseKV(sub))
			}
			if d := str(p, "data"); d != "" {
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
			return map[string]any{"id": out.ID, "channel": out.Channel, "sent": true}, nil
		},
	}
}

func agentInbox() registry.Command {
	return registry.Command{
		Path:    "agent.inbox.ls",
		Summary: "List an agent's inbox (operator), newest first",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "status", Flag: "status", Type: registry.String, Help: "pending|processed|dlq|all (default all)"},
			{Name: "limit", Flag: "limit", Type: registry.String, Help: "max rows (default 100)"},
			{Name: "before", Flag: "before", Type: registry.String, Help: "page the archive: only ids before this cursor"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/inbox"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			status := str(p, "status")
			if status == "" {
				status = "all"
			}
			limit := 0
			if ls := str(p, "limit"); ls != "" {
				if n, aerr := strconv.Atoi(ls); aerr == nil && n > 0 {
					limit = n
				}
			}
			items, err := b.Inbox(str(p, "name"), status, limit, str(p, "before"))
			if err != nil {
				return nil, api.UserError{Code: "bad_status", Msg: err.Error()}
			}
			rows := inboxItemViews(items)
			return map[string]any{"messages": rows, "count": len(rows)}, nil
		},
	}
}

func agentInboxProcessed() registry.Command {
	return registry.Command{
		Path:    "agent.inbox.processed",
		Summary: "Mark an inbox message processed (operator ack)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "message id"},
			{Name: "result", Flag: "result", Type: registry.String, Required: true, Help: "handling result (mandatory)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/inbox/{id}/processed"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			result := str(p, "result")
			if strings.TrimSpace(result) == "" {
				return nil, api.UserError{Code: "missing_result", Msg: "result is required"}
			}
			// Attribute the ack to the operator so the audit trail distinguishes
			// human vs agent processing (§8.2).
			item, err := b.MarkProcessed(str(p, "name"), str(p, "id"), "operator: "+result)
			if err != nil {
				if errors.Is(err, bus.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "no such message in this agent's inbox"}
				}
				return nil, err
			}
			return map[string]any{"id": item.ID, "processed_at": item.ProcessedAt, "result": item.Result}, nil
		},
	}
}

func agentInboxReply() registry.Command {
	return registry.Command{
		Path:    "agent.inbox.reply",
		Summary: "Reply to an inbox message (operator); auto-processes the row",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "message id"},
			{Name: "text", Flag: "text", Type: registry.String, Help: "reply text"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/inbox/{id}/reply"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, id := str(p, "name"), str(p, "id")
			// Publish the reply as the operator. bus.Reply auto-processes the
			// actor's own delivery, but "operator" holds none, so we mark the
			// agent's row processed explicitly with operator attribution (§8.2).
			reply, err := b.Reply("operator", id, str(p, "text"), nil, "")
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
			return map[string]any{"id": reply.ID, "channel": reply.Channel,
				"in_reply_to": reply.InReplyTo, "correlation_id": reply.CorrelationID, "replied": true}, nil
		},
	}
}

func agentInboxRequeue() registry.Command {
	return registry.Command{
		Path:    "agent.inbox.requeue",
		Summary: "Requeue a DLQ'd inbox message (operator)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "message id"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/inbox/{id}/requeue"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			name, id := str(p, "name"), str(p, "id")
			if err := b.Requeue(name, id); err != nil {
				if errors.Is(err, bus.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "no such message in this agent's inbox"}
				}
				return nil, err
			}
			return map[string]any{"id": id, "requeued": true}, nil
		},
	}
}

func channelWatches() registry.Command {
	return registry.Command{
		Path:    "channel.watches",
		Summary: "List a channel's distinct watches (watch, params, subscribers)",
		Args:    []registry.Arg{{Name: "channel", Type: registry.String, Required: true, Help: "channel name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/channels/{channel}/watches"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			b, err := requireBus(c)
			if err != nil {
				return nil, err
			}
			watches, err := b.WatchList(str(p, "channel"))
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(watches))
			for _, w := range watches {
				rows = append(rows, map[string]any{
					"watch": w.Watch, "params": w.Params, "subscribers": w.Subscribers,
				})
			}
			return map[string]any{"watches": rows, "count": len(rows)}, nil
		},
	}
}

func scheduleLs() registry.Command {
	return registry.Command{
		Path:    "schedule.ls",
		Summary: "List an agent's schedules (read-only)",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/schedules"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			list, err := schedule.NewStore(c.Store, nil).List(str(p, "name"))
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

func messageViews(msgs []bus.Message) []map[string]any {
	rows := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		row := map[string]any{"id": m.ID, "ts": m.TS, "type": m.Type, "source": m.Source,
			"text": m.Text, "produced_by_agent": m.ProducedByAgent, "produced_in_iteration": m.ProducedInIteration}
		addThreadingFields(row, m)
		rows = append(rows, row)
	}
	return rows
}

// inboxItemViews renders per-agent inbox items: immutable message fields plus
// threading and aggregated per-agent delivery state (§8.1). Optional fields are
// omitted when empty so a plain pending row stays terse.
func inboxItemViews(items []bus.InboxItem) []map[string]any {
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		row := map[string]any{
			"id": it.ID, "channel": it.Channel, "ts": it.TS, "source": it.Source,
			"type": it.Type, "text": it.Text, "attempts": it.Attempts, "dlq": it.DLQ,
			"produced_by_agent": it.ProducedByAgent, "produced_in_iteration": it.ProducedInIteration,
		}
		addThreadingFields(row, it.Message)
		if it.DeliveredAt != "" {
			row["delivered_at"] = it.DeliveredAt
		}
		if it.ProcessedAt != "" {
			row["processed_at"] = it.ProcessedAt
			row["result"] = it.Result
		}
		rows = append(rows, row)
	}
	return rows
}

// addThreadingFields overlays the message's optional kind/subject/threading
// fields onto row, skipping empties so terse rows stay terse.
func addThreadingFields(row map[string]any, m bus.Message) {
	if m.Kind != "" {
		row["kind"] = m.Kind
	}
	if len(m.Subject) > 0 {
		row["subject"] = m.Subject
	}
	if len(m.Data) > 0 {
		row["data"] = m.Data
	}
	if m.CorrelationID != "" {
		row["correlation_id"] = m.CorrelationID
	}
	if m.InReplyTo != "" {
		row["in_reply_to"] = m.InReplyTo
	}
	if m.ReplyTo != "" {
		row["reply_to"] = m.ReplyTo
	}
	if m.Deadline != "" {
		row["deadline"] = m.Deadline
	}
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
