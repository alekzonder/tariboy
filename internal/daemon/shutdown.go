package daemon

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/store"
)

// shutdownStack holds the daemon's stop steps. Each startup phase pushes the
// stop of a resource right after creating it, so a resource always stops
// before the ones created earlier that it uses.
type shutdownStack struct {
	steps []shutdownStep
}

type shutdownStep struct {
	name string
	fn   func()
}

func (s *shutdownStack) push(name string, fn func()) {
	s.steps = append(s.steps, shutdownStep{name: name, fn: fn})
}

// run executes the steps last-pushed first, once. A panicking step is logged
// and the remaining steps still run, so the store always closes. observe, if
// set, gets each step's name before it runs.
func (s *shutdownStack) run(log *slog.Logger, observe func(string)) {
	steps := s.steps
	s.steps = nil
	for i := len(steps) - 1; i >= 0; i-- {
		if observe != nil {
			observe(steps[i].name)
		}
		func() {
			defer func() {
				if r := recover(); r != nil && log != nil {
					log.Error("shutdown step panicked", "step", steps[i].name, "panic", r)
				}
			}()
			steps[i].fn()
		}()
	}
}

// Recorder writes daemon events. Agent-scoped events go to the agent's durable
// audit.jsonl and its live stream; agentless ones stay in the store.
type Recorder struct {
	log   *slog.Logger
	store *store.Store
	audit *audit.Registry
	hub   *events.Hub
}

func (r Recorder) Record(agent, kind, data string) {
	if agent != "" && r.audit != nil {
		var m map[string]any
		if data != "" && data != "{}" {
			if err := json.Unmarshal([]byte(data), &m); err != nil {
				m = map[string]any{"data": data}
			}
		}
		r.audit.For(agent).Record(kind, "system", "", m)
	} else if err := r.store.AddEvent(agent, kind, data); err != nil {
		// Agentless (global) events — daemon_start/stop, plugin_* — stay in the DB.
		r.log.Warn("failed to record daemon event", "kind", kind, "err", err)
	}
	// Agent-scoped audit events also stream to any SSE subscribers.
	if r.hub != nil && agent != "" {
		r.hub.Emit(events.Event{Agent: agent, Type: "audit", Time: store.FormatTime(time.Now()),
			Data: map[string]any{"kind": kind, "data": data}})
	}
}
