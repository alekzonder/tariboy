package compose

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// liveState is the daemon state every phase plans from, read once before the
// first phase.
type liveState struct {
	groups  map[string]liveGroup
	agents  map[string]liveAgent
	budgets map[string]liveBudget
	// pending is the image a running agent of the file adopts on its next
	// iteration, read only for an agent whose image differs from the file.
	pending map[string]string
	// queues and pools are read when the file declares task queues; pools
	// holds the pools of every declared queue that exists.
	queues map[string]tasks.Queue
	pools  map[string]map[string]tasks.AgentPool
	// queueSettings holds the env, secret keys and triggers of every
	// declared queue that exists.
	queueSettings map[string]liveQueueSettings
}

// liveQueueSettings is what the daemon returns for a queue's env, secrets and
// triggers. The daemon never returns a secret value: secrets and globalEnv
// hold names only.
type liveQueueSettings struct {
	env       map[string]string
	secrets   map[string]bool
	globalEnv map[string]bool // required secrets the daemon environment holds
	triggers  []tasks.QueueWorkflowTrigger
}

func (r *Runner) readLive(f File) (*liveState, error) {
	live := &liveState{pending: map[string]string{}, pools: map[string]map[string]tasks.AgentPool{},
		queueSettings: map[string]liveQueueSettings{}}
	var err error
	if live.groups, err = r.liveGroups(); err != nil {
		return nil, err
	}
	if live.agents, err = r.liveAgents(); err != nil {
		return nil, err
	}
	if live.budgets, err = r.liveBudgets(); err != nil {
		return nil, err
	}
	for _, name := range sortedKeys(f.Agents) {
		cur, ok := live.agents[name]
		if !ok {
			continue
		}
		if err := r.readAgentDetails(name, f.Agents[name], &cur); err != nil {
			return nil, fmt.Errorf("agent %s: %w", name, err)
		}
		live.agents[name] = cur
		if cur.State == "stopped" || cur.Image == normalizeImageRef(f.Agents[name].Image) {
			continue
		}
		// A failed read only means the image is set again.
		live.pending[name], _ = r.pendingImage(name)
	}
	if len(f.TaskQueues) == 0 {
		return live, nil
	}
	if live.queues, err = r.listTaskQueues(); err != nil {
		return nil, fmt.Errorf("task queues: %w", err)
	}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		if _, ok := live.queues[prefix]; !ok {
			continue
		}
		if live.pools[prefix], err = r.listTaskPools(prefix); err != nil {
			return nil, fmt.Errorf("task queue %s pools: %w", prefix, err)
		}
		if live.queueSettings[prefix], err = r.readQueueSettings(prefix); err != nil {
			return nil, fmt.Errorf("task queue %s: %w", prefix, err)
		}
	}
	return live, nil
}
