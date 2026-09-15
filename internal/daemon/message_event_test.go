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
