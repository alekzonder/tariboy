package compose

import (
	"fmt"
	"strings"
)

// groupsPhase creates each group without a lead, assigns the file's agents
// to it, then sets the lead, which the daemon accepts only for a current
// member, and the group budget. Every agent exists by now, so membership
// does not depend on the order of the agents in the file.
type groupsPhase struct{}

func (groupsPhase) name() string { return "groups" }

func (groupsPhase) plan(p *planner, f File, live *liveState) error {
	for _, name := range sortedKeys(f.Groups) {
		g := f.Groups[name]
		id := "group:" + name
		cur, exists := live.groups[name]
		leadDrift := g.Lead != "" && cur.Lead != g.Lead
		switch {
		case !exists:
			p.status(id, true, "group %-14s MISSING (desired lead=%s)", name, g.Lead)
			p.provide(id, p.add(Action{Resource: id, Kind: "create", Note: "creating group " + name, Fail: "create group " + name,
				Fields: []Field{{Name: "members", To: strings.Join(groupMembers(f, name), ", ")}},
				Call:   Call{"POST", "/api/groups", map[string]any{"name": name}}}))
		case leadDrift:
			p.status(id, true, "group %-14s lead drift: have=%s want=%s", name, cur.Lead, g.Lead)
		default:
			p.status(id, false, "group %-14s ok (lead=%s)", name, cur.Lead)
		}
		p.planMembers(name, f, live)
		if leadDrift {
			p.add(Action{Resource: id, Kind: "lead", Note: fmt.Sprintf("setting lead of group %s -> %s", name, g.Lead), Fail: "set lead of group " + name,
				Fields: []Field{{Name: "lead", From: cur.Lead, To: g.Lead}},
				Call:   Call{"PATCH", "/api/groups/" + name + "/lead", map[string]any{"lead": g.Lead}}},
				p.needs(id, "agent:"+g.Lead, "member:"+g.Lead)...)
		}
		if b := g.Budget; b != nil {
			if err := p.planBudget(id, *b, live.budgets, p.needs(id)...); err != nil {
				return err
			}
		}
	}
	return nil
}

// planMembers assigns each file agent of a group that is new or in another
// group. An agent the file leaves without a group keeps the one it has.
func (p *planner) planMembers(group string, f File, live *liveState) {
	for _, name := range groupMembers(f, group) {
		if cur, ok := live.agents[name]; ok && cur.Group == group {
			continue
		}
		from := ""
		if cur, ok := live.agents[name]; ok {
			from = cur.Group
		}
		id := "agent:" + name
		p.provide("member:"+name, p.add(Action{Resource: id, Kind: "assign", Note: fmt.Sprintf("assigning agent %s to group %s", name, group),
			Fail: "assign agent " + name, Fields: []Field{{Name: "group", From: from, To: group}},
			Call: Call{"POST", "/api/groups/" + group + "/assign", map[string]any{"agent": name}}},
			p.needs("group:"+group, id)...))
	}
}

// groupMembers returns the file's agents of a group, sorted.
func groupMembers(f File, group string) []string {
	var out []string
	for _, name := range sortedKeys(f.Agents) {
		if f.Agents[name].Group == group {
			out = append(out, name)
		}
	}
	return out
}

// planBudget sets the budget of a scope ("agent:NAME" or "group:NAME") when
// it is missing or differs.
func (p *planner) planBudget(scope string, b BudgetSpec, live map[string]liveBudget, deps ...int) error {
	id := "budget:" + scope
	mode, err := b.NormalizedMode()
	if err != nil {
		return err
	}
	cur, exists := live[scope]
	switch {
	case !exists:
		p.status(id, true, "budget %-14s MISSING (desired limit=%g mode=%s)", scope, b.LimitUSD, mode)
	case !budgetMatches(cur, b, mode):
		p.status(id, true, "budget %-14s drift: have limit=%g mode=%s", scope, cur.LimitUSD, cur.Mode)
	default:
		p.status(id, false, "budget %-14s ok", scope)
		return nil
	}
	body := map[string]any{"scope": scope, "limit-usd": fmt.Sprintf("%g", b.LimitUSD), "mode": mode}
	if b.Period != "" {
		body["period"] = b.Period
	}
	fields := []Field{{Name: "limit_usd", To: fmt.Sprintf("%g", b.LimitUSD)}, {Name: "mode", To: mode}}
	if b.Period != "" {
		fields = append(fields, Field{Name: "period", To: b.Period})
	}
	p.add(Action{Resource: id, Kind: "set", Note: "setting budget " + scope, Fail: "set budget " + scope,
		Fields: fields, Call: Call{"POST", "/api/budgets", body}}, deps...)
	return nil
}

// budgetMatches reports whether a live budget already equals the desired spec
// with normalized mode.
func budgetMatches(cur liveBudget, b BudgetSpec, mode string) bool {
	return cur.Mode == mode && cur.LimitUSD == b.LimitUSD && cur.PeriodS == desiredPeriodSeconds(b.Period)
}
