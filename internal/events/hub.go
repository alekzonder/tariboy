// Package events is the in-memory fan-in for the SSE endpoint (spec §9): message
// publications, iteration lifecycle and audit events, filtered per agent + type.
package events

import (
	"sync"
	"sync/atomic"
)

// subscriberBuffer is how many events a subscriber may fall behind before the
// hub starts dropping its events.
const subscriberBuffer = 64

// Event is one live event for an agent. Type in {message, stream, iteration,
// audit, proxy}. Proxy is reserved for M5. A "reset" event tells a subscriber
// that Data["dropped"] of its events were lost because it fell behind.
type Event struct {
	Seq   uint64         `json:"seq,omitempty"`
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
	// missed counts events dropped since the last delivered one; a reset
	// event reports it once the buffer has room again.
	missed int64
}

type Hub struct {
	mu      sync.Mutex
	subs    map[*subscriber]struct{}
	seq     uint64
	dropped atomic.Int64
}

func NewHub() *Hub { return &Hub{subs: map[*subscriber]struct{}{}} }

// Subscribe registers a listener for one agent's events, filtered by type (empty
// = all). An empty agent watches every agent, which is what a host-wide socket
// such as the chat stream needs. The returned cancel closes the channel and
// unregisters.
func (h *Hub) Subscribe(agent string, types []string) (<-chan Event, func()) {
	set := map[string]bool{}
	for _, t := range types {
		if t != "" {
			set[t] = true
		}
	}
	s := &subscriber{agent: agent, types: set, ch: make(chan Event, subscriberBuffer)}
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
// the event (never blocks the publisher); the drop is counted, and the
// subscriber's next delivered event is preceded by a reset event.
func (h *Hub) Emit(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	e.Seq = h.seq
	for s := range h.subs {
		if s.agent != "" && s.agent != e.Agent {
			continue
		}
		if len(s.types) > 0 && !s.types[e.Type] {
			continue
		}
		if s.missed > 0 {
			reset := Event{Seq: e.Seq, Agent: e.Agent, Type: "reset", Time: e.Time,
				Data: map[string]any{"dropped": s.missed}}
			select {
			case s.ch <- reset:
				s.missed = 0
			default:
				h.drop(s)
				continue
			}
		}
		select {
		case s.ch <- e:
		default:
			h.drop(s)
		}
	}
}

func (h *Hub) drop(s *subscriber) {
	s.missed++
	h.dropped.Add(1)
}

// Dropped reports how many events slow subscribers lost since the hub started.
func (h *Hub) Dropped() int64 { return h.dropped.Load() }
