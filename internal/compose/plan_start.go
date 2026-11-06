package compose

import "fmt"

// startPhase starts agents last, once their groups, pools and workflow
// bindings are set: a new agent unless the file disables its loop, an
// existing stopped loop the file enables, and a restart (stop, start) of an
// enabled agent whose harness changed, which applies the harness and ends
// the agent's current iteration. A re-provisioned agent is running already.
type startPhase struct{}

func (startPhase) name() string { return "start" }

func (startPhase) plan(p *planner, f File, live *liveState) error {
	for _, name := range sortedKeys(f.Agents) {
		a := f.Agents[name]
		id := "agent:" + name
		cur, exists := live.agents[name]
		start := func(kind string, fields []Field, deps ...int) {
			p.add(Action{Resource: id, Kind: kind, Fail: "start agent " + name, Fields: fields,
				Call: Call{"POST", "/api/agents/" + name + "/start", nil}}, deps...)
		}
		switch {
		case !exists:
			// A new agent starts once it exists and joined its group, so it
			// never runs outside the group the file gives it.
			if !loopDisabled(a) {
				start("start", nil, p.needs(id, "member:"+name)...)
			}
		case cur.State == "stopped":
		case harnessChange(name, a, cur) != nil && cur.Enabled && !loopDisabled(a):
			// The restart follows the agent's other changes and depends on
			// them, so the restarted iteration starts with every new setting.
			stop := p.add(Action{Resource: id, Kind: "restart", Note: fmt.Sprintf("restarting agent %s to apply harness %s", name, a.Harness.Type),
				Fail: "stop agent " + name, Fields: []Field{{Name: "harness", From: cur.Harness, To: a.Harness.Type}},
				Call: Call{"POST", "/api/agents/" + name + "/stop", nil}}, p.needs("settings:"+name)...)
			start("restart", nil, stop)
		case !cur.Enabled && a.Loop != nil && a.Loop.Enabled != nil && *a.Loop.Enabled:
			p.add(Action{Resource: id, Kind: "start", Note: fmt.Sprintf("start agent %s (loop.enabled=true)", name), Fail: "start agent " + name,
				Fields: []Field{{Name: "loop.enabled", From: "false", To: "true"}},
				Call:   Call{"POST", "/api/agents/" + name + "/start", nil}}, p.needs("settings:"+name)...)
		}
	}
	return nil
}
