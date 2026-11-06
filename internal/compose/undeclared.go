package compose

import (
	"fmt"
	"slices"
	"strings"
)

// undeclared lists the agents, groups and task queues on the daemon that the
// file does not declare but that are tied to what it declares. The daemon
// does not record which file created an entity, so a tie is the only sign
// that one belongs to the project:
//   - an agent in a group of the file, or in a pool of a queue of the file;
//   - a group that has an agent of the file as a member;
//   - a task queue with a pool that names an agent of the file.
//
// For a declared queue it also lists the pools, env keys, secrets and
// triggers the file omits.
//
// up never changes or removes them, and status does not count them as drift.
// It only reads: the pools of an undeclared queue are read here.
func (r *Runner) undeclared(f File, live *liveState) ([]string, error) {
	var out []string
	line := func(kind, name, why string) {
		out = append(out, fmt.Sprintf("%-10s %-14s not declared (%s)", kind, name, why))
	}
	for _, name := range sortedKeys(live.agents) {
		if fileHasAgent(f, name) {
			continue
		}
		cur := live.agents[name]
		var why []string
		if fileHasGroup(f, cur.Group) {
			why = append(why, "group="+cur.Group)
		} else if pool := filePoolOf(f, live, name); pool != "" {
			why = append(why, "pool "+pool)
		}
		if len(why) > 0 {
			line("agent", name, strings.Join(append(why, "state="+cur.State), " "))
		}
	}
	for _, name := range sortedKeys(live.groups) {
		if fileHasGroup(f, name) {
			continue
		}
		var members []string
		for _, agent := range sortedKeys(f.Agents) {
			if cur, ok := live.agents[agent]; ok && cur.Group == name {
				members = append(members, agent)
			}
		}
		if len(members) > 0 {
			line("group", name, "member "+strings.Join(members, ", "))
		}
	}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		if _, exists := live.queues[prefix]; exists {
			out = append(out, queueUndeclared(prefix, f.TaskQueues[prefix], live)...)
		}
	}
	queues := live.queues
	if queues == nil {
		// The file declares no queue, so the queue list was not read. A
		// failed read here only leaves out the undeclared queues.
		var err error
		if queues, err = r.listTaskQueues(); err != nil {
			return out, nil
		}
	}
	for _, prefix := range sortedKeys(queues) {
		if _, declared := f.TaskQueues[prefix]; declared {
			continue
		}
		pools, err := r.listTaskPools(prefix)
		if err != nil {
			return nil, fmt.Errorf("task queue %s pools: %w", prefix, err)
		}
		for _, pool := range sortedKeys(pools) {
			var members []string
			for _, agent := range pools[pool].Agents {
				if fileHasAgent(f, agent) {
					members = append(members, agent)
				}
			}
			if len(members) > 0 {
				line("task queue", prefix, "pool "+pool+" has "+strings.Join(members, ", "))
				break
			}
		}
	}
	return out, nil
}

// filePoolOf names the first pool of a file queue, as QUEUE/POOL, whose live
// agents include name; "" when none does.
func filePoolOf(f File, live *liveState, name string) string {
	for _, prefix := range sortedKeys(f.TaskQueues) {
		pools := live.pools[prefix]
		for _, pool := range sortedKeys(pools) {
			if slices.Contains(pools[pool].Agents, name) {
				return prefix + "/" + pool
			}
		}
	}
	return ""
}
