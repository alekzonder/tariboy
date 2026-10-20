package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentapi"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/telemetry"
)

// GroupProvisioner is the subset of groups.Provisioner the manager needs to
// auto-provision an agent's group on `agent.run --group` before its loop starts
// (spec §4.4). nil disables group provisioning.
type GroupProvisioner interface {
	EnsureGroup(name, lead string) error
	Reconcile(name string) error
	SharedDir(group string) string
	Inspect(name string) (map[string]any, error)
}

type ScriptResultNotifier interface {
	Wake()
}

type ManagerConfig struct {
	AgentsDir string
	// RuntimeDir holds the agent sockets. Empty keeps them in each agent
	// directory; the environment is never consulted.
	RuntimeDir string
	// BaseEnv is the baseline environment of iterations and scripts. Nil uses
	// the daemon process environment.
	BaseEnv []string
	// AdoptPollInterval, AdoptProbeInterval and AdoptProbeTimeout tune how an
	// adopted iteration is watched after a restart. Zero uses the defaults.
	AdoptPollInterval  time.Duration
	AdoptProbeInterval time.Duration
	AdoptProbeTimeout  time.Duration
	ShimBin            string
	ImgStore           *image.Store
	Store              *agent.Store
	Log                *slog.Logger
	Clock              func() time.Time
	DoneGrace          time.Duration
	Spawner            Spawner
	RunnerFactory      func(agent.Agent) IterationRunner
	ConnectShim        func(string) (*shim.Client, error)
	// BeforeRestartLaunch is a test seam invoked before a post-adoption
	// interactive Restart attempts to acquire its launch gate.
	BeforeRestartLaunch func()
	Bus                 *bus.Bus
	Schedules           *schedule.Store
	Scripts             *script.Store
	ScriptResults       ScriptResultNotifier
	ScriptTimeout       time.Duration
	Emit                func(events.Event)
	Proxy               ProxyBinder
	Groups              GroupProvisioner
	// AuditFor returns the shared per-agent audit recorder. Wired by the daemon to
	// its audit.Registry so the daemon Recorder, the engine lifecycle sink, and the
	// runner's log tailer all share one *audit.Log per agent. Nil disables audit.
	AuditFor func(agent string) Recorder
	// RecordCursorUsage persists token counts from a non-interactive cursor
	// stream-json result. Nil leaves those counts unrecorded.
	RecordCursorUsage func(agent, iteration, imageName, imageTag, imageDigest string, usage CursorResultUsage)
	// CodexApproval is passed to RunnerConfig.CodexApproval.
	CodexApproval func() string
	// HasTmuxSession / KillTmuxSession are seams for orphan-session reaping on
	// start (interactive agents). Injectable so tests avoid a real tmux; nil uses
	// the real tmux commands.
	HasTmuxSession  func(session string) bool
	KillTmuxSession func(session string) error
	// Metrics records per-iteration telemetry (spec §14); nil is a valid no-op.
	Metrics *telemetry.Metrics
	// UsageLookup is a best-effort per-iteration token/cost accessor (wired by
	// the daemon to aiproxy.Store.IterationUsage) used for OTel span attributes;
	// nil skips the attributes.
	UsageLookup func(iteration string) (int, int, float64)
	// UsageFlush synchronously persists queued proxy usage before terminal span aggregation.
	UsageFlush func()
	// PrepareImageBridge is injectable for activation boundary tests. Nil uses
	// agentdir.PrepareImageBridge.
	PrepareImageBridge func(string, string, []image.ManifestSkill, agentdir.BridgePlan) error
	// OnIterationClose, if set, is invoked exactly once per finished iteration
	// (every terminal outcome) with (agent, iterationID), after the iteration's
	// final status is persisted. The daemon wires this to gzip the AI-proxy
	// transcript (spec §9/§12) without loop importing aiproxy directly.
	OnIterationClose func(agent, iterationID string)
	// GoalSignal coalesces agent configuration changes into a goal scan.
	GoalSignal func()
	// IterationCompleted requests the next goal wake after terminal persistence.
	IterationCompleted func(agent, iterationID string)
	// RecordIterationEnd reports each terminal iteration with the Goal it ran
	// with, just before IterationCompleted, so a workflow task this iteration
	// paused is not woken again. An error is logged and changes nothing else.
	RecordIterationEnd func(context.Context, tasks.IterationEnd) error
	CurrentGoal        func(string, time.Time) (tasks.Task, bool, error)
	// WorkflowGoals supplies the workflow data for the Goal block of a workflow
	// task; nil renders the flexible text with a notice.
	WorkflowGoals WorkflowGoalSource
	SetGoal       func(agent, key string, activate func() (func(), error)) error
	// ProvidedChannels returns provider-declared channels drawn from installed
	// plugin manifests, so `tools sources` can list and annotate provider
	// channels even before their channel row exists (spec §6.1). Wired by the
	// daemon (loop must not import plugins directly); nil yields no annotations.
	ProvidedChannels func() ([]agentapi.ProvidedChannel, error)
	ExternalPlugins  plugincaps.ExternalResolver
	Tasks            registry.TaskControl
	// iterationStore overrides Store for terminal-status finalization only
	// (finalizeIteration). Unexported on purpose: it is a test seam for
	// injecting an UpdateIteration failure, not daemon configuration. Unset
	// means Store.
	iterationStore iterationStore
}

type runtime struct {
	engine    *Engine
	cancel    context.CancelFunc
	apiServer *agentapi.Server
}

var errIterationAdopting = errors.New("agent has an adopted running iteration")

type iterationTarget struct {
	id      string
	sock    string
	runtime *runtime
}

// Reattach liveness defaults: adopt polls result.json every
// AdoptPollInterval and probes the shim socket every AdoptProbeInterval,
// giving up after adoptMaxMisses probes.
const (
	defaultAdoptPollInterval  = 200 * time.Millisecond
	defaultAdoptProbeInterval = time.Second
	defaultAdoptProbeTimeout  = time.Second
	adoptMaxMisses            = 3
	shutdownWait              = 10 * time.Second
)

type Manager struct {
	cfg      ManagerConfig
	wg       sync.WaitGroup
	runtimes *agentRuntimes
	scripts  *scriptSupervisor
	rng      *rand.Rand
	// goals holds, per agent, the Goal its latest prepared iteration runs
	// with, until that iteration is reported finished. It is memory only: an
	// iteration adopted after a daemon restart reports no Goal.
	goalsMu sync.Mutex
	goals   map[string]iterationGoal
}

// iterationGoal is the Goal one iteration runs with and when it was read.
type iterationGoal struct {
	iterationID string
	key         string
	readAt      time.Time
}

// ExtendIterationTimeout makes an extension durable before attempting the
// best-effort shim resync. The runner will retry a pending hard-deadline sync,
// so a transient/unavailable shim must not turn a committed extension into an
// API failure.
func (m *Manager) ExtendIterationTimeout(name, id string) (registry.IterationTimeoutExtension, error) {
	now := m.cfg.Clock()
	it, err := m.cfg.Store.ExtendIterationTimeout(name, id, now)
	if err != nil {
		return registry.IterationTimeoutExtension{}, err
	}
	result := registry.IterationTimeoutExtension{
		TimeoutDeadline: *it.TimeoutDeadline, HardTimeoutDeadline: *it.HardTimeoutDeadline,
		TimeoutExtensions: it.TimeoutExtensions, ShimSync: "pending",
	}
	l := agentdir.New(m.cfg.AgentsDir, name).WithRuntime(m.cfg.RuntimeDir)
	if err := shim.Dial(l.ShimSock()).SetHardDeadline(result.HardTimeoutDeadline); err == nil {
		result.ShimSync = "success"
	}
	data := map[string]any{"timeout_deadline": result.TimeoutDeadline,
		"hard_timeout_deadline": result.HardTimeoutDeadline, "timeout_extensions": result.TimeoutExtensions,
		"shim_sync": result.ShimSync}
	if m.cfg.AuditFor != nil {
		if rec := m.cfg.AuditFor(name); rec != nil {
			rec.Record("iteration_timeout_extended", "system", id, data)
		}
	}
	if m.cfg.Emit != nil {
		m.cfg.Emit(events.Event{Agent: name, Type: "iteration_timeout_extended",
			Time: store.FormatTime(now), Data: map[string]any{"id": id, "shim_sync": result.ShimSync}})
	}
	return result, nil
}

func NewManager(cfg ManagerConfig) *Manager {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.DoneGrace <= 0 {
		cfg.DoneGrace = defaultDoneGrace
	}
	if cfg.Spawner == nil {
		cfg.Spawner = ExecSpawner{}
	}
	if cfg.AdoptPollInterval <= 0 {
		cfg.AdoptPollInterval = defaultAdoptPollInterval
	}
	if cfg.AdoptProbeInterval <= 0 {
		cfg.AdoptProbeInterval = defaultAdoptProbeInterval
	}
	if cfg.AdoptProbeTimeout <= 0 {
		cfg.AdoptProbeTimeout = defaultAdoptProbeTimeout
	}
	if cfg.HasTmuxSession == nil {
		cfg.HasTmuxSession = tmuxHasSession
	}
	if cfg.KillTmuxSession == nil {
		cfg.KillTmuxSession = tmuxKillSession
	}
	if cfg.ConnectShim == nil {
		cfg.ConnectShim = shim.Connect
	}
	m := &Manager{cfg: cfg, runtimes: newAgentRuntimes(), rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	m.scripts = newScriptSupervisor(&m.cfg, &m.wg)
	return m
}

func (m *Manager) signalGoals() {
	if m.cfg.GoalSignal != nil {
		m.cfg.GoalSignal()
	}
}

// noteIterationGoal records the Goal an iteration of agentName runs with, read
// at readAt when its prompt was assembled; key is "" when it has none.
func (m *Manager) noteIterationGoal(agentName, iterationID, key string, readAt time.Time) {
	m.goalsMu.Lock()
	defer m.goalsMu.Unlock()
	if m.goals == nil {
		m.goals = map[string]iterationGoal{}
	}
	m.goals[agentName] = iterationGoal{iterationID: iterationID, key: key, readAt: readAt}
}

// noteSelectedGoal records a Goal the agent selected during an iteration that
// started without one.
func (m *Manager) noteSelectedGoal(agentName, iterationID, key string) {
	m.goalsMu.Lock()
	defer m.goalsMu.Unlock()
	if goal, ok := m.goals[agentName]; ok && goal.iterationID == iterationID && goal.key == "" {
		goal.key = key
		m.goals[agentName] = goal
	}
}

// recordIterationEnd reports a terminal iteration with the Goal it ran with.
func (m *Manager) recordIterationEnd(agentName, iterationID string) {
	m.goalsMu.Lock()
	goal, ok := m.goals[agentName]
	if ok && goal.iterationID == iterationID {
		delete(m.goals, agentName)
	} else {
		goal = iterationGoal{}
	}
	m.goalsMu.Unlock()
	if m.cfg.RecordIterationEnd == nil {
		return
	}
	// A harness that could not be launched or died before producing output
	// did no work and left none undone: it is not an idle iteration.
	if goal.key != "" && m.iterationStatus(agentName, iterationID) == "harness_error" {
		return
	}
	end := tasks.IterationEnd{Agent: agentName, IterationID: iterationID, GoalTaskKey: goal.key,
		StartedAt: goal.readAt, FinishedAt: m.cfg.Clock().UTC()}
	if err := m.cfg.RecordIterationEnd(context.Background(), end); err != nil && m.cfg.Log != nil {
		m.cfg.Log.Warn("record iteration end", "agent", agentName, "id", iterationID, "goal", goal.key, "err", err)
	}
}

// iterationStatus returns the recorded status of an iteration, or "" when it
// cannot be read.
func (m *Manager) iterationStatus(agentName, iterationID string) string {
	if m.cfg.iterationStore == nil && m.cfg.Store == nil {
		return ""
	}
	it, err := m.iterations().GetIteration(agentName, iterationID)
	if err != nil {
		return ""
	}
	return it.Status
}

// iterationCompleted is the single report of a terminal iteration: the
// workflow hears of it first, then the Goal reconciler decides the next wake.
func (m *Manager) iterationCompleted(agentName, iterationID string) {
	m.recordIterationEnd(agentName, iterationID)
	if m.cfg.IterationCompleted != nil {
		m.cfg.IterationCompleted(agentName, iterationID)
	}
}

// reapOrphanSessions kills any agent-named tmux session that has no live
// iteration to adopt and no running-status iteration in the store. A tmux session
// only ever exists for an interactive incarnation, so a session with no live
// iteration is always an orphan — independent of the agent's CURRENT interactive
// flag (which may have since flipped, as in the manager jam). Such a leftover
// survives retention pruning and daemon restarts and would otherwise block every
// future iteration with a duplicate-session error. adopting names agents whose
// live iteration is being re-adopted.
func (m *Manager) reapOrphanSessions(agents []agent.Agent, adopting map[string][]<-chan struct{}) {
	for _, a := range agents {
		if _, beingAdopted := adopting[a.Name]; beingAdopted {
			continue
		}
		if !m.cfg.HasTmuxSession(a.Name) {
			continue
		}
		killed, err := killOrphanSession(m.cfg.Store, m.cfg.KillTmuxSession, a.Name)
		if err != nil {
			m.cfg.Log.Warn("reap orphan tmux session", "agent", a.Name, "err", err)
			continue
		}
		if !killed {
			continue
		}
		m.cfg.Log.Info("reaped orphan tmux session", "agent", a.Name)
		if m.cfg.AuditFor != nil {
			if rec := m.cfg.AuditFor(a.Name); rec != nil {
				rec.Record("session_reaped", "system", "", map[string]any{"reason": "orphan_no_running_iteration"})
			}
		}
	}
}

func (m *Manager) runnerFor(ag agent.Agent) IterationRunner {
	if m.cfg.RunnerFactory != nil {
		return m.cfg.RunnerFactory(ag)
	}
	return NewShimRunner(RunnerConfig{
		AgentsDir: m.cfg.AgentsDir, RuntimeDir: m.cfg.RuntimeDir, BaseEnv: m.cfg.BaseEnv, ShimBin: m.cfg.ShimBin,
		ImgStore: m.cfg.ImgStore, Store: m.cfg.Store, Spawner: m.cfg.Spawner, Clock: m.cfg.Clock,
		DoneGrace: m.cfg.DoneGrace, Logger: m.cfg.Log, Bus: m.cfg.Bus, Proxy: m.cfg.Proxy, AuditFor: m.cfg.AuditFor,
		RecordCursorUsage: m.cfg.RecordCursorUsage, CodexApproval: m.cfg.CodexApproval,
		CurrentGoal: m.cfg.CurrentGoal, WorkflowGoals: m.cfg.WorkflowGoals, Tasks: m.cfg.Tasks,
		GoalRead: m.noteIterationGoal,
	})
}

// refreshShims rewrites each agent's bin shims against the running daemon's
// skill scripts. Disabled agents are included — they may be enabled later and
// must find a working client then. One unwritable agent dir is logged and
// skipped: daemon startup is not a place to die over a single directory.
//
// Each agent validates its own capability-owned direct scripts before changing
// its shims, so a missing script skips that agent without blocking the fleet.
func (m *Manager) refreshShims(agents []agent.Agent) map[string]bool {
	failed := make(map[string]bool)
	for _, a := range agents {
		l := agentdir.New(m.cfg.AgentsDir, a.Name)
		if err := agentdir.WriteShims(l, a); err != nil {
			failed[a.Name] = true
			m.cfg.Log.Error("refresh agent shims", "agent", a.Name, "err", err)
		}
	}
	return failed
}

// StartAll reattaches live iterations and starts every persisted-running agent.
// An agent that owns a live iteration has its engine start deferred until the
// adoption completes, so we never launch a second concurrent shim for it.
func (m *Manager) StartAll(ctx context.Context) error {
	cctx, cancel := context.WithCancel(ctx)
	m.runtimes.setBase(cctx, cancel)
	agents, err := m.cfg.Store.List()
	if err != nil {
		return err
	}
	// Repoint every agent's bin shims at this daemon's client before anything
	// can run: they were written once at provision time with an absolute path
	// to the then-current release, so an upgraded daemon would otherwise keep
	// serving agents frozen (and flag-incompatible) client scripts. This
	// stays above the script supervisor: its first scan runs before the first
	// tick, so a script already due at daemon start would otherwise inherit a
	// PATH pointing at the stale shims.
	shimRefreshFailed := m.refreshShims(agents)

	if m.cfg.Scripts != nil {
		if err := m.cfg.Scripts.RecoverRunning(); err != nil {
			return fmt.Errorf("recover scripts: %w", err)
		}
		m.scripts.startScriptSupervisor(cctx)
	}

	live, err := agentdir.ListLive(m.cfg.AgentsDir, m.cfg.RuntimeDir)
	if err != nil {
		return err
	}
	// Map each agent to the adoption(s) that must finish before its engine may
	// start. A single agent normally owns at most one live iteration.
	adopting := map[string][]<-chan struct{}{}
	for _, li := range live {
		adopting[li.Agent] = append(adopting[li.Agent], m.adopt(cctx, li))
	}
	// An unfinished iteration keeps running across the restart, and its harness
	// reaches the daemon ONLY through the per-agent tools socket — including
	// i-am-done. Binding it here, after adoption has registered the live
	// iteration and before anything waits on that adoption, is what breaks the
	// deadlock: the engine's start is deferred until the iteration ends, the
	// iteration cannot end without i-am-done, and i-am-done needs this socket.
	// A disabled agent (iteration launched by hand) never gets an engine at all,
	// so for it this is the only bind there will ever be.
	m.bindLiveToolsSockets(agents, adopting)
	m.reapOrphanSessions(agents, adopting)
	for _, a := range agents {
		if shimRefreshFailed[a.Name] {
			continue
		}
		if dones, ok := adopting[a.Name]; ok {
			m.startAfter(cctx, a.Name, dones)
			continue
		}
		if !a.Enabled {
			continue
		}
		if err := m.startAgent(a); err != nil {
			m.cfg.Log.Error("reattach start", "agent", a.Name, "err", err)
		}
	}
	return nil
}

// bindLiveToolsSockets binds the tools socket of every agent that owns a live
// iteration, independently of whether its engine will start. A failure is
// logged and not fatal: an enabled agent's start() binds the same path and
// fails loudly there, while for a disabled one there is nothing else to fail.
func (m *Manager) bindLiveToolsSockets(agents []agent.Agent, adopting map[string][]<-chan struct{}) {
	for _, a := range agents {
		if _, live := adopting[a.Name]; !live {
			continue
		}
		l := agentdir.New(m.cfg.AgentsDir, a.Name).WithRuntime(m.cfg.RuntimeDir)
		release, err := m.runtimes.lockAgent(m.runtimes.baseContext(), a.Name)
		if err == nil {
			_, err = m.bindToolsSocketLocked(a, l)
			release()
		}
		if err != nil {
			m.cfg.Log.Error("bind agent tools socket for live iteration",
				"agent", a.Name, "sock", l.Sock(), "err", err)
		}
	}
}

// bindToolsSocketLocked returns the one server that owns the agent's tools
// socket, binding and serving it on first call. Re-binding a path that is
// already served would unlink the live socket file and leave its listener
// serving an orphan (agentapi.Listen removes the path before binding), so every
// caller goes through here instead of calling Listen itself. The agent's
// lifecycle gate must be held: it keeps two binds of one path apart.
func (m *Manager) bindToolsSocketLocked(ag agent.Agent, l agentdir.Layout) (*agentapi.Server, error) {
	if srv := m.runtimes.toolsServer(ag.Name); srv != nil {
		return srv, nil
	}
	srv := m.newToolsAPIServer(ag, l)
	ln, err := srv.Listen(l.Sock())
	if err != nil {
		return nil, err
	}
	m.runtimes.registerToolsServer(ag.Name, srv)
	go func() {
		if err := srv.ServeListener(ln); err != nil {
			m.cfg.Log.Error("tools socket serve", "agent", ag.Name, "err", err)
		}
	}()
	return srv, nil
}

// currentIterationID resolves the iteration a tools call belongs to. The engine
// is authoritative while it runs one; otherwise the live iteration being
// adopted answers, so a harness that outlived a daemon restart is still
// recognized as the caller of its own iteration and can close it.
func (m *Manager) currentIterationID(name string) string {
	return m.runtimes.currentIterationID(name)
}

// startAfter starts an agent's engine only once every pending adoption for it
// has finished (or the manager is shutting down), serializing reattach so a
// freshly started engine does not race the adopted iteration's shim.
func (m *Manager) startAfter(ctx context.Context, name string, dones []<-chan struct{}) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for _, d := range dones {
			select {
			case <-d:
			case <-ctx.Done():
				return
			}
		}
		ag, err := m.cfg.Store.Get(name)
		if err != nil {
			m.cfg.Log.Error("reattach load agent", "agent", name, "err", err)
			return
		}
		if !ag.Enabled {
			return
		}
		if err := m.startAgent(ag); err != nil {
			m.cfg.Log.Error("reattach start", "agent", name, "err", err)
			return
		}
		rt, restart := m.runtimes.pendingRestart(name)
		if restart && ag.Interactive && rt != nil {
			rt.engine.triggerGuarded("", func() (func(), bool) {
				if m.cfg.BeforeRestartLaunch != nil {
					m.cfg.BeforeRestartLaunch()
				}
				return m.acquireRestartLaunch(name)
			})
		}
	}()
}

// acquireRestartLaunch linearizes a post-adoption interactive launch with
// Stop through the agent's lifecycle gate. The caller holds the returned
// release function until the engine has created the iteration and is ready to
// enter its runner. Image activation runs inside it, so the lifecycle gate is
// taken before the publication gate, as everywhere else.
func (m *Manager) acquireRestartLaunch(name string) (func(), bool) {
	release, err := m.runtimes.lockAgent(m.runtimes.baseContext(), name)
	if err != nil {
		return nil, false
	}
	if !m.runtimes.takeRestart(name) {
		release()
		return nil, false
	}
	ag, err := m.cfg.Store.Get(name)
	if err != nil || !ag.Enabled {
		release()
		if err != nil {
			m.cfg.Log.Error("reattach restart load agent", "agent", name, "err", err)
		}
		return nil, false
	}
	return release, true
}

// adopt waits for a live iteration's result.json and records the outcome. If the
// shim socket stops answering Status probes and no result.json ever appears (a
// SIGKILLed shim leaves a stale shim.sock), the iteration is classified
// terminally as harness_error and the stale socket is removed. The returned
// channel is closed once adoption finishes.
func (m *Manager) adopt(ctx context.Context, li agentdir.LiveIteration) <-chan struct{} {
	done := make(chan struct{})
	m.runtimes.beginAdoption(li)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.runtimes.endAdoption(li)
			close(done)
		}()
		l := agentdir.New(m.cfg.AgentsDir, li.Agent).WithRuntime(m.cfg.RuntimeDir)
		w := iterationWatcher{
			store: m.cfg.Store, clock: m.cfg.Clock, log: m.cfg.Log, agent: li.Agent, id: li.ID,
			resultPath: li.ResultPath, shimSock: li.ShimSock, poll: m.cfg.AdoptPollInterval,
			doneGrace: m.cfg.DoneGrace, rpcTimeout: m.cfg.AdoptProbeTimeout,
			probe: &livenessProbe{every: m.cfg.AdoptProbeInterval, maxMisses: adoptMaxMisses},
		}
		res, err := w.watch(ctx)
		switch {
		case err != nil:
			// Shutdown: the iteration stays running for the next daemon.
		case res.stale:
			m.recordStaleAdoption(l, li)
		default:
			m.recordAdopted(l, li, res.result)
		}
	}()
	return done
}

// iterationStore is the narrow slice of *agent.Store that terminal-status
// finalization needs. It exists so a failing terminal CAS can be injected
// (ManagerConfig.iterationStore); every other call site keeps using the
// concrete store.
type iterationStore interface {
	GetIteration(agentName, id string) (agent.Iteration, error)
	FinalizeRunningIteration(it agent.Iteration) (bool, error)
}

// iterations returns the store used for terminal-status finalization.
func (m *Manager) iterations() iterationStore {
	if m.cfg.iterationStore != nil {
		return m.cfg.iterationStore
	}
	return m.cfg.Store
}

// errIterationLookup marks a finalizeIteration failure that happened while
// reading the iteration, before anything on disk or in the store was touched.
var errIterationLookup = errors.New("read iteration")

// finalizeIteration commits a terminal status for one running iteration and
// drops its shim.sock as a single all-or-nothing step. It reports whether the
// terminal status was committed and whether the row was already terminal
// before cleanup began.
//
// Two facts pull the order in opposite directions. The iteration's status is
// what every observer polls, so the socket must be gone *before* the terminal
// status becomes visible — otherwise a finished iteration still exposes a
// shim.sock nobody serves. But agentdir.ListLive recognises an iteration to
// re-classify only by a present shim.sock, so a *failed* status write must
// leave that marker on disk — otherwise the row stays "running" forever,
// hasRunningIteration stays true, and the orphaned tmux session is never
// reaped. No ordering of two independent operations satisfies both; removing
// first and restoring the marker when the write fails satisfies both, and is
// why all three call sites go through here instead of ordering the pair
// themselves.
//
// The removal stays inside the "still running" guard: the shim socket is
// per-agent, not per-iteration, so once this iteration is terminal the file at
// that path may already belong to the agent's next iteration.
func (m *Manager) finalizeIteration(
	l agentdir.Layout,
	agentName, id string,
	apply func(*agent.Iteration),
) (committed, terminalBeforeCleanup bool, err error) {
	it, err := m.iterations().GetIteration(agentName, id)
	if err != nil {
		return false, false, fmt.Errorf("%w: %v", errIterationLookup, err)
	}
	if it.Status != "running" {
		return false, true, nil
	}
	apply(&it)
	_ = os.Remove(l.ShimSock())
	committed, err = m.iterations().FinalizeRunningIteration(it)
	if err != nil {
		// Roll the marker back so adoption can retry this iteration. A plain
		// file is enough: ListLive only stats the path, adoption's probe of a
		// non-socket fails exactly like a dead shim, and every shim removes a
		// leftover before binding (internal/shim/shim.go).
		if f, cerr := os.OpenFile(l.ShimSock(), os.O_CREATE|os.O_WRONLY, 0o600); cerr == nil {
			_ = f.Close()
		} else {
			m.cfg.Log.Error("restore shim.sock marker after failed terminal status",
				"agent", agentName, "id", id, "err", cerr)
		}
		return false, false, err
	}
	return committed, false, nil
}

// terminalHarnessError shapes an iteration into the harness_error outcome used
// when a shim disappears without ever writing a result.
func (m *Manager) terminalHarnessError(it *agent.Iteration) {
	ec := -1
	it.Status = "harness_error"
	it.EndedAt = store.FormatTime(m.cfg.Clock())
	it.ExitCode = &ec
}

// recordAdopted persists a completed adopted iteration from its result.json.
func (m *Manager) recordAdopted(l agentdir.Layout, li agentdir.LiveIteration, res shim.IterationResult) {
	status := ""
	committed, _, err := m.finalizeIteration(l, li.Agent, li.ID, func(it *agent.Iteration) {
		ec, cpu, mem := res.ExitCode, res.CPUMs, res.MemPeakKB
		it.Status = Classify(res.ExitCode, it.DoneFlag, it.TimeoutTriggeredAt != nil, res.TerminationReason == "hard_timeout")
		it.EndedAt = store.FormatTime(m.cfg.Clock())
		it.ExitCode, it.CPUMs, it.MemPeakKB = &ec, &cpu, &mem
		status = it.Status
	})
	if err != nil && !errors.Is(err, errIterationLookup) {
		m.cfg.Log.Error("record adopted iteration: status not committed, shim.sock kept for re-adoption",
			"agent", li.Agent, "id", li.ID, "status", status, "err", err)
	}
	if !committed {
		return
	}
	if m.cfg.Proxy != nil {
		m.cfg.Proxy.RevokeIteration(li.ID)
	}
	m.iterationCompleted(li.Agent, li.ID)
	m.cfg.Log.Info("adopted live iteration", "agent", li.Agent, "id", li.ID, "status", status)
}

// recordStaleAdoption terminally classifies an iteration whose shim vanished
// without ever writing result.json (harness_error, exit_code=-1) and removes the
// stale shim.sock so reattach cannot spin on it forever.
func (m *Manager) recordStaleAdoption(l agentdir.Layout, li agentdir.LiveIteration) {
	committed, terminalBeforeCleanup, err := m.finalizeIteration(l, li.Agent, li.ID, m.terminalHarnessError)
	switch {
	case err == nil && terminalBeforeCleanup:
		// Already terminal, yet ListLive still offered this iteration — so the
		// socket outlived a committed status. Drop it (as this function always
		// did) or reattach spins on it forever. Safe here specifically because
		// adoption gates the agent's next iteration: no newer shim can own the
		// per-agent socket path while we are adopting.
		_ = os.Remove(l.ShimSock())
	case err != nil && !errors.Is(err, errIterationLookup):
		m.cfg.Log.Error("record stale adoption: status not committed, shim.sock kept for re-adoption",
			"agent", li.Agent, "id", li.ID, "err", err)
	}
	if committed && m.cfg.Proxy != nil {
		m.cfg.Proxy.RevokeIteration(li.ID)
	}
	if committed {
		m.iterationCompleted(li.Agent, li.ID)
	}
	m.cfg.Log.Warn("adopted iteration abandoned: shim not responding", "agent", li.Agent, "id", li.ID)
}

func (m *Manager) Run(spec registry.RunSpec) (string, error) {
	var name string
	err := image.WithPublicationGate(func() error {
		var err error
		name, err = m.run(spec)
		return err
	})
	return name, err
}

func (m *Manager) run(spec registry.RunSpec) (string, error) {
	ref, err := image.ParseRef(spec.ImageRef)
	if err != nil {
		return "", err
	}
	archive, err := m.cfg.ImgStore.ArchiveBytes(ref)
	if err != nil {
		return "", fmt.Errorf("image %s: %w", ref.String(), err)
	}
	man, err := image.ValidatePortableArchiveContract(archive, ref, m.imagePluginResolver())
	if err != nil {
		return "", fmt.Errorf("image %s: %w", ref.String(), err)
	}
	name, err := m.newAgentName(spec.Name)
	if err != nil {
		return "", err
	}
	resolvedPlugins, err := m.resolveRunPlugins(spec.Plugins, man)
	if err != nil {
		return "", err
	}
	ag := newRunAgent(spec, name, ref, man, resolvedPlugins)
	// Group membership validates the name and ensures the group entity and shared
	// directory exist before provisioning. Subscriptions are wired by Reconcile
	// after Create.
	if spec.Group != "" {
		if err := m.ensureRunGroup(spec.Group); err != nil {
			return "", err
		}
		ag.Group = spec.Group
	}
	l := agentdir.New(m.cfg.AgentsDir, name)
	if err := agentdir.Provision(l, ag, m.cfg.ImgStore, ref); err != nil {
		return "", err
	}
	if err := m.cfg.Store.Create(ag); err != nil {
		return "", err
	}
	if err := m.ensureOwnInbox(ag.Name); err != nil {
		return "", err
	}
	if spec.Group != "" {
		if err := m.cfg.Groups.Reconcile(spec.Group); err != nil {
			return "", err
		}
	}
	// The agent is created stopped; Start launches it outside the publication
	// gate, which is never held while an agent lifecycle gate is taken.
	return name, nil
}

// newAgentName returns the requested name, or a generated one, once it is
// valid and free.
func (m *Manager) newAgentName(requested string) (string, error) {
	name := requested
	if name == "" {
		name = agent.GenerateName(m.rng)
	}
	// Reject a user-supplied --name that could escape AgentsDir before it reaches
	// agentdir.New/Provision (MkdirAll) or Store.Create. Generated names always
	// pass, so this only refuses a malicious/malformed operator name such as
	// "../../evil". This is the M6 plugin path-traversal fix applied to agents.
	if !agent.ValidName(name) {
		return "", fmt.Errorf("invalid agent name %q: must match ^[a-z0-9][a-z0-9_-]*$", name)
	}
	if ok, _ := m.cfg.Store.Exists(name); ok {
		return "", fmt.Errorf("agent %q already exists", name)
	}
	return name, nil
}

// resolveRunPlugins returns the plugin set of a new agent. Schema v2 is exact:
// plugins are image content, never an implicit core union or a runtime
// override. Schema v1 keeps its historical precedence.
func (m *Manager) resolveRunPlugins(requested []string, man image.Manifest) ([]string, error) {
	if man.SchemaVersion == 2 || len(requested) == 0 {
		requested = nil
		for _, p := range man.Plugins {
			requested = append(requested, p.Name)
		}
	}
	if man.SchemaVersion == 2 {
		return plugincaps.ValidateExplicit(requested, m.imagePluginResolver())
	}
	pluginsDir := filepath.Join(filepath.Dir(m.cfg.ImgStore.Dir), "plugins")
	return plugincaps.ResolveWithExternal(requested, plugins.ResolveInstalled(pluginsDir))
}

func (m *Manager) ensureRunGroup(group string) error {
	if m.cfg.Groups == nil {
		return fmt.Errorf("group provisioning is not available")
	}
	if !agent.ValidName(group) {
		return fmt.Errorf("invalid group name %q: must match ^[a-z0-9][a-z0-9_-]*$", group)
	}
	return m.cfg.Groups.EnsureGroup(group, "")
}

// newRunAgent is the stopped agent row a run spec creates, with defaults for
// the settings the spec leaves unset.
func newRunAgent(spec registry.RunSpec, name string, ref image.Ref, man image.Manifest, resolvedPlugins []string) agent.Agent {
	harnessType, model, effort, env := pick(spec.Harness, "", "claude"), spec.Model, spec.Effort, spec.Env
	if man.SchemaVersion == 1 {
		harnessType = pick(spec.Harness, man.Harness.Type, "claude")
		model = pick(spec.Model, man.Harness.Model, "")
		effort = pick(spec.Effort, man.Harness.Effort, "")
		env = mergeEnv(man.Env, spec.Env)
	}
	if env == nil {
		env = map[string]string{}
	}
	goalEnabled := true
	if spec.GoalEnabled != nil {
		goalEnabled = *spec.GoalEnabled
	}
	return agent.Agent{
		Name: name, ImageRef: ref.String(), ImageDigest: man.Digest,
		Cwd: spec.Cwd, HarnessType: harnessType,
		Model: model, Effort: effort,
		Interactive: spec.Interactive || ref == image.BareRef, Enabled: false,
		IntervalS: spec.IntervalS, TimeoutS: spec.TimeoutS, HardTimeoutS: spec.HardTimeoutS,
		OnTimeout: pick(spec.OnTimeout, "restart"), OnError: pick(spec.OnError, "restart"),
		MaxIdleIterations: spec.MaxIdleIterations,
		AIStallTimeoutS:   spec.AIStallTimeoutS,
		UserPrompt:        spec.UserPrompt,
		Env:               env, Plugins: resolvedPlugins,
		MessagesBatch: orDefault(spec.MessagesBatch, 10), MessagesMaxQueue: orDefault(spec.MessagesMaxQueue, 100),
		GoalEnabled:              goalEnabled,
		GoalWaitCustomerTimeoutS: orDefault(spec.GoalWaitCustomerTimeoutS, 300),
		GoalDeliveryCooldownS:    orDefault(spec.GoalDeliveryCooldownS, 60),
		Alias:                    spec.Alias, Notes: spec.Notes, Color: spec.Color,
	}
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func (m *Manager) ensureOwnInbox(agentName string) error {
	if m.cfg.Bus == nil {
		return fmt.Errorf("message bus is not available")
	}
	_, err := m.cfg.Bus.Subscribe(agentName, bus.InboxChannel(agentName), bus.Matcher{}, nil)
	if err != nil {
		return fmt.Errorf("subscribe agent %q to own inbox: %w", agentName, err)
	}
	// A new agent gets its chats here, not only at the next daemon start, so the
	// customer can open a conversation with it immediately.
	if err := m.cfg.Bus.EnsureAgentChats(agentName); err != nil {
		return fmt.Errorf("provision chats for agent %q: %w", agentName, err)
	}
	return nil
}

// newToolsAPIServer builds the per-agent tools server; see toolsFacade.
func (m *Manager) newToolsAPIServer(ag agent.Agent, l agentdir.Layout) *agentapi.Server {
	return agentapi.NewServer(toolsFacade{
		agent: ag.Name, cfg: &m.cfg, iterations: m, status: m, goals: m, loop: m, groups: m,
		scripts: m.scripts, pluginResolver: m.imagePluginResolver,
	}.deps(ag, l))
}

// startAgent is start under the agent's lifecycle gate.
func (m *Manager) startAgent(ag agent.Agent) error {
	release, err := m.runtimes.lockAgent(m.runtimes.baseContext(), ag.Name)
	if err != nil {
		return err
	}
	defer release()
	return m.start(ag)
}

// start registers and launches the agent's engine, or wakes the live one. The
// caller holds the agent's lifecycle gate.
func (m *Manager) start(ag agent.Agent) error {
	rt, err := m.runtimes.forStart(ag.Name)
	if err != nil {
		return err
	}
	if err := m.cfg.Store.ClearError(ag.Name); err != nil {
		return err
	}
	if err := m.cfg.Store.StartResetIdle(ag.Name); err != nil {
		return err
	}
	if rt != nil {
		// The runtime is still live (e.g. Start after Stop, where the engine
		// goroutine survived). The stale halt reason is cleared and a fresh idle
		// budget granted above (reset the idle-stop boundary + clear stale idle
		// status); wake the parked engine so a re-enable is picked up now, not
		// on some future timer.
		rt.engine.Wake(WakeStart)
		return nil
	}
	agName := ag.Name
	l := agentdir.New(m.cfg.AgentsDir, ag.Name).WithRuntime(m.cfg.RuntimeDir)
	engine := NewEngine(ag, m.cfg.Store, m.runnerFor(ag), m.cfg.Log, m.cfg.Clock)
	if m.cfg.ImgStore != nil {
		engine.SetBeforeLaunch(m.activatePendingImage)
	}
	engine.metrics = m.cfg.Metrics
	engine.usageLookup = m.cfg.UsageLookup
	engine.usageFlush = m.cfg.UsageFlush
	if m.cfg.Bus != nil {
		engine.SetMessagePeek(func() (bool, error) { return m.cfg.Bus.HasPending(agName) })
	}
	if m.cfg.Emit != nil {
		engine.SetEmit(m.cfg.Emit)
	}
	if m.cfg.AuditFor != nil {
		engine.SetAudit(func(typ, source, iterationID string, data map[string]any) {
			if rec := m.cfg.AuditFor(agName); rec != nil {
				rec.Record(typ, source, iterationID, data)
			}
		})
	}
	engine.SetOnIterationClose(func(agentName, iterationID string) {
		// A stale kill is idempotent only while the recovered iteration is
		// unwinding. Once that iteration has closed, retaining its marker would
		// incorrectly make future Kill calls against an idle or stopped agent
		// succeed forever.
		m.runtimes.clearStaleKill(agentName, iterationID)
		if m.cfg.OnIterationClose != nil {
			m.cfg.OnIterationClose(agentName, iterationID)
		}
	})
	engine.SetIterationCompleted(m.iterationCompleted)

	// Bind the per-agent tools socket SYNCHRONOUSLY before launching the loop.
	// The harness reaches the daemon (loop done, context, messages, ...) only
	// through this socket; if the bind fails and we started the loop anyway, the
	// harness would run with no control plane and every iteration would silently
	// end in no_i_am_done. So a bind failure fails the start loudly and rolls the
	// agent back out of "running" instead of degrading in the dark. An early bind
	// from boot reconciliation is taken over here rather than replaced.
	apiSrv, err := m.bindToolsSocketLocked(ag, l)
	if err != nil {
		m.cfg.Log.Error("bind agent tools socket", "agent", ag.Name, "sock", l.Sock(), "err", err)
		if serr := m.cfg.Store.SetError(ag.Name, "bind tools socket failed"); serr != nil {
			m.cfg.Log.Error("rollback agent state", "agent", ag.Name, "err", serr)
		}
		return fmt.Errorf("bind agent tools socket %s: %w", l.Sock(), err)
	}

	ectx, cancel := context.WithCancel(m.runtimes.baseContext())
	m.runtimes.install(ag.Name, &runtime{engine: engine, cancel: cancel, apiServer: apiSrv})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		engine.Run(ectx)
	}()
	if m.cfg.Bus != nil {
		// Nudge the freshly launched engine so a message-capable agent (peek
		// installed above) that already has unacked pending deliveries drains
		// them now instead of waiting on some unrelated future publish. This is
		// the common entry point for a fresh Run, Start, and every reattach path
		// (StartAll's direct start and startAfter's post-adoption start), so one
		// call here covers all of them. The engine's WakeMessage handler is a
		// no-op unless the agent iterates and hasPending(), so this is harmless for
		// interval-only or currently-empty agents.
		engine.Wake(WakeMessage)
	}
	return nil
}

// Start is the master enable: it turns the whole agent on. It records
// Enabled=true, registers/wakes the loop engine, and — for an interactive agent
// — launches an iteration so the tmux/PTY session comes up (an interactive agent
// has no autonomous tick to spin one, so it would otherwise park at idle with no
// session to attach to).
// emitState tells live listeners (the agent states socket) that name's state
// may have changed, so the UI refetches it instead of polling often.
func (m *Manager) emitState(name string) {
	if m.cfg.Emit != nil {
		m.cfg.Emit(events.Event{Agent: name, Type: "state", Time: store.FormatTime(m.cfg.Clock())})
	}
}

func (m *Manager) Start(name string) error {
	ag, err := m.cfg.Store.Get(name)
	if err != nil {
		return err
	}
	ag.Enabled = true
	if err := m.cfg.Store.Update(ag); err != nil {
		return err
	}
	m.emitState(name)
	m.signalGoals()
	if err := m.startAgent(ag); err != nil {
		if errors.Is(err, errIterationAdopting) {
			return nil
		}
		return err
	}
	if ag.Interactive {
		if _, err := m.Exec(name, ""); err != nil {
			return err
		}
	}
	return nil
}

// Stop is the master disable: it turns the whole agent off. It records
// Enabled=false FIRST (so a killed iteration is not relaunched — the engine's
// reload sees the master switch off and parks), wakes the engine to repark, then
// kills any in-flight iteration / interactive session. Kill is best-effort: an
// enabled-but-idle agent has nothing to kill.
func (m *Manager) Stop(name string) error {
	// The lifecycle gate linearizes Stop with a post-adoption Restart launch:
	// either Stop cancels the pending launch, or the launch has created its
	// iteration row and the Kill below finds it.
	release, err := m.runtimes.lockAgent(m.runtimes.baseContext(), name)
	if err != nil {
		return err
	}
	ag, err := m.cfg.Store.Get(name)
	if err != nil {
		release()
		return err
	}
	ag.Enabled = false
	if err := m.cfg.Store.Update(ag); err != nil {
		release()
		return err
	}
	if rt := m.runtimes.stopRuntime(name); rt != nil {
		rt.engine.Wake(WakeStop)
	}
	release()
	m.emitState(name)
	m.signalGoals()
	_ = m.Kill(name)
	return nil
}

// RefreshLoopConfig asks a live engine to discard its timer and reload the
// persisted settings. A stopped or not-yet-started agent has nothing to wake.
func (m *Manager) RefreshLoopConfig(name string) {
	m.signalGoals()
	if rt := m.runtimes.runtime(name); rt != nil {
		rt.engine.Wake(WakeConfig)
	}
}

func (m *Manager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
	}
	ag, err := m.cfg.Store.Get(name)
	if err != nil {
		return err
	}
	ag.Enabled = true
	if err := m.cfg.Store.Update(ag); err != nil {
		return err
	}
	m.signalGoals()
	if m.runtimes.markRestartIfAdopting(name, ag.Interactive) {
		return nil
	}
	if err := m.startAgent(ag); err != nil {
		return err
	}
	if ag.Interactive {
		_, err = m.Exec(name, "")
	}
	return err
}

// WakeAgents wakes the named running engines. Used by the bus publish hook, the
// scheduler and the scripts result path to nudge affected agents.
func (m *Manager) WakeAgents(agents []string, kind WakeKind) {
	for _, engine := range m.runtimes.engines(agents) {
		engine.Wake(kind)
	}
}

func (m *Manager) Kill(name string) error {
	client, target, err := m.currentShimClient(name)
	if err != nil {
		if target.runtime != nil {
			l := agentdir.New(m.cfg.AgentsDir, name).WithRuntime(m.cfg.RuntimeDir)
			return m.recoverStaleKill(name, target.id, target.runtime, l, err)
		}
		if m.runtimes.staleKill(name) != "" {
			return nil
		}
		return err
	}
	l := agentdir.New(m.cfg.AgentsDir, name).WithRuntime(m.cfg.RuntimeDir)
	if err := client.Kill(); err != nil {
		if target.runtime == nil {
			return err
		}
		var rpcErr *shim.RPCError
		if errors.As(err, &rpcErr) {
			return err
		}
		return m.recoverStaleKill(name, target.id, target.runtime, l, err)
	}
	return nil
}

// recoverStaleKill finalizes an iteration whose shim cannot be contacted. It
// cancels only that iteration, so its normal loop remains able to launch again.
func (m *Manager) recoverStaleKill(name, id string, rt *runtime, l agentdir.Layout, rpcErr error) error {
	// Same finalizer as the adoption paths, so a failing store leaves the same
	// state everywhere: status still "running" and the shim.sock marker on disk
	// for a later adoption to retry. Before, this path alone kept the socket by
	// never reaching the removal, while the adoption paths dropped it.
	committed, terminalBeforeCleanup, err := m.finalizeIteration(l, name, id, m.terminalHarnessError)
	switch {
	case errors.Is(err, errIterationLookup):
		return fmt.Errorf("kill shim for %q: %w", name, rpcErr)
	case err != nil:
		return fmt.Errorf("recover stale shim for %q: %w", name, err)
	case terminalBeforeCleanup:
		// Already terminal: the socket must not outlive a committed status.
		_ = os.Remove(l.ShimSock())
	}
	if committed {
		m.iterationCompleted(name, id)
	}
	m.runtimes.markStaleKill(name, id, rt)
	m.cfg.Log.Warn("kill recovered unreachable shim", "agent", name, "id", id, "err", rpcErr)
	return nil
}

func (m *Manager) Exec(name, prompt string) (string, error) {
	rt := m.runtimes.runtime(name)
	if rt == nil {
		ag, err := m.cfg.Store.Get(name)
		if err != nil {
			return "", err
		}
		if err := m.startAgent(ag); err != nil {
			return "", err
		}
		rt = m.runtimes.runtime(name)
		if rt == nil {
			return "", fmt.Errorf("agent %q is not running after start", name)
		}
	}
	rt.engine.Trigger(prompt)
	return "queued", nil
}

// RerunScript queues an immediate attempt for an eligible stored definition.
func (m *Manager) RerunScript(agentName, scriptID string) (script.Run, error) {
	return m.scripts.RerunScript(agentName, scriptID)
}

// RunOnce creates one local command definition and one immediate asynchronous run.
func (m *Manager) RunOnce(agentName string, in script.CreateOnce) (script.Definition, script.Run, error) {
	return m.scripts.RunOnce(agentName, in)
}

// ScheduleScript creates a fixed-delay recurring local command and its first
// immediate run. Later runs are queued only after the previous run finishes.
func (m *Manager) ScheduleScript(agentName string, in script.CreateSchedule) (script.Definition, script.Run, error) {
	return m.scripts.ScheduleScript(agentName, in)
}

func (m *Manager) ListScripts(agentName string) ([]script.Definition, error) {
	return m.scripts.ListScripts(agentName)
}

func (m *Manager) ListScriptRuns(agentName, scriptID string) ([]script.Run, error) {
	return m.scripts.ListScriptRuns(agentName, scriptID)
}

func (m *Manager) GetScriptRun(agentName, runID string) (script.Run, error) {
	return m.scripts.GetScriptRun(agentName, runID)
}

// CancelScriptTarget records cancellation intent for a running attempt before
// terminating its process group. The run remains active until Wait confirms
// process exit, preserving the per-definition no-overlap invariant.
func (m *Manager) CancelScriptTarget(agentName, id string) error {
	return m.scripts.CancelScriptTarget(agentName, id)
}

func (m *Manager) RemoveScript(agentName, id string) error {
	return m.scripts.RemoveScript(agentName, id)
}

// LogScript returns a bounded tail after confirming the record belongs to the
// requested agent. Missing logs are represented by an empty string.
func (m *Manager) LogScriptRun(agentName, id string) (string, error) {
	return m.scripts.LogScriptRun(agentName, id)
}

func (m *Manager) OpenScriptLog(agentName, runID string) (io.ReadCloser, string, error) {
	return m.scripts.OpenScriptLog(agentName, runID)
}

// Remove tears an agent down. force only bypasses the running-guard; it is
// orthogonal to purge. With purge=false (the data-preserving default for a plain
// compose down) the agents DB row is kept and left stopped, together with every
// durable artifact — CONTEXT.md, iterations (dir + rows), and audit.jsonl
// — while only the rebuildable image/bin/workdir tree is dropped, so a later
// Reprovision re-unpacks a (possibly new) image in place over the retained
// history. With purge=true it is a full hard delete that also cleans the
// agent-keyed side-table rows Store.Delete leaves behind (see PurgeAgentData).
func (m *Manager) Remove(name string, force, purge bool) error {
	release, err := m.runtimes.lockAgent(m.runtimes.baseContext(), name)
	if err != nil {
		return err
	}
	ag, err := m.cfg.Store.Get(name)
	if err != nil {
		release()
		return err
	}
	servers, err := m.runtimes.remove(name, ag.Enabled, force)
	// Released before the graceful shutdown: an in-flight tools call of this
	// agent may itself wait for the gate (loop start/stop).
	release()
	if err != nil {
		return err
	}
	shutdownAgentAPIs(servers)

	l := agentdir.New(m.cfg.AgentsDir, name)
	rl := l.WithRuntime(m.cfg.RuntimeDir)
	// Runtime sockets are pure runtime, never data — drop them in either path.
	_ = os.Remove(rl.Sock())
	_ = os.Remove(rl.ShimSock())

	if !purge {
		// Preserve: leave the agent stopped and every durable
		// artifact intact; remove only the rebuildable tree.
		ag.Enabled = false
		if err := m.cfg.Store.Update(ag); err != nil {
			return err
		}
		for _, d := range []string{l.ImageDir(), l.BinDir(), l.Workdir()} {
			if err := os.RemoveAll(d); err != nil {
				return err
			}
		}
		return nil
	}

	// Purge: hard-delete the core rows (iterations/secrets/agents), the leaked
	// agent-keyed side-table rows, and the whole durable tree.
	if err := m.cfg.Store.Delete(name); err != nil {
		return err
	}
	if err := m.cfg.Store.PurgeAgentData(name); err != nil {
		return err
	}
	return os.RemoveAll(l.Root)
}

// Reprovision re-unpacks image into an existing agent's rebuildable tree
// (image/bin/workdir) and restarts its loop, WITHOUT touching CONTEXT.md,
// iterations or audit.jsonl. It is the up-side counterpart of a preserving
// Remove: after a data-preserving down, up calls this to bring the agent back on
// the (possibly new) image while keeping its history. When imageRef differs from
// the stored one this performs an in-place image swap (the row's image_ref/digest
// are updated to the new image). An empty imageRef keeps the agent's current
// image.
func (m *Manager) Reprovision(name, imageRef string) error {
	release, err := m.runtimes.lockAgent(m.runtimes.baseContext(), name)
	if err != nil {
		return err
	}
	defer release()
	return image.WithPublicationGate(func() error { return m.reprovision(name, imageRef) })
}

func (m *Manager) reprovision(name, imageRef string) error {
	ag, err := m.cfg.Store.Get(name)
	if err != nil {
		return err
	}
	candidate, ref, man, err := m.reprovisionCandidate(ag, imageRef)
	if err != nil {
		return err
	}
	if err := m.installReprovisionedImage(ag, candidate, ref, man); err != nil {
		return err
	}
	m.signalGoals()
	return m.start(candidate)
}

func (m *Manager) imagePluginResolver() plugincaps.ExternalResolver {
	if m.cfg.ExternalPlugins != nil {
		return m.cfg.ExternalPlugins
	}
	return plugins.ResolveInstalledMetadata(filepath.Join(filepath.Dir(m.cfg.ImgStore.Dir), "plugins"))
}

func (m *Manager) Screen(name string) (string, error) {
	client, _, err := m.currentShimClient(name)
	if err != nil {
		return "", err
	}
	return client.Screen()
}

func (m *Manager) SendKeys(name, keys string) error {
	client, _, err := m.currentShimClient(name)
	if err != nil {
		return err
	}
	return client.SendKeys(keys)
}

func (m *Manager) SendKeysItems(name string, items []shim.KeyItem) error {
	client, _, err := m.currentShimClient(name)
	if err != nil {
		return err
	}
	return client.SendKeysItems(items)
}

func (m *Manager) Attach(name string, cols, rows int) (net.Conn, error) {
	client, _, err := m.currentShimClient(name)
	if err != nil {
		return nil, err
	}
	return client.Attach(cols, rows)
}

func (m *Manager) Resize(name string, cols, rows int) error {
	client, _, err := m.currentShimClient(name)
	if err != nil {
		return err
	}
	return client.Resize(cols, rows)
}

// currentShimClient resolves the current iteration and connects to its shim
// while holding the agent's handoff lock for reading. Adoption cleanup takes
// that lock for writing before it releases startAfter, so a stable per-agent
// socket path cannot be rebound to a replacement iteration between resolution
// and connection.
func (m *Manager) currentShimClient(name string) (*shim.Client, iterationTarget, error) {
	handoff := &m.runtimes.locksFor(name).handoff
	handoff.RLock()
	defer handoff.RUnlock()
	sock := agentdir.New(m.cfg.AgentsDir, name).WithRuntime(m.cfg.RuntimeDir).ShimSock()
	target, err := m.runtimes.currentTarget(name, sock)
	if err != nil {
		return nil, iterationTarget{}, err
	}
	client, err := m.cfg.ConnectShim(target.sock)
	if err != nil {
		return nil, target, err
	}
	return client, target, nil
}

// LiveState derives an agent's reported lifecycle state from reality — never
// from a persisted column. The master !enabled switch wins over everything
// else, reporting "stopped" regardless of a stale error_reason; then error_reason wins; then an actually-executing
// iteration (a live engine with a current id, or a live iteration on disk
// during the post-crash adopt window) reports "running" unless its AI-proxy
// activity is stale; otherwise an
// enabled agent with nothing live is "idle".
func (m *Manager) LiveState(name string) (string, error) {
	a, err := m.cfg.Store.Get(name)
	if err != nil {
		return "", err
	}
	return m.LiveStateOf(a), nil
}

// LiveStateOf is LiveState for an agent row the caller has already read.
func (m *Manager) LiveStateOf(a agent.Agent) string {
	name := a.Name
	// Master switch wins: a disabled agent is "stopped" regardless of a stale
	// error_reason.
	if !a.Enabled {
		return "stopped"
	}
	if a.ErrorReason != "" {
		return "error"
	}
	if iterationID := m.runtimes.currentIterationID(name); iterationID != "" {
		it, err := m.cfg.Store.GetIteration(name, iterationID)
		if err == nil {
			last := it.StartedAt
			if it.LastAIRequestAt != "" {
				last = it.LastAIRequestAt
			}
			if at, err := store.ParseTime(last); err == nil &&
				m.cfg.Clock().Sub(at) >= time.Duration(a.AIStallTimeoutS)*time.Second {
				return "error"
			}
		}
		return "running"
	}
	if m.hasLiveIterationOnDisk(name) {
		return "running"
	}
	// Enabled but nothing executing (loop on-waiting or loop off) → idle.
	return "idle"
}

// hasLiveIterationOnDisk reports whether a shim socket exists for the agent's
// current iteration — the signal that an iteration is mid-flight during the
// reattach/adopt window before the engine is registered in runs.
func (m *Manager) hasLiveIterationOnDisk(name string) bool {
	sock := agentdir.New(m.cfg.AgentsDir, name).WithRuntime(m.cfg.RuntimeDir).ShimSock()
	_, err := os.Stat(sock)
	return err == nil
}

// ActiveAgents is the number of running per-agent loop engines — the
// active-agents gauge for telemetry (spec §14).
func (m *Manager) ActiveAgents() int {
	return m.runtimes.count()
}

// Shutdown cancels every engine and reattach goroutine and waits for them to
// quiesce (bounded by shutdownWait) so daemon teardown does not race the store
// Close that follows it.
func (m *Manager) Shutdown() {
	shutdownAgentAPIs(m.runtimes.shutdown())

	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(shutdownWait):
		m.cfg.Log.Warn("shutdown: goroutines did not quiesce in time", "timeout", shutdownWait)
	}
}

func shutdownAgentAPIs(servers map[*agentapi.Server]struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownWait)
	defer cancel()
	for srv := range servers {
		if srv != nil {
			_ = srv.Shutdown(ctx)
		}
	}
}

func (m *Manager) callerGroup(caller string) (agent.Agent, string, string, error) {
	ag, err := m.cfg.Store.Get(caller)
	if err != nil {
		return agent.Agent{}, "", "", err
	}
	if ag.Group == "" {
		return agent.Agent{}, "", "", fmt.Errorf("agent %q is not assigned to a group", caller)
	}
	if m.cfg.Groups == nil {
		return agent.Agent{}, "", "", fmt.Errorf("group control is not configured")
	}
	info, err := m.cfg.Groups.Inspect(ag.Group)
	if err != nil {
		return agent.Agent{}, "", "", err
	}
	lead, _ := info["lead"].(string)
	return ag, ag.Group, lead, nil
}

func (m *Manager) groupMember(caller, member string) (agent.Agent, string, string, error) {
	_, group, lead, err := m.callerGroup(caller)
	if err != nil {
		return agent.Agent{}, "", "", err
	}
	if member == "" {
		return agent.Agent{}, "", "", fmt.Errorf("member is required")
	}
	members, err := m.cfg.Store.ListByGroup(group)
	if err != nil {
		return agent.Agent{}, "", "", err
	}
	for _, a := range members {
		if a.Name == member {
			return a, group, lead, nil
		}
	}
	return agent.Agent{}, "", "", fmt.Errorf("agent %q is not in your group", member)
}

func (m *Manager) groupToolsInfo(caller string) (map[string]any, error) {
	ag, group, lead, err := m.callerGroup(caller)
	if err != nil {
		return nil, err
	}
	info, err := m.cfg.Groups.Inspect(group)
	if err != nil {
		return nil, err
	}
	info["agent"] = ag.Name
	info["group"] = group
	info["role"] = roleOf(ag.Name, lead)
	return info, nil
}

func (m *Manager) groupToolsStatus(caller, member string) (map[string]any, error) {
	_, group, lead, err := m.callerGroup(caller)
	if err != nil {
		return nil, err
	}
	members, err := m.cfg.Store.ListByGroup(group)
	if err != nil {
		return nil, err
	}
	if member != "" {
		for _, a := range members {
			if a.Name == member {
				return map[string]any{"member": m.groupStatusRow(a, lead)}, nil
			}
		}
		return nil, fmt.Errorf("agent %q is not in your group", member)
	}
	rows := make([]map[string]any, 0, len(members))
	for _, a := range members {
		rows = append(rows, m.groupStatusRow(a, lead))
	}
	return map[string]any{"members": rows, "count": len(rows)}, nil
}

func (m *Manager) groupStatusRow(a agent.Agent, lead string) map[string]any {
	state, _ := m.LiveState(a.Name)
	row := map[string]any{
		"name": a.Name, "role": roleOf(a.Name, lead), "state": state,
		"enabled": a.Enabled, "status_message": a.StatusMessage,
		"status_updated": a.StatusUpdated,
	}
	if m.runtimes.runtime(a.Name) != nil {
		row["current_iteration"] = m.runtimes.engineIterationID(a.Name)
	}
	return row
}

func roleOf(agentName, lead string) string {
	if agentName == lead {
		return "lead"
	}
	return "member"
}

func (m *Manager) groupToolsSend(caller, iterationID, member, typ, text, deadline string) (map[string]any, error) {
	if m.cfg.Bus == nil {
		return nil, fmt.Errorf("bus not configured")
	}
	_, group, _, err := m.groupMember(caller, member)
	if err != nil {
		return nil, err
	}
	// group request is the request primitive (spec §4.2): publish kind=request to
	// the member's group:<g>:direct:<member> channel with a fresh correlation id,
	// reply_to=caller inbox, and — with a deadline — an armed one-shot timeout.
	// The member's reply retires the caller's pending (# Awaiting replies) and
	// cancels the timeout. No bespoke request bookkeeping: this replaces the old
	// path that hand-stuffed data["deadline"] onto a plain publish and dropped
	// kind/correlation_id.
	if typ == "group.request" {
		req, err := m.cfg.Bus.Request(caller, bus.GroupDirect(group, member), text, deadline)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sent": true, "id": req.ID, "target": member, "channel": req.Channel,
			"type": req.Type, "kind": req.Kind, "correlation_id": req.CorrelationID}, nil
	}
	msg, err := m.cfg.Bus.Publish(bus.Message{
		Channel: bus.InboxChannel(member), Type: typ, Text: text, Source: "agent:" + caller,
		ProducedByAgent: caller, ProducedInIteration: iterationID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"sent": true, "id": msg.ID, "target": member, "channel": msg.Channel, "type": msg.Type}, nil
}

func (m *Manager) groupToolsObserve(caller, member string, tail int) (map[string]any, error) {
	if _, _, _, err := m.groupMember(caller, member); err != nil {
		return nil, err
	}
	evs, err := audit.ReadEvents(agentdir.New(m.cfg.AgentsDir, member).AuditLog(), tail, 0)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(evs))
	for _, ev := range evs {
		data := "{}"
		if ev.Data != nil {
			if b, err := json.Marshal(ev.Data); err == nil {
				data = string(b)
			}
		}
		rows = append(rows, map[string]any{"seq": ev.Seq, "kind": ev.Type, "source": ev.Source,
			"iteration_id": ev.IterationID, "data": data, "at": ev.TS})
	}
	return map[string]any{"member": member, "events": rows, "count": len(rows), "tail": tail}, nil
}

func (m *Manager) groupToolsLoop(caller, member, action string) (map[string]any, error) {
	if _, _, _, err := m.groupMember(caller, member); err != nil {
		return nil, err
	}
	updated, err := m.toolsLoopControl(member, action)
	if err != nil {
		return nil, err
	}
	return map[string]any{"member": member, "action": action,
		"enabled": updated.Enabled}, nil
}

// toolsLoopControl is the agent-facing Start/Stop and returns the stored row.
func (m *Manager) toolsLoopControl(name, action string) (agent.Agent, error) {
	switch action {
	case "start":
		if err := m.Start(name); err != nil {
			return agent.Agent{}, err
		}
	case "stop":
		if err := m.Stop(name); err != nil {
			return agent.Agent{}, err
		}
	default:
		return agent.Agent{}, fmt.Errorf("unknown loop action %q", action)
	}
	return m.cfg.Store.Get(name)
}

// buildImageForAgent authors + builds a new image from a Tariboyfile. Relative
// paths resolve against the managed workdir; absolute paths are used as given.
// It calls image.BuildV2 against the daemon's shared image store through the
// one publication mechanism the operator CLI uses, so an existing ref — latest
// included — is moved onto the new content instead of being refused.
func buildImageForAgent(imgStore *image.Store, workdir, name, tag, path string, externalPlugins ...plugincaps.ExternalResolver) (map[string]any, error) {
	var result map[string]any
	err := image.WithPublicationGate(func() error {
		var err error
		result, err = buildImageForAgentLocked(imgStore, workdir, name, tag, path, externalPlugins...)
		return err
	})
	return result, err
}

func buildImageForAgentLocked(imgStore *image.Store, workdir, name, tag, path string, externalPlugins ...plugincaps.ExternalResolver) (map[string]any, error) {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(workdir, abs)
	}
	abs, err := filepath.EvalSymlinks(filepath.Clean(abs))
	if err != nil {
		return nil, err
	}
	parsed, err := imagefile.ParseV2(abs)
	if err != nil {
		return nil, err
	}
	// An empty tag publishes the declared image_version and latest, exactly as
	// the operator CLI does.
	var requested []string
	if tag != "" {
		requested = []string{tag}
	}
	refs, _, err := image.BuildRefs(name, requested, parsed.ImageVersion)
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(imgStore.Dir)
	layout := paths.Paths{Base: baseDir}
	pluginsDir := layout.PluginsDir()
	if len(parsed.Extends) > 0 {
		assembled, cleanup, err := imagefile.Assemble(abs, filepath.Join(baseDir, "image-build"), imagefile.ResolveRoots{Plugins: pluginsDir}, nil)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		if parsed, err = imagefile.ParseV2(assembled); err != nil {
			return nil, err
		}
	}
	resolver := plugins.ResolveInstalledMetadata(pluginsDir)
	if len(externalPlugins) > 0 && externalPlugins[0] != nil {
		resolver = externalPlugins[0]
	}
	if err := imgStore.Migrate(); err != nil {
		return nil, err
	}
	man, err := image.BuildV2(parsed, imagefile.ResolveRoots{Plugins: pluginsDir}, refs[0], imgStore, time.Now, resolver)
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref != refs[0] {
			if err := imgStore.SetTag(ref, man.Digest); err != nil {
				return nil, err
			}
		}
		tags = append(tags, ref.Tag)
	}
	return map[string]any{"name": man.Name, "tag": refs[0].Tag, "tags": tags, "digest": man.Digest, "layers": len(man.Layers)}, nil
}

func cwdOf(ag agent.Agent, l agentdir.Layout) string {
	if ag.Cwd != "" {
		return ag.Cwd
	}
	return l.Workdir()
}

func pick(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func mergeEnv(base, over map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// ensure the interface is satisfied at compile time
var _ registry.ServiceControl = (*Manager)(nil)
