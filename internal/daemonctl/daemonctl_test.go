package daemonctl

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain lets the test binary double as a fake tariboyd: when
// DAEMONCTL_FAKE_DAEMON is set (which EnsureUp's child inherits from the parent
// env), it binds the socket and writes its pidfile like a real daemon instead of
// running the test suite. This gives Restart a bin that actually "comes up"
// without depending on any external tool.
func TestMain(m *testing.M) {
	if os.Getenv("DAEMONCTL_FAKE_DAEMON") != "" {
		runFakeDaemon()
		return
	}
	os.Exit(m.Run())
}

func runFakeDaemon() {
	if path := os.Getenv("DAEMONCTL_FAKE_ARGV_FILE"); path != "" {
		_ = os.WriteFile(path, []byte(strings.Join(os.Args[1:], "\n")), 0o600)
	}
	// When asked to, swallow SIGTERM so the process survives Down's graceful
	// signal and forces the SIGKILL-escalation path. Only SIGKILL then stops us.
	if os.Getenv("DAEMONCTL_FAKE_IGNORE_SIGTERM") != "" {
		signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM)
	}
	sock := os.Getenv("DAEMONCTL_FAKE_SOCK")
	pidfile := os.Getenv("DAEMONCTL_FAKE_PIDFILE")
	if pidfile != "" {
		_ = os.WriteFile(pidfile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	}
	os.Remove(sock) // clear any stale socket left by a predecessor
	ln, err := net.Listen("unix", sock)
	if err != nil {
		os.Exit(1)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"result":{"version":"fake","pid":1}}`)
	})}
	srv.Serve(ln) // blocks until SIGTERM terminates the process
}

// reap waits on a fake daemon pid in the background. EnsureUp detaches its child
// (Release, no Wait), so a killed fake would otherwise linger as an unreaped
// zombie whose pid still answers kill(pid,0). Reaping lets Down see ESRCH
// promptly and keeps the process table clean.
func reap(pid int) {
	go func() {
		var ws syscall.WaitStatus
		_, _ = syscall.Wait4(pid, &ws, 0, nil)
	}()
}

// startFakeDaemonEnv points cfg.DaemonBin at this test binary and sets the env
// the child inherits so it runs runFakeDaemon against cfg's socket/pidfile.
func startFakeDaemonEnv(t *testing.T, cfg *Config) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DaemonBin = exe
	t.Setenv("DAEMONCTL_FAKE_DAEMON", "1")
	t.Setenv("DAEMONCTL_FAKE_SOCK", cfg.Socket)
	t.Setenv("DAEMONCTL_FAKE_PIDFILE", cfg.PidFile)
}

func cfgIn(dir string) Config {
	return Config{
		RuntimeDir:   dir,
		Socket:       filepath.Join(dir, "tariboyd.sock"),
		PidFile:      filepath.Join(dir, "tariboyd.pid"),
		LogFile:      filepath.Join(dir, "tariboyd.log"),
		ReadyTimeout: 1500 * time.Millisecond,
		PollInterval: 50 * time.Millisecond,
	}
}

func TestResolveConfigAcceptsExplicitLoopbackHTTPAddr(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ResolveConfig(func(key string) string {
		switch key {
		case "HOME", "TARIBOY_RUNTIME_DIR":
			return dir
		case "TARIBOY_HTTP_ADDR":
			return "127.0.0.1:18444"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:18444" {
		t.Fatalf("HTTPAddr=%q", cfg.HTTPAddr)
	}
}

func TestEnsureUpForwardsExplicitHTTPAddrAsLiteralArgv(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	cfg.HTTPAddr = "127.0.0.1:18444"
	startFakeDaemonEnv(t, &cfg)
	argvFile := filepath.Join(dir, "argv")
	t.Setenv("DAEMONCTL_FAKE_ARGV_FILE", argvFile)

	started, err := EnsureUp(context.Background(), cfg, io.Discard)
	if err != nil || !started {
		t.Fatalf("EnsureUp started=%v err=%v", started, err)
	}
	if pid, ok := readPid(cfg); ok {
		reap(pid)
	}
	defer Down(context.Background(), cfg, io.Discard)
	got, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "--http-addr\n127.0.0.1:18444" {
		t.Fatalf("argv=%q", got)
	}
}

// serveStatus binds a unix socket answering GET /api/daemon/status, mimicking a
// live daemon so EnsureUp/Status see it as up.
func serveStatus(t *testing.T, sock string) func() {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"result":{"version":"test","pid":1}}`)
	})}
	go srv.Serve(ln)
	return func() { srv.Close() }
}

func TestEnsureUpNoopWhenAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	stop := serveStatus(t, cfg.Socket)
	defer stop()

	started, err := EnsureUp(context.Background(), cfg, io.Discard)
	if err != nil {
		t.Fatalf("EnsureUp err = %v", err)
	}
	if started {
		t.Fatal("started = true, want false (already running)")
	}
}

func TestEnsureUpTimeoutSurfacesLog(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	bin := filepath.Join(dir, "fakedaemon.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho BOOT_FAILURE >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.DaemonBin = bin

	started, err := EnsureUp(context.Background(), cfg, io.Discard)
	if started || err == nil {
		t.Fatalf("EnsureUp started=%v err=%v, want started=false + error", started, err)
	}
	if !strings.Contains(err.Error(), "BOOT_FAILURE") {
		t.Fatalf("error should surface the log tail, got: %v", err)
	}
}

func TestDownTerminatesPidAndClearsPidfile(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	cfg.ReadyTimeout = 2 * time.Second
	child := exec.Command("sleep", "300")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pid := child.Process.Pid
	if err := os.WriteFile(cfg.PidFile, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	go child.Wait() // reap when killed

	if err := Down(context.Background(), cfg, io.Discard); err != nil {
		t.Fatalf("Down err = %v", err)
	}
	// give the kernel a moment to finish reaping before probing liveness
	for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatal("process still alive after Down")
	}
	if _, err := os.Stat(cfg.PidFile); !os.IsNotExist(err) {
		t.Fatalf("pidfile should be gone, stat err = %v", err)
	}
}

func TestRestartStartsWhenStopped(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	startFakeDaemonEnv(t, &cfg)

	if alive(cfg) {
		t.Fatal("precondition: daemon should be down before Restart")
	}
	if err := Restart(context.Background(), cfg, io.Discard); err != nil {
		t.Fatalf("Restart from stopped err = %v", err)
	}
	if pid, ok := readPid(cfg); ok {
		reap(pid)
	}
	defer Down(context.Background(), cfg, io.Discard) // clean up the started fake
	if !alive(cfg) {
		t.Fatal("daemon not up after Restart from a stopped state")
	}
}

func TestRestartStopsThenStarts(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	startFakeDaemonEnv(t, &cfg)

	// Bring up a first instance so Restart has a running daemon to stop.
	started, err := EnsureUp(context.Background(), cfg, io.Discard)
	if err != nil || !started {
		t.Fatalf("initial EnsureUp started=%v err=%v", started, err)
	}
	defer Down(context.Background(), cfg, io.Discard)
	pid1, ok := readPid(cfg)
	if !ok {
		t.Fatal("no pidfile after initial start")
	}
	reap(pid1) // let Down observe pid1 exit instead of a lingering zombie

	if err := Restart(context.Background(), cfg, io.Discard); err != nil {
		t.Fatalf("Restart err = %v", err)
	}
	if !alive(cfg) {
		t.Fatal("daemon not up after Restart")
	}
	pid2, ok := readPid(cfg)
	if !ok {
		t.Fatal("no pidfile after Restart")
	}
	reap(pid2)
	if pid2 == pid1 {
		t.Fatalf("Restart reused pid %d; expected the old instance to be stopped and a new one started", pid1)
	}
	// The original process should be gone (SIGTERM'd by Down).
	for i := 0; i < 50 && syscall.Kill(pid1, 0) == nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(pid1, 0); err == nil {
		t.Fatalf("old daemon pid %d still alive after Restart", pid1)
	}
}

func TestWaitPidGone(t *testing.T) {
	// A live process is not gone within the timeout.
	child := exec.Command("sleep", "300")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pid := child.Process.Pid
	go child.Wait() // reap on kill
	if waitPidGone(context.Background(), pid, 150*time.Millisecond, 20*time.Millisecond) {
		t.Fatal("waitPidGone reported a live process as gone")
	}
	// After it dies and is reaped, waitPidGone confirms it gone.
	_ = child.Process.Kill()
	if !waitPidGone(context.Background(), pid, 2*time.Second, 20*time.Millisecond) {
		t.Fatal("waitPidGone did not observe the killed process exit")
	}
	// A nonsensical pid is treated as already gone.
	if !waitPidGone(context.Background(), 0, time.Second, 20*time.Millisecond) {
		t.Fatal("waitPidGone(0) should report gone")
	}
}

// TestRestartRecoversAfterSigkillEscalation drives the escalation path: the fake
// daemon ignores SIGTERM, so Down times out and SIGKILLs it, then Restart must
// still bring up a fresh instance (new pid, old pid gone) rather than mistaking
// the dying daemon for a live one and skipping the start.
func TestRestartRecoversAfterSigkillEscalation(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	cfg.ReadyTimeout = 600 * time.Millisecond // keep the SIGTERM timeout short
	startFakeDaemonEnv(t, &cfg)
	t.Setenv("DAEMONCTL_FAKE_IGNORE_SIGTERM", "1")

	started, err := EnsureUp(context.Background(), cfg, io.Discard)
	if err != nil || !started {
		t.Fatalf("initial EnsureUp started=%v err=%v", started, err)
	}
	defer Down(context.Background(), cfg, io.Discard)
	pid1, ok := readPid(cfg)
	if !ok {
		t.Fatal("no pidfile after initial start")
	}
	reap(pid1)

	if err := Restart(context.Background(), cfg, io.Discard); err != nil {
		t.Fatalf("Restart err = %v", err)
	}
	if !alive(cfg) {
		t.Fatal("daemon not up after Restart across a SIGKILL escalation")
	}
	pid2, ok := readPid(cfg)
	if !ok {
		t.Fatal("no pidfile after Restart")
	}
	reap(pid2)
	if pid2 == pid1 {
		t.Fatalf("Restart reused pid %d; expected a fresh instance", pid1)
	}
	if !waitPidGone(context.Background(), pid1, 2*time.Second, 20*time.Millisecond) {
		t.Fatalf("old daemon pid %d still alive after Restart", pid1)
	}
}

func TestGetStatusReflectsSocket(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	if s := GetStatus(cfg); s.Running {
		t.Fatal("status running=true with no socket")
	}
	stop := serveStatus(t, cfg.Socket)
	defer stop()
	if s := GetStatus(cfg); !s.Running {
		t.Fatal("status running=false with live socket")
	}
}

func TestTailLogReturnsLastLines(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgIn(dir)
	if err := os.WriteFile(cfg.LogFile, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := TailLog(context.Background(), cfg, 2, false, &sb); err != nil {
		t.Fatal(err)
	}
	if got := sb.String(); got != "c\nd\n" {
		t.Fatalf("tail = %q, want \"c\\nd\\n\"", got)
	}
}
