package mail

import (
	"encoding/json"
	"sort"
	"strings"
)

// SetWakeHook wires the agent wake: fn receives the names of the agents that
// got an open letter, after the commit.
func (m *Mail) SetWakeHook(fn func(agents []string)) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.wake = fn
}

// SetPluginWakeHook wires the plugin inbox drainer wake: fn receives the names
// of the plugins that got an open letter, after the commit.
func (m *Mail) SetPluginWakeHook(fn func(plugins []string)) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.pluginWake = fn
}

// SetEventHook wires the live hint: fn is called once per written row.
func (m *Mail) SetEventHook(fn func(Hint)) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.event = fn
}

// SetAuditHook wires the audit sink. agent is the agent the event belongs to,
// or "" when no agent takes part.
func (m *Mail) SetAuditHook(fn func(agent, kind, dataJSON string)) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.audit = fn
}

// SetDeadlines wires request deadlines. Without it a deadline is stored on the
// letter but arms nothing.
func (m *Mail) SetDeadlines(d Deadlines) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.deadlines = d
}

func (m *Mail) deadlineSeam() Deadlines {
	m.hookMu.RLock()
	defer m.hookMu.RUnlock()
	return m.deadlines
}

// written is what one committed write hands to the hooks.
type written struct {
	letters []Letter // rows written; open ones wake their recipient
	audits  []auditEvent
}

type auditEvent struct {
	agent string
	kind  string
	data  map[string]any
}

// emit fires the hooks for a committed write. Never call it inside a
// transaction: the wake reads the rows it announces.
func (m *Mail) emit(w written) {
	m.hookMu.RLock()
	wake, pluginWake, event, audit := m.wake, m.pluginWake, m.event, m.audit
	m.hookMu.RUnlock()
	agents, plugins := map[string]bool{}, map[string]bool{}
	for _, l := range w.letters {
		if !l.Open() {
			continue
		}
		if name, ok := strings.CutPrefix(l.To, "agent:"); ok {
			agents[name] = true
		}
		if name, ok := strings.CutPrefix(l.To, "plugin:"); ok {
			plugins[name] = true
		}
	}
	if wake != nil && len(agents) > 0 {
		wake(sortedKeys(agents))
	}
	if pluginWake != nil && len(plugins) > 0 {
		pluginWake(sortedKeys(plugins))
	}
	if event != nil {
		for _, l := range w.letters {
			event(Hint{ID: l.ID, From: l.From, To: l.To, Kind: l.Kind, TS: l.CreatedAt})
		}
	}
	if audit != nil {
		for _, a := range w.audits {
			data, err := json.Marshal(a.data)
			if err != nil {
				data = []byte("{}")
			}
			audit(a.agent, a.kind, string(data))
		}
	}
}

// auditAgent is the agent an event about l belongs to: the agent that sent
// it, otherwise the agent that received it.
func auditAgent(l Letter) string {
	if name, ok := strings.CutPrefix(l.From, "agent:"); ok {
		return name
	}
	if name, ok := strings.CutPrefix(l.To, "agent:"); ok {
		return name
	}
	return ""
}

func letterAudit(kind string, l Letter, extra map[string]any) auditEvent {
	data := map[string]any{"id": l.ID, "from": l.From, "to": l.To, "kind": l.Kind}
	for k, v := range extra {
		data[k] = v
	}
	return auditEvent{agent: auditAgent(l), kind: kind, data: data}
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
