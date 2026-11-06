package compose

import (
	"strings"
)

// DryRun prints what up would do, phase by phase, from one read of the live
// state: the verb and the fields of each resource a phase changes, then the
// entities that are not declared. It builds nothing and issues no request
// but GET. It prints the names of env keys only, never a value or a request
// body.
func (r *Runner) DryRun(f File) error {
	if err := f.Validate(); err != nil {
		return err
	}
	live, err := r.readLive(f)
	if err != nil {
		return err
	}
	plan, err := r.planFrom(f, live)
	if err != nil {
		return err
	}
	undeclared, err := r.undeclared(f, live)
	if err != nil {
		return err
	}
	changing := map[string]bool{}
	for _, ph := range phases {
		lines := phaseLines(plan, ph.name())
		if len(lines) == 0 {
			continue
		}
		r.logf("phase %s", ph.name())
		for _, l := range lines {
			changing[l.resource] = true
			if len(l.fields) == 0 {
				r.logf("  %-8s %s", l.verb, l.resource)
				continue
			}
			r.logf("  %-8s %s (%s)", l.verb, l.resource, strings.Join(l.fields, "; "))
		}
	}
	if len(undeclared) > 0 {
		r.logf("not declared")
		for _, l := range undeclared {
			r.logf("  %s", l)
		}
	}
	r.logf("dry-run: %d resources to change, %d ok; nothing applied", len(changing), len(plan.Resources)-len(changing))
	return nil
}

type planLine struct {
	verb, resource string
	fields         []string
}

// phaseLines folds the actions of one phase into one line per resource, in
// the order of their first action.
func phaseLines(plan Plan, phase string) []planLine {
	var out []planLine
	index := map[string]int{}
	for _, a := range plan.Actions {
		// A secret refresh rewrites a secret the queue has; it changes nothing
		// up can see, so dry-run does not list it.
		if a.Phase != phase || a.Kind == secretRefresh || a.Kind == storeCheck {
			continue
		}
		i, ok := index[a.Resource]
		if !ok {
			i = len(out)
			index[a.Resource] = i
			out = append(out, planLine{verb: "update", resource: a.Resource})
		}
		l := &out[i]
		l.verb = strongerVerb(l.verb, a.Kind)
		for _, f := range a.Fields {
			l.fields = append(l.fields, f.String())
		}
		if len(a.Fields) == 0 && a.Kind != "start" && a.Kind != "restart" {
			l.fields = append(l.fields, a.Kind)
		}
	}
	return out
}

// strongerVerb is the verb a resource line shows: build, create, restart,
// start and set name the action; anything else is an update.
func strongerVerb(verb, kind string) string {
	rank := map[string]int{"update": 0, "set": 1, "start": 2, "restart": 3, "create": 4, "build": 5}
	if r, ok := rank[kind]; ok && r > rank[verb] {
		return kind
	}
	return verb
}
