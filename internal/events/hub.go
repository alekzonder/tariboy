// Package events is the in-memory fan-in for the SSE endpoint (spec §9): message
// publications, iteration lifecycle and audit events, filtered per agent + type.
package events

import "sync"

// Event is one live event for an agent. Type in {message, stream, iteration,
// audit, proxy}. Proxy is reserved for M5.
type Event struct {
	Agent string         `json:"agent"`
	Type  string         `json:"type"`
	Time  string         `json:"time"`
	Data  map[string]any `json:"data,omitempty"`
}

// Source is the read side consumed by the SSE handler (satisfied by *Hub).
type Source interface {
	Subscribe(agent string, types []string) (<-chan Event, func())
}

type subscriber struct {
	agent string
	types map[string]bool
	ch    chan Event
}

type Hub struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[*subscriber]struct{}{}} }

// Subscribe registers a listener for one agent's events, filtered by type (empty
// = all). The returned cancel closes the channel and unregisters.
func (h *Hub) Subscribe(agent string, types []string) (<-chan Event, func()) {
	set := map[string]bool{}
	for _, t := range types {
		if t != "" {
			set[t] = true
		}
	}
	s := &subscriber{agent: agent, types: set, ch: make(chan Event, 64)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, s)
			h.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel
}

// Emit fans an event out to matching subscribers. A full subscriber buffer drops
// the event (never blocks the publisher).
func (h *Hub) Emit(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.agent != e.Agent {
			continue
		}
		if len(s.types) > 0 && !s.types[e.Type] {
			continue
		}
		select {
		case s.ch <- e:
		default:
		}
	}
}
