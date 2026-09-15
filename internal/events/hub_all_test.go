package events

import "testing"

// One socket watches every chat on a host, so a subscriber that names no agent
// receives every agent's events instead of none.
func TestSubscriberWithoutAgentReceivesEveryAgent(t *testing.T) {
	hub := NewHub()
	all, cancelAll := hub.Subscribe("", []string{"message"})
	defer cancelAll()
	one, cancelOne := hub.Subscribe("worker", []string{"message"})
	defer cancelOne()

	hub.Emit(Event{Agent: "other", Type: "message"})
	hub.Emit(Event{Agent: "worker", Type: "message"})
	hub.Emit(Event{Agent: "worker", Type: "audit"})

	if got := len(all); got != 2 {
		t.Fatalf("agentless subscriber received %d events, want 2", got)
	}
	if got := len(one); got != 1 {
		t.Fatalf("per-agent subscriber received %d events, want 1", got)
	}
}
