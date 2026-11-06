package compose

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Call is one daemon request an action issues.
type Call struct {
	Method string
	Route  string
	Body   any
}

// Field is one value an action sets, as `up --dry-run` prints it. It never
// holds a secret: for env it names the keys only.
type Field struct {
	Name string
	From string // the live value; "" when the action sets a new one
	To   string
}

func (f Field) String() string {
	switch {
	case f.To == "" && f.From == "":
		return f.Name
	case f.From == "":
		return f.Name + "=" + f.To
	default:
		return f.Name + " " + f.From + " -> " + f.To
	}
}

// Action is one step of a Plan. Apply issues Call, or runs the action's
// resolver when the request depends on what an earlier action returned. An
// action whose DependsOn names an action that failed or was skipped is
// skipped.
type Action struct {
	Phase     string // the phase that planned the action
	Resource  string // the Resource.ID the action changes
	Kind      string // create, assign, image, harness, stop, start, ...
	Note      string // progress line printed before the call; empty prints none
	Fail      string // prefix of the error the action reports
	Call      Call
	Fields    []Field // what the action sets, without secret values
	DependsOn []int   // indexes into Plan.Actions
	// SkipReason makes Apply skip the action without a call; the report
	// gives it as the reason, such as "skipped (archive)".
	SkipReason string

	// run replaces Call for an action that resolves its request at apply
	// time, from the results of earlier actions, or reads the state around
	// it. result is what later actions read; changed is false when the
	// resolved state already matched.
	run func(results []json.RawMessage) (result json.RawMessage, changed bool, err error)
}

// Resource is one thing the file declares (an image, a workflow build, a
// task queue, an agent, a group or a budget), with what `compose status`
// prints for it.
type Resource struct {
	ID     string
	status []string // status lines
	drift  int      // drifted items status counts for this resource
	queue  string   // task queue prefix whose workflow status follows; "" otherwise
}

// Plan is the ordered list of actions that converge the daemon toward a file,
// computed from one read of the live state. Actions follow the phase order;
// a resource is listed where an action or a status line first names it.
type Plan struct {
	Resources []Resource
	Actions   []Action
}

// phase is one step of compose up. plan reads the File and the live state
// (read once, before any phase) and adds actions; it never calls the daemon
// with a mutating request. Apply is common to every phase, so a new kind of
// entity adds its actions to its phase only.
type phase interface {
	name() string
	plan(p *planner, f File, live *liveState) error
}

// phases is the order of compose up: what a later phase binds exists by then,
// and agents start last, once their groups and pools are set.
var phases = []phase{storesPhase{}, workflowsPhase{}, imagesPhase{}, queuesPhase{},
	agentsPhase{}, groupsPhase{}, bindingsPhase{}, startPhase{}}

type planner struct {
	r         *Runner
	plan      Plan
	phase     string
	resources map[string]int
	// keys names actions for later phases: "image:NAME", "workflow:SELECTOR",
	// "queue:PREFIX", "agent:NAME", "settings:NAME", "group:NAME",
	// "member:AGENT" and "pool:PREFIX". A key exists only when an action is
	// planned for it, so an entity that already matches adds no dependency.
	keys map[string][]int
}

// Plan reads the live state once and returns the actions of every phase that
// converge it toward the file. It never issues a mutating call.
func (r *Runner) Plan(f File) (Plan, error) {
	live, err := r.readLive(f)
	if err != nil {
		return Plan{}, err
	}
	return r.planFrom(f, live)
}

func (r *Runner) planFrom(f File, live *liveState) (Plan, error) {
	p := &planner{r: r, resources: map[string]int{}, keys: map[string][]int{}}
	for _, ph := range phases {
		p.phase = ph.name()
		if err := ph.plan(p, f, live); err != nil {
			return Plan{}, err
		}
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

// add appends an action to the current phase and returns its index. Negative
// dependencies mean "none" and are dropped.
func (p *planner) add(a Action, deps ...int) int {
	p.resource(a.Resource)
	a.Phase = p.phase
	for _, d := range deps {
		if d >= 0 && !containsInt(a.DependsOn, d) {
			a.DependsOn = append(a.DependsOn, d)
		}
	}
	p.plan.Actions = append(p.plan.Actions, a)
	return len(p.plan.Actions) - 1
}

// provide names an action so a later phase can depend on it.
func (p *planner) provide(key string, action int) {
	p.keys[key] = append(p.keys[key], action)
}

// needs returns the actions named by keys; an unknown key adds none.
func (p *planner) needs(keys ...string) []int {
	var out []int
	for _, k := range keys {
		out = append(out, p.keys[k]...)
	}
	return out
}

// imageKey is the key of the build of the file image an agent image ref
// names, or "" when the file does not build that image. An archive imports
// every agent image, declared or not.
func (p *planner) imageKey(f File, ref string) string {
	name, _, _ := strings.Cut(ref, ":")
	if _, ok := f.Images[name]; !ok && p.r.archive == nil {
		return ""
	}
	return "image:" + name
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
