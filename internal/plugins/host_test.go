package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
)

func newHost(t *testing.T, runner Runner) (*Host, *bus.Bus, *Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	clock := func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) }
	b := bus.New(s, clock)
	ps := NewStore(s, clock)
	h := NewHost(HostConfig{
		Store: ps, Bus: b, Tokens: NewTokenRegistry(nil),
		PluginsDir: filepath.Join(t.TempDir(), "plugins"), DaemonSocket: "/tmp/d.sock",
		Runner: runner, Clock: clock,
		After: func(time.Duration) <-chan time.Time { return time.After(time.Millisecond) },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return h, b, ps
}

func TestPluginActionNotRunning(t *testing.T) {
	h, _, _ := newHost(t, nil)
	if _, err := h.PluginAction("ghost", map[string]any{"action": "bind"}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("PluginAction: want ErrNotRunning, got %v", err)
	}
	if _, err := h.PluginRoutes("ghost"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("PluginRoutes: want ErrNotRunning, got %v", err)
	}
}

func TestHostContributionsReturnsEnabledInstalledManifests(t *testing.T) {
	h, _, ps := newHost(t, nil)
	install := func(name string, enabled bool) {
		dir := h.versionDir(name, "1.0.0")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		manifest := fmt.Sprintf(`{
  "name":%q,"version":"1.0.0","protocol_version":1,
  "types":["channel-source"],"exec":"run.sh",
  "channels":{"publish":["chat:%s:*"] ,"subscribe":[]},
  "operator_commands":[{"path":"status","summary":"Show status","action":"status"}]
}`, name, name)
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ps.Upsert(Record{Name: name, Version: "1.0.0", ProtocolVersion: 1, Types: []string{"channel-source"}, Exec: "run.sh", Enabled: enabled}); err != nil {
			t.Fatal(err)
		}
		if err := ps.SetActiveVersion(name, "1.0.0"); err != nil {
			t.Fatal(err)
		}
	}
	install("telegram", true)
	install("disabled", false)

	got, err := h.Contributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "telegram" || got[0].Commands[0].Path != "status" {
		t.Fatalf("contributions = %+v", got)
	}
}

// writeExec creates the plugin dir and a real same-dir exec file so the
// symlink-safe ResolveExec containment gate (wired in Start) resolves it.
// sampleRecord uses Exec="echo.py".
func writeExec(t *testing.T, h *Host, name string) {
	t.Helper()
	dir := filepath.Join(h.cfg.PluginsDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "echo.py"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func waitRunning(t *testing.T, h *Host, name string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		list, _ := h.List()
		for _, e := range list {
			if e["name"] == name && e["state"] == "running" {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("plugin %s never running: %+v", name, list)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHostStartReachesRunningAndEmitsEvent(t *testing.T) {
	h, b, ps := newHost(t, nil) // runner set per-plugin below via a health server + fake
	rec := sampleRecord("echo")
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	// Serve /health on the host-assigned socket, and inject a fakeRunner whose
	// handshake announces the same name.
	sock := h.SocketPath("echo")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, h, "echo")
	status := 200
	healthServer(t, sock, &status)
	h.cfg.Runner = &fakeRunner{line: `{"name":"echo","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"` + sock + `"}`}

	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Poll List until running.
	deadline := time.Now().Add(2 * time.Second)
	for {
		list, _ := h.List()
		if len(list) == 1 && list[0]["state"] == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("plugin never running: %+v", list)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// A system:plugins event was published. onState flips the live state to
	// "running" (observed by the poll loop above) BEFORE its Bus.Publish call
	// completes, so under heavy -race scheduling a single Tail read right after
	// the state flip can race the publish and miss it. Poll with a bounded
	// deadline instead of reading once, to close that happens-before gap
	// deterministically without weakening the assertion.
	var msgs []bus.Message
	evDeadline := time.Now().Add(2 * time.Second)
	for {
		msgs, _ = b.Tail("system:plugins", 10)
		if len(msgs) > 0 {
			break
		}
		if time.Now().After(evDeadline) {
			t.Fatal("no system:plugins event")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// A token was minted for the plugin.
	if h.cfg.Tokens.Count() != 1 {
		t.Fatalf("token count = %d", h.cfg.Tokens.Count())
	}
	h.StopAll()
	if h.cfg.Tokens.Count() != 0 {
		t.Fatalf("token not revoked on stop: %d", h.cfg.Tokens.Count())
	}
}

// envRunner records the Spec.Env of the last spawned life and behaves like a
// healthy fakeRunner so we can assert the daemon-assigned socket/token env.
type envRunner struct {
	line string
	mu   sync.Mutex
	env  []string
}

func (r *envRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	r.mu.Lock()
	r.env = append([]string(nil), spec.Env...)
	r.mu.Unlock()
	h := &fakeHandle{hs: make(chan HandshakeResult, 1), done: make(chan struct{})}
	h.hs <- HandshakeResult{Line: []byte(r.line)}
	return h, nil
}

func (r *envRunner) lastEnv() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.env...)
}

func hasEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func hasEnvKey(env []string, key string) bool {
	for _, e := range env {
		if len(e) > len(key) && e[:len(key)+1] == key+"=" {
			return true
		}
	}
	return false
}

func TestHostInjectsSocketAndTokenEnv(t *testing.T) {
	h, _, ps := newHost(t, nil)
	rec := sampleRecord("echo")
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("echo")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, h, "echo")
	status := 200
	healthServer(t, sock, &status)
	er := &envRunner{line: `{"name":"echo","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"` + sock + `"}`}
	h.cfg.Runner = er

	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "echo")

	env := er.lastEnv()
	// (a) daemon-assigned socket + workdir + daemon socket.
	if !hasEnv(env, "TARIBOY_PLUGIN_SOCKET="+sock) {
		t.Fatalf("socket env not injected: %v", env)
	}
	if !hasEnv(env, "TARIBOY_PLUGIN_NAME=echo") {
		t.Fatalf("name env not injected: %v", env)
	}
	if !hasEnv(env, "TARIBOY_PLUGIN_WORKDIR="+filepath.Join(h.cfg.PluginsDir, "echo", "workdir")) {
		t.Fatalf("workdir env not injected: %v", env)
	}
	if !hasEnv(env, "TARIBOY_DAEMON_SOCKET=/tmp/d.sock") {
		t.Fatalf("daemon socket env not injected: %v", env)
	}
	// (b) per-life token injected by the supervisor seam.
	if !hasEnvKey(env, "TARIBOY_PLUGIN_TOKEN") {
		t.Fatalf("token env not injected: %v", env)
	}
	h.StopAll()
}

// TestHostScrubsRealProviderKeys proves the real upstream provider keys the
// daemon holds (to forward to its own proxy) are STRUCTURALLY stripped from the
// plugin spawn env, so a plugin process can never read the real key and bypass
// the accounted proxy. A benign inherited var (PATH) must survive. This mirrors
// the identical scrub the agent-loop runner applies (internal/loop/runner.go).
func TestHostScrubsRealProviderKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "real-anthropic-secret")
	t.Setenv("OPENAI_API_KEY", "real-openai-secret")

	h, _, ps := newHost(t, nil)
	rec := sampleRecord("echo")
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("echo")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, h, "echo")
	status := 200
	healthServer(t, sock, &status)
	er := &envRunner{line: `{"name":"echo","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"` + sock + `"}`}
	h.cfg.Runner = er

	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "echo")

	env := er.lastEnv()
	if hasEnvKey(env, "ANTHROPIC_API_KEY") {
		t.Fatalf("real ANTHROPIC_API_KEY leaked into plugin env: %v", env)
	}
	if hasEnvKey(env, "OPENAI_API_KEY") {
		t.Fatalf("real OPENAI_API_KEY leaked into plugin env: %v", env)
	}
	// A benign inherited var must survive the scrub.
	if !hasEnvKey(env, "PATH") {
		t.Fatalf("PATH was stripped by the scrub (should survive): %v", env)
	}
	h.StopAll()
}

func TestHostStopAllDrainsBeforeClose(t *testing.T) {
	h, _, ps := newHost(t, nil)
	for _, name := range []string{"a", "b", "c"} {
		rec := sampleRecord(name)
		if err := ps.Upsert(rec); err != nil {
			t.Fatal(err)
		}
		sock := h.SocketPath(name)
		if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
			t.Fatal(err)
		}
		writeExec(t, h, name)
		status := 200
		healthServer(t, sock, &status)
	}
	// Every plugin announces its own name; the fakeRunner ignores name but the
	// handshake must match, so start each with its own runner.
	h.cfg.Runner = &nameRunner{}

	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		waitRunning(t, h, name)
	}
	if h.cfg.Tokens.Count() != 3 {
		t.Fatalf("expected 3 tokens, got %d", h.cfg.Tokens.Count())
	}
	// StopAll must fully drain (wg.Wait) so no goroutine touches the store after.
	done := make(chan struct{})
	go func() { h.StopAll(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("StopAll did not drain within 5s")
	}
	if h.cfg.Tokens.Count() != 0 {
		t.Fatalf("tokens not all revoked after drain: %d", h.cfg.Tokens.Count())
	}
}

// nameRunner returns a handshake matching each spawned plugin's own name (read
// from SpawnSpec.Name), so several differently-named plugins can share it.
type nameRunner struct{}

func (nameRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	line := `{"name":"` + spec.Name + `","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"` + spec.Socket + `"}`
	h := &fakeHandle{hs: make(chan HandshakeResult, 1), done: make(chan struct{})}
	h.hs <- HandshakeResult{Line: []byte(line)}
	return h, nil
}

// countingNameRunner is a nameRunner that records how many processes it has
// spawned per plugin name, so a test can prove a restart actually re-spawned the
// target's process (and only the target's).
type countingNameRunner struct {
	mu     sync.Mutex
	spawns map[string]int
}

type blockingStopHandle struct {
	*fakeHandle
	stopEntered chan struct{}
	releaseStop chan struct{}
}

func (h *blockingStopHandle) Stop(grace time.Duration) error {
	select {
	case <-h.stopEntered:
	default:
		close(h.stopEntered)
	}
	<-h.releaseStop
	return h.fakeHandle.Stop(grace)
}

type blockingUpgradeRunner struct {
	mu          sync.Mutex
	starts      int
	stopEntered chan struct{}
	releaseStop chan struct{}
}

func (r *blockingUpgradeRunner) Start(spec SpawnSpec, _ io.Writer) (ProcHandle, error) {
	r.mu.Lock()
	r.starts++
	first := r.starts == 1
	r.mu.Unlock()
	line := `{"name":"` + spec.Name + `","version":"0.1.0","types":["tool"],"protocol_version":1,"socket":"` + spec.Socket + `"}`
	h := &fakeHandle{hs: make(chan HandshakeResult, 1), done: make(chan struct{})}
	h.hs <- HandshakeResult{Line: []byte(line)}
	if first {
		return &blockingStopHandle{fakeHandle: h, stopEntered: r.stopEntered, releaseStop: r.releaseStop}, nil
	}
	return h, nil
}

func (r *blockingUpgradeRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts
}

func (r *countingNameRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	r.mu.Lock()
	if r.spawns == nil {
		r.spawns = map[string]int{}
	}
	r.spawns[spec.Name]++
	r.mu.Unlock()
	line := `{"name":"` + spec.Name + `","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"` + spec.Socket + `"}`
	h := &fakeHandle{hs: make(chan HandshakeResult, 1), done: make(chan struct{})}
	h.hs <- HandshakeResult{Line: []byte(line)}
	return h, nil
}

func (r *countingNameRunner) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spawns[name]
}

// TestHostRestart proves a single named plugin is cycled in place — its process
// re-spawned, a fresh per-life token minted — while a sibling plugin is left
// untouched (not re-spawned, still running). It also proves the guards: an
// unknown plugin is ErrNotFound and a disabled one is refused (not silently run).
func TestHostRestart(t *testing.T) {
	h, _, ps := newHost(t, nil)
	runner := &countingNameRunner{}
	h.cfg.Runner = runner
	h.baseCtx = context.Background()

	for _, name := range []string{"a", "b"} {
		if err := ps.Upsert(sampleRecord(name)); err != nil {
			t.Fatal(err)
		}
		sock := h.SocketPath(name)
		if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
			t.Fatal(err)
		}
		writeExec(t, h, name)
		status := 200
		healthServer(t, sock, &status)
	}
	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "a")
	waitRunning(t, h, "b")
	if runner.count("a") != 1 || runner.count("b") != 1 {
		t.Fatalf("initial spawns a=%d b=%d, want 1/1", runner.count("a"), runner.count("b"))
	}
	if h.cfg.Tokens.Count() != 2 {
		t.Fatalf("expected 2 live tokens before restart, got %d", h.cfg.Tokens.Count())
	}

	// Restart only "a": its process is re-spawned; "b" is never re-spawned.
	if err := h.Restart("a"); err != nil {
		t.Fatalf("Restart(a): %v", err)
	}
	waitRunning(t, h, "a")
	if runner.count("a") != 2 {
		t.Fatalf("a spawns = %d after restart, want 2 (re-spawned)", runner.count("a"))
	}
	if runner.count("b") != 1 {
		t.Fatalf("b spawns = %d after restarting a, want 1 (untouched)", runner.count("b"))
	}
	// "b" kept running throughout, and exactly one live token per plugin remains
	// (the old "a" life's token was revoked on drain, a fresh one minted).
	waitRunning(t, h, "b")
	if h.cfg.Tokens.Count() != 2 {
		t.Fatalf("expected 2 live tokens after restart, got %d", h.cfg.Tokens.Count())
	}

	// Guards: unknown -> ErrNotFound; disabled -> refused.
	if err := h.Restart("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Restart(ghost) = %v, want ErrNotFound", err)
	}
	disabled := sampleRecord("c")
	disabled.Enabled = false
	if err := ps.Upsert(disabled); err != nil {
		t.Fatal(err)
	}
	if err := h.Restart("c"); err == nil {
		t.Fatal("Restart of a disabled plugin must be refused")
	}

	h.StopAll()
	if h.cfg.Tokens.Count() != 0 {
		t.Fatalf("tokens leaked after drain: %d", h.cfg.Tokens.Count())
	}
}

func TestHostConcurrentStartStop(t *testing.T) {
	h, _, ps := newHost(t, nil)
	h.cfg.Runner = &nameRunner{}
	names := []string{"p0", "p1", "p2", "p3", "p4", "p5"}
	for _, n := range names {
		if err := ps.Upsert(sampleRecord(n)); err != nil {
			t.Fatal(err)
		}
		sock := h.SocketPath(n)
		if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
			t.Fatal(err)
		}
		writeExec(t, h, n)
		status := 200
		healthServer(t, sock, &status)
	}
	h.baseCtx = context.Background()

	// Hammer Start/Stop/List concurrently: the registry map must be race-free and
	// a plugin failure must never crash the host.
	var wg sync.WaitGroup
	for _, n := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_ = h.Start(sampleRecord(name))
				_, _ = h.List()
				_ = h.Stop(name)
			}
		}(n)
	}
	wg.Wait()
	h.StopAll()
}

func TestHostInstallRemove(t *testing.T) {
	h, _, ps := newHost(t, nil)
	h.cfg.Runner = &nameRunner{}

	// Build a minimal source plugin tree with a valid manifest + exec.
	src := t.TempDir()
	manifest := `{"name":"widget","version":"1.0.0","protocol_version":1,"types":["channel-sink"],"exec":"run.sh","channels":{"publish":["chat:*"],"subscribe":["chat:in"]}}`
	if err := os.WriteFile(filepath.Join(src, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := h.Install(src)
	if err != nil {
		t.Fatal(err)
	}
	if info["name"] != "widget" || info["installed"] != true {
		t.Fatalf("install info wrong: %+v", info)
	}
	// Install copies the tree + starts the plugin; stand up the health server on
	// the daemon-assigned socket now (the supervisor retries fast until it dials).
	sock := h.SocketPath("widget")
	status := 200
	healthServer(t, sock, &status)
	// The record was persisted enabled and the tree copied.
	got, ok, err := ps.Get("widget")
	if err != nil || !ok || !got.Enabled {
		t.Fatalf("widget not persisted enabled: ok=%v err=%v rec=%+v", ok, err, got)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.PluginsDir, "widget", "1.0.0", "run.sh")); err != nil {
		t.Fatalf("exec not copied: %v", err)
	}
	waitRunning(t, h, "widget")

	if err := h.Remove("widget"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ps.Get("widget"); ok {
		t.Fatal("widget row not deleted")
	}
	if _, err := os.Stat(filepath.Join(h.cfg.PluginsDir, "widget")); !os.IsNotExist(err) {
		t.Fatalf("widget dir not removed: %v", err)
	}
	h.StopAll()
	if h.cfg.Tokens.Count() != 0 {
		t.Fatalf("tokens leaked after drain: %d", h.cfg.Tokens.Count())
	}
}

func TestHostInstallKeepsVersionsSideBySideAndActivatesLatest(t *testing.T) {
	h, _, ps := newHost(t, nil)
	h.cfg.Runner = &nameRunner{}
	install := func(version, script string) string {
		dir := t.TempDir()
		manifest := fmt.Sprintf(`{"name":"versioned","version":%q,"protocol_version":1,"types":["tool"],"exec":"run.sh","channels":{"publish":[],"subscribe":[]}}`, version)
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Install(dir); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	install("1.0.0", "one")
	second := install("2.0.0", "two")
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if _, err := os.Stat(filepath.Join(h.cfg.PluginsDir, "versioned", version, "plugin.json")); err != nil {
			t.Fatalf("version %s missing: %v", version, err)
		}
	}
	if active, ok, err := ps.ActiveVersion("versioned"); err != nil || !ok || active != "2.0.0" {
		t.Fatalf("active = %q,%v,%v", active, ok, err)
	}
	if _, err := h.Install(second); err != nil {
		t.Fatalf("identical reinstall: %v", err)
	}
	if err := os.WriteFile(filepath.Join(second, "run.sh"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Install(second); err == nil {
		t.Fatal("different bytes accepted for immutable version")
	}
	h.StopAll()
}

func TestHostInstallWaitsForPreviousPluginToDrain(t *testing.T) {
	runner := &blockingUpgradeRunner{stopEntered: make(chan struct{}), releaseStop: make(chan struct{})}
	h, _, ps := newHost(t, runner)
	pluginSource := func(version string) string {
		dir := t.TempDir()
		manifest := fmt.Sprintf(`{"name":"single","version":%q,"protocol_version":1,"types":["tool"],"exec":"run.sh","channels":{"publish":[],"subscribe":[]}}`, version)
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	sock := h.SocketPath("single")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	status := 200
	healthServer(t, sock, &status)
	if _, err := h.Install(pluginSource("1.0.0")); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "single")

	second := pluginSource("2.0.0")
	done := make(chan error, 1)
	go func() {
		_, err := h.Install(second)
		done <- err
	}()
	select {
	case <-runner.stopEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("previous plugin was not asked to stop")
	}
	if got := runner.count(); got != 1 {
		t.Fatalf("new plugin started before previous drain: starts=%d", got)
	}
	if active, ok, err := ps.ActiveVersion("single"); err != nil || !ok || active != "1.0.0" {
		t.Fatalf("active version changed before new process could start: %q,%v,%v", active, ok, err)
	}
	close(runner.releaseStop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "single")
	if got := runner.count(); got != 2 {
		t.Fatalf("starts after drain=%d, want 2", got)
	}
	h.StopAll()
}

func TestHostInstallDrainTimeoutRestartsRolledBackActiveVersion(t *testing.T) {
	runner := &blockingUpgradeRunner{stopEntered: make(chan struct{}), releaseStop: make(chan struct{})}
	h, _, ps := newHost(t, runner)
	sock := h.SocketPath("single")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	status := 200
	healthServer(t, sock, &status)
	pluginSource := func(version string) string {
		dir := t.TempDir()
		manifest := fmt.Sprintf(`{"name":"single","version":%q,"protocol_version":1,"types":["tool"],"exec":"run.sh","channels":{"publish":[],"subscribe":[]}}`, version)
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	if _, err := h.Install(pluginSource("1.0.0")); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "single")
	timeout := make(chan time.Time, 1)
	timeout <- time.Now()
	h.cfg.DrainAfter = func(time.Duration) <-chan time.Time { return timeout }
	second := pluginSource("2.0.0")
	done := make(chan error, 1)
	go func() {
		_, err := h.Install(second)
		done <- err
	}()
	select {
	case <-runner.stopEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("previous plugin was not asked to stop")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("upgrade succeeded after drain timeout")
		}
	case <-time.After(2 * time.Second):
		close(runner.releaseStop)
		t.Fatal("install ignored configured drain timeout")
	}
	close(runner.releaseStop)
	deadline := time.Now().Add(2 * time.Second)
	for runner.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if runner.count() != 2 {
		t.Fatalf("rolled-back active process was not restarted: starts=%d", runner.count())
	}
	if active, ok, err := ps.ActiveVersion("single"); err != nil || !ok || active != "1.0.0" {
		t.Fatalf("active after rollback = %q,%v,%v", active, ok, err)
	}
	h.StopAll()
}

func TestStartAllMigratesLegacyPackageIntoVersionDirectory(t *testing.T) {
	h, _, ps := newHost(t, &nameRunner{})
	rec := sampleRecord("legacy")
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(h.cfg.PluginsDir, rec.Name)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"name":%q,"version":%q,"protocol_version":1,"types":["tool"],"exec":"echo.py","channels":{"publish":[],"subscribe":[]}}`, rec.Name, rec.Version)
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "echo.py"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := h.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"plugin.json", "echo.py"} {
		if _, err := os.Stat(filepath.Join(root, rec.Version, name)); err != nil {
			t.Fatalf("migrated %s: %v", name, err)
		}
	}
	active, err := os.ReadFile(filepath.Join(root, "active-version"))
	if err != nil || string(active) != rec.Version+"\n" {
		t.Fatalf("active-version = %q, %v", active, err)
	}
	h.StopAll()
}

func TestHostInstallRejectsNonRegularFile(t *testing.T) {
	h, _, _ := newHost(t, &nameRunner{})
	src := t.TempDir()
	manifest := `{"name":"fifo-plugin","version":"1.0.0","protocol_version":1,"types":["tool"],"exec":"run.sh","channels":{"publish":[],"subscribe":[]}}`
	if err := os.WriteFile(filepath.Join(src, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(src, "payload.fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Install(src); err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("Install error = %v", err)
	}
}

func TestHostInstallFailureRestoresPreviousActiveVersion(t *testing.T) {
	h, _, ps := newHost(t, &nameRunner{})
	pluginSource := func(version string) string {
		dir := t.TempDir()
		manifest := fmt.Sprintf(`{"name":"rollback","version":%q,"protocol_version":1,"types":["tool"],"exec":"run.sh","channels":{"publish":[],"subscribe":[]}}`, version)
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	if _, err := h.Install(pluginSource("1.0.0")); err != nil {
		t.Fatal(err)
	}
	h.StopAll()
	logs := filepath.Join(h.cfg.PluginsDir, "rollback", "logs")
	if err := os.RemoveAll(logs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logs, []byte("blocks log directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Install(pluginSource("2.0.0")); err == nil {
		t.Fatal("install unexpectedly succeeded")
	}
	rec, ok, err := ps.Get("rollback")
	if err != nil || !ok || rec.Version != "1.0.0" {
		t.Fatalf("record = %+v, %v, %v", rec, ok, err)
	}
	active, ok, err := ps.ActiveVersion("rollback")
	if err != nil || !ok || active != "1.0.0" {
		t.Fatalf("active = %q, %v, %v", active, ok, err)
	}
	marker, err := os.ReadFile(filepath.Join(h.cfg.PluginsDir, "rollback", "active-version"))
	if err != nil || string(marker) != "1.0.0\n" {
		t.Fatalf("marker = %q, %v", marker, err)
	}
}

// TestStartRefusesSymlinkEscapeExec proves the CF-1 containment guarantee is
// wired at spawn: a plugin whose exec is a symlink to a file OUTSIDE its dir is
// refused (no process, state="error", not in the running map), while a normal
// same-dir exec still starts.
func TestStartRefusesSymlinkEscapeExec(t *testing.T) {
	h, b, ps := newHost(t, nil)
	h.cfg.Runner = &nameRunner{}
	h.baseCtx = context.Background()

	// evil: exec is a symlink pointing outside the plugin dir.
	evil := sampleRecord("evil")
	evil.Exec = "ok.py"
	if err := ps.Upsert(evil); err != nil {
		t.Fatal(err)
	}
	evilDir := filepath.Join(h.cfg.PluginsDir, "evil")
	if err := os.MkdirAll(evilDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.sh")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(evilDir, "ok.py")); err != nil {
		t.Fatal(err)
	}

	if err := h.Start(evil); err == nil {
		t.Fatal("Start must refuse a symlink-escape exec")
	}
	// No supervisor spawned: no token minted.
	if h.cfg.Tokens.Count() != 0 {
		t.Fatalf("token minted for a refused plugin: %d", h.cfg.Tokens.Count())
	}
	// Not left in the running map.
	h.mu.Lock()
	_, stillRunning := h.running["evil"]
	h.mu.Unlock()
	if stillRunning {
		t.Fatal("refused plugin left in running map")
	}
	// State persisted as "error".
	list, _ := h.List()
	for _, e := range list {
		if e["name"] == "evil" && e["state"] != "error" {
			t.Fatalf("evil state = %v, want error", e["state"])
		}
	}
	// The error was announced on system:plugins.
	if msgs, _ := b.Tail("system:plugins", 10); len(msgs) == 0 {
		t.Fatal("no system:plugins event for the refused plugin")
	}

	// A normal same-dir exec still starts.
	good := sampleRecord("good")
	if err := ps.Upsert(good); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("good")
	writeExec(t, h, "good")
	status := 200
	healthServer(t, sock, &status)
	if err := h.Start(good); err != nil {
		t.Fatalf("normal same-dir exec must start: %v", err)
	}
	waitRunning(t, h, "good")
	h.StopAll()
	if h.cfg.Tokens.Count() != 0 {
		t.Fatalf("tokens leaked after drain: %d", h.cfg.Tokens.Count())
	}
}

// TestHostInstallRejectsSymlinkInTree proves the install-time defense layer
// (carry-forward from Task 2/7 review): a plugin source tree containing a
// symlink ANYWHERE (not just at the top level) must be refused whole, whether
// it points outside the tree or even at an in-tree file — install is a
// whole-tree reject, not a per-symlink resolve. Nothing must land in
// PluginsDir and no supervisor must be started.
func TestHostInstallRejectsSymlinkInTree(t *testing.T) {
	h, _, ps := newHost(t, nil)
	h.cfg.Runner = &nameRunner{}

	src := t.TempDir()
	manifest := `{"name":"sneaky","version":"1.0.0","protocol_version":1,"types":["channel-sink"],"exec":"sub/run.sh","channels":{"publish":["chat:*"],"subscribe":["chat:in"]}}`
	if err := os.WriteFile(filepath.Join(src, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	// The symlink is nested inside a subdirectory, not at the top level, so a
	// shallow (top-level-only) check would miss it.
	sub := filepath.Join(src, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.sh")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(sub, "run.sh")); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Install(src); err == nil {
		t.Fatal("Install must refuse a source tree containing a symlink")
	}
	// Nothing was recorded or copied.
	if _, ok, _ := ps.Get("sneaky"); ok {
		t.Fatal("sneaky must not be persisted")
	}
	if _, err := os.Stat(filepath.Join(h.cfg.PluginsDir, "sneaky")); !os.IsNotExist(err) {
		t.Fatalf("sneaky dir must not exist: %v", err)
	}
}

// TestHostRejectsTraversalName proves the primitive-level guard: every
// name-taking host method refuses a traversing/invalid name BEFORE it touches
// the filesystem or store, so a sibling file OUTSIDE PluginsDir survives a
// `Remove("../victim")`, and Logs/Inspect never read outside the dir. A valid
// name is still accepted (reaches not_found / real behaviour, not the name
// error).
func TestHostRejectsTraversalName(t *testing.T) {
	h, _, _ := newHost(t, nil)

	// A sibling directory + file OUTSIDE PluginsDir. "../victim" from inside
	// PluginsDir would resolve here if the guard were missing.
	base := filepath.Dir(h.cfg.PluginsDir)
	victimDir := filepath.Join(base, "victim")
	if err := os.MkdirAll(victimDir, 0o700); err != nil {
		t.Fatal(err)
	}
	victimFile := filepath.Join(victimDir, "keep.txt")
	if err := os.WriteFile(victimFile, []byte("do not delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	// PluginsDir itself must exist so a valid name reaches real behaviour.
	if err := os.MkdirAll(h.cfg.PluginsDir, 0o700); err != nil {
		t.Fatal(err)
	}

	bad := []string{"../victim", "..", "a/b", "/etc", ""}
	for _, name := range bad {
		if err := h.Remove(name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Remove(%q) = %v, want ErrInvalidName", name, err)
		}
		if _, err := h.Logs(name, 10); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Logs(%q) = %v, want ErrInvalidName", name, err)
		}
		if _, err := h.Inspect(name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Inspect(%q) = %v, want ErrInvalidName", name, err)
		}
	}

	// Nothing outside PluginsDir was touched.
	if _, err := os.Stat(victimFile); err != nil {
		t.Fatalf("victim file must survive traversal attempts: %v", err)
	}
	if _, err := os.Stat(victimDir); err != nil {
		t.Fatalf("victim dir must survive traversal attempts: %v", err)
	}

	// A valid name is accepted by the guard and reaches real behaviour, NOT the
	// invalid-name error: Inspect on an unknown-but-valid name is ErrNotFound,
	// Logs is empty (no log file yet), Remove is a clean no-op.
	if _, err := h.Inspect("echo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Inspect(\"echo\") = %v, want ErrNotFound", err)
	}
	if lines, err := h.Logs("echo", 10); err != nil || len(lines) != 0 {
		t.Fatalf("Logs(\"echo\") = %v, %v, want [], nil", lines, err)
	}
	if err := h.Remove("echo"); err != nil {
		t.Fatalf("Remove(\"echo\") = %v, want nil", err)
	}
}

// TestStartSameNameConcurrentNoLeak hammers N concurrent Start("p"): exactly one
// supervisor runs (one token), the rest no-op, and StopAll cancels everything
// promptly with no orphaned goroutine/token (the pre-fix TOCTOU orphaned the
// first rp's cancel, so StopAll could hang and leak).
func TestStartSameNameConcurrentNoLeak(t *testing.T) {
	h, _, ps := newHost(t, nil)
	h.cfg.Runner = &nameRunner{}
	h.baseCtx = context.Background()

	if err := ps.Upsert(sampleRecord("p")); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("p")
	writeExec(t, h, "p")
	status := 200
	healthServer(t, sock, &status)

	const N = 16
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.Start(sampleRecord("p"))
		}()
	}
	wg.Wait()

	waitRunning(t, h, "p")
	if h.cfg.Tokens.Count() != 1 {
		t.Fatalf("expected exactly 1 supervisor/token, got %d", h.cfg.Tokens.Count())
	}

	done := make(chan struct{})
	go func() { h.StopAll(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("StopAll hung (orphaned supervisor from concurrent same-name Start)")
	}
	if h.cfg.Tokens.Count() != 0 {
		t.Fatalf("tokens leaked after drain: %d", h.cfg.Tokens.Count())
	}
}
