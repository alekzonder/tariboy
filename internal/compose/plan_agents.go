package compose

import (
	"fmt"
	"maps"
	"strings"
)

// change is one agent setting that differs between the file and the live row.
type change struct {
	kind   string
	issue  string // what `compose status` reports; "" when it reports nothing
	note   string
	fail   string
	call   Call
	fields []Field // what `up --dry-run` prints; never a secret value
}

func (p *planner) addChange(resource string, c change, deps ...int) int {
	return p.add(Action{Resource: resource, Kind: c.kind, Note: c.note, Fail: c.fail, Call: c.call, Fields: c.fields}, deps...)
}

// agentsPhase creates each agent without a group, converges the settings of
// an existing one, stops one the file disables, and sets the agent budget.
// Group membership follows in the groups phase and the start in the start
// phase. It names each create or re-provision "agent:NAME" and each setting
// "settings:NAME".
type agentsPhase struct{}

func (agentsPhase) name() string { return "agents" }

func (agentsPhase) plan(p *planner, f File, live *liveState) error {
	for _, name := range sortedKeys(f.Agents) {
		a := f.Agents[name]
		var err error
		if cur, exists := live.agents[name]; exists {
			err = p.planExistingAgent(f, name, a, cur, live.pending[name])
		} else {
			err = p.planNewAgent(f, name, a)
		}
		if err != nil {
			return err
		}
		if b := a.Budget; b != nil {
			if err := p.planBudget("agent:"+name, *b, live.budgets, p.needs("agent:"+name)...); err != nil {
				return err
			}
		}
	}
	return nil
}

// planNewAgent creates the agent stopped and without a group, then sets what
// the create endpoint cannot carry. Every step after the create depends on
// it, and the create depends on the build of its image.
func (p *planner) planNewAgent(f File, name string, a AgentSpec) error {
	id := "agent:" + name
	p.status(id, true, "agent %-14s MISSING (desired image=%s group=%s)", name, a.Image, a.Group)
	body, err := p.r.createBody(name, a)
	if err != nil {
		return err
	}
	create := p.add(Action{Resource: id, Kind: "create", Note: "creating agent " + name, Fail: "create agent " + name,
		Fields: createFields(a, body), Call: Call{"POST", "/api/agents", body}}, p.needs(p.imageKey(f, a.Image))...)
	p.provide(id, create)
	changes, err := loopChanges(name, a, nil)
	if err != nil {
		return err
	}
	settings, err := settingChanges(name, a, nil)
	if err != nil {
		return err
	}
	for _, c := range append(changes, settings...) {
		p.provide("settings:"+name, p.addChange(id, c, create))
	}
	p.planAgentSecrets(name, a, nil)
	return nil
}

// createFields is what `up --dry-run` prints for a create: the values of the
// body except env, of which it prints the keys only.
func createFields(a AgentSpec, body map[string]any) []Field {
	var out []Field
	for _, key := range []string{"image", "harness", "model", "effort", "cwd", "plugins"} {
		if v, ok := body[key].(string); ok && v != "" {
			out = append(out, Field{Name: key, To: v})
		}
	}
	if len(a.Env) > 0 {
		out = append(out, Field{Name: "env keys", To: strings.Join(sortedKeys(a.Env), ", ")})
	}
	return out
}

// planExistingAgent converges an agent the daemon already has.
//
// A stopped agent that a data-preserving `down` left without its image tree
// (image_removed) is re-provisioned in place first: the daemon re-unpacks the
// file's image over the retained history and restarts the loop with
// Enabled=true at once. The settings after it are diffed against that
// predicted row, so a file with loop.enabled:false drives the loop back off
// and ends where a first create would, and every later setting depends on the
// re-provision. A stopped agent that still has its image tree, such as one
// the file keeps stopped with loop.enabled:false, is converged like a running
// one: re-provisioning it would turn its loop on at every up.
//
// A harness change is stored here and applied by a restart (stop, start) of
// an enabled agent in the start phase, after the agent's other changes. A
// re-provision starts the agent with the harness anyway, and a file that
// disables the loop leaves the next start to apply it. An image change is set
// as the pending image, which the agent adopts on its next iteration without
// a restart.
func (p *planner) planExistingAgent(f File, name string, a AgentSpec, cur liveAgent, pending string) error {
	id := "agent:" + name
	kept := cur.State == "stopped" && cur.ImageRemoved
	if err := p.statusAgent(id, name, a, cur, pending); err != nil {
		return err
	}
	if harness := harnessChange(name, a, cur); harness != nil {
		p.provide("settings:"+name, p.addChange(id, *harness))
		cur.Harness = a.Harness.Type
	}
	image := p.needs(p.imageKey(f, a.Image))
	base := []int(nil)
	if kept {
		reprovision := p.add(Action{Resource: id, Kind: "reprovision", Note: fmt.Sprintf("reprovisioning stopped agent %s (image %s)", name, a.Image),
			Fail: "reprovision agent " + name, Fields: []Field{{Name: "reprovision", To: "image " + a.Image + ", starts the loop"}},
			Call: Call{"POST", "/api/agents/" + name + "/reprovision", map[string]any{"image": a.Image}}}, image...)
		p.provide(id, reprovision)
		base = []int{reprovision}
		cur.Image, cur.Enabled, cur.State, cur.reprovisioned = normalizeImageRef(a.Image), true, "running", true
	}
	changes, err := p.r.agentChanges(name, a, cur)
	if err != nil {
		return err
	}
	for _, c := range changes {
		deps := base
		if c.kind == "image" {
			if pending == normalizeImageRef(a.Image) {
				continue // already pending; adopted on the next iteration
			}
			deps = append(append([]int(nil), base...), image...)
		}
		p.provide("settings:"+name, p.addChange(id, c, deps...))
	}
	p.planAgentSecrets(name, a, &cur)
	return nil
}

// statusAgent records what `compose status` prints for an existing agent,
// diffed against the live row as it is.
func (p *planner) statusAgent(id, name string, a AgentSpec, cur liveAgent, pending string) error {
	changes, err := p.r.agentChanges(name, a, cur)
	if err != nil {
		return err
	}
	if h := harnessChange(name, a, cur); h != nil {
		changes = append([]change{*h}, changes...)
	}
	var issues []string
	if a.Group != "" && cur.Group != a.Group {
		issues = append(issues, fmt.Sprintf("group drift: have=%s want=%s", cur.Group, a.Group))
	}
	if c := instantSendersChange(name, a, &cur); c != nil {
		changes = append(changes, *c)
	}
	for _, c := range changes {
		switch {
		case c.issue == "":
		case c.kind == "image" && pending == normalizeImageRef(a.Image):
			issues = append(issues, fmt.Sprintf("image pending: have=%s want=%s (adopted on the next iteration)", cur.Image, pending))
		default:
			issues = append(issues, c.issue)
		}
	}
	if issue := pluginsPending(a, cur); issue != "" {
		issues = append(issues, issue)
	}
	if len(issues) > 0 {
		p.status(id, true, "agent %-14s DRIFT %s (state=%s)", name, strings.Join(issues, "; "), cur.State)
		return nil
	}
	p.status(id, false, "agent %-14s ok (state=%s group=%s)", name, cur.State, cur.Group)
	return nil
}

func loopDisabled(a AgentSpec) bool {
	return a.Loop != nil && a.Loop.Enabled != nil && !*a.Loop.Enabled
}

// agentChanges lists the settings of an existing agent that differ from the
// file, in apply order: image, cwd, env, interactive, loop, goal. Group
// membership is set in the groups phase, and a start in the start phase.
func (r *Runner) agentChanges(name string, a AgentSpec, cur liveAgent) ([]change, error) {
	var out []change
	if want := normalizeImageRef(a.Image); want != "" && cur.Image != want {
		out = append(out, change{kind: "image", issue: fmt.Sprintf("image drift: have=%s want=%s", cur.Image, want),
			note: fmt.Sprintf("setting image for agent %s -> %s (adopted on the next iteration)", name, want), fail: "set image " + name,
			call:   Call{"POST", "/api/agents/" + name + "/image", map[string]any{"image": want}},
			fields: []Field{{Name: "image", From: cur.Image, To: want + " (pending, adopted on the next iteration)"}}})
	}
	// cwd converges toward the file's value, defaulting to the compose file's
	// directory so the agent runs on the project.
	if want := effectiveCwd(a.Cwd, r.workdir); want != "" && cur.Cwd != want {
		body := map[string]any{"value": want}
		if a.CreateCwd {
			body["create"] = true
		}
		out = append(out, change{kind: "cwd", issue: fmt.Sprintf("cwd drift: have=%s want=%s", cur.Cwd, want),
			note: fmt.Sprintf("setting cwd for agent %s -> %s", name, want), fail: "set cwd " + name,
			call:   Call{"POST", "/api/agents/" + name + "/cwd", body},
			fields: []Field{{Name: "cwd", From: cur.Cwd, To: want}}})
	}
	out = append(out, envChanges(name, a, cur)...)
	loop, err := loopChanges(name, a, &cur)
	if err != nil {
		return nil, err
	}
	goal, err := goalChanges(name, a, cur)
	if err != nil {
		return nil, err
	}
	settings, err := settingChanges(name, a, &cur)
	if err != nil {
		return nil, err
	}
	out = append(append(append(out, loop...), goal...), settings...)
	return out, nil
}

// envChanges converges the env keys the file declares and, when the file has a
// harness block, the interactive mode. Both apply from the next iteration
// without a restart. With env_mode merge (the default) the env call sends
// the live env merged with the file's keys, so keys the file does not
// declare (for example image manifest env) are kept; with env_mode replace
// it sends the file's env, so the daemon removes those keys. The issue names
// keys only, never their values.
func envChanges(name string, a AgentSpec, cur liveAgent) []change {
	var out []change
	var drifted, extra []string
	for _, k := range sortedKeys(a.Env) {
		if have, ok := cur.Env[k]; !ok || have != a.Env[k] {
			drifted = append(drifted, k)
		}
	}
	want := make(map[string]string, len(cur.Env)+len(a.Env))
	if a.EnvMode == "replace" {
		for _, k := range sortedKeys(cur.Env) {
			if _, ok := a.Env[k]; !ok {
				extra = append(extra, k)
			}
		}
	} else {
		maps.Copy(want, cur.Env)
	}
	maps.Copy(want, a.Env)
	if len(drifted)+len(extra) > 0 {
		var issues []string
		var fields []Field
		if len(drifted) > 0 {
			issues = append(issues, "env drift: "+strings.Join(drifted, ","))
			fields = append(fields, Field{Name: "env keys", To: strings.Join(drifted, ", ")})
		}
		if len(extra) > 0 {
			issues = append(issues, "env extra: "+strings.Join(extra, ","))
			fields = append(fields, Field{Name: "env removed keys", To: strings.Join(extra, ", ")})
		}
		out = append(out, change{kind: "env", issue: strings.Join(issues, "; "),
			note: fmt.Sprintf("setting env for agent %s (applied from the next iteration)", name),
			fail: "set env " + name, call: Call{"POST", "/api/agents/" + name + "/env", map[string]any{"value": want}},
			fields: fields})
	}
	if h := a.Harness; h != nil && cur.Interactive != h.Interactive {
		out = append(out, change{kind: "interactive", issue: fmt.Sprintf("interactive drift: have=%t want=%t", cur.Interactive, h.Interactive),
			note: fmt.Sprintf("setting interactive for agent %s -> %t (applied from the next iteration)", name, h.Interactive),
			fail: "set interactive " + name, call: Call{"POST", "/api/agents/" + name + "/interactive", map[string]any{"value": h.Interactive}},
			fields: []Field{{Name: "interactive", From: fmt.Sprint(cur.Interactive), To: fmt.Sprint(h.Interactive)}}})
	}
	return out
}

func harnessChange(name string, a AgentSpec, cur liveAgent) *change {
	if a.Harness == nil || a.Harness.Type == "" || cur.Harness == a.Harness.Type {
		return nil
	}
	return &change{kind: "harness", issue: fmt.Sprintf("harness drift: have=%s want=%s", cur.Harness, a.Harness.Type),
		note: fmt.Sprintf("setting harness for agent %s -> %s", name, a.Harness.Type), fail: "set harness " + name,
		call:   Call{"POST", "/api/agents/" + name + "/harness", map[string]any{"value": a.Harness.Type}},
		fields: []Field{{Name: "harness", From: cur.Harness, To: a.Harness.Type}}}
}

// loopChanges lists the loop settings that differ from the file: timeout,
// interval, on-timeout, on-error, max-idle and, for an existing enabled agent
// (cur not nil) the file disables, a stop. With cur nil (a fresh create) every
// set field is a change. A start is planned in the start phase.
func loopChanges(name string, a AgentSpec, cur *liveAgent) ([]change, error) {
	var out []change
	from := func(v string) string {
		if cur == nil {
			return ""
		}
		return v
	}
	loopInt := func(route, label, unit string, have, want int) {
		if cur != nil && have == want {
			return
		}
		out = append(out, change{kind: route, issue: fmt.Sprintf("%s drift: have=%d%s want=%d%s", label, have, unit, want, unit),
			note: fmt.Sprintf("setting %s for agent %s -> %d%s", route, name, want, unit), fail: fmt.Sprintf("set %s %s", route, name),
			call:   Call{"POST", "/api/agents/" + name + "/loop/" + route, map[string]any{"value": want}},
			fields: []Field{{Name: label, From: from(fmt.Sprintf("%d%s", have, unit)), To: fmt.Sprintf("%d%s", want, unit)}}})
	}
	loopStr := func(route, field, have, want string) {
		if want == "" || (cur != nil && have == want) {
			return
		}
		out = append(out, change{kind: route, issue: fmt.Sprintf("%s drift: have=%s want=%s", field, have, want),
			note: fmt.Sprintf("setting %s for agent %s -> %s", field, name, want), fail: fmt.Sprintf("set %s %s", field, name),
			call:   Call{"POST", "/api/agents/" + name + "/loop/" + route, map[string]any{"value": want}},
			fields: []Field{{Name: field, From: from(have), To: want}}})
	}
	live := liveAgent{}
	if cur != nil {
		live = *cur
	}
	if a.effectiveTimeout() != "" {
		secs, err := a.effectiveTimeoutSeconds()
		if err != nil {
			return nil, fmt.Errorf("agent %s %w", name, err)
		}
		loopInt("timeout", "timeout", "s", live.TimeoutS, secs)
	}
	if a.Loop == nil {
		return out, nil
	}
	if secs, set, err := a.intervalSeconds(); err != nil {
		return nil, fmt.Errorf("agent %s loop %w", name, err)
	} else if set {
		loopInt("interval", "interval", "s", live.IntervalS, secs)
	}
	loopStr("on-timeout", "on_timeout", live.OnTimeout, a.Loop.OnTimeout)
	loopStr("on-error", "on_error", live.OnError, a.Loop.OnError)
	if a.Loop.MaxIdleIterations != nil {
		loopInt("max-idle", "max_idle_iterations", "", live.MaxIdleIterations, *a.Loop.MaxIdleIterations)
	}
	// loop.enabled is the agent's on/off switch: a stop converges it here,
	// a start in the start phase.
	if cur != nil && live.Enabled && loopDisabled(a) {
		out = append(out, change{kind: "stop", note: fmt.Sprintf("stop agent %s (loop.enabled=false)", name),
			fail: "stop agent " + name, call: Call{"POST", "/api/agents/" + name + "/stop", nil},
			fields: []Field{{Name: "loop.enabled", From: "true", To: "false"}}})
	}
	return out, nil
}

// goalChanges lists the Goal settings the file sets that differ from the
// live row. Omitted Goal fields keep their live values.
func goalChanges(name string, a AgentSpec, cur liveAgent) ([]change, error) {
	if a.Goal == nil {
		return nil, nil
	}
	var out []change
	if want := a.Goal.Enabled; want != nil && cur.GoalEnabled != *want {
		out = append(out, change{kind: "goal-enabled", issue: fmt.Sprintf("goal_enabled drift: have=%t want=%t", cur.GoalEnabled, *want),
			fail: "set goal enabled " + name, call: Call{"POST", "/api/agents/" + name + "/goal-enabled", map[string]any{"enabled": *want}},
			fields: []Field{{Name: "goal.enabled", From: fmt.Sprint(cur.GoalEnabled), To: fmt.Sprint(*want)}}})
	}
	seconds, set, err := a.goalWaitCustomerTimeoutSeconds()
	if err != nil {
		return nil, fmt.Errorf("agent %s goal: %w", name, err)
	}
	if set && cur.GoalWaitCustomerTimeoutS != seconds {
		out = append(out, change{kind: "goal-wait-customer-timeout",
			issue: fmt.Sprintf("goal_wait_customer_timeout_s drift: have=%d want=%d", cur.GoalWaitCustomerTimeoutS, seconds),
			fail:  "set goal wait customer timeout " + name, call: Call{"POST", "/api/agents/" + name + "/goal-wait-customer-timeout", map[string]any{"seconds": seconds}},
			fields: []Field{{Name: "goal.wait_customer_timeout", From: fmt.Sprintf("%ds", cur.GoalWaitCustomerTimeoutS), To: fmt.Sprintf("%ds", seconds)}}})
	}
	seconds, set, err = a.goalDeliveryCooldownSeconds()
	if err != nil {
		return nil, fmt.Errorf("agent %s goal: %w", name, err)
	}
	if set && cur.GoalDeliveryCooldownS != seconds {
		out = append(out, change{kind: "goal-delivery-cooldown",
			issue: fmt.Sprintf("goal_delivery_cooldown_s drift: have=%d want=%d", cur.GoalDeliveryCooldownS, seconds),
			fail:  "set goal delivery cooldown " + name, call: Call{"POST", "/api/agents/" + name + "/goal-delivery-cooldown", map[string]any{"seconds": seconds}},
			fields: []Field{{Name: "goal.delivery_cooldown", From: fmt.Sprintf("%ds", cur.GoalDeliveryCooldownS), To: fmt.Sprintf("%ds", seconds)}}})
	}
	return out, nil
}

// createBody is the POST /api/agents body for a new agent. It carries no
// group: the groups phase assigns the agent once every agent exists.
func (r *Runner) createBody(name string, a AgentSpec) (map[string]any, error) {
	// Always explicit: the create endpoint defaults interactive to true.
	body := map[string]any{"image": a.Image, "name": name, "interactive": a.Harness != nil && a.Harness.Interactive}
	if h := a.Harness; h != nil && h.Type != "" {
		body["harness"] = h.Type
	}
	settings, err := a.scalarSettings(liveAgentView{})
	if err != nil {
		return nil, fmt.Errorf("agent %s %w", name, err)
	}
	for _, s := range settings {
		body[s.create] = s.want
	}
	if a.CreateCwd {
		body["create_cwd"] = true
	}
	if len(a.Env) > 0 {
		body["env"] = kvList(a.Env)
	}
	if len(a.Plugins) > 0 {
		body["plugins"] = strings.Join(a.Plugins, ",")
	}
	if cwd := effectiveCwd(a.Cwd, r.workdir); cwd != "" {
		body["cwd"] = cwd
	}
	if a.Goal == nil {
		return body, nil
	}
	if a.Goal.Enabled != nil {
		body["goal_enabled"] = *a.Goal.Enabled
	}
	if seconds, set, err := a.goalWaitCustomerTimeoutSeconds(); err != nil {
		return nil, fmt.Errorf("agent %s goal: %w", name, err)
	} else if set {
		body["goal_wait_customer_timeout_s"] = seconds
	}
	if seconds, set, err := a.goalDeliveryCooldownSeconds(); err != nil {
		return nil, fmt.Errorf("agent %s goal: %w", name, err)
	} else if set {
		body["goal_delivery_cooldown_s"] = seconds
	}
	return body, nil
}
