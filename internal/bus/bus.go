// Package bus is the daemon channel bus (spec §6): channels, messages,
// subscriptions and per-subscription deliveries, all through an injected clock.
// bus.go holds the shared types and channel-name helpers; the store-backed Bus
// methods live in store.go and the content matcher in matcher.go.
package bus

import (
	"regexp"
	"strings"
)

var (
	channelSegRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	// channelPrefixes is the set of kind prefixes ValidChannel recognises (design
	// §1). `plugin` and `system` join the core four so plugin-facing and
	// system channels are well-formed; provider-declared channels carry their own
	// plugin-owned prefixes (e.g. `issue-provider:query`) and are accepted on the
	// subscribe path via the provider registry, not by this static check.
	channelPrefixes = map[string]bool{
		"agent": true, "group": true, "user": true, "chat": true,
		"plugin": true, "system": true,
	}
)

// ValidChannel reports whether name is a well-formed bus channel: a known prefix
// plus every ':'-separated segment matching the name rule. Mirrors the messenger
// plugin's routing.valid_channel and agent.ValidName, and is the single validator
// the operator subscribe path uses to avoid creating junk channels from typos.
func ValidChannel(name string) bool {
	if name == "" || !strings.Contains(name, ":") {
		return false
	}
	parts := strings.Split(name, ":")
	if !channelPrefixes[parts[0]] {
		return false
	}
	for _, p := range parts {
		if !channelSegRE.MatchString(p) {
			return false
		}
	}
	return true
}

// Message is one bus message with full attribution (spec §6). The threading
// axis (Kind/CorrelationID/InReplyTo/ReplyTo/Deadline) is ported from the old
// MESSAGE_SCHEMA (design §2); it flows through Publish/scan end-to-end but does
// not yet drive any routing.
type Message struct {
	ID                  string
	IdempotencyKey      string
	Channel             string
	TS                  string // RFC3339Nano, assigned by Publish
	Source              string
	Type                string
	Subject             map[string]any
	Text                string
	Data                map[string]any
	ProducedByAgent     string
	ProducedInIteration string
	ProducedByPlugin    string
	Kind                string // event | request | reply; empty defaults to "event" on publish
	CorrelationID       string
	InReplyTo           string
	ReplyTo             string
	Deadline            string
}

// Subscription is an agent's standing interest in a channel. Params/Watch/Locked
// (design §5.1) carry provider parameters, the canonical (channel, params) watch
// fingerprint, and the system-managed lock; they flow through scan/select but do
// not yet drive any provider behaviour.
type Subscription struct {
	ID         string
	Agent      string
	Channel    string
	Matcher    Matcher
	TypeFilter []string
	CreatedAt  string
	Params     map[string]any
	Watch      string
	Locked     bool
}

// Channel is a named stream with an inferred kind.
type Channel struct {
	Name      string
	Kind      string
	CreatedAt string
}

func InboxChannel(agent string) string   { return "agent:" + agent + ":inbox" }
func StreamChannel(agent string) string  { return "agent:" + agent + ":stream" }
func GroupBroadcast(group string) string { return "group:" + group + ":broadcast" }
func GroupInbox(group string) string     { return "group:" + group + ":inbox" }
func GroupDirect(group, agent string) string {
	return "group:" + group + ":direct:" + agent
}
func ChatChannel(name string) string { return "chat:" + name }
func UserChannel(name string) string { return "user:" + name }

// IsProtectedSubscription reports whether an operator must not detach agent from
// channel: its own inbox (agent:<agent>:inbox) and any group channel (group:*),
// both of which are managed by the system / group provisioner, not the operator.
func IsProtectedSubscription(agent, channel string) bool {
	if channel == "agent:"+agent+":inbox" {
		return true
	}
	return strings.HasPrefix(channel, "group:")
}

// ChannelKind infers the kind constant from a channel name. Unknown shapes
// default to "chat" (a free-form user channel).
func ChannelKind(name string) string {
	switch {
	case strings.HasPrefix(name, "agent:") && strings.HasSuffix(name, ":inbox"):
		return "agent_inbox"
	case strings.HasPrefix(name, "agent:") && strings.HasSuffix(name, ":stream"):
		return "agent_stream"
	case strings.HasPrefix(name, "group:") && strings.HasSuffix(name, ":inbox"):
		return "group_inbox"
	case strings.HasPrefix(name, "group:") && strings.HasSuffix(name, ":broadcast"):
		return "group_broadcast"
	case strings.HasPrefix(name, "group:") && strings.Contains(name, ":direct:"):
		return "group_direct"
	case strings.HasPrefix(name, "user:"):
		return "user"
	case strings.HasPrefix(name, "chat:"):
		return "chat"
	default:
		return "chat"
	}
}
