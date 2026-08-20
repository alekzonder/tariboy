package plugins

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SpawnSpec is everything the runner needs to launch one plugin process.
type SpawnSpec struct {
	Name   string
	Exec   string // absolute path to the plugin executable (already symlink-resolved via ResolveExec)
	Dir    string // working dir
	Socket string // unix socket the plugin must listen on
	Env    []string
}

// HandshakeResult carries the plugin's first stdout line (or a read error).
type HandshakeResult struct {
	Line []byte
	Err  error
}

// ProcHandle is a running plugin process. Done() closes when it exits.
type ProcHandle interface {
	Handshake() <-chan HandshakeResult
	Done() <-chan struct{}
	Stop(grace time.Duration) error
	Pid() int
}

// Runner starts a plugin process. execRunner is the real one; tests fake it.
type Runner interface {
	Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error)
}

// ResolveExec is the symlink-safe gate for what the daemon will exec (carry-forward
// from the manifest task). Manifest.Validate is a string-only check; it cannot see
// through symlinks. ResolveExec resolves both the plugin dir and the exec to their
// real paths and refuses any exec whose real path escapes the real plugin dir (e.g.
// a symlink to /bin/sh). The caller (host) passes the returned absolute path as
// SpawnSpec.Exec.
func ResolveExec(pluginDir, exec string) (string, error) {
	realDir, err := filepath.EvalSymlinks(pluginDir)
	if err != nil {
		return "", fmt.Errorf("resolve plugin dir %q: %w", pluginDir, err)
	}
	real, err := filepath.EvalSymlinks(filepath.Join(realDir, exec))
	if err != nil {
		return "", fmt.Errorf("resolve exec %q: %w", exec, err)
	}
	rel, err := filepath.Rel(realDir, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin exec %q resolves to %q, outside plugin dir %q", exec, real, realDir)
	}
	return real, nil
}

type execRunner struct{}

// NewExecRunner returns the real os/exec + setsid runner.
func NewExecRunner() Runner { return execRunner{} }

func (execRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	// Serialise all writes to the plugin log so the stderr copier and the
	// stdout tee cannot race (carry-forward 6: a plugin never trips -race).
	safe := &syncWriter{w: logw}
	cmd := exec.Command(spec.Exec)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // own process group (spec §13)
	cmd.Stderr = safe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &execHandle{
		pid:  cmd.Process.Pid,
		hs:   make(chan HandshakeResult, 1),
		done: make(chan struct{}),
	}
	go h.readHandshake(stdout, safe)
	go func() {
		_ = cmd.Wait()
		close(h.done)
	}()
	return h, nil
}

type execHandle struct {
	pid  int
	hs   chan HandshakeResult
	done chan struct{}
}

func (h *execHandle) Handshake() <-chan HandshakeResult { return h.hs }
func (h *execHandle) Done() <-chan struct{}             { return h.done }
func (h *execHandle) Pid() int                          { return h.pid }

// maxHandshakeLine bounds the handshake read (readHandshake below) so a
// plugin that streams data with no newline during the handshake window
// cannot balloon daemon memory.
const maxHandshakeLine = 64 * 1024

// readHandshake sends the first stdout line, then tees the rest to the log file.
func (h *execHandle) readHandshake(stdout io.Reader, logw io.Writer) {
	limited := &io.LimitedReader{R: stdout, N: maxHandshakeLine}
	br := bufio.NewReader(limited)
	line, err := br.ReadBytes('\n')
	if err != nil && limited.N <= 0 {
		// The cap was hit before a newline was found: refuse to buffer any
		// further rather than reading an unbounded amount from the plugin.
		h.hs <- HandshakeResult{Err: fmt.Errorf("handshake line too long (exceeds %d bytes)", maxHandshakeLine)}
		return
	}
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 && err != nil {
		h.hs <- HandshakeResult{Err: err}
		return
	}
	h.hs <- HandshakeResult{Line: trimmed}
	fmt.Fprintf(logw, "%s\n", trimmed)
	_, _ = io.Copy(logw, br)
}

// Stop signals the whole process group: SIGTERM, then SIGKILL after grace.
func (h *execHandle) Stop(grace time.Duration) error {
	if h.pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-h.pid, syscall.SIGTERM)
	select {
	case <-h.done:
		return nil
	case <-time.After(grace):
		return syscall.Kill(-h.pid, syscall.SIGKILL)
	}
}

// syncWriter guards a shared io.Writer with a mutex.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

const (
	defaultHandshakeTimeout = 5 * time.Second
	defaultHealthInterval   = 5 * time.Second
	defaultStopGrace        = 5 * time.Second
	healthFailThreshold     = 3
)

// backoff is capped exponential: 500ms, 1s, 2s, 4s, ... capped at 30s.
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 500 * time.Millisecond << (attempt - 1)
	if d > 30*time.Second || d <= 0 {
		return 30 * time.Second
	}
	return d
}

// SupervisorConfig is the fully-injected configuration for one plugin's
// supervisor. Clock/After/Runner/Client are seams so tests are deterministic.
type SupervisorConfig struct {
	Name     string
	Manifest Manifest
	Spec     SpawnSpec
	Runner   Runner
	Client   *Client
	OnState  func(state string, health map[string]any)
	Clock    func() time.Time
	After    func(time.Duration) <-chan time.Time
	Log      *slog.Logger

	// LogWriter captures the plugin's stdout tail + stderr (default io.Discard).
	LogWriter io.Writer

	// MintToken/RevokeToken manage the per-life plugin token (carry-forward 5).
	// When MintToken is set, the supervisor mints a fresh token at the start of
	// every life, injects it as TARIBOY_PLUGIN_TOKEN, and revokes it on EVERY
	// exit path (spawn fail, handshake reject/timeout, exit, unhealthy, cancel).
	MintToken   func() (string, error)
	RevokeToken func(token string)

	HandshakeTimeout time.Duration
	HealthInterval   time.Duration
	StopGrace        time.Duration
}

// Supervisor is a per-plugin state machine. It owns no store/bus state; it
// reports state changes through OnState (the host turns those into a store
// update + a system:plugins bus event).
type Supervisor struct{ cfg SupervisorConfig }

func NewSupervisor(cfg SupervisorConfig) *Supervisor {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.After == nil {
		cfg.After = time.After
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.LogWriter == nil {
		cfg.LogWriter = io.Discard
	}
	if cfg.HandshakeTimeout <= 0 {
		cfg.HandshakeTimeout = defaultHandshakeTimeout
	}
	if cfg.HealthInterval <= 0 {
		cfg.HealthInterval = defaultHealthInterval
	}
	if cfg.StopGrace <= 0 {
		cfg.StopGrace = defaultStopGrace
	}
	return &Supervisor{cfg: cfg}
}

func (s *Supervisor) state(state string, detail string) {
	if s.cfg.OnState != nil {
		s.cfg.OnState(state, map[string]any{
			"checked_at": s.cfg.Clock().UTC().Format(time.RFC3339),
			"detail":     detail,
		})
	}
}

// Run supervises the plugin until ctx is cancelled. On every return path it has
// graceful-stopped the process and reported "stopped".
func (s *Supervisor) Run(ctx context.Context) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			s.state("stopped", "daemon shutdown")
			return
		}
		reason, reachedRunning := s.oneLife(ctx)
		if reason == lifeCancelled {
			s.state("stopped", "daemon shutdown")
			return
		}
		if reachedRunning {
			// The life reached "running" (handshake ok + first health check
			// passed), so it wasn't a startup crash loop. Reset the backoff
			// counter so the NEXT restart starts from the floor instead of
			// carrying a near-cap delay from earlier, unrelated failures.
			attempt = 0
		}
		attempt++
		s.cfg.Log.Warn("plugin restarting", "plugin", s.cfg.Name, "reason", reason, "attempt", attempt)
		select {
		case <-ctx.Done():
			s.state("stopped", "daemon shutdown")
			return
		case <-s.cfg.After(backoff(attempt)):
		}
	}
}

type lifeReason string

const (
	lifeCancelled lifeReason = "cancelled"
	lifeExited    lifeReason = "exited"
	lifeUnhealthy lifeReason = "unhealthy"
	lifeRejected  lifeReason = "rejected"
)

// oneLife runs a single spawn→handshake→health cycle and returns why it
// ended, plus whether the life ever reached the "running" state (handshake
// ok + first health check passed). Run uses reachedRunning to reset the
// backoff counter: a life that got healthy before failing again is not part
// of a startup crash loop.
func (s *Supervisor) oneLife(ctx context.Context) (reason lifeReason, reachedRunning bool) {
	spec := s.cfg.Spec

	// Carry-forward 5: mint a fresh per-life token and guarantee revoke on EVERY
	// exit path below (defer runs whether we reject, exit, go unhealthy or cancel).
	if s.cfg.MintToken != nil {
		tok, err := s.cfg.MintToken()
		if err != nil {
			s.state("unhealthy", "token mint failed: "+err.Error())
			return lifeExited, reachedRunning
		}
		if s.cfg.RevokeToken != nil {
			defer s.cfg.RevokeToken(tok)
		}
		env := append([]string(nil), spec.Env...)
		env = append(env, "TARIBOY_PLUGIN_TOKEN="+tok)
		spec.Env = env
	}

	h, err := s.cfg.Runner.Start(spec, s.cfg.LogWriter)
	if err != nil {
		s.state("unhealthy", "spawn failed: "+err.Error())
		return lifeExited, reachedRunning
	}
	// Graceful process-group stop on every exit path. Carry-forward 2: on a
	// handshake timeout this is what unblocks the blocked ReadHandshake reader
	// (killing the group closes the plugin's stdout).
	defer h.Stop(s.cfg.StopGrace)

	// Handshake within the timeout.
	select {
	case <-ctx.Done():
		return lifeCancelled, reachedRunning
	case hr := <-h.Handshake():
		if hr.Err != nil {
			s.state("unhealthy", "handshake read: "+hr.Err.Error())
			return lifeRejected, reachedRunning
		}
		hs, err := ReadHandshake(bytes.NewReader(append(hr.Line, '\n')))
		if err != nil {
			s.state("unhealthy", "handshake parse: "+err.Error())
			return lifeRejected, reachedRunning
		}
		if err := hs.Validate(s.cfg.Manifest); err != nil {
			s.state("unhealthy", "handshake invalid: "+err.Error())
			return lifeRejected, reachedRunning
		}
		// Carry-forward 4: the daemon owns the socket. Never trust the socket
		// the plugin echoes; the Client always dials the daemon-assigned path.
		if hs.Socket != "" && hs.Socket != s.cfg.Spec.Socket {
			s.cfg.Log.Warn("plugin announced a different socket; ignoring it",
				"plugin", s.cfg.Name, "announced", hs.Socket, "assigned", s.cfg.Spec.Socket)
		}
		// Carry-forward 3: the installed manifest's types are authoritative for
		// supervision/capability decisions. A mismatched handshake is a warning,
		// never a capability source and (here) never a rejection.
		if !sameTypeSet(hs.Types, s.cfg.Manifest.Types) {
			s.cfg.Log.Warn("plugin handshake types differ from manifest; using manifest",
				"plugin", s.cfg.Name, "announced", hs.Types, "manifest", s.cfg.Manifest.Types)
		}
	case <-s.cfg.After(s.cfg.HandshakeTimeout):
		s.state("unhealthy", "handshake timeout")
		return lifeRejected, reachedRunning
	}

	// Confirm the socket is listenable before declaring the plugin up.
	if err := s.probe(ctx); err != nil {
		s.state("unhealthy", "initial health: "+err.Error())
		return lifeExited, reachedRunning
	}
	s.state("running", "")
	reachedRunning = true

	// Health loop.
	fails := 0
	for {
		select {
		case <-ctx.Done():
			return lifeCancelled, reachedRunning
		case <-h.Done():
			s.state("unhealthy", "process exited")
			return lifeExited, reachedRunning
		case <-s.cfg.After(s.cfg.HealthInterval):
			if err := s.probe(ctx); err != nil {
				fails++
				if fails >= healthFailThreshold {
					s.state("unhealthy", "health failed x"+fmt.Sprint(fails))
					return lifeUnhealthy, reachedRunning
				}
			} else {
				fails = 0
			}
		}
	}
}

func (s *Supervisor) probe(ctx context.Context) error {
	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.cfg.Client.Health(pctx)
}

// sameTypeSet reports whether a and b contain the same set of types (order and
// duplicates ignored).
func sameTypeSet(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, t := range a {
		set[t] = true
	}
	other := make(map[string]bool, len(b))
	for _, t := range b {
		other[t] = true
	}
	if len(set) != len(other) {
		return false
	}
	for t := range set {
		if !other[t] {
			return false
		}
	}
	return true
}
