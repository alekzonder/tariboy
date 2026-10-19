package loop

import (
	"context"
	"fmt"
	"sync"

	"github.com/alekzonder/tariboy/internal/agentapi"
	"github.com/alekzonder/tariboy/internal/agentdir"
)

// agentRuntimes is the manager's in-memory registry of per-agent runtimes.
//
// Locks in this package are taken in one order: an agent's lifecycle gate,
// then the image publication gate, then mu. mu is a leaf: it guards the maps
// below and the engine's non-blocking field accessors, never a Store call, a
// socket, or a callback. The type holds no Store, shim, or configuration, so
// I/O under mu is not possible from here.
type agentRuntimes struct {
	mu   sync.Mutex
	base context.Context
	stop context.CancelFunc
	runs map[string]*runtime
	// toolsAPI holds per-agent tools servers that are bound and serving but not
	// (yet) owned by a runtime: the daemon binds them at boot for every agent
	// with an unfinished iteration, whose engine may start much later or never.
	// A name lives in exactly one of toolsAPI and runs — install moves it — so
	// a socket path always has a single owner and a single listener.
	toolsAPI   map[string]*agentapi.Server
	adopting   map[string]agentdir.LiveIteration
	restarting map[string]bool
	staleKills map[string]string
	// locks are never removed: an entry is a few bytes per agent name, and
	// removing one would race a concurrent lookup that recreates it.
	locks map[string]*agentLocks
}

type agentLocks struct {
	// gate is a one-slot semaphore that serializes one agent's lifecycle:
	// start, Stop, Remove, Reprovision, and a post-adoption Restart launch from
	// its decision until its iteration row exists.
	gate chan struct{}
	// handoff is held for reading while a control request resolves an adopted
	// shim and connects to it, and for writing while adoption ends. A stable
	// per-agent socket path therefore cannot be rebound to a replacement
	// iteration between resolution and connection.
	handoff sync.RWMutex
}

func newAgentRuntimes() *agentRuntimes {
	return &agentRuntimes{
		runs: map[string]*runtime{}, toolsAPI: map[string]*agentapi.Server{},
		adopting: map[string]agentdir.LiveIteration{}, restarting: map[string]bool{},
		staleKills: map[string]string{}, locks: map[string]*agentLocks{},
	}
}

func (r *agentRuntimes) setBase(ctx context.Context, stop context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.base, r.stop = ctx, stop
}

// baseContext is the manager context engines and lifecycle waits derive from.
func (r *agentRuntimes) baseContext() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.base == nil {
		return context.Background()
	}
	return r.base
}

func (r *agentRuntimes) locksFor(name string) *agentLocks {
	r.mu.Lock()
	defer r.mu.Unlock()
	l := r.locks[name]
	if l == nil {
		l = &agentLocks{gate: make(chan struct{}, 1)}
		r.locks[name] = l
	}
	return l
}

// lockAgent takes the agent's lifecycle gate. It gives up when ctx ends, so a
// shutdown never waits behind a lifecycle operation.
func (r *agentRuntimes) lockAgent(ctx context.Context, name string) (release func(), err error) {
	gate := r.locksFor(name).gate
	select {
	case gate <- struct{}{}:
	default:
		// Only a contended gate may give up: a free one is always taken, even
		// after shutdown.
		select {
		case gate <- struct{}{}:
		case <-ctx.Done():
			return nil, fmt.Errorf("lock agent %q: %w", name, ctx.Err())
		}
	}
	var once sync.Once
	return func() { once.Do(func() { <-gate }) }, nil
}

func (r *agentRuntimes) runtime(name string) *runtime {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs[name]
}

func (r *agentRuntimes) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

// forStart returns the agent's live runtime, or an error while one of its
// iterations is being adopted.
func (r *agentRuntimes) forStart(name string) (*runtime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if live, ok := r.adopting[name]; ok {
		return nil, fmt.Errorf("%w %q for agent %q", errIterationAdopting, live.ID, name)
	}
	return r.runs[name], nil
}

// install registers a started runtime. The runtime now owns the tools socket,
// so the boot-time registration is dropped and teardown closes exactly one
// server for this agent.
func (r *agentRuntimes) install(name string, rt *runtime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[name] = rt
	delete(r.toolsAPI, name)
}

// toolsServer returns the one server that owns the agent's tools socket, if
// any is bound.
func (r *agentRuntimes) toolsServer(name string) *agentapi.Server {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rt, ok := r.runs[name]; ok && rt.apiServer != nil {
		return rt.apiServer
	}
	return r.toolsAPI[name]
}

func (r *agentRuntimes) registerToolsServer(name string, srv *agentapi.Server) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolsAPI[name] = srv
}

// currentIterationID is the engine's running iteration, else the iteration
// being adopted, else "".
func (r *agentRuntimes) currentIterationID(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rt := r.runs[name]; rt != nil {
		if id := rt.engine.CurrentIterationID(); id != "" {
			return id
		}
	}
	return r.adopting[name].ID
}

// engineIterationID is the iteration the agent's engine runs, or "".
func (r *agentRuntimes) engineIterationID(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rt := r.runs[name]; rt != nil {
		return rt.engine.CurrentIterationID()
	}
	return ""
}

// currentTarget resolves the iteration a control request is for.
func (r *agentRuntimes) currentTarget(name string, engineSock string) (iterationTarget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt := r.runs[name]
	if rt != nil {
		if id := rt.engine.CurrentIterationID(); id != "" {
			return iterationTarget{id: id, sock: engineSock, runtime: rt}, nil
		}
	}
	if live, ok := r.adopting[name]; ok {
		return iterationTarget{id: live.ID, sock: live.ShimSock}, nil
	}
	if rt == nil {
		return iterationTarget{}, fmt.Errorf("agent %q is not running", name)
	}
	return iterationTarget{}, fmt.Errorf("agent %q has no running iteration", name)
}

func (r *agentRuntimes) beginAdoption(li agentdir.LiveIteration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adopting[li.Agent] = li
}

// endAdoption drops li from adopting under the agent's handoff lock, so no
// control request is still connecting to its shim once a replacement may
// start.
func (r *agentRuntimes) endAdoption(li agentdir.LiveIteration) {
	handoff := &r.locksFor(li.Agent).handoff
	handoff.Lock()
	defer handoff.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.adopting[li.Agent]; ok && current.ID == li.ID {
		delete(r.adopting, li.Agent)
	}
}

// markRestartIfAdopting records an interactive Restart for the launch that
// follows adoption. It reports whether the agent is adopting; the check and
// the mark are one step, so adoption cannot end between them unnoticed.
func (r *agentRuntimes) markRestartIfAdopting(name string, interactive bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.adopting[name]; !ok {
		return false
	}
	if interactive {
		r.restarting[name] = true
	}
	return true
}

// pendingRestart reports a Restart recorded during adoption, and the runtime
// that will launch it.
func (r *agentRuntimes) pendingRestart(name string) (*runtime, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs[name], r.restarting[name]
}

// takeRestart consumes a Restart recorded during adoption.
func (r *agentRuntimes) takeRestart(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	restart := r.restarting[name]
	delete(r.restarting, name)
	return restart
}

// stopRuntime cancels a pending post-adoption Restart and returns the runtime
// to repark.
func (r *agentRuntimes) stopRuntime(name string) *runtime {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.restarting, name)
	return r.runs[name]
}

// markStaleKill records that the iteration id of rt was recovered from an
// unreachable shim and aborts it, if rt still runs it. The marker lives until
// that iteration closes.
func (r *agentRuntimes) markStaleKill(name, id string, rt *runtime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs[name] == rt && rt.engine.CurrentIterationID() == id {
		r.staleKills[name] = id
		rt.engine.AbortCurrent(id)
	}
}

func (r *agentRuntimes) staleKill(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.staleKills[name]
}

func (r *agentRuntimes) clearStaleKill(name, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.staleKills[name] == id {
		delete(r.staleKills, name)
	}
}

// engines returns the live engines of the named agents.
func (r *agentRuntimes) engines(names []string) []*Engine {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Engine, 0, len(names))
	for _, name := range names {
		if rt := r.runs[name]; rt != nil {
			out = append(out, rt.engine)
		}
	}
	return out
}

// remove unregisters the agent and returns the tools servers to shut down.
// Without force it refuses an agent that is enabled or iterating.
func (r *agentRuntimes) remove(name string, enabled, force bool) (map[*agentapi.Server]struct{}, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt := r.runs[name]
	// Refuse to nuke an agent whose loop is enabled (intent to run) or that has
	// an iteration in flight right now — unless forced. This is the
	// reality-derived equivalent of the old state=="running" guard.
	iterating := rt != nil && rt.engine.CurrentIterationID() != ""
	if (enabled || iterating) && !force {
		return nil, fmt.Errorf("agent %q is running; stop it first or use --force", name)
	}
	servers := map[*agentapi.Server]struct{}{}
	if rt != nil {
		rt.cancel()
		servers[rt.apiServer] = struct{}{}
		delete(r.runs, name)
	}
	if srv, ok := r.toolsAPI[name]; ok {
		servers[srv] = struct{}{}
		delete(r.toolsAPI, name)
	}
	return servers, nil
}

// shutdown cancels the manager context and every runtime, and returns every
// tools server to shut down.
func (r *agentRuntimes) shutdown() map[*agentapi.Server]struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stop != nil {
		r.stop() // unblock adopt/startAfter goroutines
	}
	servers := map[*agentapi.Server]struct{}{}
	for name, rt := range r.runs {
		rt.cancel()
		servers[rt.apiServer] = struct{}{}
		delete(r.runs, name)
	}
	// Agents that only ever got the boot-time bind (engine never started) own
	// their tools server here, not in runs.
	for name, srv := range r.toolsAPI {
		servers[srv] = struct{}{}
		delete(r.toolsAPI, name)
	}
	return servers
}
