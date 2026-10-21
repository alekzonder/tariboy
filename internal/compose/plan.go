package compose

import (
	"encoding/json"
	"fmt"
)

// Call is one daemon request an action issues.
type Call struct {
	Method string
	Route  string
	Body   any
}

// Action is one step of a Plan. Apply issues Call, or runs the action's
// resolver when the request depends on what an earlier action returned. An
// action whose DependsOn names an action that failed or was skipped is
// skipped.
type Action struct {
	Resource  string // the Resource.ID the action changes
	Kind      string // create, assign, image, harness, stop, start, ...
	Note      string // progress line printed before the call; empty prints none
	Fail      string // prefix of the error the action reports
	Call      Call
	DependsOn []int // indexes into Plan.Actions

	// run replaces Call for an action that resolves its request at apply
	// time from the results of earlier actions. changed is false when the
	// resolved state already matches.
	run func(results []json.RawMessage) (changed bool, err error)
}

// Resource is one thing the file declares (a group, agent, budget, task
// queue, or workflow build), with what `compose status` prints for it.
type Resource struct {
	ID     string
	status []string // status lines
	drift  int      // drifted items status counts for this resource
	queue  string   // task queue prefix whose workflow status follows; "" otherwise
}

// Plan is the ordered list of actions that converge the daemon toward a file,
// computed from one read of the live state. Resource and action order follow
// the reconcile order: groups, agents, budgets, task queues, workflows.
type Plan struct {
	Resources []Resource
	Actions   []Action
}

type planner struct {
	r         *Runner
	plan      Plan
	resources map[string]int
	// groupCreate and agentCreate hold the index of the create action of a
	// group or agent missing on the daemon, so resources that need it can
	// depend on it.
	groupCreate map[string]int
	agentCreate map[string]int
}

// Plan reads the live state and returns the actions that converge it toward
// the file. It never issues a mutating call.
func (r *Runner) Plan(f File) (Plan, error) {
	p := &planner{r: r, resources: map[string]int{}, groupCreate: map[string]int{}, agentCreate: map[string]int{}}
	groups, err := r.liveGroups()
	if err != nil {
		return Plan{}, err
	}
	agents, err := r.liveAgents()
	if err != nil {
		return Plan{}, err
	}
	budgets, err := r.liveBudgets()
	if err != nil {
		return Plan{}, err
	}
	p.planGroups(f, groups)
	if err := p.planAgents(f, agents); err != nil {
		return Plan{}, err
	}
	if err := p.planBudgets(f, budgets); err != nil {
		return Plan{}, err
	}
	if err := p.planTaskQueues(f); err != nil {
		return Plan{}, err
	}
	if err := p.planQueueWorkflows(f); err != nil {
		return Plan{}, err
	}
	return p.plan, nil
}

// resource returns the resource with id, adding it in plan order.
func (p *planner) resource(id string) *Resource {
	i, ok := p.resources[id]
	if !ok {
		i = len(p.plan.Resources)
		p.resources[id] = i
		p.plan.Resources = append(p.plan.Resources, Resource{ID: id})
	}
	return &p.plan.Resources[i]
}

// status adds a status line to a resource; drift counts it as drift.
func (p *planner) status(id string, drift bool, format string, a ...any) {
	res := p.resource(id)
	res.status = append(res.status, fmt.Sprintf(format, a...))
	if drift {
		res.drift++
	}
}

// add appends an action and returns its index. Negative dependencies mean
// "none" and are dropped.
func (p *planner) add(a Action, deps ...int) int {
	p.resource(a.Resource)
	for _, d := range deps {
		if d >= 0 {
			a.DependsOn = append(a.DependsOn, d)
		}
	}
	p.plan.Actions = append(p.plan.Actions, a)
	return len(p.plan.Actions) - 1
}

func (p *planner) planGroups(f File, live map[string]liveGroup) {
	for _, name := range sortedKeys(f.Groups) {
		g := f.Groups[name]
		id := "group:" + name
		cur, exists := live[name]
		switch {
		case !exists:
			p.status(id, true, "group %-14s MISSING (desired lead=%s)", name, g.Lead)
		case cur.Lead != g.Lead:
			p.status(id, true, "group %-14s lead drift: have=%s want=%s", name, cur.Lead, g.Lead)
		default:
			p.status(id, false, "group %-14s ok (lead=%s)", name, cur.Lead)
			continue
		}
		// Ensure groups first so lead/inbox wiring exists before members join.
		i := p.add(Action{Resource: id, Kind: "ensure", Note: fmt.Sprintf("ensuring group %s (lead %s)", name, g.Lead),
			Fail: "ensure group " + name, Call: Call{"POST", "/api/groups", map[string]any{"name": name, "lead": g.Lead}}})
		if !exists {
			p.groupCreate[name] = i
		}
	}
}

func (p *planner) planBudgets(f File, live map[string]liveBudget) error {
	for _, name := range sortedKeys(f.Groups) {
		if b := f.Groups[name].Budget; b != nil {
			if err := p.planBudget("group:"+name, *b, live, createDep(p.groupCreate, name)); err != nil {
				return err
			}
		}
	}
	for _, name := range sortedKeys(f.Agents) {
		if b := f.Agents[name].Budget; b != nil {
			if err := p.planBudget("agent:"+name, *b, live, createDep(p.agentCreate, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// createDep returns the create action index for name, or -1.
func createDep(creates map[string]int, name string) int {
	if i, ok := creates[name]; ok {
		return i
	}
	return -1
}

func (p *planner) planBudget(scope string, b BudgetSpec, live map[string]liveBudget, dep int) error {
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
	p.add(Action{Resource: id, Kind: "set", Note: "setting budget " + scope, Fail: "set budget " + scope,
		Call: Call{"POST", "/api/budgets", body}}, dep)
	return nil
}

// budgetMatches reports whether a live budget already equals the desired spec
// with normalized mode.
func budgetMatches(cur liveBudget, b BudgetSpec, mode string) bool {
	return cur.Mode == mode && cur.LimitUSD == b.LimitUSD && cur.PeriodS == desiredPeriodSeconds(b.Period)
}
