package events

import "testing"

func TestHubEmitAssignsIncreasingSeq(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("smoke", nil)
	defer cancel()
	h.Emit(Event{Agent: "smoke", Type: "message"})
	h.Emit(Event{Agent: "smoke", Type: "message"})
	first, second := <-ch, <-ch
	if first.Seq == 0 || second.Seq <= first.Seq {
		t.Fatalf("seq = %d, %d; want increasing and non-zero", first.Seq, second.Seq)
	}
}

func TestHubFullBufferCountsDropsAndSendsReset(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("smoke", []string{"message"})
	defer cancel()
	for i := 0; i < subscriberBuffer+3; i++ {
		h.Emit(Event{Agent: "smoke", Type: "message"})
	}
	if got := h.Dropped(); got != 3 {
		t.Fatalf("Dropped() = %d, want 3", got)
	}
	for i := 0; i < subscriberBuffer; i++ {
		<-ch
	}
	h.Emit(Event{Agent: "smoke", Type: "message"})
	reset := <-ch
	if reset.Type != "reset" || reset.Agent != "smoke" || reset.Data["dropped"] != int64(3) {
		t.Fatalf("first event after drain = %+v, want reset with dropped=3", reset)
	}
	if next := <-ch; next.Type != "message" {
		t.Fatalf("event after reset = %+v, want message", next)
	}
	h.Emit(Event{Agent: "smoke", Type: "message"})
	if next := <-ch; next.Type != "message" {
		t.Fatalf("second reset sent: %+v", next)
	}
}
