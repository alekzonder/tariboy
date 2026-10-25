package commands

import (
	"flag"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

var updateMessagesGolden = flag.Bool("update-messages-golden", false, "rewrite testdata/messages_responses.golden.json")

// messagesFixtureIDs names the messages the messages fixture publishes.
type messagesFixtureIDs struct {
	// plain is a bare note on chat:room; threaded carries every optional
	// message field; processed is already acknowledged; dead is dead-lettered.
	plain, threaded, processed, dead string
	// customerTS is the timestamp of the customer's message in the worker chat.
	customerTS string
}

// steppingClock returns a clock that advances one second per reading, so
// message ids and timestamps are deterministic and strictly ordered.
func steppingClock() func() time.Time {
	var mu sync.Mutex
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Second)
		return now
	}
}

// messagesFixture stores agents worker and other with the channel, inbox and
// chat states the messages responses cover: a plain, a threaded, a processed
// and a dead-lettered inbox message, a protected subscription, a watch with
// params and one without, and the worker's provisioned chats.
func messagesFixture(t *testing.T) (*registry.Ctx, *bus.Bus, messagesFixtureIDs) {
	t.Helper()
	s := storetest.Open(t)
	b := bus.New(s, steppingClock())
	c := &registry.Ctx{Store: s, Bus: b}
	as := agent.NewStore(s)
	for _, name := range []string{"worker", "other"} {
		if err := as.Create(agent.Agent{Name: name, ImageRef: "basic:latest"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []string{"chat:room", "group:dev:inbox"} {
		if _, err := b.Subscribe("worker", ch, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.SubscribeParams("worker", "issue-provider:issues", nil, nil, map[string]any{"q": "status:open"}); err != nil {
		t.Fatal(err)
	}
	bare, err := b.SubscribeParams("other", "issue-provider:issues", nil, nil, map[string]any{"q": "status:closed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE subscriptions SET params = NULL WHERE id = ?`, bare.ID); err != nil {
		t.Fatal(err)
	}
	publish := func(msg bus.Message) bus.Message {
		t.Helper()
		out, err := b.Publish(msg)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	var ids messagesFixtureIDs
	ids.plain = publish(bus.Message{Channel: "chat:room", Type: "note", Text: "one", Source: "operator"}).ID
	ids.threaded = publish(bus.Message{Channel: "chat:room", Type: "request.review", Text: "two", Source: "agent:other",
		Kind: "request", Subject: map[string]any{"pr": "7"}, Data: map[string]any{"n": float64(1)},
		ReplyTo: "agent:other:inbox", Deadline: "2026-07-15T00:00:00Z", ProducedByAgent: "other", ProducedInIteration: "other-1"}).ID
	ids.processed = publish(bus.Message{Channel: "chat:room", Type: "note", Text: "three", Source: "operator"}).ID
	if _, err := b.MarkProcessed("worker", ids.processed, "done"); err != nil {
		t.Fatal(err)
	}
	ids.dead = publish(bus.Message{Channel: "chat:room", Type: "note", Text: "four", Source: "operator"}).ID
	if _, err := s.DB.Exec(`UPDATE deliveries SET dlq = 1, attempts = 3, delivered_at = '2026-07-14T10:30:00Z' WHERE message_id = ?`, ids.dead); err != nil {
		t.Fatal(err)
	}
	ids.customerTS = publish(bus.Message{Channel: "agent:worker:inbox", Source: "user:customer", Type: "message", Text: "customer says hi"}).TS
	publish(bus.Message{Channel: "user:customer", Source: "system:tasks", Type: "task.question",
		Data: map[string]any{"from": "agent:worker"}, Text: "worker asks"})
	publish(bus.Message{Channel: "chat:dm:worker", Source: "user:customer", Type: "message", Text: "please answer",
		Subject: map[string]any{"topic": "deploy"}})
	return c, b, ids
}

// messagesGoldenCall is one handler call of the messages golden test.
type messagesGoldenCall struct {
	label, path string
	params      registry.Params
}

func messagesGoldenCalls(ids messagesFixtureIDs) []messagesGoldenCall {
	return []messagesGoldenCall{
		{"channel.ls", "channel.ls", registry.Params{}},
		{"channel.inspect", "channel.inspect", registry.Params{"channel": "chat:room"}},
		{"channel.tail", "channel.tail", registry.Params{"channel": "chat:room"}},
		{"channel.tail since", "channel.tail", registry.Params{"channel": "chat:room", "since": ids.plain}},
		{"channel.watches", "channel.watches", registry.Params{"channel": "issue-provider:issues"}},
		{"agent.subscriptions", "agent.subscriptions", registry.Params{"name": "worker"}},
		{"agent.subscribe plain", "agent.subscribe", registry.Params{"name": "worker", "channel": "chat:extra"}},
		{"agent.subscribe params", "agent.subscribe", registry.Params{"name": "worker", "channel": "plugin:issue-provider",
			"type": "run.finished", "matcher": map[string]any{"data.status": "failed"}, "params": map[string]any{"query": "Q1"}}},
		{"agent.unsubscribe", "agent.unsubscribe", registry.Params{"name": "worker", "channel": "chat:extra"}},
		{"message.send", "message.send", registry.Params{"channel": "chat:room", "type": "note", "text": "hi",
			"subject": "pr=9", "data": `{"k":"v"}`, "reply_to": "user:customer"}},
		{"agent.inbox.ls all", "agent.inbox.ls", registry.Params{"name": "worker"}},
		{"agent.inbox.processed", "agent.inbox.processed", registry.Params{"name": "worker", "id": ids.plain, "result": "looked"}},
		{"agent.inbox.reply", "agent.inbox.reply", registry.Params{"name": "worker", "id": ids.threaded, "text": "here"}},
		{"agent.inbox.requeue", "agent.inbox.requeue", registry.Params{"name": "worker", "id": ids.dead}},
		{"agent.inbox.ls processed", "agent.inbox.ls", registry.Params{"name": "worker", "status": "processed"}},
		{"agent.inbox.ls dlq empty", "agent.inbox.ls", registry.Params{"name": "worker", "status": "dlq"}},
		{"chat.ls", "chat.ls", registry.Params{}},
		{"chat.messages", "chat.messages", registry.Params{"agent": "worker"}},
		{"chat.read", "chat.read", registry.Params{"agent": "worker", "ts": ids.customerTS}},
		{"chat.unanswered", "chat.unanswered", registry.Params{"chat": "dm:worker", "principal": "agent:worker"}},
		{"chat.participants", "chat.participants", registry.Params{"chat": "dm:worker"}},
		{"chat.create", "chat.create", registry.Params{"id": "team-alpha", "title": "Team Alpha", "participants": "agent:worker"}},
		{"chat.join", "chat.join", registry.Params{"chat": "team-alpha", "principal": "agent:other", "role": "observer"}},
		{"chat.leave", "chat.leave", registry.Params{"chat": "team-alpha", "principal": "agent:other"}},
		{"agent.messages.batch", "agent.messages.batch", registry.Params{"name": "worker", "value": float64(7)}},
		{"agent.messages.max-queue", "agent.messages.max-queue", registry.Params{"name": "worker", "value": float64(70)}},
		{"agent.inbox.clear", "agent.inbox.clear", registry.Params{"name": "worker"}},
	}
}

// Catches a typed messages or chats response changing the JSON a client sees:
// the set of keys, their JSON types, and their values must match the recorded
// responses.
func TestMessagesResponsesMatchGolden(t *testing.T) {
	c, _, ids := messagesFixture(t)
	recorder := newGoldenRecorder(t, map[string]bool{})
	for _, call := range messagesGoldenCalls(ids) {
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, value)
	}
	recorder.check("messages_responses.golden.json", *updateMessagesGolden)
}
