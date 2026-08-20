package plugins

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
)

// providerKeyEnv lists the real upstream provider API keys the daemon holds in
// its own env solely to forward requests to its in-process AI proxy
// (internal/aiproxy). A plugin subprocess must NEVER see them: a plugin only
// ever needs the scoped per-request proxy token, and a leaked real key would let
// the plugin bypass the accounted proxy and reach the provider directly. This is
// the SAME var list the agent-loop runner scrubs (internal/loop/runner.go
// BuildEnv), applied here so plugin isolation is STRUCTURAL, not by-convention.
var providerKeyEnv = []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY"}

// scrubProviderKeys returns env with every providerKeyEnv assignment removed,
// leaving all other inherited vars (PATH, etc.) intact. It never mutates the
// caller's slice.
func scrubProviderKeys(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		drop := false
		for _, k := range providerKeyEnv {
			if strings.HasPrefix(kv, k+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// ErrNotFound is returned by Inspect for an unknown plugin.
var ErrNotFound = errors.New("plugin not found")

// ErrInvalidName is returned by the name-taking primitives (Remove/Logs/Inspect)
// when name fails the manifest name pattern. This is the primitive-level guard
// that keeps a traversing name (e.g. "../victim") from reaching any filesystem
// operation, regardless of caller — defense in depth alongside the CLI check.
var ErrInvalidName = errors.New("invalid plugin name")

// checkName is the single choke point every name-taking primitive calls before
// it touches the filesystem or store. It reuses the manifest name pattern via
// ValidName so there is exactly one definition of a legal plugin name.
func checkName(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("%w: %q (want ^[a-z0-9][a-z0-9_-]*$)", ErrInvalidName, name)
	}
	return nil
}

type HostConfig struct {
	Store        *Store
	Bus          *bus.Bus
	Tokens       *TokenRegistry
	PluginsDir   string
	DaemonSocket string
	Runner       Runner
	Clock        func() time.Time
	After        func(time.Duration) <-chan time.Time
	DrainAfter   func(time.Duration) <-chan time.Time
	Log          *slog.Logger
}

// Host is the registry of running plugins: it starts a Supervisor per enabled
// plugin, tracks them concurrency-safely, and drains them all (cancel + wait)
// before the daemon closes the store (spec §13, M5 drain-before-close).
type Host struct {
	cfg     HostConfig
	baseCtx context.Context
	mu      sync.Mutex
	running map[string]*runningPlugin
	wg      sync.WaitGroup

	// pushMu guards pushQueue: the per-(plugin,channel) watch-push serializers
	// that keep provider reconciles ordered (spec §6.2). See PushWatches.
	pushMu    sync.Mutex
	pushQueue map[string]*watchPush
}

// watchPush serializes watch-list delivery for one (plugin, channel) pair. The
// provider reconciles by full replace, so only the newest snapshot matters: a
// fresh PushWatches overwrites any queued-but-not-yet-delivered snapshot
// (coalescing) and a single worker drains them one at a time. Together this
// guarantees the provider observes snapshots in the order the subscribe/
// unsubscribe events occurred and always converges on the final state, instead
// of a stale snapshot from an out-of-order goroutine winning the race.
type watchPush struct {
	pending        []WatchDTO
	pendingCurrent bool
	hasPending     bool
	busy           bool
}

type runningPlugin struct {
	rec    Record
	cancel context.CancelFunc
	mu     sync.Mutex
	token  string
	state  string
	health map[string]any
	// done closes when this instance's supervisor goroutine has fully exited
	// (process stopped, token revoked). Restart waits on it so a fresh instance
	// never contends with the old one for the same socket. Nil until the
	// supervisor goroutine is spawned (a setup-failure path never starts it).
	done chan struct{}
}

func NewHost(cfg HostConfig) *Host {
	if cfg.Runner == nil {
		cfg.Runner = NewExecRunner()
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.After == nil {
		cfg.After = time.After
	}
	if cfg.DrainAfter == nil {
		cfg.DrainAfter = time.After
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Host{
		cfg:       cfg,
		baseCtx:   context.Background(),
		running:   map[string]*runningPlugin{},
		pushQueue: map[string]*watchPush{},
	}
}

func (h *Host) SocketPath(name string) string {
	return filepath.Join(h.cfg.PluginsDir, name, "plugin.sock")
}

// ErrNotRunning is returned by PluginRoutes/PluginAction when the target plugin
// is installed but not currently in the "running" state.
var ErrNotRunning = errors.New("plugin not running")

// socketIfRunning returns the plugin's socket path only when it is running.
func (h *Host) socketIfRunning(name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	h.mu.Lock()
	rp := h.running[name]
	var st string
	if rp != nil {
		rp.mu.Lock()
		st = rp.state
		rp.mu.Unlock()
	}
	h.mu.Unlock()
	if rp == nil || st != "running" {
		return "", ErrNotRunning
	}
	return h.SocketPath(name), nil
}

// PluginRoutes forwards to the running plugin's GET /routes.
func (h *Host) PluginRoutes(name string) (map[string]any, error) {
	sock, err := h.socketIfRunning(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return NewClient(sock).Routes(ctx)
}

// PluginAction forwards a body to the running plugin's POST /action (a longer
// timeout because create may hit a slow upstream).
func (h *Host) PluginAction(name string, body map[string]any) (map[string]any, error) {
	sock, err := h.socketIfRunning(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return NewClientWithTimeout(sock, 15*time.Second).Action(ctx, body)
}
func (h *Host) dir(name string) string                 { return filepath.Join(h.cfg.PluginsDir, name) }
func (h *Host) versionDir(name, version string) string { return filepath.Join(h.dir(name), version) }
func (h *Host) workdir(name string) string             { return filepath.Join(h.dir(name), "workdir") }
func (h *Host) logPath(name string) string             { return filepath.Join(h.dir(name), "logs", "plugin.log") }

// StartAll re-launches every enabled plugin (spec §7.2 step 2).
func (h *Host) StartAll(ctx context.Context) error {
	h.baseCtx = ctx
	recs, err := h.cfg.Store.List()
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if err := h.migrateLegacyPackage(rec); err != nil {
			h.cfg.Log.Warn("plugin legacy package migration failed", "plugin", rec.Name, "err", err)
			continue
		}
		if !rec.Enabled {
			continue
		}
		if err := h.Start(rec); err != nil {
			h.cfg.Log.Warn("plugin start failed", "plugin", rec.Name, "err", err)
		}
	}
	return nil
}

func (h *Host) migrateLegacyPackage(rec Record) error {
	dest := h.versionDir(rec.Name, rec.Version)
	if _, err := os.Stat(dest); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	root := h.dir(rec.Name)
	if _, err := os.Stat(filepath.Join(root, "plugin.json")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(root, rec.Version+".migrate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	reserved := map[string]bool{"active-version": true, "logs": true, "workdir": true, "plugin.sock": true, filepath.Base(tmp): true}
	for _, entry := range entries {
		if reserved[entry.Name()] {
			continue
		}
		if err := copyTree(filepath.Join(root, entry.Name()), filepath.Join(tmp, entry.Name())); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	return writeActiveVersion(root, rec.Version)
}

// Start launches and supervises one plugin. Never returns the plugin's own
// runtime errors (those become health state); it only fails on setup errors.
func (h *Host) Start(rec Record) error {
	name := rec.Name

	// Reserve the name atomically under the lock (TOCTOU-safe): a concurrent
	// Start(sameName) sees the reservation and bails, so exactly one supervisor
	// ever runs per name. The reservation already carries a usable cancel so
	// StopAll can always cancel it even before the supervisor goroutine exists.
	h.mu.Lock()
	if _, ok := h.running[name]; ok {
		h.mu.Unlock()
		return nil // already running or starting
	}
	ctx, cancel := context.WithCancel(h.baseCtx)
	rp := &runningPlugin{rec: rec, cancel: cancel, state: "starting", health: map[string]any{}, done: make(chan struct{})}
	h.running[name] = rp
	h.mu.Unlock()

	// fail releases the reservation (and cancels) on any setup-failure path so a
	// refused/broken plugin never lingers in the running map.
	fail := func(err error) error {
		h.mu.Lock()
		if h.running[name] == rp {
			delete(h.running, name)
		}
		h.mu.Unlock()
		cancel()
		return err
	}

	sock := h.SocketPath(name)
	for _, d := range []string{h.dir(name), h.workdir(name), filepath.Dir(h.logPath(name))} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fail(err)
		}
	}

	// SECURITY (CF-1 containment): resolve the exec through the symlink-safe gate
	// before spawning. Manifest.Validate is string-only and cannot see through
	// symlinks; a same-dir-looking exec (e.g. ok.py -> /bin/sh) must be refused.
	// The plugin is marked "error" (persisted + system:plugins event via onState)
	// and NOT spawned.
	packageDir := h.versionDir(name, rec.Version)
	if _, statErr := os.Stat(packageDir); os.IsNotExist(statErr) {
		packageDir = h.dir(name)
	}
	resolved, err := ResolveExec(packageDir, rec.Exec)
	if err != nil {
		h.cfg.Log.Warn("plugin exec failed containment check; refusing to start",
			"plugin", name, "exec", rec.Exec, "err", err)
		h.onState(name, "error", map[string]any{
			"checked_at": h.cfg.Clock().UTC().Format(time.RFC3339),
			"detail":     "exec containment: " + err.Error(),
		})
		return fail(err)
	}

	logf, err := os.OpenFile(h.logPath(name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fail(err)
	}
	// Timestamp every captured plugin log line at the single capture choke point,
	// so each line carries a date+time no matter what the plugin emits. logtw is
	// flushed (trailing partial line) then logf closed when the life ends below.
	logtw := newTSWriter(logf, h.cfg.Clock)

	// Carry-forward (a): the daemon assigns the socket + workdir and injects them
	// (plus the daemon socket) into the plugin's env. The plugin-token itself is
	// minted PER LIFE by the supervisor's MintToken seam below and injected as
	// TARIBOY_PLUGIN_TOKEN, so a restarted plugin always gets a fresh token.
	//
	// SECURITY: the inherited daemon env is scrubbed of the real upstream provider
	// keys (scrubProviderKeys) BEFORE the plugin's own vars are appended, so an
	// eval/llm-judge plugin can only ever reach an LLM through the scoped proxy
	// token — the real key never reaches the plugin process. This isolation is
	// structural (the key is absent from the child env), not merely by convention.
	spec := SpawnSpec{
		Name:   name,
		Exec:   resolved, // containment-checked absolute path (never the raw join)
		Dir:    h.workdir(name),
		Socket: sock,
		Env: append(scrubProviderKeys(os.Environ()),
			"TARIBOY_PLUGIN_NAME="+name,
			"TARIBOY_PLUGIN_SOCKET="+sock,
			"TARIBOY_PLUGIN_WORKDIR="+h.workdir(name),
			"TARIBOY_DAEMON_SOCKET="+h.cfg.DaemonSocket),
	}

	// Carry-forward (b): the token scope is the plugin's declared bus surface.
	// Sink carries the subscribe patterns ONLY for a channel-sink plugin, so the
	// publish handler can seed concrete sink subscriptions for a glob like chat:*
	// (see api.go publish); a non-sink leaves it nil and never seeds.
	identity := Identity{
		Name:      name,
		Publish:   rec.Channels.Publish,
		Subscribe: rec.Channels.Subscribe,
		Provide:   providedChannelNames(rec.Channels.Provide),
		Sink:      sinkPatterns(rec),
	}

	sup := NewSupervisor(SupervisorConfig{
		Name: name, Manifest: manifestFromRecord(rec), Spec: spec,
		Runner: h.cfg.Runner, Client: NewClient(sock), LogWriter: logtw,
		Clock: h.cfg.Clock, After: h.cfg.After, Log: h.cfg.Log,
		OnState: func(state string, health map[string]any) { h.onState(name, state, health) },
		MintToken: func() (string, error) {
			tok, err := h.cfg.Tokens.Mint(identity)
			if err == nil {
				rp.mu.Lock()
				rp.token = tok
				rp.mu.Unlock()
			}
			return tok, err
		},
		RevokeToken: func(tok string) {
			h.cfg.Tokens.Revoke(tok)
			rp.mu.Lock()
			if rp.token == tok {
				rp.token = ""
			}
			rp.mu.Unlock()
		},
	})

	// Task 9 hook: subscribe sink channels + start the drainer here.
	h.startSink(ctx, rec, sock)

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		// Signal Restart (and any per-instance waiter) that this instance has
		// fully drained. Closed last, after the supervisor has graceful-stopped
		// the process and revoked its token on the way out of sup.Run.
		defer close(rp.done)
		defer func() { _ = logtw.Flush(); _ = logf.Close() }()
		// Defense-in-depth: a panic in the host-side onState/Bus.Publish path must
		// not crash the daemon. Plugin process failures are already out-of-process.
		defer func() {
			if r := recover(); r != nil {
				h.cfg.Log.Warn("plugin supervisor goroutine panicked", "plugin", name, "panic", r)
				h.onState(name, "error", map[string]any{
					"checked_at": h.cfg.Clock().UTC().Format(time.RFC3339),
					"detail":     fmt.Sprintf("supervisor panic: %v", r),
				})
			}
		}()
		sup.Run(ctx)
	}()
	return nil
}

// onState fans a supervisor state change to the live snapshot, the durable
// store, and a system:plugins bus event (carry-forward (c)). Repeated idempotent
// "running" emissions are deduped so a healthy plugin does not spam the bus.
func (h *Host) onState(name, state string, health map[string]any) {
	h.mu.Lock()
	rp := h.running[name]
	h.mu.Unlock()
	dup := false
	var provided []Provided
	if rp != nil {
		rp.mu.Lock()
		if rp.state == state && state == "running" {
			dup = true
		}
		rp.state, rp.health = state, health
		if state == "running" && !dup {
			provided = append(provided, rp.rec.Channels.Provide...)
		}
		rp.mu.Unlock()
	}
	if dup {
		return
	}
	hb, _ := json.Marshal(health)
	_ = h.cfg.Store.SetState(name, state, string(hb))
	detail, _ := health["detail"].(string)
	_, _ = h.cfg.Bus.Publish(bus.Message{
		Channel: "system:plugins", Type: "plugin.state", Source: "daemon",
		Text:             fmt.Sprintf("plugin %s -> %s", name, state),
		ProducedByPlugin: name,
		Data:             map[string]any{"plugin": name, "state": state, "detail": detail},
	})
	// A plugin can print its handshake before the host has registered the new
	// life token, so its one-shot startup pull may race and fail authorization.
	// Reconcile from the host side whenever this concrete life first becomes
	// healthy; this also restores watches after a daemon restart without waiting
	// for an unrelated subscribe/unsubscribe event.
	for _, p := range provided {
		h.pushCurrentWatches(name, p.Channel)
	}
}

// Stop cancels the supervisor and removes it from the registry. The per-life
// token is revoked by the supervisor's RevokeToken seam on the exit path.
func (h *Host) Stop(name string) error {
	h.mu.Lock()
	rp := h.running[name]
	delete(h.running, name)
	h.mu.Unlock()
	if rp == nil {
		return nil
	}
	rp.cancel()
	h.stopSink(name) // Task 9 hook
	return nil
}

// restartDrainGrace bounds how long Restart waits for the previous instance's
// supervisor goroutine to exit before launching a fresh one. It sits above the
// supervisor's StopGrace (default 5s) so a graceful SIGTERM has time to land
// before we give up and start anyway.
const restartDrainGrace = defaultStopGrace + 5*time.Second

// Restart cycles a single named plugin without disturbing any other plugin or
// the daemon: it stops the current instance, waits for that instance's
// supervisor goroutine to fully drain (process stopped, token revoked, socket
// released), then starts a fresh instance from the plugin's stored record. Only
// this one plugin's process is replaced; sibling supervisors keep running.
//
// A disabled plugin is not restartable — restart re-launches the process, and a
// disabled plugin is not meant to run — so it returns an error rather than
// silently enabling one.
func (h *Host) Restart(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	rec, ok, err := h.cfg.Store.Get(name)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if !rec.Enabled {
		return fmt.Errorf("plugin %q is disabled; enable it before restart", name)
	}
	// Capture the live instance (if any) BEFORE Stop removes it from the map, so
	// we can wait on its done channel. Stop cancels the supervisor ctx, which
	// unwinds sup.Run (graceful process-group stop + token revoke) and closes done.
	h.mu.Lock()
	rp := h.running[name]
	h.mu.Unlock()
	if rp != nil {
		_ = h.Stop(name)
		select {
		case <-rp.done:
		case <-h.cfg.DrainAfter(restartDrainGrace):
			h.cfg.Log.Warn("plugin restart: previous instance did not drain before deadline; starting fresh instance anyway",
				"plugin", name)
		}
	}
	return h.Start(rec)
}

// StopAll gracefully stops every plugin and waits (drain-before-close, spec §13).
func (h *Host) StopAll() {
	h.mu.Lock()
	names := make([]string, 0, len(h.running))
	for n := range h.running {
		names = append(names, n)
	}
	h.mu.Unlock()
	for _, n := range names {
		_ = h.Stop(n)
	}
	h.wg.Wait()
}

// Install validates+copies a plugin dir into PluginsDir, records it, and starts it.
func (h *Host) Install(sourcePath string) (map[string]any, error) {
	// SECURITY (Task 2/7 carry-forward): reject the whole source tree if it
	// contains ANY symlink, anywhere (not just the top level). A plugin package
	// shipping a symlink whose target lies outside the plugin dir would let a
	// later exec or read escape containment once copied into PluginsDir — this
	// is the install-time defense layer alongside ResolveExec's spawn-time gate.
	if sym, err := firstSymlink(sourcePath); err != nil {
		return nil, err
	} else if sym != "" {
		return nil, fmt.Errorf("plugin source %s contains a symlink (%s); symlinks are not permitted in a plugin package", sourcePath, sym)
	}
	if err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("plugin source %s contains non-regular file %s", sourcePath, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	m, err := LoadManifest(sourcePath)
	if err != nil {
		return nil, err
	}
	dest := h.versionDir(m.Name, m.Version)
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		equal, compareErr := equalTrees(sourcePath, dest)
		if compareErr != nil {
			return nil, compareErr
		}
		if !equal {
			return nil, fmt.Errorf("plugin %s version %s already exists with different bytes", m.Name, m.Version)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	} else {
		tmp, err := os.MkdirTemp(filepath.Dir(dest), m.Version+".tmp-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		if err := copyTree(sourcePath, tmp); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, dest); err != nil {
			return nil, err
		}
	}
	if err := os.Chmod(filepath.Join(dest, m.Exec), 0o700); err != nil {
		return nil, err
	}
	rec := Record{
		Name: m.Name, Version: m.Version, Types: m.Types, ProtocolVersion: m.ProtocolVersion,
		Exec: m.Exec, SourcePath: sourcePath, Channels: m.Channels, Enabled: true, State: "installed",
	}
	previous, hadPrevious, err := h.cfg.Store.Get(m.Name)
	if err != nil {
		return nil, err
	}
	previousActive, hadPreviousActive, err := h.cfg.Store.ActiveVersion(m.Name)
	if err != nil {
		return nil, err
	}
	rollback := func() {
		if hadPrevious {
			_ = h.cfg.Store.Upsert(previous)
			if hadPreviousActive {
				_ = h.cfg.Store.SetActiveVersion(m.Name, previousActive)
				_ = writeActiveVersion(h.dir(m.Name), previousActive)
			} else {
				_ = h.cfg.Store.ClearActiveVersion(m.Name)
				_ = os.Remove(filepath.Join(h.dir(m.Name), "active-version"))
			}
		} else {
			_ = h.cfg.Store.Delete(m.Name)
			_ = os.Remove(filepath.Join(h.dir(m.Name), "active-version"))
		}
	}
	restartPrevious := func(after *runningPlugin) {
		if !hadPrevious || !hadPreviousActive || !previous.Enabled {
			return
		}
		restart := func() {
			active, ok, activeErr := h.cfg.Store.ActiveVersion(m.Name)
			record, recordOK, recordErr := h.cfg.Store.Get(m.Name)
			if activeErr == nil && recordErr == nil && ok && recordOK &&
				active == previousActive && record.Version == previous.Version && record.Enabled {
				if startErr := h.Start(previous); startErr != nil {
					h.cfg.Log.Error("restart rolled-back plugin", "plugin", m.Name, "err", startErr)
				}
			}
		}
		if after == nil {
			restart()
			return
		}
		go func() {
			<-after.done
			restart()
		}()
	}
	h.mu.Lock()
	previousRunning := h.running[m.Name]
	h.mu.Unlock()
	_ = h.Stop(m.Name)
	if previousRunning != nil {
		select {
		case <-previousRunning.done:
		case <-h.cfg.DrainAfter(restartDrainGrace):
			restartPrevious(previousRunning)
			return nil, fmt.Errorf("plugin %s previous version did not drain before activation deadline", m.Name)
		}
	}
	if err := h.Start(rec); err != nil {
		restartPrevious(nil)
		return nil, err
	}
	h.mu.Lock()
	newRunning := h.running[m.Name]
	h.mu.Unlock()
	persist := func(err error) (map[string]any, error) {
		_ = h.Stop(m.Name)
		rollback()
		restartPrevious(newRunning)
		return nil, err
	}
	if err := h.cfg.Store.Upsert(rec); err != nil {
		return persist(err)
	}
	if err := h.cfg.Store.SetActiveVersion(m.Name, m.Version); err != nil {
		return persist(err)
	}
	if err := writeActiveVersion(h.dir(m.Name), m.Version); err != nil {
		return persist(err)
	}
	return map[string]any{"name": m.Name, "version": m.Version, "types": m.Types, "installed": true}, nil
}

// Remove stops the plugin, drops its subscriptions, deletes the row and files.
func (h *Host) Remove(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	_ = h.Stop(name)
	h.unsubscribeSink(name) // Task 9 hook
	if err := h.cfg.Store.Delete(name); err != nil {
		return err
	}
	return os.RemoveAll(h.dir(name))
}

func (h *Host) List() ([]map[string]any, error) {
	recs, err := h.cfg.Store.List()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		state, health := r.State, r.Health
		h.mu.Lock()
		if rp := h.running[r.Name]; rp != nil {
			rp.mu.Lock()
			if rp.state != "" {
				state = rp.state
			}
			if hb, err := json.Marshal(rp.health); err == nil {
				health = string(hb)
			}
			rp.mu.Unlock()
		}
		h.mu.Unlock()
		out = append(out, map[string]any{
			"name": r.Name, "version": r.Version, "types": r.Types,
			"enabled": r.Enabled, "state": state, "health": json.RawMessage(health),
		})
	}
	return out, nil
}

func (h *Host) Inspect(name string) (map[string]any, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	rec, ok, err := h.cfg.Store.Get(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	list, _ := h.List()
	state := rec.State
	for _, e := range list {
		if e["name"] == name {
			state, _ = e["state"].(string)
		}
	}
	return map[string]any{
		"name": rec.Name, "version": rec.Version, "types": rec.Types,
		"protocol_version": rec.ProtocolVersion, "exec": rec.Exec, "source_path": rec.SourcePath,
		"channels": rec.Channels, "enabled": rec.Enabled, "state": state,
		"installed_at": rec.InstalledAt, "socket": h.SocketPath(name),
	}, nil
}

func (h *Host) Logs(name string, tail int) ([]string, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if tail <= 0 {
		tail = 200
	}
	f, err := os.Open(h.logPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > tail {
			lines = lines[1:]
		}
	}
	return lines, sc.Err()
}

func manifestFromRecord(r Record) Manifest {
	return Manifest{Name: r.Name, Version: r.Version, ProtocolVersion: r.ProtocolVersion,
		Types: r.Types, Exec: r.Exec, Channels: r.Channels}
}

// errSymlinkFound is the Walk-abort sentinel used by firstSymlink; it is never
// returned to a caller of firstSymlink itself.
var errSymlinkFound = errors.New("symlink found")

// firstSymlink walks the entire tree rooted at root and returns the path of the
// first symlink encountered (empty if none). It uses Lstat semantics (via
// filepath.Walk) so symlinks are detected rather than followed.
func firstSymlink(root string) (string, error) {
	var found string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			found = p
			return errSymlinkFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, errSymlinkFound) {
		return "", err
	}
	return found, nil
}

// copyTree recursively copies src into dst (files 0600, dirs 0700).
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}

func equalTrees(a, b string) (bool, error) {
	left, err := treeDigest(a)
	if err != nil {
		return false, err
	}
	right, err := treeDigest(b)
	if err != nil {
		return false, err
	}
	return left == right, nil
}

func treeDigest(root string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(h, filepath.ToSlash(rel)+"\x00"); err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeActiveVersion(dir, version string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".active-version-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, version+"\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "active-version"))
}

// --- channel-sink hooks (Task 9) ---

const sinkPollInterval = 500 * time.Millisecond

// sinkPatterns returns the declared subscribe patterns a channel-sink plugin
// drains, or nil for any other plugin type. It is the single source that both
// the token Identity.Sink scope and the seed-on-publish gate agree on.
func sinkPatterns(rec Record) []string {
	if !manifestFromRecord(rec).HasType("channel-sink") {
		return nil
	}
	return rec.Channels.Subscribe
}

// startSink subscribes a channel-sink plugin to its declared channels and runs a
// per-plugin drainer that delivers matching bus messages to the plugin's
// /deliver (spec §7.3). The subscriptions are durable; the drainer exits on ctx.
//
// The bus fans out by EXACT channel (store.subscriptionsFor: WHERE channel=?),
// so a glob subscribe entry like chat:* would match nothing AND is not even a
// valid channel name — subscribing it literally is dead weight. We therefore
// subscribe only the concrete/valid entries here (keeps a manifest that lists
// real channels working), and let a glob entry be realized into a concrete
// subscription lazily, per target chat, when inbound flows through the publish
// handler (api.go publish seeds via Identity.MatchesSink). The drainer still
// starts so those later-seeded subscriptions get drained.
func (h *Host) startSink(ctx context.Context, rec Record, socket string) {
	if !manifestFromRecord(rec).HasType("channel-sink") || len(rec.Channels.Subscribe) == 0 {
		return
	}
	subscriber := "plugin:" + rec.Name
	for _, ch := range rec.Channels.Subscribe {
		if !bus.ValidChannel(ch) {
			// A wildcard/glob (e.g. chat:*) or otherwise non-concrete entry: not a
			// real channel the exact-match bus can deliver to. Skip the literal
			// subscribe; publish-time seeding will register the concrete channels.
			continue
		}
		if _, err := h.cfg.Bus.Subscribe(subscriber, ch, bus.Matcher{}, nil); err != nil {
			h.cfg.Log.Warn("sink subscribe failed", "plugin", rec.Name, "channel", ch, "err", err)
		}
	}
	client := NewClient(socket)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-h.cfg.After(sinkPollInterval):
				h.drainOnce(ctx, rec.Name, subscriber, client)
			}
		}
	}()
}

// drainOnce delivers all currently-pending messages for one plugin. A delivery
// failure is left unacked (bus redelivery + DLQ handle it) and surfaced as a
// system:plugins event. Never panics on plugin I/O (spec §13).
func (h *Host) drainOnce(ctx context.Context, name, subscriber string, client *Client) {
	msgs, err := h.cfg.Bus.Pending(subscriber, 50)
	if err != nil {
		h.cfg.Log.Warn("sink pending", "plugin", name, "err", err)
		return
	}
	for _, m := range msgs {
		// ECHO SUPPRESSION: once a sink is subscribed to a concrete chat channel
		// (seeded on publish), it is subscribed to a channel it ALSO publishes
		// INBOUND on — and the bus creates a delivery row for every matching
		// subscription regardless of msg.Source. Without this guard the plugin
		// would receive its own inbound back at /deliver and loop. Deliver only
		// genuine outbound (agent replies / other sources); ack+skip anything this
		// same plugin produced so it drops out of Pending instead of retrying.
		if m.ProducedByPlugin == name || m.Source == subscriber {
			_ = h.cfg.Bus.Ack(subscriber, []string{m.ID})
			continue
		}
		dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := client.Deliver(dctx, toDTO(m))
		cancel()
		if err != nil {
			h.cfg.Log.Warn("sink deliver failed", "plugin", name, "msg", m.ID, "err", err)
			_, _ = h.cfg.Bus.Publish(bus.Message{
				Channel: "system:plugins", Type: "plugin.deliver_failed", Source: "daemon",
				Text:             fmt.Sprintf("plugin %s deliver failed for %s", name, m.ID),
				ProducedByPlugin: name,
				Data:             map[string]any{"plugin": name, "message": m.ID, "detail": err.Error()},
			})
			continue // leave unacked for redelivery/DLQ
		}
		_ = h.cfg.Bus.Ack(subscriber, []string{m.ID})
	}
}

func (h *Host) stopSink(name string) {} // drainer exits on the plugin ctx cancel

// ApplyActionSubscriptions validates and applies the protocol-level subscription
// effects returned by a successful plugin action. It accepts effects only from a
// running sink, confines channels to the manifest scope, preserves standing
// manifest subscriptions and route-backed channels, and rolls back bus changes
// if a later mutation fails.
func (h *Host) ApplyActionSubscriptions(name string, response map[string]any) error {
	add, remove, present, err := parseSubscriptionEffects(response)
	if err != nil || !present {
		return err
	}
	if _, err := h.socketIfRunning(name); err != nil {
		return err
	}
	rec, ok, err := h.cfg.Store.Get(name)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	sink := Identity{Sink: sinkPatterns(rec)}
	if len(sink.Sink) == 0 {
		return fmt.Errorf("plugin %s is not a channel-sink", name)
	}

	seen := map[string]string{}
	for _, effect := range []struct {
		kind     string
		channels []string
	}{{"add", add}, {"remove", remove}} {
		for _, channel := range effect.channels {
			if !bus.ValidChannel(channel) {
				return fmt.Errorf("subscriptions.%s contains invalid channel %q", effect.kind, channel)
			}
			if !sink.MatchesSink(channel) {
				return fmt.Errorf("subscriptions.%s channel %q is outside plugin scope", effect.kind, channel)
			}
			if previous, exists := seen[channel]; exists && previous != effect.kind {
				return fmt.Errorf("subscription effect both adds and removes %q", channel)
			}
			seen[channel] = effect.kind
		}
	}
	for _, channel := range remove {
		for _, declared := range rec.Channels.Subscribe {
			if declared == channel {
				return fmt.Errorf("cannot remove declared subscription %q", channel)
			}
		}
	}
	if len(remove) > 0 {
		routes, err := h.PluginRoutes(name)
		if err != nil {
			return fmt.Errorf("verify routes before subscription removal: %w", err)
		}
		for _, channel := range remove {
			if containsActionRoute(routes["routes"], channel) {
				return fmt.Errorf("cannot remove subscription %q while a route still uses it", channel)
			}
		}
	}

	subscriber := "plugin:" + name
	existingRows, err := h.cfg.Bus.ListSubscriptions(subscriber)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, subscription := range existingRows {
		existing[subscription.Channel] = true
	}
	added := []string{}
	removed := []string{}
	rollback := func() {
		for _, channel := range added {
			_, _ = h.cfg.Bus.UnsubscribeChannel(subscriber, channel)
		}
		for _, channel := range removed {
			_, _ = h.cfg.Bus.Subscribe(subscriber, channel, bus.Matcher{}, nil)
		}
	}
	for _, channel := range add {
		if err := h.cfg.Bus.EnsureChannel(channel); err != nil {
			rollback()
			return err
		}
		if _, err := h.cfg.Bus.Subscribe(subscriber, channel, bus.Matcher{}, nil); err != nil {
			rollback()
			return err
		}
		if !existing[channel] {
			added = append(added, channel)
		}
	}
	for _, channel := range remove {
		_, err := h.cfg.Bus.UnsubscribeChannel(subscriber, channel)
		if err != nil && !errors.Is(err, bus.ErrNotFound) {
			rollback()
			return err
		}
		if err == nil && existing[channel] {
			removed = append(removed, channel)
		}
	}
	return nil
}

func containsActionRoute(value any, channel string) bool {
	switch typed := value.(type) {
	case string:
		return typed == channel
	case []any:
		for _, child := range typed {
			if containsActionRoute(child, channel) {
				return true
			}
		}
	case map[string]any:
		for _, child := range typed {
			if containsActionRoute(child, channel) {
				return true
			}
		}
	}
	return false
}

func parseSubscriptionEffects(response map[string]any) (add, remove []string, present bool, err error) {
	raw, present := response["subscriptions"]
	if !present {
		return nil, nil, false, nil
	}
	metadata, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, true, fmt.Errorf("subscriptions must be an object")
	}
	for key := range metadata {
		if key != "add" && key != "remove" {
			return nil, nil, true, fmt.Errorf("subscriptions contains unknown field %q", key)
		}
	}
	parse := func(key string) ([]string, error) {
		rawList, exists := metadata[key]
		if !exists {
			return nil, nil
		}
		var values []any
		switch list := rawList.(type) {
		case []any:
			values = list
		case []string:
			values = make([]any, len(list))
			for i := range list {
				values[i] = list[i]
			}
		default:
			return nil, fmt.Errorf("subscriptions.%s must be an array of strings", key)
		}
		out := make([]string, 0, len(values))
		seen := map[string]bool{}
		for _, value := range values {
			channel, ok := value.(string)
			if !ok || channel == "" {
				return nil, fmt.Errorf("subscriptions.%s must be an array of non-empty strings", key)
			}
			if !seen[channel] {
				out = append(out, channel)
				seen[channel] = true
			}
		}
		return out, nil
	}
	add, err = parse("add")
	if err != nil {
		return nil, nil, true, err
	}
	remove, err = parse("remove")
	return add, remove, true, err
}

// pushWatchesMaxAttempts bounds the push retry loop. Push is best-effort: the
// plugin's GET /api/plugin/watches pull path guarantees eventual consistency,
// so after this many failed attempts we give up rather than block forever.
const pushWatchesMaxAttempts = 5

// providedChannelNames extracts the channel names from a manifest's provide
// declarations, dropping the params_schema/help detail the token scope needs.
func providedChannelNames(provided []Provided) []string {
	if len(provided) == 0 {
		return nil
	}
	out := make([]string, 0, len(provided))
	for _, p := range provided {
		out = append(out, p.Channel)
	}
	return out
}

// watchDTOs converts the bus's live watch list into the plugin wire form.
func watchDTOs(ws []bus.WatchInfo) []WatchDTO {
	out := make([]WatchDTO, 0, len(ws))
	for _, w := range ws {
		subs := w.Subscribers
		if subs == nil {
			subs = []string{}
		}
		out = append(out, WatchDTO{Watch: w.Watch, Params: w.Params, Subscribers: subs})
	}
	return out
}

// PushWatches asynchronously delivers the full current watch list for a provided
// channel to its provider plugin's socket (spec §6.2). Delivery is serialized
// per (plugin, channel): concurrent subscribe/unsubscribe events would otherwise
// each spawn an independent goroutine, and a stale full-list snapshot could
// overtake a newer one — leaving the provider reconciled to a watch that is gone
// (or missing one that is live) until the next event or restart. Here a single
// worker per pair drains snapshots in call order, and a newer snapshot coalesces
// over any not-yet-delivered one, so the provider always converges on the final
// state. Each delivery is still best-effort with capped backoff (see
// deliverWatches); the worker is tracked on h.wg and exits on daemon-ctx cancel,
// so StopAll drains it.
func (h *Host) PushWatches(plugin, channel string, watches []bus.WatchInfo) {
	h.enqueueWatches(plugin, channel, watchDTOs(watches), false)
}

// pushCurrentWatches enqueues a lazy authoritative refresh. The serialized
// worker reads WatchList only after it owns this queue item, so a startup
// reconcile cannot carry an old pre-subscribe snapshot and overwrite a newer
// subscription hook. Reading outside pushMu avoids coupling the bus/store lock
// order to the delivery queue.
func (h *Host) pushCurrentWatches(plugin, channel string) {
	h.enqueueWatches(plugin, channel, nil, true)
}

func (h *Host) enqueueWatches(plugin, channel string, dtos []WatchDTO, current bool) {
	key := plugin + "\x00" + channel

	h.pushMu.Lock()
	wp := h.pushQueue[key]
	if wp == nil {
		wp = &watchPush{}
		h.pushQueue[key] = wp
	}
	wp.pending = dtos
	wp.pendingCurrent = current
	wp.hasPending = true
	if wp.busy {
		// A worker is already draining this pair; it will pick up the snapshot
		// we just stored (overwriting any earlier undelivered one).
		h.pushMu.Unlock()
		return
	}
	wp.busy = true
	h.pushMu.Unlock()

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for {
			h.pushMu.Lock()
			if !wp.hasPending {
				// Drained: drop the entry so map presence == "worker live".
				delete(h.pushQueue, key)
				wp.busy = false
				h.pushMu.Unlock()
				return
			}
			dtos := wp.pending
			current := wp.pendingCurrent
			wp.pending = nil
			wp.pendingCurrent = false
			wp.hasPending = false
			h.pushMu.Unlock()
			if current {
				watches, err := h.cfg.Bus.WatchList(channel)
				if err != nil {
					h.cfg.Log.Warn("plugin running reconcile: watch list", "plugin", plugin, "channel", channel, "err", err)
					continue
				}
				dtos = watchDTOs(watches)
			}

			h.deliverWatches(plugin, channel, dtos)
		}
	}()
}

// deliverWatches sends one watch snapshot to a running provider's socket with
// capped backoff. Best-effort: if the plugin is not running, or every attempt
// fails, it gives up — the plugin reconciles via GET /api/plugin/watches on
// (re)start. Called only from the serialized PushWatches worker, so snapshots
// never overlap for a given (plugin, channel).
func (h *Host) deliverWatches(plugin, channel string, dtos []WatchDTO) {
	for attempt := 0; attempt < pushWatchesMaxAttempts; attempt++ {
		sock, err := h.socketIfRunning(plugin)
		if err != nil {
			return // not running: pull reconciles on (re)start
		}
		ctx, cancel := context.WithTimeout(h.baseCtx, 3*time.Second)
		err = NewClient(sock).PushWatches(ctx, channel, dtos)
		cancel()
		if err == nil {
			return
		}
		h.cfg.Log.Warn("push watches failed", "plugin", plugin, "channel", channel,
			"attempt", attempt+1, "err", err)
		select {
		case <-h.baseCtx.Done():
			return
		case <-h.cfg.After(backoff(attempt)):
		}
	}
}

// unsubscribeSink drops a removed plugin's durable subscriptions (and their
// orphan deliveries) so no outbox is left behind.
func (h *Host) unsubscribeSink(name string) {
	subscriber := "plugin:" + name
	subs, err := h.cfg.Bus.ListSubscriptions(subscriber)
	if err != nil {
		return
	}
	for _, s := range subs {
		_ = h.cfg.Bus.Unsubscribe(subscriber, s.ID)
	}
}

func toDTO(m bus.Message) MessageDTO {
	return MessageDTO{
		ID: m.ID, Channel: m.Channel, TS: m.TS, Source: m.Source, Type: m.Type,
		Subject: m.Subject, Text: m.Text, Data: m.Data,
		ProducedByAgent: m.ProducedByAgent, ProducedInIteration: m.ProducedInIteration,
		ProducedByPlugin: m.ProducedByPlugin,
		Kind:             m.Kind,
		CorrelationID:    m.CorrelationID,
		InReplyTo:        m.InReplyTo,
		ReplyTo:          m.ReplyTo,
		Deadline:         m.Deadline,
	}
}
