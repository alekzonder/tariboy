package loop

import (
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/events"
)

// The UI refetches an agent's state on a "state" event instead of polling it
// often, so every manager path that changes the state must emit one.
func TestManagerEmitsStateEvents(t *testing.T) {
	m, as, _, _ := newManager(t, &fakeRunner{outcomes: []Outcome{{Status: "done"}}})
	var mu sync.Mutex
	var got []string
	m.cfg.Emit = func(e events.Event) {
		if e.Type == "state" {
			mu.Lock()
			got = append(got, e.Agent)
			mu.Unlock()
		}
	}
	if err := as.Create(agent.Agent{Name: "smoke", ImageRef: "basic:latest", HarnessType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("smoke"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "smoke" {
		t.Fatalf("state events = %v, want one for each change", got)
	}
}
