package plugins

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var discardLog = slog.New(slog.NewTextHandler(io.Discard, nil))

// Carry-forward 1: symlink-safe exec. ResolveExec must refuse an exec that
// resolves outside the plugin dir (e.g. a symlink to /bin/sh).
func TestResolveExecContainedOK(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "run")
	if err := os.WriteFile(bin, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveExec(dir, "run")
	if err != nil {
		t.Fatalf("ResolveExec refused a contained exec: %v", err)
	}
	real, _ := filepath.EvalSymlinks(bin)
	if got != real {
		t.Fatalf("ResolveExec = %q, want %q", got, real)
	}
}

func TestResolveExecSymlinkEscapeRefused(t *testing.T) {
	dir := t.TempDir()
	evil := filepath.Join(dir, "evil")
	if err := os.Symlink("/bin/sh", evil); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExec(dir, "evil"); err == nil {
		t.Fatal("ResolveExec accepted a symlink escaping the plugin dir")
	}
}

func TestResolveExecMissingRefused(t *testing.T) {
	if _, err := ResolveExec(t.TempDir(), "nope"); err == nil {
		t.Fatal("ResolveExec accepted a missing exec")
	}
}

// silentRunner returns handles that never deliver a handshake, forcing the
// supervisor down its handshake-timeout path.
type silentRunner struct {
	mu      sync.Mutex
	handles []*fakeHandle
	starts  int
}

func (r *silentRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	h := &fakeHandle{hs: make(chan HandshakeResult), done: make(chan struct{})}
	r.mu.Lock()
	r.handles = append(r.handles, h)
	r.starts++
	r.mu.Unlock()
	return h, nil
}

func (r *silentRunner) totalStops() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, h := range r.handles {
		h.mu.Lock()
		n += h.stops
		h.mu.Unlock()
	}
	return n
}

// Carry-forward 2 (state machine): a silent handshake must time out, be
// reported unhealthy, and the process handle must be Stop()'d (killed).
func TestSupervisorHandshakeTimeoutStopsHandle(t *testing.T) {
	m, _ := ParseManifest([]byte(goodManifest))
	r := &silentRunner{}
	details := make(chan string, 64)
	fireNow := func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	sup := NewSupervisor(SupervisorConfig{
		Name: "echo", Manifest: m,
		Spec:             SpawnSpec{Name: "echo", Socket: "/nonexistent.sock"},
		Runner:           r,
		Client:           NewClient("/nonexistent.sock"),
		OnState:          func(_ string, h map[string]any) { details <- h["detail"].(string) },
		After:            fireNow,
		Log:              discardLog,
		HandshakeTimeout: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sup.Run(ctx); close(done) }()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case d := <-details:
			if strings.Contains(d, "handshake timeout") {
				goto sawTimeout
			}
		case <-deadline:
			t.Fatal("no handshake-timeout state emitted")
		}
	}
sawTimeout:
	cancel()
	// Supervisor.state calls OnState synchronously, so the OnState above blocks
	// as soon as details is full — and Run emits more states on its way out
	// ("stopped"/"daemon shutdown"), so it needs a reader even after cancel().
	// Keep consuming until Run has actually returned. The deadline is created
	// ONCE, outside the loop: the restart spin is microsecond-fast, so a
	// time.After() rebuilt per iteration would never fire and the fast, named
	// failure below would decay back into a whole-package timeout panic.
	returned := time.After(5 * time.Second)
drain:
	for {
		select {
		case <-done:
			break drain
		case <-details:
		case <-returned:
			t.Fatal("Run did not return within 5s of ctx cancel")
		}
	}
	if r.totalStops() == 0 {
		t.Fatal("handle was not stopped on handshake timeout")
	}
}

// Carry-forward 2 (real process): the real execRunner spawns a stubborn silent
// plugin that ignores SIGTERM; Stop must escalate to SIGKILL on the process
// group and reap it, so the blocked handshake reader unblocks.
func TestExecRunnerKillsSilentProcessGroup(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "silent.sock")
	spec := SpawnSpec{
		Name: "silent", Exec: os.Args[0], Dir: t.TempDir(), Socket: sock,
		Env: append(os.Environ(), "TARIBOY_FAKE_PLUGIN=silent"),
	}
	h, err := NewExecRunner().Start(spec, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	// No handshake ever arrives.
	select {
	case hr := <-h.Handshake():
		t.Fatalf("unexpected handshake from silent plugin: %q err=%v", hr.Line, hr.Err)
	case <-time.After(300 * time.Millisecond):
	}
	pid := h.Pid()
	if err := h.Stop(150 * time.Millisecond); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-h.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("silent process not reaped after Stop (SIGKILL escalation failed)")
	}
	// The process (group leader) must be gone.
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("pid %d still alive after Stop", pid)
	}
}

// Carry-forward 3: manifest types are authoritative. A handshake announcing
// types inconsistent with the manifest must NOT be rejected — the supervisor
// reaches running and uses the manifest for capability decisions.
func TestSupervisorHandshakeTypesMismatchStillRuns(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "p.sock")
	status := 200
	healthServer(t, sock, &status)
	m, _ := ParseManifest([]byte(goodManifest)) // types: channel-source, channel-sink
	// Handshake announces a completely different (and even unknown) type set.
	line := `{"name":"echo","version":"0.1.0","types":["tool","harness"],"protocol_version":1,"socket":"/attacker/owned.sock"}`
	states := make(chan string, 8)
	sup := NewSupervisor(SupervisorConfig{
		Name: "echo", Manifest: m,
		Spec:    SpawnSpec{Name: "echo", Socket: sock}, // daemon-assigned socket, not the announced one
		Runner:  &fakeRunner{line: line},
		Client:  NewClient(sock),
		OnState: func(state string, _ map[string]any) { states <- state },
		After:   func(time.Duration) <-chan time.Time { return time.After(time.Millisecond) },
		Log:     discardLog,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { sup.Run(ctx); close(done) }()
	if got := <-states; got != "running" {
		t.Fatalf("state = %q, want running (manifest types are authoritative)", got)
	}
	cancel()
	<-done
}

// Carry-forward 5: the per-life token must be revoked on every exit path
// (handshake reject, exit, unhealthy, cancel).
func TestSupervisorRevokesTokenOnEveryExit(t *testing.T) {
	m, _ := ParseManifest([]byte(goodManifest))
	reg := NewTokenRegistry(nil)
	var minted []string
	var mintedEnv []string
	r := &silentRunner{}
	// Capture the env the runner receives so we can assert the token is injected.
	capturing := &capturingRunner{inner: r, envs: &mintedEnv}
	lifeCompleted := make(chan struct{}, 1)
	fireNow := func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	var mu sync.Mutex
	sup := NewSupervisor(SupervisorConfig{
		Name: "echo", Manifest: m,
		Spec:   SpawnSpec{Name: "echo", Socket: "/nonexistent.sock"},
		Runner: capturing,
		Client: NewClient("/nonexistent.sock"),
		MintToken: func() (string, error) {
			tok, err := reg.Mint(Identity{Name: "echo"})
			mu.Lock()
			minted = append(minted, tok)
			mu.Unlock()
			return tok, err
		},
		RevokeToken: reg.Revoke,
		OnState: func(_ string, h map[string]any) {
			if !strings.Contains(h["detail"].(string), "handshake timeout") {
				return
			}
			select {
			case lifeCompleted <- struct{}{}:
			default:
			}
		},
		After:            fireNow,
		Log:              discardLog,
		HandshakeTimeout: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sup.Run(ctx); close(done) }()
	// Wait for at least one life to complete (a handshake timeout).
	deadline := time.After(3 * time.Second)
	select {
	case <-lifeCompleted:
	case <-deadline:
		t.Fatal("no life completed")
	}
	cancel()
	<-done
	if reg.Count() != 0 {
		t.Fatalf("token registry not empty after stop: %d tokens leaked", reg.Count())
	}
	mu.Lock()
	n := len(minted)
	mu.Unlock()
	if n == 0 {
		t.Fatal("no token was minted")
	}
	// The injected token must have reached the runner env.
	found := false
	for _, e := range mintedEnv {
		if strings.HasPrefix(e, "TARIBOY_PLUGIN_TOKEN=") {
			found = true
		}
	}
	if !found {
		t.Fatal("minted token was not injected into the plugin env")
	}
}

// capturingRunner records the env of the last spawn spec and delegates.
type capturingRunner struct {
	inner Runner
	mu    sync.Mutex
	envs  *[]string
}

func (c *capturingRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	c.mu.Lock()
	*c.envs = append(*c.envs, spec.Env...)
	c.mu.Unlock()
	return c.inner.Start(spec, logw)
}
