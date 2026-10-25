package commands

import "github.com/alekzonder/tariboy/internal/bus"

// Response types of the channel, inbox, subscription, chat and message-setting
// commands. Each command declares its type with registry.WithResult, which
// derives the OpenAPI result schema from it.

// ChannelRow is one channel in the channel.ls list.
type ChannelRow struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ChannelList is every channel (channel.ls).
type ChannelList struct {
	Channels []ChannelRow `json:"channels"`
	Count    int          `json:"count"`
}

// ChannelInspectView is a channel's kind and message count (channel.inspect).
type ChannelInspectView struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Messages int    `json:"messages"`
}

// MessageThreading carries a message's optional kind, subject, data and
// threading keys; each appears only when non-empty.
type MessageThreading struct {
	Kind          string         `json:"kind,omitempty"`
	Subject       map[string]any `json:"subject,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	InReplyTo     string         `json:"in_reply_to,omitempty"`
	ReplyTo       string         `json:"reply_to,omitempty"`
	Deadline      string         `json:"deadline,omitempty"`
}

// ChannelMessage is one message on a channel.
type ChannelMessage struct {
	ID                  string `json:"id"`
	TS                  string `json:"ts"`
	Type                string `json:"type"`
	Source              string `json:"source"`
	Text                string `json:"text"`
	ProducedByAgent     string `json:"produced_by_agent"`
	ProducedInIteration string `json:"produced_in_iteration"`
	MessageThreading
}

// ChannelTail is a channel's recent messages, oldest first (channel.tail).
type ChannelTail struct {
	Messages []ChannelMessage `json:"messages"`
	Count    int              `json:"count"`
}

// ChannelWatch is one distinct provider watch on a channel: its fingerprint,
// the params that produced it (null when none were stored) and its
// subscribers.
type ChannelWatch struct {
	Watch       string         `json:"watch"`
	Params      map[string]any `json:"params" jsonschema_extras:"nullable=true"`
	Subscribers []string       `json:"subscribers"`
}

// ChannelWatchList is a channel's distinct watches (channel.watches).
type ChannelWatchList struct {
	Watches []ChannelWatch `json:"watches"`
	Count   int            `json:"count"`
}

// MessageSendResult acknowledges an operator publish (message.send).
type MessageSendResult struct {
	ID      string `json:"id"`
	Channel string `json:"channel"`
	Sent    bool   `json:"sent"`
}

// SubscriptionChannel is one distinct channel an agent is subscribed to.
type SubscriptionChannel struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Protected bool   `json:"protected"`
}

// SubscriptionChannelList is an agent's subscribed channels
// (agent.subscriptions).
type SubscriptionChannelList struct {
	Channels []SubscriptionChannel `json:"channels"`
	Count    int                   `json:"count"`
}

// SubscribeResult is the subscription an agent.subscribe call added or
// found; watch and params appear only for a parameterized subscription.
type SubscribeResult struct {
	Name    string         `json:"name"`
	Channel string         `json:"channel"`
	Kind    string         `json:"kind"`
	ID      string         `json:"id"`
	Changed bool           `json:"changed"`
	Watch   string         `json:"watch,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
}

// UnsubscribeResult reports how many subscriptions agent.unsubscribe removed.
type UnsubscribeResult struct {
	Name    string `json:"name"`
	Channel string `json:"channel"`
	Removed int    `json:"removed"`
}

// InboxMessage is one message in an agent's inbox with its aggregated
// delivery state. delivered_at appears once delivered; processed_at and
// result appear once processed.
type InboxMessage struct {
	ID                  string `json:"id"`
	Channel             string `json:"channel"`
	TS                  string `json:"ts"`
	Source              string `json:"source"`
	Type                string `json:"type"`
	Text                string `json:"text"`
	Attempts            int    `json:"attempts"`
	DLQ                 bool   `json:"dlq"`
	ProducedByAgent     string `json:"produced_by_agent"`
	ProducedInIteration string `json:"produced_in_iteration"`
	MessageThreading
	DeliveredAt string  `json:"delivered_at,omitempty"`
	ProcessedAt string  `json:"processed_at,omitempty"`
	Result      *string `json:"result,omitempty"`
}

// InboxList is an agent's inbox, newest first (agent.inbox.ls).
type InboxList struct {
	Messages []InboxMessage `json:"messages"`
	Count    int            `json:"count"`
}

// InboxProcessedResult is the processed mark agent.inbox.processed set.
type InboxProcessedResult struct {
	ID          string `json:"id"`
	ProcessedAt string `json:"processed_at"`
	Result      string `json:"result"`
}

// InboxClearResult counts what agent.inbox.clear deleted.
type InboxClearResult struct {
	DeletedDeliveries int64 `json:"deleted_deliveries"`
	DeletedMessages   int64 `json:"deleted_messages"`
}

// InboxReplyResult is the reply agent.inbox.reply published.
type InboxReplyResult struct {
	ID            string `json:"id"`
	Channel       string `json:"channel"`
	InReplyTo     string `json:"in_reply_to"`
	CorrelationID string `json:"correlation_id"`
	Replied       bool   `json:"replied"`
}

// InboxRequeueResult acknowledges agent.inbox.requeue.
type InboxRequeueResult struct {
	ID       string `json:"id"`
	Requeued bool   `json:"requeued"`
}

// ChatSummaryRow is one conversation in the chat.ls list.
type ChatSummaryRow struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Channel      string   `json:"channel"`
	Agent        string   `json:"agent"`
	LastTS       string   `json:"last_ts"`
	LastFrom     string   `json:"last_from"`
	LastType     string   `json:"last_type"`
	LastText     string   `json:"last_text"`
	Unread       int      `json:"unread"`
	ReadTS       string   `json:"read_ts"`
	Participants []string `json:"participants"`
}

// ChatList is the customer's chats, most recently active first (chat.ls).
type ChatList struct {
	Customer string           `json:"customer"`
	Chats    []ChatSummaryRow `json:"chats"`
	Count    int              `json:"count"`
}

// ChatMessage is one chat message with the channel it travelled on and the
// principal it speaks for.
type ChatMessage struct {
	ChannelMessage
	Channel string `json:"channel"`
	From    string `json:"from"`
}

// ChatFeed is one chat's messages, oldest first, with the customer's read
// mark (chat.messages).
type ChatFeed struct {
	Customer string        `json:"customer"`
	Agent    string        `json:"agent"`
	Chat     string        `json:"chat"`
	Channel  string        `json:"channel"`
	Kind     string        `json:"kind"`
	Title    string        `json:"title"`
	Messages []ChatMessage `json:"messages"`
	Count    int           `json:"count"`
	ReadTS   string        `json:"read_ts"`
}

// ChatReadResult is the customer's read mark after chat.read.
type ChatReadResult struct {
	Agent  string `json:"agent"`
	Chat   string `json:"chat"`
	ReadTS string `json:"read_ts"`
}

// ChatUnansweredList is the chat messages a principal has not answered yet
// (chat.unanswered).
type ChatUnansweredList struct {
	Chat      string        `json:"chat"`
	Principal string        `json:"principal"`
	Messages  []ChatMessage `json:"messages"`
	Count     int           `json:"count"`
}

// ChatParticipantRow is one principal taking part in a chat.
type ChatParticipantRow struct {
	Principal string `json:"principal"`
	Role      string `json:"role"`
	JoinedAt  string `json:"joined_at"`
	ReadTS    string `json:"read_ts"`
	Muted     bool   `json:"muted"`
}

// ChatParticipantList is a chat's participants (chat.participants).
type ChatParticipantList struct {
	Chat         string               `json:"chat"`
	Participants []ChatParticipantRow `json:"participants"`
	Count        int                  `json:"count"`
}

// ChatView is the chat a chat.create, chat.join or chat.leave leaves behind.
type ChatView struct {
	Chat         string   `json:"chat"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Channel      string   `json:"channel"`
	Agent        string   `json:"agent"`
	Participants []string `json:"participants"`
}

// AgentMessagesBatch is an agent's message batch size after
// agent.messages.batch.
type AgentMessagesBatch struct {
	Name          string `json:"name"`
	MessagesBatch int    `json:"messages_batch"`
}

// AgentMessagesMaxQueue is an agent's message queue limit after
// agent.messages.max-queue.
type AgentMessagesMaxQueue struct {
	Name             string `json:"name"`
	MessagesMaxQueue int    `json:"messages_max_queue"`
}

// messageThreading copies a message's optional threading fields.
func messageThreading(m bus.Message) MessageThreading {
	return MessageThreading{Kind: m.Kind, Subject: m.Subject, Data: m.Data, CorrelationID: m.CorrelationID,
		InReplyTo: m.InReplyTo, ReplyTo: m.ReplyTo, Deadline: m.Deadline}
}
