package tasks

import (
	"context"
	"sync"
)

type EventHint struct {
	Type         string `json:"type"`
	Sequence     int64  `json:"sequence"`
	Kind         string `json:"kind,omitempty"`
	TaskKey      string `json:"task_key,omitempty"`
	Queue        string `json:"queue,omitempty"`
	TaskRevision int64  `json:"task_revision,omitempty"`
}

type EventSource interface {
	Replay(context.Context, Actor, int64, int) ([]EventHint, bool, error)
	Subscribe() (<-chan struct{}, func())
}

type Hub struct {
	service *Service
	mu      sync.Mutex
	nextID  int
	subs    map[int]chan struct{}
}

func NewHub(service *Service) *Hub {
	return &Hub{service: service, subs: map[int]chan struct{}{}}
}

func (h *Hub) Replay(ctx context.Context, actor Actor, after int64, limit int) ([]EventHint, bool, error) {
	reset := false
	if after > 0 {
		var minimum int64
		if err := h.service.db.QueryRowContext(ctx,
			`SELECT COALESCE(MIN(sequence), 0) FROM task_events`).Scan(&minimum); err != nil {
			return nil, false, err
		}
		reset = minimum > 0 && minimum > after+1
	}
	events, err := h.service.ListEvents(ctx, actor, "", after, limit)
	if err != nil {
		return nil, false, err
	}
	hints := make([]EventHint, 0, len(events))
	for _, event := range events {
		hints = append(hints, EventHint{
			Type: "event", Sequence: event.Sequence, Kind: event.Kind,
			TaskKey: event.TaskKey, Queue: event.Queue, TaskRevision: event.TaskRevision,
		})
	}
	return hints, reset, nil
}

func (h *Hub) Subscribe() (<-chan struct{}, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	ch := make(chan struct{}, 1)
	h.subs[id] = ch
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, id)
			close(ch)
			h.mu.Unlock()
		})
	}
}

func (h *Hub) Nudge() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
