package loop

import (
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/proctree"
	"github.com/alekzonder/tariboy/internal/registry"
)

const (
	// defaultStallProbeTimeout bounds one batch of stall probes: the shim
	// status calls and the process table read.
	defaultStallProbeTimeout = time.Second
	// stallProbeCacheTTL is how long a probe result, failures included, holds
	// for the same iteration.
	stallProbeCacheTTL = 15 * time.Second
	// StateReasonAgentCommand is the state reason of a stalled iteration whose
	// harness waits for a live agent command.
	StateReasonAgentCommand = "waiting for agent command"
)

// stallProbes holds, per agent, the hook commands of its latest iteration and
// its latest stall probe. It is memory only: an iteration adopted after a
// daemon restart has no hook commands.
type stallProbes struct {
	mu     sync.Mutex
	hooks  map[string]iterationHooks
	probes map[string]*stallProbe
}

type iterationHooks struct {
	iterationID string
	commands    []string
}

// stallProbe is one probe of a stalled iteration. command is set before done
// closes.
type stallProbe struct {
	name        string
	iterationID string
	hooks       []string
	at          time.Time // manager clock at start, for the cache period
	deadline    time.Time // wall clock bound for waiting on the result
	done        chan struct{}
	command     bool
}

// noteHookCommands records the instant mail hook commands of an iteration.
func (m *Manager) noteHookCommands(agentName, iterationID string, commands []string) {
	m.stall.mu.Lock()
	defer m.stall.mu.Unlock()
	if m.stall.hooks == nil {
		m.stall.hooks = map[string]iterationHooks{}
	}
	m.stall.hooks[agentName] = iterationHooks{iterationID: iterationID, commands: commands}
}

// LiveStatusOf is the live state of an agent row with its reason.
func (m *Manager) LiveStatusOf(a agent.Agent) registry.LiveStatus {
	return m.LiveStatuses([]agent.Agent{a})[0]
}

// LiveStatuses derives the live state of each agent. A stalled iteration, one
// without AI-proxy requests for longer than its stall timeout, is "error"
// unless the stall probe finds a live agent command under its harness: then it
// is "running" with StateReasonAgentCommand. All stalled agents are probed in
// one batch with one process table read, so the call waits for one probe
// timeout at most. A probe failure or timeout keeps "error".
func (m *Manager) LiveStatuses(agents []agent.Agent) []registry.LiveStatus {
	out := make([]registry.LiveStatus, len(agents))
	probes := make([]*stallProbe, len(agents))
	var started []*stallProbe
	for i, a := range agents {
		state, stalled := m.liveStateWithoutProbe(a)
		out[i].State = state
		if stalled == "" {
			continue
		}
		p, fresh := m.stallProbeFor(a.Name, stalled)
		probes[i] = p
		if fresh {
			started = append(started, p)
		}
	}
	if len(started) > 0 {
		go m.runStallProbes(started)
	}
	for i, p := range probes {
		if p != nil && p.wait() {
			out[i] = registry.LiveStatus{State: "running", Reason: StateReasonAgentCommand}
		}
	}
	return out
}

// stallProbeFor returns the cached or running probe of the iteration, or a new
// one that the caller must start (fresh). One probe per agent runs at a time.
func (m *Manager) stallProbeFor(name, iterationID string) (*stallProbe, bool) {
	now := m.cfg.Clock()
	m.stall.mu.Lock()
	defer m.stall.mu.Unlock()
	if p := m.stall.probes[name]; p != nil && p.iterationID == iterationID &&
		(!p.finished() || now.Sub(p.at) < stallProbeCacheTTL) {
		return p, false
	}
	p := &stallProbe{name: name, iterationID: iterationID, at: now,
		deadline: time.Now().Add(m.cfg.StallProbeTimeout), done: make(chan struct{})}
	if h := m.stall.hooks[name]; h.iterationID == iterationID {
		p.hooks = h.commands
	}
	if m.stall.probes == nil {
		m.stall.probes = map[string]*stallProbe{}
	}
	m.stall.probes[name] = p
	return p, true
}

// runStallProbes asks every shim for its harness pid in parallel, reads the
// process table once for all of them and applies the agent command rule.
func (m *Manager) runStallProbes(probes []*stallProbe) {
	pids := make([]int, len(probes))
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pids[i] = m.harnessPID(p.name, p.iterationID)
		}()
	}
	wg.Wait()
	var roots []int
	for _, pid := range pids {
		if pid > 0 {
			roots = append(roots, pid)
		}
	}
	var procs []proctree.Proc
	var err error
	if len(roots) > 0 {
		procs, err = m.cfg.ReadProcs(roots)
	}
	for i, p := range probes {
		p.command = err == nil && pids[i] > 0 && proctree.AgentCommand(procs, pids[i], p.hooks)
		close(p.done)
	}
}

// harnessPID is the harness pid the iteration's shim reports, or 0 when the
// shim is unreachable, runs another iteration or does not know the pid.
func (m *Manager) harnessPID(name, iterationID string) int {
	client, target, err := m.currentShimClient(name)
	if err != nil {
		return 0
	}
	client.Timeout = m.cfg.StallProbeTimeout
	st, err := client.Status()
	if err != nil || !st.Running || target.id != iterationID {
		return 0
	}
	return st.PID
}

func (p *stallProbe) finished() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// wait returns the probe result, or false when the probe misses its deadline.
func (p *stallProbe) wait() bool {
	if p.finished() {
		return p.command
	}
	t := time.NewTimer(time.Until(p.deadline))
	defer t.Stop()
	select {
	case <-p.done:
		return p.command
	case <-t.C:
		return false
	}
}
