package compose

import (
	"fmt"
	"strings"
)

// change is one agent setting that differs between the file and the live row.
type change struct {
	kind  string
	issue string // what `compose status` reports; "" when it reports nothing
	note  string
	fail  string
	call  Call
}

func (p *planner) addChange(resource string, c change, deps ...int) int {
	return p.add(Action{Resource: resource, Kind: c.kind, Note: c.note, Fail: c.fail, Call: c.call}, deps...)
}

func (p *planner) planAgents(f File, live map[string]liveAgent) error {
	for _, name := range sortedKeys(f.Agents) {
		a := f.Agents[name]
		var err error
		if cur, exists := live[name]; exists {
			err = p.planExistingAgent(name, a, cur)
		} else {
			err = p.planNewAgent(name, a)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// planNewAgent creates the agent, sets what the create endpoint cannot carry,
// and starts it unless the file disables its loop. Every step after the create
// depends on it.
func (p *planner) planNewAgent(name string, a AgentSpec) error {
	id := "agent:" + name
	p.status(id, true, "agent %-14s MISSING (desired image=%s group=%s)", name, a.Image, a.Group)
	body, err := p.r.createBody(name, a)
	if err != nil {
		return err
	}
	create := p.add(Action{Resource: id, Kind: "create", Note: "creating agent " + name, Fail: "create agent " + name,
		Call: Call{"POST", "/api/agents", body}}, createDep(p.groupCreate, a.Group))
	p.agentCreate[name] = create
	changes, err := loopChanges(name, a, nil)
	if err != nil {
		return err
	}
	for _, c := range changes {
		p.addChange(id, c, create)
	}
	if a.Loop == nil || a.Loop.Enabled == nil || *a.Loop.Enabled {
		p.add(Action{Resource: id, Kind: "start", Fail: "start agent " + name,
			Call: Call{"POST", "/api/agents/" + name + "/start", nil}}, create)
	}
	return nil
}

// planExistingAgent converges an agent the daemon already has.
//
// A stopped agent that still has its row (after a data-preserving `down`) is
// re-provisioned in place first: the daemon re-unpacks the file's image over
// the retained history and restarts the loop with Enabled=true. The settings
// after it are diffed against that predicted row, so a file with
// loop.enabled:false drives the loop back off and ends where a first create
// would, and every later setting depends on the re-provision.
//
// A harness change is stored first and applied by a restart (stop, start) of
// an enabled agent. The restart runs after the agent's other changes and
// depends on them, so the restarted iteration starts with every new setting.
// A re-provision starts the agent with the harness anyway, and a file that
// disables the loop leaves the next start to apply it. An image change
// is set as the pending image, which the agent adopts on its next iteration
// without a restart.
func (p *planner) planExistingAgent(name string, a AgentSpec, cur liveAgent) error {
	id := "agent:" + name
	stopped := cur.State == "stopped"
	pending := ""
	if want := normalizeImageRef(a.Image); !stopped && cur.Image != want {
		// A failed read only means the image is set again.
		pending, _ = p.r.pendingImage(name)
	}
	if err := p.statusAgent(id, name, a, cur, pending); err != nil {
		return err
	}
	harness := harnessChange(name, a, cur)
	set := -1
	if harness != nil {
		set = p.addChange(id, *harness)
		cur.Harness = a.Harness.Type
	}
	base := -1
	restart := harness != nil && cur.Enabled && !loopDisabled(a)
	if stopped {
		base = p.add(Action{Resource: id, Kind: "reprovision", Note: fmt.Sprintf("reprovisioning stopped agent %s (image %s)", name, a.Image),
			Fail: "reprovision agent " + name, Call: Call{"POST", "/api/agents/" + name + "/reprovision", map[string]any{"image": a.Image}}})
		cur.Image, cur.Enabled, cur.State = normalizeImageRef(a.Image), true, "running"
		restart = false
	}
	changes, err := p.r.agentChanges(name, a, cur)
	if err != nil {
		return err
	}
	restartDeps := []int{set}
	for _, c := range changes {
		if c.kind == "image" && pending == normalizeImageRef(a.Image) {
			continue // already pending; adopted on the next iteration
		}
		restartDeps = append(restartDeps, p.addChange(id, c, base))
	}
	if restart {
		stop := p.add(Action{Resource: id, Kind: "stop", Note: fmt.Sprintf("restarting agent %s to apply harness %s", name, a.Harness.Type),
			Fail: "stop agent " + name, Call: Call{"POST", "/api/agents/" + name + "/stop", nil}}, restartDeps...)
		p.add(Action{Resource: id, Kind: "start", Fail: "start agent " + name,
			Call: Call{"POST", "/api/agents/" + name + "/start", nil}}, stop)
	}
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
	for _, c := range changes {
		switch {
		case c.issue == "":
		case c.kind == "image" && pending == normalizeImageRef(a.Image):
			issues = append(issues, fmt.Sprintf("image pending: have=%s want=%s (adopted on the next iteration)", cur.Image, pending))
		default:
			issues = append(issues, c.issue)
		}
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
// file, in apply order: group, image, cwd, loop, goal.
func (r *Runner) agentChanges(name string, a AgentSpec, cur liveAgent) ([]change, error) {
	var out []change
	if a.Group != "" && cur.Group != a.Group {
		out = append(out, change{kind: "assign", issue: fmt.Sprintf("group drift: have=%s want=%s", cur.Group, a.Group),
			note: fmt.Sprintf("assigning agent %s to group %s", name, a.Group), fail: "assign agent " + name,
			call: Call{"POST", "/api/groups/" + a.Group + "/assign", map[string]any{"agent": name}}})
	}
	if want := normalizeImageRef(a.Image); want != "" && cur.Image != want {
		out = append(out, change{kind: "image", issue: fmt.Sprintf("image drift: have=%s want=%s", cur.Image, want),
			note: fmt.Sprintf("setting image for agent %s -> %s (adopted on the next iteration)", name, want), fail: "set image " + name,
			call: Call{"POST", "/api/agents/" + name + "/image", map[string]any{"image": want}}})
	}
	// cwd converges toward the file's value, defaulting to the compose file's
	// directory so the agent runs on the project.
	if want := effectiveCwd(a.Cwd, r.workdir); want != "" && cur.Cwd != want {
		out = append(out, change{kind: "cwd", issue: fmt.Sprintf("cwd drift: have=%s want=%s", cur.Cwd, want),
			note: fmt.Sprintf("setting cwd for agent %s -> %s", name, want), fail: "set cwd " + name,
			call: Call{"POST", "/api/agents/" + name + "/cwd", map[string]any{"value": want}}})
	}
	loop, err := loopChanges(name, a, &cur)
	if err != nil {
		return nil, err
	}
	goal, err := goalChanges(name, a, cur)
	if err != nil {
		return nil, err
	}
	out = append(append(out, loop...), goal...)
	return out, nil
}

func harnessChange(name string, a AgentSpec, cur liveAgent) *change {
	if a.Harness == nil || a.Harness.Type == "" || cur.Harness == a.Harness.Type {
		return nil
	}
	return &change{kind: "harness", issue: fmt.Sprintf("harness drift: have=%s want=%s", cur.Harness, a.Harness.Type),
		note: fmt.Sprintf("setting harness for agent %s -> %s", name, a.Harness.Type), fail: "set harness " + name,
		call: Call{"POST", "/api/agents/" + name + "/harness", map[string]any{"value": a.Harness.Type}}}
}

// loopChanges lists the loop settings that differ from the file: timeout,
// interval, on-timeout, on-error, max-idle and, for an existing agent (cur not
// nil), enabled. With cur nil (a fresh create) every set field is a change;
// the create body carries the initial enabled value.
func loopChanges(name string, a AgentSpec, cur *liveAgent) ([]change, error) {
	var out []change
	loopInt := func(route, label, unit string, have, want int) {
		if cur != nil && have == want {
			return
		}
		out = append(out, change{kind: route, issue: fmt.Sprintf("%s drift: have=%d%s want=%d%s", label, have, unit, want, unit),
			note: fmt.Sprintf("setting %s for agent %s -> %d%s", route, name, want, unit), fail: fmt.Sprintf("set %s %s", route, name),
			call: Call{"POST", "/api/agents/" + name + "/loop/" + route, map[string]any{"value": want}}})
	}
	loopStr := func(route, field, have, want string) {
		if want == "" || (cur != nil && have == want) {
			return
		}
		out = append(out, change{kind: route, issue: fmt.Sprintf("%s drift: have=%s want=%s", field, have, want),
			note: fmt.Sprintf("setting %s for agent %s -> %s", field, name, want), fail: fmt.Sprintf("set %s %s", field, name),
			call: Call{"POST", "/api/agents/" + name + "/loop/" + route, map[string]any{"value": want}}})
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
	// loop.enabled is the agent's on/off switch: converge it with start/stop.
	if want := a.Loop.Enabled; want != nil && cur != nil && live.Enabled != *want {
		verb := "stop"
		if *want {
			verb = "start"
		}
		out = append(out, change{kind: verb, note: fmt.Sprintf("%s agent %s (loop.enabled=%v)", verb, name, *want),
			fail: fmt.Sprintf("%s agent %s", verb, name), call: Call{"POST", "/api/agents/" + name + "/" + verb, nil}})
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
			fail: "set goal enabled " + name, call: Call{"POST", "/api/agents/" + name + "/goal-enabled", map[string]any{"enabled": *want}}})
	}
	seconds, set, err := a.goalWaitCustomerTimeoutSeconds()
	if err != nil {
		return nil, fmt.Errorf("agent %s goal: %w", name, err)
	}
	if set && cur.GoalWaitCustomerTimeoutS != seconds {
		out = append(out, change{kind: "goal-wait-customer-timeout",
			issue: fmt.Sprintf("goal_wait_customer_timeout_s drift: have=%d want=%d", cur.GoalWaitCustomerTimeoutS, seconds),
			fail:  "set goal wait customer timeout " + name, call: Call{"POST", "/api/agents/" + name + "/goal-wait-customer-timeout", map[string]any{"seconds": seconds}}})
	}
	seconds, set, err = a.goalDeliveryCooldownSeconds()
	if err != nil {
		return nil, fmt.Errorf("agent %s goal: %w", name, err)
	}
	if set && cur.GoalDeliveryCooldownS != seconds {
		out = append(out, change{kind: "goal-delivery-cooldown",
			issue: fmt.Sprintf("goal_delivery_cooldown_s drift: have=%d want=%d", cur.GoalDeliveryCooldownS, seconds),
			fail:  "set goal delivery cooldown " + name, call: Call{"POST", "/api/agents/" + name + "/goal-delivery-cooldown", map[string]any{"seconds": seconds}}})
	}
	return out, nil
}

// createBody is the POST /api/agents body for a new agent.
func (r *Runner) createBody(name string, a AgentSpec) (map[string]any, error) {
	// Always explicit: the create endpoint defaults interactive to true.
	body := map[string]any{"image": a.Image, "name": name, "interactive": a.Harness != nil && a.Harness.Interactive}
	if a.Group != "" {
		body["group"] = a.Group
	}
	if h := a.Harness; h != nil {
		for key, value := range map[string]string{"harness": h.Type, "model": h.Model, "effort": h.Effort} {
			if value != "" {
				body[key] = value
			}
		}
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
