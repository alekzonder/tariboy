package audit

import (
	"sync"
	"time"
)

// Registry hands out one shared *Log per agent. All writers for an agent (the
// daemon's recordEvent, the loop engine's lifecycle sink, and the runner's log
// tailer) MUST obtain their Log from the same Registry so the seq counter and
// file handle stay consistent — two independent *Log values over the same file
// would each seed seq from the last line and then collide.
type Registry struct {
	mu      sync.Mutex
	clock   func() time.Time
	pathFor func(agent string) string
	logs    map[string]*Log
}

// NewRegistry builds a Registry. pathFor maps an agent name to its audit.jsonl
// path (the daemon passes agentdir.Layout.AuditLog). clock defaults to time.Now.
func NewRegistry(pathFor func(agent string) string, clock func() time.Time) *Registry {
	if clock == nil {
		clock = time.Now
	}
	return &Registry{clock: clock, pathFor: pathFor, logs: map[string]*Log{}}
}

// For returns the shared Log for agent, creating it on first use.
func (r *Registry) For(agent string) *Log {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.logs[agent]; ok {
		return l
	}
	l := Open(r.pathFor(agent), r.clock)
	r.logs[agent] = l
	return l
}
