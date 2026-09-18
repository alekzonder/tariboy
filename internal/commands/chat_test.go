package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
)

// The chat endpoints are the read side of the customer's conversations: one
// ranked list for the agent sidebar, one merged feed per agent, and a read mark
// the unread counter is measured against.
func TestChatListFeedAndReadMark(t *testing.T) {
	c, b := ctxWithBus(t)
	for _, msg := range []bus.Message{
		{Channel: "agent:worker:inbox", Source: "user:customer", Type: "message", Text: "customer says hi"},
		{Channel: "user:customer", Source: "system:tasks", Type: "task.question",
			Data: map[string]any{"from": "agent:worker"}, Text: "worker asks"},
		{Channel: "agent:worker:inbox", Source: "system:tasks", Type: "task.goal", Text: "own wake"},
	} {
		if _, err := b.Publish(msg); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := h(t, "chat.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	list := listed.(map[string]any)
	if list["customer"] != "user:customer" {
		t.Fatalf("customer = %v", list["customer"])
	}
	chats := list["chats"].([]map[string]any)
	if len(chats) != 1 || chats[0]["agent"] != "worker" || chats[0]["unread"] != 1 {
		t.Fatalf("chats = %#v", chats)
	}
	lastTS, _ := chats[0]["last_ts"].(string)
	if lastTS == "" || chats[0]["last_from"] != "agent:worker" {
		t.Fatalf("chat last message = %#v", chats[0])
	}

	fed, err := h(t, "chat.messages")(c, registry.Params{"agent": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	messages := fed.(map[string]any)["messages"].([]map[string]any)
	if len(messages) != 2 || messages[0]["from"] != "user:customer" || messages[1]["from"] != "agent:worker" {
		t.Fatalf("feed = %#v", messages)
	}

	// The feed carries the read mark the chat was opened at, which is what the
	// "New messages" separator is drawn from; before any read it is empty.
	if got := fed.(map[string]any)["read_ts"]; got != "" {
		t.Fatalf("read_ts before the first read = %v, want empty", got)
	}

	if _, err := h(t, "chat.read")(c, registry.Params{"agent": "worker", "ts": lastTS}); err != nil {
		t.Fatal(err)
	}
	fed, err = h(t, "chat.messages")(c, registry.Params{"agent": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if got := fed.(map[string]any)["read_ts"]; got != lastTS {
		t.Fatalf("read_ts after the read = %v, want %v", got, lastTS)
	}
	listed, err = h(t, "chat.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.(map[string]any)["chats"].([]map[string]any)[0]["unread"]; got != 0 {
		t.Fatalf("unread after read = %v, want 0", got)
	}

	// The mark only ever moves forward, so a late or duplicate request from a
	// second window cannot resurrect messages the customer has already seen.
	if _, err := h(t, "chat.read")(c, registry.Params{"agent": "worker", "ts": "2000-01-01T00:00:00.000000000Z"}); err != nil {
		t.Fatal(err)
	}
	value, _, err := c.Store.ConfigGet("chat_read_v1")
	if err != nil {
		t.Fatal(err)
	}
	if value != `{"worker":"`+lastTS+`"}` {
		t.Fatalf("read state = %s", value)
	}
}

// An operator message is the customer speaking, and it carries the customer's
// channel as its reply target so an agent's reply lands in the chat rather than
// back in its own inbox.
func TestMessageSendSpeaksAsTheCustomer(t *testing.T) {
	c, b := ctxWithBus(t)
	if _, err := h(t, "message.send")(c, registry.Params{
		"channel": "agent:worker:inbox", "type": "message", "text": "hi", "reply_to": "user:customer",
	}); err != nil {
		t.Fatal(err)
	}
	tail, _ := b.Tail("agent:worker:inbox", 10)
	if len(tail) != 1 || tail[0].Source != "user:customer" || tail[0].ReplyTo != "user:customer" {
		t.Fatalf("tail = %+v", tail)
	}
}
