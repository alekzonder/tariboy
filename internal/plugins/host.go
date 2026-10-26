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
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/safefs"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tidy"
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

// ErrHostStopping is returned by Start once StopAll has begun.
var ErrHostStopping = errors.New("plugin host is stopping")

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
	Mail         Mailbox
	Tokens       *TokenRegistry
	PluginsDir   string
	DaemonSocket string
	Runner       Runner
	Clock        func() time.Time
	After        func(time.Duration) <-chan time.Time
	DrainAfter   func(time.Duration) <-chan time.Time
	// SinkAfter is the sink drainer's fallback timer (default time.After).
	SinkAfter func(time.Duration) <-chan time.Time
	Log       *slog.Logger
}

// Host is the registry of running plugins: it starts a Supervisor per enabled
// plugin, tracks them concurrency-safely, and drains them all (cancel + wait)
// before the daemon closes the store.
type Host struct {
	cfg HostConfig
	mu  sync.Mutex
	// rootCtx parents every plugin life and background worker; StopAll cancels
	// it through rootCancel. stopping refuses Start and new workers once StopAll
	// has begun, so nothing joins h.wg after its Wait. All three are under mu.
	rootCtx    context.Context
	rootCancel context.CancelFunc
	stopping   bool
	running    map[string]*runningPlugin
	wg         sync.WaitGroup

	// wakeMu guards wakes: one buffered signal per plugin that tells its mail
	// drainer a letter arrived. See WakePlugins.
	wakeMu sync.Mutex
	wakes  map[string]chan struct{}
}

type Contribution struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Commands    []OperatorCommand     `json:"operator_commands,omitempty"`
	Settings    *SettingsContribution `json:"settings,omitempty"`
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
	if cfg.SinkAfter == nil {
		cfg.SinkAfter = time.After
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &Host{
		cfg:        cfg,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		running:    map[string]*runningPlugin{},
		wakes:      map[string]chan struct{}{},
	}
}

// track registers one background goroutine on h.wg and returns the root
// context it must exit on, or false once StopAll has begun.
func (h *Host) track() (context.Context, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping {
		return nil, false
	}
	h.wg.Add(1)
	return h.rootCtx, true
}

func (h *Host) SocketPath(name string) string {
	return filepath.Join(h.cfg.PluginsDir, name, socketFileName)
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

func (h *Host) Contributions() ([]Contribution, error) {
	recs, err := h.cfg.Store.List()
	if err != nil {
		return nil, err
	}
	out := []Contribution{}
	for _, rec := range recs {
		if !rec.Enabled {
			continue
		}
		active, ok, err := h.cfg.Store.ActiveVersion(rec.Name)
		if err != nil {
			return nil, err
		}
		if !ok || active != rec.Version {
			continue
		}
		manifest, err := LoadManifest(h.versionDir(rec.Name, active))
		if err != nil {
			return nil, err
		}
		if len(manifest.OperatorCommands) == 0 && manifest.Settings == nil {
			continue
		}
		out = append(out, Contribution{
			Name: manifest.Name, Description: manifest.Description,
			Commands: manifest.OperatorCommands, Settings: manifest.Settings,
		})
	}
	return out, nil
}

func (h *Host) ValidateOperatorAction(name, action string, data map[string]any) error {
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
	active, ok, err := h.cfg.Store.ActiveVersion(name)
	if err != nil {
		return err
	}
	if !ok || active != rec.Version {
		return fmt.Errorf("plugin %s has no active manifest", name)
	}
	manifest, err := LoadManifest(h.versionDir(name, active))
	if err != nil {
		return err
	}
	return manifest.ValidateOperatorAction(action, data)
}
func (h *Host) dir(name string) string                 { return filepath.Join(h.cfg.PluginsDir, name) }
func (h *Host) versionDir(name, version string) string { return filepath.Join(h.dir(name), version) }
func (h *Host) workdir(name string) string             { return filepath.Join(h.dir(name), workdirName) }
func (h *Host) logPath(name string) string {
	return filepath.Join(h.dir(name), logsDirName, "plugin.log")
}

// StartAll re-launches every enabled plugin after a daemon restart.
func (h *Host) StartAll(ctx context.Context) error {
	h.mu.Lock()
	if h.stopping {
		h.mu.Unlock()
		return ErrHostStopping
	}
	// Derive a cancellable root from ctx. Keep the previous root's cancel in the
	// chain: anything started before StartAll still stops with StopAll.
	rootCtx, cancel := context.WithCancel(ctx)
	previous := h.rootCancel
	h.rootCtx, h.rootCancel = rootCtx, func() { cancel(); previous() }
	h.mu.Unlock()
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
	for _, entry := range entries {
		if reservedDirName(entry.Name()) || entry.Name() == filepath.Base(tmp) {
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
	if h.stopping {
		h.mu.Unlock()
		return ErrHostStopping
	}
	if _, ok := h.running[name]; ok {
		h.mu.Unlock()
		return nil // already running or starting
	}
	ctx, cancel := context.WithCancel(h.rootCtx)
	rp := &runningPlugin{rec: rec, cancel: cancel, state: "starting", health: map[string]any{}, done: make(chan struct{})}
	h.running[name] = rp
	// The reservation holds h.wg from here, under mu, so StopAll's Wait can
	// never miss this life; the supervisor goroutine (or fail) releases it.
	h.wg.Add(1)
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
		h.wg.Done()
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
			"checked_at": store.FormatTime(h.cfg.Clock()),
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
	// plugins can only ever reach an LLM through the scoped proxy
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

	// Carry-forward (b): the token scope is the plugin's declared mail surface.
	identity := Identity{Name: name, Send: rec.Mail.Send && manifestFromRecord(rec).HasType("mail-source")}

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

	// A mail-sink plugin gets the drainer of its inbox.
	h.startDrain(ctx, rec, sock)

	go func() {
		defer h.wg.Done() // the reservation's h.wg.Add above
		// Signal Restart (and any per-instance waiter) that this instance has
		// fully drained. Closed last, after the supervisor has graceful-stopped
		// the process and revoked its token on the way out of sup.Run.
		defer close(rp.done)
		defer func() {
			tidy.Log(h.cfg.Log, "plugin log flush failed", logtw.Flush(), "plugin", name)
			tidy.Log(h.cfg.Log, "plugin log close failed", logf.Close(), "plugin", name)
		}()
		// Defense-in-depth: a panic in the host-side onState path must not crash
		// the daemon. Plugin process failures are already out-of-process.
		defer func() {
			if r := recover(); r != nil {
				h.cfg.Log.Warn("plugin supervisor goroutine panicked", "plugin", name, "panic", r)
				h.onState(name, "error", map[string]any{
					"checked_at": store.FormatTime(h.cfg.Clock()),
					"detail":     fmt.Sprintf("supervisor panic: %v", r),
				})
			}
		}()
		sup.Run(ctx)
	}()
	return nil
}

// onState fans a supervisor state change to the live snapshot, the durable
// store, and the daemon log. Repeated idempotent "running" emissions are
// deduped so a healthy plugin does not spam the log.
func (h *Host) onState(name, state string, health map[string]any) {
	h.mu.Lock()
	rp := h.running[name]
	h.mu.Unlock()
	if rp != nil {
		rp.mu.Lock()
		dup := rp.state == state && state == "running"
		rp.state, rp.health = state, health
		rp.mu.Unlock()
		if dup {
			return
		}
	}
	hb, _ := json.Marshal(health)
	if err := h.cfg.Store.SetState(name, state, string(hb)); err != nil {
		h.cfg.Log.Error("plugin state persist failed", "plugin", name, "state", state, "err", err)
	}
	detail, _ := health["detail"].(string)
	h.cfg.Log.Info("plugin state", "plugin", name, "state", state, "detail", detail)
}

// Stop cancels the supervisor and removes it from the registry. The per-life
// token is revoked by the supervisor's RevokeToken seam on the exit path.
func (h *Host) Stop(name string) error {
	h.stop(name)
	return nil
}

// stop cancels name's life, if any. The supervisor and sink drainer exit on
// that context.
func (h *Host) stop(name string) {
	h.mu.Lock()
	rp := h.running[name]
	delete(h.running, name)
	h.mu.Unlock()
	if rp != nil {
		rp.cancel()
	}
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
		h.stop(name)
		select {
		case <-rp.done:
		case <-h.cfg.DrainAfter(restartDrainGrace):
			h.cfg.Log.Warn("plugin restart: previous instance did not drain before deadline; starting fresh instance anyway",
				"plugin", name)
		}
	}
	return h.Start(rec)
}

// StopAll gracefully stops every plugin and waits, so nothing touches the
// store after the daemon closes it.
// It first refuses new work and cancels the root, so a pending rollback restart
// or watch push exits instead of starting anything; Start returns
// ErrHostStopping from then on.
func (h *Host) StopAll() {
	h.mu.Lock()
	h.stopping = true
	cancel := h.rootCancel
	names := make([]string, 0, len(h.running))
	for n := range h.running {
		names = append(names, n)
	}
	h.mu.Unlock()
	cancel()
	for _, n := range names {
		h.stop(n)
	}
	h.wg.Wait()
}

// Install validates+copies a plugin dir into PluginsDir, records it, and starts it.
func (h *Host) Install(sourcePath string) (map[string]any, error) {
	// SECURITY: reject the whole source tree if it
	// contains ANY symlink, anywhere (not just the top level). A plugin package
	// shipping a symlink whose target lies outside the plugin dir would let a
	// later exec or read escape containment once copied into PluginsDir — this
	// is the install-time defense layer alongside ResolveExec's spawn-time gate.
	if err := checkPluginSource(sourcePath); err != nil {
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
		Exec: m.Exec, SourcePath: sourcePath, Mail: m.Mail, Enabled: true, State: "installed",
	}
	previous, hadPrevious, err := h.cfg.Store.Get(m.Name)
	if err != nil {
		return nil, err
	}
	previousActive, hadPreviousActive, err := h.cfg.Store.ActiveVersion(m.Name)
	if err != nil {
		return nil, err
	}
	// rollback restores the previous record and active version and returns
	// every step that failed, so the caller can report them with the cause.
	rollback := func() error {
		removeMarker := func() error {
			if err := os.Remove(filepath.Join(h.dir(m.Name), activeVersionFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}
		if !hadPrevious {
			return errors.Join(h.cfg.Store.Delete(m.Name), removeMarker())
		}
		if hadPreviousActive {
			return errors.Join(h.cfg.Store.Upsert(previous),
				h.cfg.Store.SetActiveVersion(m.Name, previousActive),
				writeActiveVersion(h.dir(m.Name), previousActive))
		}
		return errors.Join(h.cfg.Store.Upsert(previous), h.cfg.Store.ClearActiveVersion(m.Name), removeMarker())
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
				if startErr := h.Start(previous); startErr != nil && !errors.Is(startErr, ErrHostStopping) {
					h.cfg.Log.Error("restart rolled-back plugin", "plugin", m.Name, "err", startErr)
				}
			}
		}
		if after == nil {
			restart()
			return
		}
		h.afterDrain(after, restart)
	}
	h.mu.Lock()
	previousRunning := h.running[m.Name]
	h.mu.Unlock()
	h.stop(m.Name)
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
		h.stop(m.Name)
		if rollbackErr := rollback(); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("roll back plugin %s: %w", m.Name, rollbackErr))
		}
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

// afterDrain runs restart once the previous instance after has fully exited.
// The wait is tracked on h.wg and gives up when StopAll cancels the root, so a
// rollback never starts a plugin during shutdown.
func (h *Host) afterDrain(after *runningPlugin, restart func()) {
	ctx, ok := h.track()
	if !ok {
		return
	}
	go func() {
		defer h.wg.Done()
		select {
		case <-after.done:
		case <-ctx.Done():
			return
		}
		if ctx.Err() == nil {
			restart()
		}
	}()
}

// Remove stops the plugin, deletes the row and files.
func (h *Host) Remove(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	h.stop(name)
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
	list, err := h.List()
	if err != nil {
		return nil, err
	}
	state := rec.State
	for _, e := range list {
		if e["name"] == name {
			state, _ = e["state"].(string)
		}
	}
	return map[string]any{
		"name": rec.Name, "version": rec.Version, "types": rec.Types,
		"protocol_version": rec.ProtocolVersion, "exec": rec.Exec, "source_path": rec.SourcePath,
		"mail": rec.Mail, "enabled": rec.Enabled, "state": state,
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
		Types: r.Types, Exec: r.Exec, Mail: r.Mail}
}

// checkPluginSource rejects the whole source tree if it holds a symlink
// (the root included) or any other non-regular file, anywhere.
func checkPluginSource(sourcePath string) error {
	err := safefs.WalkRegular(sourcePath, safefs.WalkOptions{}, func(string, fs.DirEntry, fs.FileInfo) error { return nil })
	var pathErr *safefs.PathError
	if !errors.As(err, &pathErr) {
		return err
	}
	full := filepath.Join(sourcePath, filepath.FromSlash(pathErr.Rel))
	if errors.Is(err, safefs.ErrSymlink) {
		return fmt.Errorf("plugin source %s contains a symlink (%s); symlinks are not permitted in a plugin package", sourcePath, full)
	}
	return fmt.Errorf("plugin source %s contains non-regular file %s", sourcePath, full)
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
	tmp, err := os.CreateTemp(dir, "."+activeVersionFile+"-")
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
	return os.Rename(name, filepath.Join(dir, activeVersionFile))
}

// --- mail-sink delivery ---

// sinkFallbackInterval bounds how long a drainer sleeps without a wake. A
// wake carries new letters; the timer retries failed deliveries.
const sinkFallbackInterval = 5 * time.Second

// sinkBatch is how many open letters one drain pass takes.
const sinkBatch = 50

// Mailbox is the slice of the mail service the host needs: the inbox drain
// of a plugin principal, with the attempt and dead-letter rules, and the close
// of a delivered letter.
type Mailbox interface {
	DrainBatch(principal string, n int) ([]mail.Letter, error)
	Close(as, id, note string) (mail.Letter, error)
}

// WakePlugins signals the drainers of the named plugins that a letter
// arrived. The daemon wires it as the mail plugin wake hook.
func (h *Host) WakePlugins(names []string) {
	for _, name := range names {
		select {
		case h.wakeChan(name) <- struct{}{}:
		default:
		}
	}
}

func (h *Host) wakeChan(name string) chan struct{} {
	h.wakeMu.Lock()
	defer h.wakeMu.Unlock()
	ch, ok := h.wakes[name]
	if !ok {
		ch = make(chan struct{}, 1)
		h.wakes[name] = ch
	}
	return ch
}

// startDrain runs the drainer of a mail-sink plugin's inbox: it delivers each
// open letter to plugin:<name> at the plugin's /deliver and closes it as
// delivered. It drains at start, on every wake, and at the latest every
// sinkFallbackInterval to retry failed deliveries; it exits on ctx.
func (h *Host) startDrain(ctx context.Context, rec Record, socket string) {
	if h.cfg.Mail == nil || !rec.Mail.Receive || !manifestFromRecord(rec).HasType("mail-sink") {
		return
	}
	principal := "plugin:" + rec.Name
	client := NewClient(socket)
	// Take the signal before the first drain so a send in between still
	// leaves a pending wake.
	wake := h.wakeChan(rec.Name)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for {
			for ctx.Err() == nil && h.drainOnce(ctx, rec.Name, principal, client) {
			}
			select {
			case <-ctx.Done():
				return
			case <-wake:
			case <-h.cfg.SinkAfter(sinkFallbackInterval):
			}
		}
	}()
}

// drainOnce delivers one batch of open letters to one plugin. Taking a letter
// counts an attempt, so a letter the plugin keeps failing is dead-lettered
// after five; a failed delivery stays open for the next pass. It reports
// whether a full batch went through cleanly, so more may be waiting now;
// after a failure the drainer waits instead of spending attempts in a loop.
func (h *Host) drainOnce(ctx context.Context, name, principal string, client *Client) bool {
	letters, err := h.cfg.Mail.DrainBatch(principal, sinkBatch)
	if err != nil {
		h.cfg.Log.Warn("plugin inbox drain", "plugin", name, "err", err)
		return false
	}
	clean := true
	for _, l := range letters {
		dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := client.Deliver(dctx, l)
		cancel()
		if err != nil {
			h.cfg.Log.Warn("plugin deliver failed", "plugin", name, "letter", l.ID, "err", err)
			clean = false
			continue
		}
		if _, err := h.cfg.Mail.Close(principal, l.ID, mail.CloseNoteDelivered); err != nil {
			// The letter stays open and is offered again.
			h.cfg.Log.Warn("plugin delivered letter not closed", "plugin", name, "letter", l.ID, "err", err)
			clean = false
		}
	}
	return clean && len(letters) == sinkBatch
}
