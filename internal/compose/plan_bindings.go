package compose

import (
	"fmt"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// bindingsPhase binds the agent pools of each task queue, then sets its
// CUSTOMER_AGENT_POOL and responsible agent, binds its workflow and creates
// its triggers. The daemon binds a pool only to agents that exist, and a
// workflow only to a queue whose pools it names, so this phase follows the
// agents and groups.
type bindingsPhase struct{}

func (bindingsPhase) name() string { return "queue bindings" }

func (bindingsPhase) plan(p *planner, f File, live *liveState) error {
	for _, prefix := range sortedKeys(f.TaskQueues) {
		p.planPools(prefix, f.TaskQueues[prefix], live)
	}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		want := f.TaskQueues[prefix]
		p.planQueueEnv(prefix, want, live, true)
		p.planResponsibleAgent(prefix, want, live)
		if err := p.planQueueWorkflow(prefix, want); err != nil {
			return err
		}
		p.planTriggers(prefix, want, live)
	}
	return nil
}

// planPools converges the pools of one queue. A pool depends on the queue's
// create and on the create of every agent it names.
func (p *planner) planPools(prefix string, want TaskQueueSpec, live *liveState) {
	id := "queue:" + prefix
	_, exists := live.queues[prefix]
	pools := live.pools[prefix]
	for _, poolName := range sortedKeys(want.Pools) {
		agents := normalizeAgentNames(want.Pools[poolName])
		current, found := pools[poolName]
		switch {
		case found && slices.Equal(current.Agents, agents):
			p.status(id, false, "task queue %-10s pool %s ok", prefix, poolName)
			continue
		case found:
			p.status(id, true, "task queue %-10s pool %s drift: have=%v want=%v", prefix, poolName, current.Agents, agents)
		case exists:
			p.status(id, true, "task queue %-10s pool %s MISSING (want=%v)", prefix, poolName, agents)
		}
		deps := p.needs(id)
		for _, name := range agents {
			deps = append(deps, p.needs("agent:"+name)...)
		}
		p.provide("pool:"+prefix, p.add(Action{Resource: id, Kind: "pool", Note: fmt.Sprintf("binding task queue %s pool %s", prefix, poolName),
			Fail:   fmt.Sprintf("bind pool %s/%s", prefix, poolName),
			Fields: []Field{{Name: "pool " + poolName, From: poolText(current, found), To: "[" + strings.Join(agents, ", ") + "]"}},
			Call: Call{"PATCH", "/api/task-queues/" + prefix + "/pools/" + poolName, map[string]any{"agents": agents, "revision": current.Revision,
				"idempotency_key": composeIdempotency("pool", prefix, poolName, current.Revision, agents)}}}, deps...))
	}
}

func poolText(pool tasks.AgentPool, found bool) string {
	if !found {
		return "[]"
	}
	return "[" + strings.Join(pool.Agents, ", ") + "]"
}

// planQueueWorkflow binds the workflow of one queue to the digest it resolves
// to at apply time. The bind depends on the queue's other actions (create,
// update, env, secrets and pools) and on the build of its Store workflow, so
// a secret the workflow requires is set before it binds. A queue with no
// workflow value keeps whatever binding it has; Compose never clears one.
func (p *planner) planQueueWorkflow(prefix string, want TaskQueueSpec) error {
	ref, declared, err := want.WorkflowRef()
	if err != nil {
		return fmt.Errorf("task queue %s workflow: %w", prefix, err)
	}
	if !declared {
		return nil
	}
	id := "queue:" + prefix
	build, field := -1, Field{Name: "workflow", To: ref.Ref() + " (resolved at apply time)"}
	if ref.Store != "" {
		if b := p.needs("workflow:" + ref.Selector()); len(b) > 0 {
			build = b[0]
		}
		if ref.Tag == "" {
			field.To = ref.Selector() + " (resolved after the build)"
		}
	}
	p.add(Action{Resource: id, Kind: "workflow", Fields: []Field{field}, run: p.r.bindWorkflow(prefix, ref, build)},
		append(p.actionsOf(id), build)...)
	return nil
}

// actionsOf returns the indexes of the actions planned so far for a resource.
func (p *planner) actionsOf(id string) []int {
	var out []int
	for i, a := range p.plan.Actions {
		if a.Resource == id {
			out = append(out, i)
		}
	}
	return out
}
