// Package loop runs one iteration scheduler per agent (spec §5.2). The state
// machine wakes on interval cadence, the RRULE schedule, a message or a manual
// trigger, runs a single iteration to completion, records it, and applies
// on_timeout/on_error policy.
package loop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/telemetry"
)

type TickResult string

const (
	TickDisabled         TickResult = "disabled"
	TickStarted          TickResult = "started"
	TickRestarted        TickResult = "restarted"
	TickCompletedWaiting TickResult = "completed_waiting"
	TickAwaitingPrevExit TickResult = "awaiting_prev_exit"
	TickRunning          TickResult = "running"
	TickTimeoutStop      TickResult = "timeout_stop"
	TickIdleStop         TickResult = "idle_stop"
	TickTimeoutRestart   TickResult = "timeout_restart"
	TickProcessDied      TickResult = "process_died"
	TickErrorHalt        TickResult = "error_halt"
	TickError            TickResult = "tick_error"
	TickSkipped          TickResult = "skipped"
	TickDetached         TickResult = "detached"
)

// Outcome is what the runner reports for one iteration.
type Outcome struct {
	Status   string // done | no_i_am_done | harness_error | timeout | killed
	ExitCode int
	DoneFlag bool
	// Productive carries the iteration's productive flag (see agent.Iteration).
	// It is read back from the DB so the engine's belt-and-suspenders
	// SetIterationDone does not clobber a `--idle` declaration made via the API.
	Productive bool
	CPUMs      int
	MemPeakKB  int
	// Error is the human-readable reason of a harness_error outcome, recorded
	// on the iteration_finished audit event.
	Error string
}

type IterationRunner interface {
	Run(ctx context.Context, ag agent.Agent, trigger, iterationID, oneShot string) (Outcome, error)
}

type manualReq struct {
	oneShot    string
	launchGate func() (release func(), ok bool)
}

// WakeKind names an external event that must re-evaluate or run the loop.
// WakeConfig/WakeStart/WakeStop only re-read config (the run loop stops its
// timer and reloads); WakeMessage additionally runs a message-triggered
// iteration and is wired in Task 5.
type WakeKind int

const (
	WakeConfig WakeKind = iota
	WakeStart
	WakeStop
	WakeMessage
)

// loopTimer is the stoppable timer seam used by Run. Production wraps
// time.NewTimer; tests inject a fake to count creations and record Stop calls so
// abandoned timers cannot silently leak.
type loopTimer interface {
	C() <-chan time.Time
	Stop()
}

type realTimer struct{ t *time.Timer }

func (r *realTimer) C() <-chan time.Time { return r.t.C }
func (r *realTimer) Stop()               { r.t.Stop() }

func newRealTimer(d time.Duration) loopTimer { return &realTimer{t: time.NewTimer(d)} }

type Engine struct {
	ag       agent.Agent
	store    *agent.Store
	runner   IterationRunner
	log      *slog.Logger
	clock    func() time.Time
	newTimer func(d time.Duration) loopTimer

	metrics     *telemetry.Metrics
	usageLookup func(iteration string) (int, int, float64)
	usageFlush  func()

	manualCh chan manualReq
	events   chan WakeKind
	// msgWake owns the message-trigger signal, coalescing state and retry
	// timer (see messageWake).
	msgWake            *messageWake
	peek               func() (bool, error)
	emit               func(events.Event)
	audit              func(typ, source, iterationID string, data map[string]any)
	onClose            func(agent, iterationID string)
	iterationCompleted func(agent, iterationID string)
	beforeLaunch       func(*agent.Agent) (activatedImage, error)

	mu            sync.Mutex
	current       string // current iteration id ("" when idle)
	currentCancel context.CancelFunc
	lastRun       time.Time // start of the previous iteration
}

func (e *Engine) SetBeforeLaunch(fn func(*agent.Agent) (activatedImage, error)) { e.beforeLaunch = fn }

func NewEngine(ag agent.Agent, st *agent.Store, runner IterationRunner, log *slog.Logger,
	clock func() time.Time) *Engine {
	return &Engine{
		ag: ag, store: st, runner: runner, log: log, clock: clock, newTimer: newRealTimer,
		manualCh: make(chan manualReq, 8),
		events:   make(chan WakeKind, 16),
		msgWake:  newMessageWake(),
	}
}

// Trigger requests a manual iteration with an optional one-shot prompt.
func (e *Engine) Trigger(oneShot string) {
	e.queueManual(manualReq{oneShot: oneShot})
}

func (e *Engine) triggerGuarded(oneShot string, gate func() (func(), bool)) {
	e.queueManual(manualReq{oneShot: oneShot, launchGate: gate})
}

func (e *Engine) queueManual(req manualReq) {
	select {
	case e.manualCh <- req:
	default:
		e.log.Warn("manual trigger dropped: queue full", "agent", e.ag.Name)
	}
}

// Wake nudges the run loop to re-evaluate (start/stop/config) or, for
// WakeMessage, to run a message-triggered iteration. Non-blocking: an
// undrained or shutting-down engine simply drops the nudge.
func (e *Engine) Wake(k WakeKind) {
	if k == WakeMessage {
		e.msgWake.queue()
		return
	}
	select {
	case e.events <- k:
	default:
	}
}

// SetMessagePeek installs a non-consuming "are there pending messages?" probe
// (bus.HasPending). Nil means the engine never message-triggers.
func (e *Engine) SetMessagePeek(fn func() (bool, error)) { e.peek = fn }

// SetEmit installs an iteration-lifecycle event sink (SSE hub). Nil disables it.
func (e *Engine) SetEmit(fn func(events.Event)) { e.emit = fn }

// SetAudit installs the durable audit-log sink (per-agent audit.jsonl). Nil
// disables it. Best-effort by contract — the installed function must not fail
// into the loop.
func (e *Engine) SetAudit(fn func(typ, source, iterationID string, data map[string]any)) {
	e.audit = fn
}

func (e *Engine) recordAudit(typ, source, iterationID string, data map[string]any) {
	if e.audit != nil {
		e.audit(typ, source, iterationID, data)
	}
}

// SetOnIterationClose installs a hook invoked exactly once per finished
// iteration (every terminal outcome: done, no_i_am_done, harness_error,
// timeout, killed), after the iteration row's final status is persisted and no
// further writes under that iteration's dir can occur. Wired by the daemon to
// gzip the proxy transcript (spec §9/§12); nil disables it. Best-effort by
// contract of the installed function itself — the engine does not inspect or
// retry errors from it.
func (e *Engine) SetOnIterationClose(fn func(agent, iterationID string)) { e.onClose = fn }

// SetIterationCompleted installs the durable continuation hook used after a
// terminal iteration row is finalized. Nil disables it.
func (e *Engine) SetIterationCompleted(fn func(agent, iterationID string)) {
	e.iterationCompleted = fn
}

// emitState tells live listeners that the agent's state changed outside an
// iteration event: a halt or an idle stop.
func (e *Engine) emitState() {
	if e.emit != nil {
		e.emit(events.Event{Agent: e.ag.Name, Type: "state", Time: store.FormatTime(e.clock())})
	}
}

func (e *Engine) emitIteration(id, trigger, status, phase string) {
	if e.emit == nil {
		return
	}
	e.emit(events.Event{Agent: e.ag.Name, Type: "iteration", Time: store.FormatTime(e.clock()),
		Data: map[string]any{"id": id, "trigger": trigger, "status": status, "phase": phase}})
}

func (e *Engine) CurrentIterationID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.current
}

// AbortCurrent stops a specific in-flight iteration without stopping its loop.
// It is used when the manager has established that the iteration's shim is no
// longer reachable, so the runner cannot otherwise observe its completion.
func (e *Engine) AbortCurrent(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.current != id || e.currentCancel == nil {
		return false
	}
	e.currentCancel()
	return true
}

// iterates reports whether the engine may launch loop iterations on its own:
// the agent is on, not halted by a stop policy, and not on the terminal-only
// bare image.
func (e *Engine) iterates() bool {
	return e.ag.Enabled && e.ag.ErrorReason == "" && e.ag.ImageRef != image.BareRef.String()
}

// reload re-reads the agent config so runtime-mutable settings apply on the next
// iteration (spec §5.1).
func (e *Engine) reload() {
	if a, err := e.store.Get(e.ag.Name); err == nil {
		e.ag = a
	}
}

func (e *Engine) Run(ctx context.Context) {
	w := e.msgWake
	for {
		e.reload()
		// Event-only agents (IntervalS <= 0, no schedule) create no timer at
		// all: a nil channel blocks forever in select, so we never allocate a
		// runtime timer that a manual trigger would then abandon and leak.
		var timer loopTimer
		var timerC <-chan time.Time
		wait, armedRule, armed := e.nextTimer()
		if armed {
			timer = e.newTimer(wait)
			timerC = timer.C()
		}
		stopTimer := func() {
			if timer != nil {
				timer.Stop()
			}
		}
		select {
		case <-ctx.Done():
			stopTimer()
			w.stopRetry()
			w.abandon()
			return
		case <-w.ch:
			stopTimer()
			// An armed retry already owns this pending condition. Extra
			// publishes only coalesce; they neither create another timer nor
			// spin against the still-live interactive session.
			if !w.retryArmed() {
				w.begin()
				e.tryMessage(ctx)
			} else {
				w.coalesce(e.reloadIterates, e.hasPending)
			}
		case <-e.events:
			stopTimer()
			w.onConfigWake(e.newTimer, e.reloadIterates, e.hasPending)
			// Any wake falls through to the top of the loop, where reload()
			// re-reads config and recreates or omits the timer. This both wakes
			// a parked engine (start/re-enable) and cancels a doomed timer
			// (stop); WakeMessage additionally runs a message iteration above.
		case m := <-e.manualCh:
			stopTimer()
			e.runOnceGuarded(ctx, "manual", m.oneShot, m.launchGate)
		case <-timerC:
			// The timer fired and drained its channel; no Stop needed.
			e.timerFired(ctx, armedRule)
		case <-w.retryC:
			stopTimer()
			// The retry timer fired and its channel is drained, so detach it
			// without Stop before attempting. A still-blocked attempt can then
			// arm the next single retry.
			w.retryFired()
			w.begin()
			e.tryMessage(ctx)
		}
	}
}

// reloadIterates re-reads the agent config and reports whether the loop may
// launch iterations on its own.
func (e *Engine) reloadIterates() bool {
	e.reload()
	return e.iterates()
}

// timerFired runs the interval or schedule iteration whose timer fired.
// Pause/config can race the run loop's select with an already-ready timer.
// Re-read persisted intent at the last responsible moment so stale state
// cannot launch one extra billable iteration.
func (e *Engine) timerFired(ctx context.Context, armedRule string) {
	e.reload()
	switch {
	case !e.iterates():
	case armedRule != "" && armedRule == e.ag.ScheduleRRule:
		e.runOnce(ctx, "schedule", "")
	case armedRule == "" && e.ag.IntervalS > 0:
		e.runOnce(ctx, "interval", "")
	}
}

// tryMessage attempts one message-triggered iteration and settles the message
// wake: a launch blocked by a live interactive session (still pending) arms the
// single retry; everything else finishes or parks the wake.
func (e *Engine) tryMessage(ctx context.Context) {
	w := e.msgWake
	if ctx.Err() != nil {
		w.cancel()
		return
	}
	e.reload()
	if !e.iterates() {
		w.hold()
		return
	}
	if !e.hasPending() {
		w.clear()
		return
	}
	result := e.runOnce(ctx, "message", "")
	if result == TickSkipped && e.hasPending() {
		w.block(e.newTimer)
		return
	}
	w.clear()
}

func (e *Engine) hasPending() bool {
	if e.peek == nil {
		return false
	}
	ok, err := e.peek()
	if err != nil {
		e.log.Error("message peek", "agent", e.ag.Name, "err", err)
		return false
	}
	return ok
}

// nextTimer picks the nearer of the interval firing and the next schedule
// occurrence. armedRule is the schedule rule when the schedule won (a tie goes
// to the schedule) and "" when the interval won; armed is false when neither
// applies. Occurrences missed while an iteration ran are not replayed: the next
// one is strictly after now.
func (e *Engine) nextTimer() (wait time.Duration, armedRule string, armed bool) {
	if !e.iterates() {
		return 0, "", false
	}
	if e.ag.IntervalS > 0 {
		wait, armed = e.nextWait(), true
	}
	now := e.clock()
	if at, ok := schedule.NextRRule(e.ag.ScheduleRRule, now, time.Local); ok {
		d := at.Sub(now)
		if !armed || d <= wait {
			wait, armedRule, armed = d, e.ag.ScheduleRRule, true
		}
	}
	return wait, armedRule, armed
}

// nextWait returns the delay until the next interval firing, measured from the
// previous iteration's start.
func (e *Engine) nextWait() time.Duration {
	if e.ag.IntervalS <= 0 {
		return time.Duration(1 << 62) // effectively "never" for event-only agents
	}
	e.mu.Lock()
	last := e.lastRun
	e.mu.Unlock()
	if last.IsZero() {
		return 0
	}
	elapsed := e.clock().Sub(last)
	remaining := time.Duration(e.ag.IntervalS)*time.Second - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (e *Engine) runOnce(ctx context.Context, trigger, oneShot string) TickResult {
	return e.runOnceGuarded(ctx, trigger, oneShot, nil)
}

// launchGuard holds a manual trigger's launch gate. The gate is acquired at
// most once, held from acquisition through image activation and iteration row
// creation, and released once the iteration is current (or its failed row
// exists); runOnceGuarded releases it on every other exit.
type launchGuard struct {
	gate     func() (release func(), ok bool)
	release  func()
	acquired bool
}

func (g *launchGuard) acquire() bool {
	if g.gate == nil || g.acquired {
		return true
	}
	var ok bool
	g.release, ok = g.gate()
	g.acquired = ok
	return ok
}

func (g *launchGuard) done() {
	if g.release != nil {
		g.release()
		g.release = nil
	}
}

// launch is a created iteration row ready to hand to the runner.
type launch struct {
	it        agent.Iteration
	activated activatedImage
	now       time.Time
}

func (e *Engine) runOnceGuarded(
	ctx context.Context,
	trigger, oneShot string,
	launchGate func() (release func(), ok bool),
) TickResult {
	guard := &launchGuard{gate: launchGate}
	defer guard.done()
	if ctx.Err() != nil {
		return TickDisabled
	}
	if res, blocked := e.launchBlocked(trigger, guard); blocked {
		return res
	}
	l, res, ok := e.prepareLaunch(trigger, guard)
	if !ok {
		return res
	}
	return e.runIteration(ctx, trigger, oneShot, l, guard)
}

// launchBlocked applies the launch guard (spec: tmux session collision). An
// interactive agent whose tmux session is already alive must not spawn a
// second iteration on top: a loop-triggered tick is skipped, a manual one is
// failed with a visible reason. It reports true when the launch must not
// proceed, with the tick result to return.
func (e *Engine) launchBlocked(trigger string, guard *launchGuard) (TickResult, bool) {
	b, ok := e.runner.(interface {
		SessionBlocked(agent.Agent) bool
	})
	if !ok || !b.SessionBlocked(e.ag) {
		return "", false
	}
	// Self-heal an orphaned session before failing. A tmux session left by a
	// crashed daemon / failed adoption has no running iteration to own it and
	// would otherwise block every future iteration forever (on_error=restart
	// never sees this — it is a pre-launch guard, not an iteration outcome).
	// Reap it and fall through to launch; a session still owned by a live
	// iteration is a real conflict and is left untouched.
	reaped := false
	if rp, ok := e.runner.(interface {
		ReapOrphanBlock(agent.Agent) bool
	}); ok {
		reaped = rp.ReapOrphanBlock(e.ag)
	}
	if reaped {
		e.log.Info("reaped orphan tmux session before launch", "agent", e.ag.Name)
		e.recordAudit("session_reaped", "system", "",
			map[string]any{"reason": "orphan_before_launch"})
		return "", false
	}
	if trigger == "manual" {
		return e.failManualCollision(trigger, guard), true
	}
	e.log.Info("loop iteration skipped: tmux session still alive", "agent", e.ag.Name)
	e.recordAudit("iteration_skipped", "system", "",
		map[string]any{"reason": "session_alive"})
	return TickSkipped, true
}

// failManualCollision records a manual trigger that collided with a live tmux
// session as a failed iteration row.
func (e *Engine) failManualCollision(trigger string, guard *launchGuard) TickResult {
	if !guard.acquire() {
		return TickDisabled
	}
	now := e.clock()
	id, err := e.store.NextIterationID(e.ag.Name, now)
	if err != nil {
		e.log.Error("next iteration id", "agent", e.ag.Name, "err", err)
		return TickError
	}
	ts := store.FormatTime(now)
	it := agent.Iteration{
		ID: id, Agent: e.ag.Name, Trigger: trigger, Status: "failed",
		StartedAt: ts, EndedAt: ts,
	}
	if err := e.store.CreateIteration(it); err != nil {
		e.log.Error("create iteration", "agent", e.ag.Name, "err", err)
		return TickError
	}
	guard.done()
	e.log.Warn("manual iteration failed: tmux session already alive",
		"agent", e.ag.Name, "id", id)
	e.recordAudit("iteration_failed", "system", id,
		map[string]any{"reason": "tmux_session_exists"})
	e.emitIteration(id, trigger, "failed", "finish")
	if e.iterationCompleted != nil {
		e.iterationCompleted(e.ag.Name, id)
	}
	return TickError
}

// prepareLaunch acquires the launch gate, activates the image and creates the
// running iteration row. It reports false with the tick result to return when
// the launch cannot proceed.
func (e *Engine) prepareLaunch(trigger string, guard *launchGuard) (launch, TickResult, bool) {
	if !guard.acquire() {
		return launch{}, TickDisabled, false
	}
	activated := activatedImage{}
	if e.beforeLaunch != nil {
		var err error
		activated, err = e.beforeLaunch(&e.ag)
		if err != nil {
			// Pre-launch failures (unverifiable manifest, missing image skills,
			// unusable harness) produce no iteration row, so the daemon log was
			// their only trace and the loop repeated the same failure every
			// tick. Halt with the cause in error_reason, which the agent status
			// surfaces as halt_reason; Start/Restart clears it.
			e.log.Error("prepare image activation", "agent", e.ag.Name, "err", err)
			e.disableLoop("image activation failed: " + err.Error())
			e.recordAudit("iteration_failed", "system", "", map[string]any{
				"reason": "image_activation_failed", "error": err.Error(),
			})
			return launch{}, TickErrorHalt, false
		}
	}
	now := e.clock()
	id, err := e.store.NextIterationID(e.ag.Name, now)
	if err != nil {
		e.log.Error("next iteration id", "agent", e.ag.Name, "err", err)
		return launch{}, TickError, false
	}
	it := agent.Iteration{
		ID: id, Agent: e.ag.Name, Trigger: trigger, Status: "running",
		StartedAt: store.FormatTime(now), ImageRef: e.ag.ImageRef,
		ImageVersion: activated.ImageVersion, ImageDigest: e.ag.ImageDigest, PromptTemplateSHA256: activated.PromptTemplateSHA256,
	}
	if err := e.store.CreateIteration(it); err != nil {
		e.log.Error("create iteration", "agent", e.ag.Name, "err", err)
		return launch{}, TickError, false
	}
	return launch{it: it, activated: activated, now: now}, "", true
}

// runIteration makes the created row current, releases the launch gate, runs
// the harness and records the outcome.
func (e *Engine) runIteration(ctx context.Context, trigger, oneShot string, l launch, guard *launchGuard) TickResult {
	id, activated := l.it.ID, l.activated
	// Root iteration span (spec §14). OTel-off installs a no-op tracer, so span
	// start/end and attribute-setting are free and never alter iteration
	// behavior. span.End() is deferred so it fires on every exit path
	// (done/no_i_am_done/harness_error/timeout/killed); ctx carries the span so
	// the runner's child spans nest under it.
	runCtx, cancel := context.WithCancel(ctx)
	runCtx = context.WithValue(runCtx, activatedImageContextKey{}, activated)
	e.mu.Lock()
	e.current = id
	e.lastRun = l.now
	e.currentCancel = cancel
	e.mu.Unlock()
	defer cancel()
	guard.done()
	e.log.Info("iteration started", "agent", e.ag.Name, "id", id, "trigger", trigger)
	e.emitIteration(id, trigger, "running", "start")
	e.recordAudit("iteration_started", "system", id, map[string]any{
		"trigger": trigger, "image_ref": e.ag.ImageRef,
		"image_version": activated.ImageVersion, "image_digest": e.ag.ImageDigest,
	})
	tr := otel.Tracer("tariboy/loop")
	spanStart := e.clock()
	runCtx, span := tr.Start(runCtx, "iteration", oteltrace.WithAttributes(
		attribute.String("agent", e.ag.Name),
		attribute.String("iteration_id", id),
		attribute.String("trigger", trigger),
		attribute.String("image_ref", e.ag.ImageRef),
	))
	defer span.End()

	// Fires after every closed runner below (harness_error included), but not
	// after a restart handoff: the adopted shim still owns that iteration. Goal
	// completion is signaled separately by the component that successfully
	// commits the terminal row.
	detached := false
	defer func() {
		if detached {
			return
		}
		if e.onClose != nil {
			e.onClose(e.ag.Name, id)
		}
	}()

	outcome, runErr := e.runner.Run(runCtx, e.ag, trigger, id, oneShot)

	e.mu.Lock()
	e.current = ""
	e.currentCancel = nil
	e.mu.Unlock()

	end := store.FormatTime(e.clock())
	if errors.Is(runErr, ErrIterationDetached) {
		detached = true
		e.log.Info("iteration detached for daemon handoff", "agent", e.ag.Name, "id", id)
		return TickDetached
	}
	if runErr != nil {
		e.finishHarnessError(id, trigger, end, runErr)
		e.finishSpan(span, spanStart, id, "harness_error", 0, 0)
		return TickError
	}
	return e.finishOutcome(l.it, trigger, end, outcome, span, spanStart)
}

// finishHarnessError finalizes the row of an iteration whose runner failed.
func (e *Engine) finishHarnessError(id, trigger, end string, runErr error) {
	e.log.Error("iteration runner failed", "agent", e.ag.Name, "id", id, "err", runErr)
	// A manager-side stale-shim recovery may have finalized this row while
	// the runner was unwinding its cancelled context. Do not erase that
	// durable terminal detail (notably its synthetic exit code).
	current, err := e.store.GetIteration(e.ag.Name, id)
	if err != nil {
		return
	}
	current.Status = "harness_error"
	current.EndedAt = end
	committed, err := e.store.FinalizeRunningIteration(current)
	if err != nil {
		e.log.Error("update iteration", "agent", e.ag.Name, "id", id, "err", err)
		return
	}
	if !committed {
		return
	}
	e.emitIteration(id, trigger, "harness_error", "finish")
	e.recordAudit("iteration_finished", "system", id, map[string]any{"status": "harness_error", "error": runErr.Error()})
	if e.iterationCompleted != nil {
		e.iterationCompleted(e.ag.Name, id)
	}
}

// finishOutcome persists the runner's outcome, applies the on_timeout/on_error
// policy and signals completion.
func (e *Engine) finishOutcome(it agent.Iteration, trigger, end string, outcome Outcome,
	span oteltrace.Span, spanStart time.Time) TickResult {
	id := it.ID
	ec := outcome.ExitCode
	cpu := outcome.CPUMs
	mem := outcome.MemPeakKB
	it.Status = outcome.Status
	it.EndedAt = end
	it.DoneFlag = outcome.DoneFlag
	it.ExitCode = &ec
	it.CPUMs = &cpu
	it.MemPeakKB = &mem
	terminalPersisted, err := e.store.FinalizeRunningIteration(it)
	if err != nil {
		e.log.Error("update iteration", "agent", e.ag.Name, "id", id, "err", err)
		return TickSkipped
	}
	if !terminalPersisted {
		return TickSkipped
	}
	if outcome.DoneFlag {
		// done_flag is owned exclusively by SetIterationDone (spec §5.2).
		// outcome.Productive was read from the DB, so an `--idle` declaration made
		// via the API during the iteration is preserved rather than overwritten.
		if err := e.store.SetIterationDone(id, outcome.Productive); err != nil {
			e.log.Error("set iteration done", "agent", e.ag.Name, "id", id, "err", err)
		}
	}
	e.log.Info("iteration finished", "agent", e.ag.Name, "id", id, "status", outcome.Status)
	e.emitIteration(id, trigger, outcome.Status, "finish")
	finished := map[string]any{"status": outcome.Status}
	if outcome.Error != "" {
		finished["error"] = outcome.Error
	}
	e.recordAudit("iteration_finished", "system", id, finished)

	e.finishSpan(span, spanStart, id, outcome.Status, outcome.CPUMs, outcome.MemPeakKB)
	result := e.applyPolicy(outcome)
	if e.iterationCompleted != nil {
		e.iterationCompleted(e.ag.Name, id)
	}
	return result
}

// finishSpan sets the terminal attributes/status on the iteration span and
// records the iteration metric (Task 2). Tokens/cost are best-effort span
// attributes (spec §14): usageLookup reflects what the async proxy ingester has
// persisted at classify time, which may lag the last request — the accurate
// accounting view remains usage/budget (the DB). Every step here is nil-safe
// and cannot fail the iteration: a no-op tracer yields no-op spans, and a nil
// usageLookup/metrics is skipped.
func (e *Engine) finishSpan(span oteltrace.Span, start time.Time, id, outcome string, cpu, mem int) {
	dur := float64(e.clock().Sub(start).Milliseconds())
	span.SetAttributes(
		attribute.String("outcome", outcome),
		attribute.Int("cpu_ms", cpu),
		attribute.Int("mem_peak_kb", mem),
	)
	if e.usageLookup != nil {
		if e.usageFlush != nil {
			e.usageFlush()
		}
		in, out, cost := e.usageLookup(id)
		span.SetAttributes(
			attribute.Int("tokens_in", in),
			attribute.Int("tokens_out", out),
			attribute.Float64("cost_usd", cost),
		)
	}
	if outcome == "harness_error" || outcome == "timeout" {
		span.SetStatus(codes.Error, outcome)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	e.metrics.RecordIteration(context.Background(), outcome, dur)
}

func (e *Engine) applyPolicy(o Outcome) TickResult {
	switch o.Status {
	case "done", "no_i_am_done":
		if res, stopped := e.maybeIdleStop(); stopped {
			return res
		}
		return TickCompletedWaiting
	case "killed":
		return TickProcessDied
	case "timeout":
		if e.ag.OnTimeout == "stop" {
			e.disableLoop("timeout")
			return TickTimeoutStop
		}
		return TickTimeoutRestart
	case "harness_error":
		if e.ag.OnError == "stop" {
			e.disableLoop("harness_error")
			return TickErrorHalt
		}
		return TickRestarted
	default:
		return TickCompletedWaiting
	}
}

// disableLoop halts the loop when a stop policy (on_timeout=stop / on_error=stop)
// fires. SetError records the halt reason, which stops further iterations until
// Start clears it, so the agent surfaces as state=error (derived).
// The engine goroutine itself stays alive, blocked on a manual trigger.
func (e *Engine) disableLoop(reason string) {
	e.ag.ErrorReason = "halted: " + reason
	if err := e.store.SetError(e.ag.Name, e.ag.ErrorReason); err != nil {
		e.log.Error("disable loop", "agent", e.ag.Name, "err", err)
	}
	e.emitState()
}

// maybeIdleStop applies the idle-autostop policy after a completed iteration
// (Task 2). When MaxIdleIterations > 0 and the agent's consecutive idle streak
// (self-declared via `i-am-done --idle`) has reached the threshold, it cleanly
// stops the loop and returns (TickIdleStop, true). MaxIdleIterations == 0 means
// the feature is disabled and it never stops. Any query error is logged and
// treated as "do not stop" so a transient DB hiccup cannot silently halt a loop.
func (e *Engine) maybeIdleStop() (TickResult, bool) {
	if e.ag.MaxIdleIterations <= 0 {
		return "", false
	}
	streak, err := e.store.IdleStreak(e.ag.Name)
	if err != nil {
		e.log.Error("idle streak query", "agent", e.ag.Name, "err", err)
		return "", false
	}
	if streak < e.ag.MaxIdleIterations {
		return "", false
	}
	// The streak is scoped to iterations since the last Start (IdleStreak +
	// StartResetIdle), so at the trip it equals the threshold. Clamp defensively
	// so the reported reason can never exceed MaxIdleIterations.
	count := streak
	if count > e.ag.MaxIdleIterations {
		count = e.ag.MaxIdleIterations
	}
	e.idleStop(fmt.Sprintf(agent.IdleStopPrefix+" (%d idle iterations)", count))
	return TickIdleStop, true
}

// idleStop performs the clean idle halt: it clears the enabled switch and records the reason in status_message via SetIdleStopped
// (NOT SetError), so the agent surfaces as derived state=stopped with a
// visible, non-error reason. The engine goroutine stays alive, blocked until a
// manual trigger or a Start re-enables it.
func (e *Engine) idleStop(reason string) {
	e.ag.Enabled = false
	updated := store.FormatTime(e.clock())
	if err := e.store.SetIdleStopped(e.ag.Name, reason, updated); err != nil {
		e.log.Error("idle stop", "agent", e.ag.Name, "err", err)
	}
	e.emitState()
	e.log.Info("loop idle-stopped", "agent", e.ag.Name, "reason", reason)
	e.recordAudit("loop_idle_stopped", "system", "", map[string]any{"reason": reason})
}
