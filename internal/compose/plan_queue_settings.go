package compose

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// secretRefresh is the kind of the action that writes a secret the queue
// already has. The daemon never returns a secret value, so up cannot compare
// it and writes it again; the action reports ok and dry-run does not list it.
const secretRefresh = "secret refresh"

// planQueueEnv converges the declared env keys of one queue. The queues phase
// sets every key but CUSTOMER_AGENT_POOL; the bindings phase sets that one
// after the pools, because the daemon accepts it on a bound queue only when
// the pool exists. Env values are never printed, only keys.
func (p *planner) planQueueEnv(prefix string, want TaskQueueSpec, live *liveState, customerPool bool) {
	id := "queue:" + prefix
	_, exists := live.queues[prefix]
	have := live.queueSettings[prefix].env
	for _, key := range sortedKeys(want.Env) {
		if (key == tasks.CustomerAgentPoolEnv) != customerPool {
			continue
		}
		value, found := have[key]
		switch {
		case found && value == want.Env[key]:
			p.status(id, false, "task queue %-10s env %s ok", prefix, key)
			continue
		case found:
			p.status(id, true, "task queue %-10s env %s drift", prefix, key)
		case exists:
			p.status(id, true, "task queue %-10s env %s MISSING", prefix, key)
		}
		deps := p.needs(id)
		if customerPool {
			deps = append(deps, p.needs("pool:"+prefix)...)
		}
		p.add(Action{Resource: id, Kind: "env", Note: fmt.Sprintf("setting env %s of task queue %s", key, prefix),
			Fail: fmt.Sprintf("set env %s of task queue %s", key, prefix), Fields: []Field{{Name: "env " + key}},
			Call: Call{"PUT", "/api/task-queues/" + prefix + "/env/" + key, map[string]any{"value": want.Env[key]}}}, deps...)
	}
}

// planQueueSecrets plans one action for each declared secret. The value is
// read from secrets.KEY.from_env only when the action runs, so it never
// reaches the plan, a status line or dry-run; at plan time only whether the
// variable is set is read. An empty variable counts as not set.
func (p *planner) planQueueSecrets(prefix string, want TaskQueueSpec, live *liveState) {
	id := "queue:" + prefix
	settings := live.queueSettings[prefix]
	for _, key := range sortedKeys(want.Secrets) {
		from := want.Secrets[key].FromEnv
		varSet := os.Getenv(from) != ""
		switch {
		case varSet && settings.secrets[key]:
			p.status(id, false, "task queue %-10s secret %s ok (value not compared)", prefix, key)
			p.add(Action{Resource: id, Kind: secretRefresh, run: p.r.setQueueSecret(prefix, key, from, false)}, p.needs(id)...)
		case varSet:
			if _, exists := live.queues[prefix]; exists {
				p.status(id, true, "task queue %-10s secret %s MISSING", prefix, key)
			}
			p.add(Action{Resource: id, Kind: "secret", Fields: []Field{{Name: "secret " + key, To: "from " + from}},
				run: p.r.setQueueSecret(prefix, key, from, true)}, p.needs(id)...)
		case settings.secrets[key]:
			p.status(id, false, "task queue %-10s secret %s ok (%s not set, kept)", prefix, key, from)
		case settings.globalEnv[key]:
			p.status(id, false, "task queue %-10s secret %s ok (daemon environment)", prefix, key)
		default:
			p.status(id, true, "task queue %-10s secret %s MISSING (%s not set)", prefix, key, from)
			p.add(Action{Resource: id, Kind: "secret", Fields: []Field{{Name: "secret " + key, To: from + " not set, fails"}},
				run: p.r.setQueueSecret(prefix, key, from, true)}, p.needs(id)...)
		}
	}
}

// setQueueSecret writes one secret from the variable from. Its errors never
// hold the value: a transport error that echoes the request is redacted.
func (r *Runner) setQueueSecret(prefix, key, from string, changed bool) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		value := os.Getenv(from)
		if value == "" {
			return nil, false, fmt.Errorf("secret %s of task queue %s: %s is not set and the queue has no such secret\n"+
				"set %s, or set the secret with the value on standard input, for example:\n  printf '%%s' \"$VALUE\" | ttasks queue secret set %s %s",
				key, prefix, from, from, prefix, key)
		}
		if changed {
			r.logf("setting secret %s of task queue %s", key, prefix)
		}
		if _, err := r.call.Call("PUT", "/api/task-queues/"+prefix+"/secrets/"+key, map[string]any{"value": value}); err != nil {
			return nil, false, fmt.Errorf("set secret %s of task queue %s: %s", key, prefix, strings.ReplaceAll(err.Error(), value, "[redacted]"))
		}
		return nil, changed, nil
	}
}

// planResponsibleAgent sets the responsible agent after the agents exist.
// The queues phase may have raised the queue's revision, so the action reads
// the queue when it runs and patches with the fresh revision.
func (p *planner) planResponsibleAgent(prefix string, want TaskQueueSpec, live *liveState) {
	if want.ResponsibleAgent == "" {
		return
	}
	id := "queue:" + prefix
	queue, exists := live.queues[prefix]
	if exists && queue.ResponsibleAgent == want.ResponsibleAgent {
		p.status(id, false, "task queue %-10s responsible_agent %s ok", prefix, want.ResponsibleAgent)
		return
	}
	if exists {
		p.status(id, true, "task queue %-10s responsible_agent drift: have=%s want=%s", prefix, queue.ResponsibleAgent, want.ResponsibleAgent)
	}
	var deps []int
	for _, i := range p.actionsOf(id) {
		if k := p.plan.Actions[i].Kind; k == "create" || k == "update" {
			deps = append(deps, i)
		}
	}
	p.add(Action{Resource: id, Kind: "update", Fields: []Field{{Name: "responsible_agent", From: queue.ResponsibleAgent, To: want.ResponsibleAgent}},
		run: p.r.setResponsibleAgent(prefix, want.ResponsibleAgent)}, deps...)
}

func (r *Runner) setResponsibleAgent(prefix, agent string) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		var queue tasks.Queue
		if err := r.get("/api/task-queues/"+prefix, &queue); err != nil {
			return nil, false, fmt.Errorf("read task queue %s: %w", prefix, err)
		}
		if queue.ResponsibleAgent == agent {
			return nil, false, nil
		}
		r.logf("setting responsible agent %s of task queue %s", agent, prefix)
		if _, err := r.call.Call("PATCH", "/api/task-queues/"+prefix, map[string]any{"responsible_agent": agent, "revision": queue.Revision}); err != nil {
			return nil, false, fmt.Errorf("set responsible agent of task queue %s: %w", prefix, err)
		}
		return nil, true, nil
	}
}

// planTriggers creates each declared trigger the queue lacks, matched by
// pattern, correlation key and action. The API cannot change a trigger, so a
// trigger changed in the file is created anew and the old one stays. They
// follow the workflow binding, so a task from a new letter follows the
// workflow.
func (p *planner) planTriggers(prefix string, want TaskQueueSpec, live *liveState) {
	id := "queue:" + prefix
	_, exists := live.queues[prefix]
	for _, t := range want.Triggers {
		t = t.normalized()
		if hasTrigger(live.queueSettings[prefix].triggers, t) {
			p.status(id, false, "task queue %-10s trigger %s ok", prefix, t)
			continue
		}
		if exists {
			p.status(id, true, "task queue %-10s trigger %s MISSING", prefix, t)
		}
		p.add(Action{Resource: id, Kind: "trigger", Note: fmt.Sprintf("creating trigger %s of task queue %s", t, prefix),
			Fail: fmt.Sprintf("create trigger %s of task queue %s", t, prefix), Fields: []Field{{Name: "trigger " + t.String()}},
			Call: Call{"POST", "/api/task-queues/" + prefix + "/workflow-triggers",
				map[string]any{"pattern": t.Pattern, "correlation_key": t.CorrelationKey, "action": t.Action}}},
			p.actionsOf(id)...)
	}
}

func hasTrigger(live []tasks.QueueWorkflowTrigger, t TriggerSpec) bool {
	for _, l := range live {
		if l.Pattern == t.Pattern && l.CorrelationKey == t.CorrelationKey && l.Action == t.Action {
			return true
		}
	}
	return false
}

func declaresTrigger(want []TriggerSpec, l tasks.QueueWorkflowTrigger) bool {
	for _, t := range want {
		if hasTrigger([]tasks.QueueWorkflowTrigger{l}, t.normalized()) {
			return true
		}
	}
	return false
}

// queueUndeclared lists the pools, env keys, secrets and triggers of a
// declared queue that the file omits. up never removes them.
func queueUndeclared(prefix string, want TaskQueueSpec, live *liveState) []string {
	var out []string
	line := func(item string) {
		out = append(out, fmt.Sprintf("%-10s %-14s %s not declared", "task queue", prefix, item))
	}
	for _, pool := range sortedKeys(live.pools[prefix]) {
		if _, ok := want.Pools[pool]; !ok {
			line("pool " + pool)
		}
	}
	settings := live.queueSettings[prefix]
	for _, key := range sortedKeys(settings.env) {
		if _, ok := want.Env[key]; !ok {
			line("env " + key)
		}
	}
	for _, key := range sortedKeys(settings.secrets) {
		if _, ok := want.Secrets[key]; !ok {
			line("secret " + key)
		}
	}
	for _, l := range settings.triggers {
		if !declaresTrigger(want.Triggers, l) {
			line("trigger " + TriggerSpec{Pattern: l.Pattern, CorrelationKey: l.CorrelationKey, Action: l.Action}.String())
		}
	}
	return out
}

// readQueueSettings reads the env, secret names and triggers of one queue.
func (r *Runner) readQueueSettings(prefix string) (liveQueueSettings, error) {
	out := liveQueueSettings{env: map[string]string{}, secrets: map[string]bool{}, globalEnv: map[string]bool{}}
	var env struct {
		Env []tasks.QueueEnvVar `json:"env"`
	}
	if err := r.get("/api/task-queues/"+prefix+"/env", &env); err != nil {
		return out, fmt.Errorf("env: %w", err)
	}
	for _, item := range env.Env {
		out.env[item.Key] = item.Value
	}
	var secrets struct {
		Secrets   []tasks.QueueSecretInfo `json:"secrets"`
		GlobalEnv []string                `json:"global_env"`
	}
	if err := r.get("/api/task-queues/"+prefix+"/secrets", &secrets); err != nil {
		return out, fmt.Errorf("secrets: %w", err)
	}
	for _, item := range secrets.Secrets {
		out.secrets[item.Key] = true
	}
	for _, name := range secrets.GlobalEnv {
		out.globalEnv[name] = true
	}
	var triggers struct {
		Items []tasks.QueueWorkflowTrigger `json:"items"`
	}
	if err := r.get("/api/task-queues/"+prefix+"/workflow-triggers", &triggers); err != nil {
		return out, fmt.Errorf("triggers: %w", err)
	}
	out.triggers = triggers.Items
	return out, nil
}
