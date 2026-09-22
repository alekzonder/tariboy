package daemon

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/events"
)

// A message an agent sends to the customer has no agent recipient, so without
// attributing the event to its author the customer's side of a chat would never
// reach a live client.
func TestMessageEventCarriesSenderAndReachesTheAuthoredChat(t *testing.T) {
	hub := events.NewHub()
	stream, cancel := hub.Subscribe("", nil)
	defer cancel()

	emitMessageEvent(hub, bus.Message{
		ID: "m1", Channel: "user:customer", TS: "t", Type: "task.question",
		Source: "system:tasks", Data: map[string]any{"from": "agent:worker"},
	}, nil)

	select {
	case event := <-stream:
		if event.Agent != "worker" {
			t.Fatalf("event agent = %q, want worker", event.Agent)
		}
		if event.Data["from"] != "agent:worker" {
			t.Fatalf("event from = %v, want agent:worker", event.Data["from"])
		}
	default:
		t.Fatal("customer-channel publish emitted no event")
	}
}

// A chat publish names its chat, so a live client can refetch exactly that
// conversation instead of the whole list. The agent's own message into its
// personal chat reaches no agent delivery, so without attributing it to its
// author the customer's side of the chat would stay dark until the next poll.
func TestChatMessageEventNamesItsChatAndItsAuthor(t *testing.T) {
	hub := events.NewHub()
	stream, cancel := hub.Subscribe("", nil)
	defer cancel()

	emitMessageEvent(hub, bus.Message{
		ID: "m1", Channel: "chat:dm:worker", TS: "t", Type: "message",
		Source: "agent:worker", Data: map[string]any{"from": "agent:worker"},
	}, nil)

	select {
	case event := <-stream:
		if event.Agent != "worker" {
			t.Fatalf("event agent = %q, want worker", event.Agent)
		}
		if event.Data["chat"] != "dm:worker" {
			t.Fatalf("event chat = %v, want dm:worker", event.Data["chat"])
		}
	default:
		t.Fatal("chat publish emitted no event")
	}
	select {
	case event := <-stream:
		t.Fatalf("one publication is one event, got a second: %+v", event)
	default:
	}
}

// A message outside any chat carries no chat id, so a consumer cannot mistake
// a channel name for one.
func TestNonChatMessageEventCarriesNoChatID(t *testing.T) {
	hub := events.NewHub()
	stream, cancel := hub.Subscribe("", nil)
	defer cancel()

	emitMessageEvent(hub, bus.Message{
		ID: "m2", Channel: "agent:worker:inbox", TS: "t", Type: "message",
		Source: "user:customer", Data: map[string]any{"from": "user:customer"},
	}, nil)

	select {
	case event := <-stream:
		if _, present := event.Data["chat"]; present {
			t.Fatalf("a non-chat channel must carry no chat id, got %v", event.Data["chat"])
		}
	default:
		t.Fatal("inbox publish emitted no event")
	}
}
