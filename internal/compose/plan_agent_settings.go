package compose

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/schedule"
)

// scalarSetting is one agent setting that the create request carries and one
// call converges on an existing agent. Every one applies from the next
// iteration without a restart.
type scalarSetting struct {
	field  string // the file field, as status and dry-run name it
	route  string // after /api/agents/NAME
	key    string // body key of the setting call
	create string // body key of the create request
	unit   string
	want   any
	have   any
	hidden bool // status names the field only: the text is long
	fold   bool // compared without case
}

// scalarSettings lists the scalar settings the file sets, with the live
// values of v. An empty model or effort is not a setting: the daemon reads an
// empty value as a read, so compose cannot clear one.
func (a AgentSpec) scalarSettings(v liveAgentView) ([]scalarSetting, error) {
	var out []scalarSetting
	if h := a.Harness; h != nil {
		if h.Model != "" {
			out = append(out, scalarSetting{field: "model", route: "/model", key: "value", create: "model", want: h.Model, have: v.Model})
		}
		if h.Effort != "" {
			out = append(out, scalarSetting{field: "effort", route: "/effort", key: "value", create: "effort", want: h.Effort, have: v.Effort})
		}
	}
	if s, set, err := a.hardTimeoutSeconds(); err != nil {
		return nil, err
	} else if set {
		out = append(out, scalarSetting{field: "hard_timeout", route: "/loop/hard-timeout", key: "value", create: "hard_timeout_s", unit: "s", want: s, have: v.HardTimeoutS})
	}
	if s, set, err := a.aiStallTimeoutSeconds(); err != nil {
		return nil, err
	} else if set {
		out = append(out, scalarSetting{field: "ai_stall_timeout", route: "/loop/ai-stall-timeout", key: "value", create: "ai_stall_timeout_s", unit: "s", want: s, have: v.AIStallTimeoutS})
	}
	if a.UserPrompt != nil {
		out = append(out, scalarSetting{field: "user_prompt", route: "/user-prompt", key: "text", create: "user_prompt", want: *a.UserPrompt, have: v.UserPrompt, hidden: true})
	}
	if a.MessagesBatch != nil {
		out = append(out, scalarSetting{field: "messages_batch", route: "/messages/batch", key: "value", create: "messages_batch", want: *a.MessagesBatch, have: v.MessagesBatch})
	}
	if a.MessagesMaxQueue != nil {
		out = append(out, scalarSetting{field: "messages_max_queue", route: "/messages/max-queue", key: "value", create: "messages_max_queue", want: *a.MessagesMaxQueue, have: v.MessagesMaxQueue})
	}
	if a.Alias != nil {
		out = append(out, scalarSetting{field: "alias", route: "/alias", key: "value", create: "alias", want: *a.Alias, have: v.Alias})
	}
	if a.Notes != nil {
		out = append(out, scalarSetting{field: "notes", route: "/notes", key: "value", create: "notes", want: *a.Notes, have: v.Notes, hidden: true})
	}
	if a.Color != nil {
		out = append(out, scalarSetting{field: "color", route: "/color", key: "value", create: "color", want: *a.Color, have: v.Color, fold: true})
	}
	return out, nil
}

func (s scalarSetting) change(name string) *change {
	have, want := fmt.Sprint(s.have), fmt.Sprint(s.want)
	if have == want || s.fold && strings.EqualFold(have, want) {
		return nil
	}
	c := &change{kind: s.field, issue: fmt.Sprintf("%s drift: have=%s%s want=%s%s", s.field, have, s.unit, want, s.unit),
		note: fmt.Sprintf("setting %s for agent %s (applied from the next iteration)", s.field, name), fail: fmt.Sprintf("set %s %s", s.field, name),
		call:   Call{"POST", "/api/agents/" + name + s.route, map[string]any{s.key: s.want}},
		fields: []Field{{Name: s.field, From: have + s.unit, To: want + s.unit}}}
	if s.hidden {
		c.issue, c.fields = s.field+" drift", []Field{{Name: s.field}}
	}
	return c
}

// settingChanges lists the agent.inspect settings, the plugins, the schedule
// and the retention that differ from the file. For a new agent (cur nil) it
// lists only what the create request cannot carry.
func settingChanges(name string, a AgentSpec, cur *liveAgent) ([]change, error) {
	var out []change
	live := liveAgent{}
	if cur != nil {
		live = *cur
		settings, err := a.scalarSettings(live.View)
		if err != nil {
			return nil, fmt.Errorf("agent %s %w", name, err)
		}
		for _, s := range settings {
			if c := s.change(name); c != nil {
				out = append(out, *c)
			}
		}
	}
	if c := pluginsChange(name, a, cur); c != nil {
		out = append(out, *c)
	}
	if c := scheduleChange(name, a, live.View.Schedule); c != nil {
		out = append(out, *c)
	}
	if c := retentionChange(name, a, live.Retention); c != nil {
		out = append(out, *c)
	}
	return out, nil
}

// pluginsChange sets the file's plugins. A new image replaces the plugins
// with its own when the agent adopts it, so while the agent's image differs
// from the file nothing is set: status reports the plugins as pending. A
// re-provision may change them, so they are set after one. The daemon gives
// a new agent on a schema-v2 image the plugins of the image and ignores the
// list of the create request, so they are also set after a create.
func pluginsChange(name string, a AgentSpec, cur *liveAgent) *change {
	if a.Plugins == nil {
		return nil
	}
	var have []string
	switch {
	case cur == nil:
	case cur.Image != normalizeImageRef(a.Image):
		return nil
	case !cur.reprovisioned && sameNames(cur.View.Plugins, a.Plugins):
		return nil
	default:
		have = cur.View.Plugins
	}
	return &change{kind: "plugins", issue: fmt.Sprintf("plugins drift: have=%s want=%s", nameList(have), nameList(a.Plugins)),
		note: fmt.Sprintf("setting plugins for agent %s (applied from the next iteration)", name), fail: "set plugins " + name,
		call:   Call{"POST", "/api/agents/" + name + "/plugins", map[string]any{"value": a.Plugins}},
		fields: []Field{{Name: "plugins", From: nameList(have), To: nameList(a.Plugins)}}}
}

// pluginsPending is the status issue of plugins the agent gets only after
// it adopts the file's image; "" when there is none.
func pluginsPending(a AgentSpec, cur liveAgent) string {
	if a.Plugins == nil || cur.State == "stopped" || cur.Image == normalizeImageRef(a.Image) {
		return ""
	}
	return fmt.Sprintf("plugins pending: applied after the agent adopts image %s; run up again", normalizeImageRef(a.Image))
}

func sameNames(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

func nameList(names []string) string {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	return "[" + strings.Join(sorted, ",") + "]"
}

// scheduleChange sets or clears loop.schedule. The daemon stores DTSTART and
// RRULE and fills DTSTART with the time of the write when it is missing, so
// a file without DTSTART is compared by its RRULE only.
func scheduleChange(name string, a AgentSpec, have string) *change {
	if a.Loop == nil || a.Loop.Schedule == nil {
		return nil
	}
	want := *a.Loop.Schedule
	body := map[string]any{"value": want}
	switch {
	case want == "" && have == "":
		return nil
	case want == "":
		body = map[string]any{"clear": true}
	case sameSchedule(have, want):
		return nil
	}
	from, to := strings.ReplaceAll(have, "\n", " "), want
	if from == "" {
		from = "(none)"
	}
	if to == "" {
		to = "(none)"
	}
	return &change{kind: "schedule", issue: fmt.Sprintf("schedule drift: have=%s want=%s", from, to),
		note: fmt.Sprintf("setting schedule for agent %s -> %s", name, to), fail: "set schedule " + name,
		call: Call{"POST", "/api/agents/" + name + "/loop/schedule", body}, fields: []Field{{Name: "schedule", From: from, To: to}}}
}

func sameSchedule(have, want string) bool {
	norm, err := schedule.NormalizeRRule(want, time.Now(), time.Local)
	if have == "" || err != nil {
		return false
	}
	if strings.Contains(strings.ToUpper(want), "DTSTART") {
		return norm == have
	}
	_, haveRule, _ := strings.Cut(have, "\n")
	_, wantRule, _ := strings.Cut(norm, "\n")
	return haveRule == wantRule
}

// retentionChange sets the agent's retention override. It sends the fields
// the file sets; the daemon keeps the others.
func retentionChange(name string, a AgentSpec, have *liveRetention) *change {
	w := a.Retention
	if w == nil {
		return nil
	}
	var cur liveRetention
	if have != nil {
		cur = *have
	}
	body := map[string]any{}
	var drift []string
	var fields []Field
	add := func(field string, want, have int64) {
		body[strings.ReplaceAll(field, "_", "-")] = want
		if want != have {
			drift = append(drift, fmt.Sprintf("%s have=%d want=%d", field, have, want))
			fields = append(fields, Field{Name: "retention " + field, From: fmt.Sprint(have), To: fmt.Sprint(want)})
		}
	}
	if w.KeepIterations != nil {
		add("keep_iterations", int64(*w.KeepIterations), int64(cur.KeepIterations))
	}
	if w.KeepDays != nil {
		add("keep_days", int64(*w.KeepDays), int64(cur.KeepDays))
	}
	if w.MaxBytes != nil {
		add("max_bytes", *w.MaxBytes, cur.MaxBytes)
	}
	if len(drift) == 0 {
		return nil
	}
	return &change{kind: "retention", issue: "retention drift: " + strings.Join(drift, ", "),
		note: "setting retention for agent " + name, fail: "set retention " + name,
		call: Call{"POST", "/api/agents/" + name + "/retention", body}, fields: fields}
}

// instantSendersChange sets mail_instant_senders. The daemon refuses a sender
// that does not exist, so it is planned in the groups phase, once every agent
// of the file exists. An empty list is sent as [""], which the daemon reads
// as an empty list.
func instantSendersChange(name string, a AgentSpec, cur *liveAgent) *change {
	if a.MailInstantSenders == nil {
		return nil
	}
	have := "(not set)"
	var live []string
	if cur != nil && cur.View.MailInstantSenders != nil {
		for _, s := range cur.View.MailInstantSenders {
			live = append(live, strings.TrimPrefix(s, "agent:"))
		}
		have = nameList(live)
		if sameNames(live, a.MailInstantSenders) {
			return nil
		}
	}
	senders := []string{""}
	if len(a.MailInstantSenders) > 0 {
		senders = nil
		for _, s := range a.MailInstantSenders {
			senders = append(senders, "agent:"+s)
		}
	}
	want := nameList(a.MailInstantSenders)
	return &change{kind: "mail_instant_senders", issue: fmt.Sprintf("mail_instant_senders drift: have=%s want=%s", have, want),
		note: "setting mail_instant_senders for agent " + name, fail: "set mail_instant_senders " + name,
		call:   Call{"POST", "/api/agents/" + name + "/messages/instant-senders", map[string]any{"senders": senders}},
		fields: []Field{{Name: "mail_instant_senders", From: have, To: want}}}
}

// planInstantSenders plans the instant senders of each agent of the file in
// the groups phase, after the create of the agent and of each sender the
// file declares.
func (p *planner) planInstantSenders(f File, live *liveState) {
	for _, name := range sortedKeys(f.Agents) {
		a := f.Agents[name]
		var cur *liveAgent
		if row, ok := live.agents[name]; ok {
			cur = &row
		}
		c := instantSendersChange(name, a, cur)
		if c == nil {
			continue
		}
		deps := p.needs("agent:" + name)
		for _, sender := range a.MailInstantSenders {
			deps = append(deps, p.needs("agent:"+sender)...)
		}
		p.addChange("agent:"+name, *c, deps...)
	}
}

// planAgentSecrets plans one action for each declared agent secret, by the
// rules of the queue secrets (planQueueSecrets). cur is nil for a new agent.
func (p *planner) planAgentSecrets(name string, a AgentSpec, cur *liveAgent) {
	id := "agent:" + name
	for _, key := range sortedKeys(a.Secrets) {
		from := a.Secrets[key].FromEnv
		varSet := os.Getenv(from) != ""
		has := cur != nil && cur.Secrets[key]
		switch {
		case varSet && has:
			p.status(id, false, "agent %-14s secret %s ok (value not compared)", name, key)
			p.add(Action{Resource: id, Kind: secretRefresh, run: p.r.setAgentSecret(name, key, from, false)}, p.needs(id)...)
		case varSet:
			if cur != nil {
				p.status(id, true, "agent %-14s secret %s MISSING", name, key)
			}
			p.add(Action{Resource: id, Kind: "secret", Fields: []Field{{Name: "secret " + key, To: "from " + from}},
				run: p.r.setAgentSecret(name, key, from, true)}, p.needs(id)...)
		case has:
			p.status(id, false, "agent %-14s secret %s ok (%s not set, kept)", name, key, from)
		default:
			if cur != nil {
				p.status(id, true, "agent %-14s secret %s MISSING (%s not set)", name, key, from)
			}
			p.add(Action{Resource: id, Kind: "secret", Fields: []Field{{Name: "secret " + key, To: from + " not set, fails"}},
				run: p.r.setAgentSecret(name, key, from, true)}, p.needs(id)...)
		}
	}
}

// setAgentSecret writes one agent secret from the variable from. Its errors
// never hold the value: a transport error that echoes the request is
// redacted.
func (r *Runner) setAgentSecret(name, key, from string, changed bool) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		value := os.Getenv(from)
		if value == "" {
			return nil, false, fmt.Errorf("secret %s of agent %s: %s is not set and the agent has no such secret\n"+
				"set %s, or set the secret with the value on standard input, for example:\n  printf '%%s' \"$VALUE\" | tariboy secret set %s %s",
				key, name, from, from, name, key)
		}
		if changed {
			r.logf("setting secret %s of agent %s", key, name)
		}
		if _, err := r.call.Call("POST", "/api/agents/"+name+"/secrets", map[string]any{"key": key, "value": value}); err != nil {
			return nil, false, fmt.Errorf("set secret %s of agent %s: %s", key, name, strings.ReplaceAll(err.Error(), value, "[redacted]"))
		}
		return nil, changed, nil
	}
}

// agentUndeclared lists the secrets of a declared agent that the file omits.
// up never removes them.
func agentUndeclared(name string, a AgentSpec, cur liveAgent) []string {
	var out []string
	for _, key := range sortedKeys(cur.Secrets) {
		if _, ok := a.Secrets[key]; !ok {
			out = append(out, fmt.Sprintf("%-10s %-14s secret %s not declared", "agent", name, key))
		}
	}
	return out
}
