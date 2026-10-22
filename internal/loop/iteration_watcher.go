package loop

import (
	"context"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/shim"
)

// iterationWatcher observes one running iteration until its shim writes
// result.json. On every poll it resyncs a changed persisted hard deadline to
// the shim, enforces the persisted soft deadline, and ends an iteration that
// signalled done but kept running past the done grace; the soft deadline and
// the done grace share one cooperative Kill. The runner watches the iterations
// it launched, adoption watches iterations a previous daemon launched.
type iterationWatcher struct {
	store      *agent.Store
	clock      func() time.Time
	log        *slog.Logger
	agent, id  string
	resultPath string
	shimSock   string
	poll       time.Duration
	doneGrace  time.Duration
	// rpcTimeout bounds each shim RPC; zero means the client default.
	rpcTimeout time.Duration
	// probe, when set, also checks shim liveness: a shim that misses
	// probe.maxMisses consecutive probes without a result is stale.
	probe *livenessProbe
	// shimExited, when set, closes once the launched shim process is reaped.
	shimExited <-chan struct{}
}

type livenessProbe struct {
	every     time.Duration
	maxMisses int
}

type watchResult struct {
	result shim.IterationResult
	// softHit reports that this watcher saw the soft deadline trigger.
	softHit bool
	// stale reports that the shim stopped answering without a result.
	stale bool
	// shimGone reports that the shim process exited without a result.
	shimGone bool
}

// watch returns when the result appears, the probe declares the shim stale,
// or the shim process exits without a result.
// When ctx ends first it returns ctx.Err(); the iteration may still run.
func (w *iterationWatcher) watch(ctx context.Context) (watchResult, error) {
	tk := time.NewTicker(w.poll)
	defer tk.Stop()
	var out watchResult
	lastHardDeadline := ""
	doneGrace := doneGraceTracker{}
	kill := cooperativeKill{}
	// Kill is sent once, so it keeps the client default timeout.
	killShim := func() error {
		err := kill.send(func() error { return shim.Dial(w.shimSock).Kill() })
		logCleanup(w.log, "kill iteration shim", err, "agent", w.agent, "id", w.id)
		return err
	}
	misses := 0
	lastProbe := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-w.shimExited:
			// The shim writes result.json before it exits, so a result that is
			// still missing after the reap will never appear.
			if res, ok := readResult(w.resultPath); ok {
				out.result = res
				return out, nil
			}
			out.shimGone = true
			return out, nil
		case <-tk.C:
		}
		if res, ok := readResult(w.resultPath); ok {
			out.result = res
			return out, nil
		}
		if it, err := w.store.GetIteration(w.agent, w.id); err == nil {
			if it.HardTimeoutDeadline != nil && *it.HardTimeoutDeadline != lastHardDeadline {
				// The DB is authoritative, also across a daemon restart: retry
				// until the shim accepts the latest persisted deadline.
				if err := w.client().SetHardDeadline(*it.HardTimeoutDeadline); err == nil {
					lastHardDeadline = *it.HardTimeoutDeadline
				} else {
					w.log.Debug("resync shim hard deadline", "agent", w.agent, "id", w.id, "err", err)
				}
			}
			triggered, err := enforceSoftTimeout(w.store, it, w.clock(), killShim)
			if err != nil {
				w.log.Debug("enforce shim soft deadline", "agent", w.agent, "id", w.id, "err", err)
			}
			out.softHit = out.softHit || triggered
			doneGrace.observe(it.DoneFlag, w.clock(), w.doneGrace, killShim)
		}
		if w.probe == nil || time.Since(lastProbe) < w.probe.every {
			continue
		}
		lastProbe = time.Now()
		if _, err := w.client().Status(); err == nil {
			misses = 0
			continue
		}
		if misses++; misses < w.probe.maxMisses {
			continue
		}
		// One last check: a result may have landed between the poll above and
		// the failing probe.
		if res, ok := readResult(w.resultPath); ok {
			out.result = res
			return out, nil
		}
		out.stale = true
		return out, nil
	}
}

func (w *iterationWatcher) client() *shim.Client {
	c := shim.Dial(w.shimSock)
	if w.rpcTimeout > 0 {
		c.Timeout = w.rpcTimeout
	}
	return c
}

// hasRunningIteration reports whether the store holds a still-running
// iteration for name: the owner of a live tmux session.
func hasRunningIteration(st *agent.Store, name string) bool {
	if st == nil {
		return false
	}
	its, err := st.ListIterations(name)
	if err != nil {
		return false
	}
	for _, it := range its {
		if it.Status == agent.IterationRunning {
			return true
		}
	}
	return false
}

// killOrphanSession kills an agent's live tmux session unless a running
// iteration owns it, and reports whether it did. A session with no running
// iteration is a leftover of a crashed daemon or a failed adoption.
func killOrphanSession(st *agent.Store, kill func(string) error, name string) (bool, error) {
	if hasRunningIteration(st, name) {
		return false, nil
	}
	if err := kill(name); err != nil {
		return false, err
	}
	return true, nil
}
