package events

import "testing"

func TestHubFanoutAndFilter(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("smoke", []string{"message", "iteration"})
	defer cancel()

	h.Emit(Event{Agent: "smoke", Type: "message", Data: map[string]any{"id": "m1"}})
	h.Emit(Event{Agent: "smoke", Type: "audit"})   // filtered out
	h.Emit(Event{Agent: "other", Type: "message"}) // wrong agent
	h.Emit(Event{Agent: "smoke", Type: "iteration", Data: map[string]any{"phase": "start"}})

	got := []string{}
	for i := 0; i < 2; i++ {
		e := <-ch
		got = append(got, e.Type)
	}
	if got[0] != "message" || got[1] != "iteration" {
		t.Fatalf("received = %v", got)
	}
}

func TestHubEmptyTypesMeansAll(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("smoke", nil)
	defer cancel()
	h.Emit(Event{Agent: "smoke", Type: "audit"})
	if e := <-ch; e.Type != "audit" {
		t.Fatalf("empty filter should pass all, got %q", e.Type)
	}
}

func TestHubCancelStopsDelivery(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("smoke", nil)
	cancel()
	// Emitting after cancel must not panic and the channel is closed/idle.
	h.Emit(Event{Agent: "smoke", Type: "message"})
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("delivered after cancel")
		}
	default:
	}
}
