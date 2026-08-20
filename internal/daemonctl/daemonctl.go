// Package daemonctl is the CLI-local lifecycle for tariboyd: it starts the
// daemon detached, stops it, reports status, and tails its log. It never routes
// through the daemon API for start/stop, so it is safe to call when the daemon
// is down.
package daemonctl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/paths"
)

type Config struct {
	RuntimeDir   string
	Socket       string
	PidFile      string
	LogFile      string
	DaemonBin    string
	HTTPAddr     string
	ReadyTimeout time.Duration
	PollInterval time.Duration
}

// ResolveConfig builds a Config from the environment: runtime paths from the
// paths package, and the tariboyd binary from $TARIBOY_DAEMON_BIN or the
// dir of the running executable.
func ResolveConfig(getenv func(string) string) (Config, error) {
	p, err := paths.Resolve(getenv)
	if err != nil {
		return Config{}, err
	}
	bin := getenv("TARIBOY_DAEMON_BIN")
	if bin == "" {
		if exe, eerr := os.Executable(); eerr == nil {
			bin = filepath.Join(filepath.Dir(exe), "tariboyd")
		} else {
			bin = "tariboyd"
		}
	}
	return Config{
		RuntimeDir:   p.RuntimeDir(),
		Socket:       p.Socket(),
		PidFile:      p.PidFile(),
		LogFile:      p.LogFile(),
		DaemonBin:    bin,
		HTTPAddr:     getenv("TARIBOY_HTTP_ADDR"),
		ReadyTimeout: 5 * time.Second,
		PollInterval: 100 * time.Millisecond,
	}, nil
}

// alive reports whether a daemon answers on the socket. Any non-"daemon down"
// error (e.g. an API error) still means the process is answering, which is what
// readiness means.
func alive(cfg Config) bool {
	_, err := client.New(cfg.Socket).Call("GET", "/api/daemon/status", nil)
	return err == nil || !client.IsDaemonDown(err)
}

// EnsureUp starts tariboyd detached if it is not already answering. Returns
// started=false with no error when it was already up.
func EnsureUp(ctx context.Context, cfg Config, out io.Writer) (bool, error) {
	if alive(cfg) {
		return false, nil
	}
	if err := os.MkdirAll(cfg.RuntimeDir, 0o700); err != nil {
		return false, fmt.Errorf("create runtime dir: %w", err)
	}
	logf, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("open log file: %w", err)
	}
	defer logf.Close()
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return false, err
	}
	defer devnull.Close()

	cmd := exec.Command(cfg.DaemonBin)
	if cfg.HTTPAddr != "" {
		cmd.Args = append(cmd.Args, "--http-addr", cfg.HTTPAddr)
	}
	cmd.Stdin = devnull
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start %s: %w", cfg.DaemonBin, err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release() // detach: do not wait/reap

	deadline := time.Now().Add(cfg.ReadyTimeout)
	for {
		if alive(cfg) {
			fmt.Fprintf(out, "tariboyd started (pid %d)\n", pid)
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("tariboyd did not become ready in %s; last log:\n%s",
				cfg.ReadyTimeout, tail(cfg.LogFile, 40))
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(cfg.PollInterval):
		}
	}
}

type StatusInfo struct {
	Running bool
	Pid     int
	Version string
	Raw     json.RawMessage
}

// GetStatus probes the socket; when up it returns the routed status payload,
// otherwise Running=false (never a connection error).
func GetStatus(cfg Config) StatusInfo {
	raw, err := client.New(cfg.Socket).Call("GET", "/api/daemon/status", nil)
	if err != nil {
		return StatusInfo{Running: false}
	}
	si := StatusInfo{Running: true, Raw: raw}
	var body struct {
		Pid     int    `json:"pid"`
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &body)
	si.Pid, si.Version = body.Pid, body.Version
	return si
}

func readPid(cfg Config) (int, bool) {
	b, err := os.ReadFile(cfg.PidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// Down stops the daemon: SIGTERM, poll for the process to exit, escalate to
// SIGKILL after the timeout, then clear the pidfile.
func Down(ctx context.Context, cfg Config, out io.Writer) error {
	pid, ok := readPid(cfg)
	if !ok {
		if alive(cfg) {
			return fmt.Errorf("daemon appears up but no pidfile at %s; stop it manually", cfg.PidFile)
		}
		fmt.Fprintln(out, "tariboyd is not running")
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH { // already gone
			os.Remove(cfg.PidFile)
			fmt.Fprintln(out, "tariboyd is not running (stale pidfile removed)")
			return nil
		}
		return fmt.Errorf("signal pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(cfg.ReadyTimeout)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.PollInterval):
		}
	}
	os.Remove(cfg.PidFile)
	fmt.Fprintf(out, "tariboyd stopped (pid %d)\n", pid)
	return nil
}

// waitPidGone blocks until the process is gone (kill(pid,0)==ESRCH) or the
// deadline passes, returning true only once the process is confirmed gone. A
// pid<=0 is treated as already gone.
func waitPidGone(ctx context.Context, pid int, timeout, poll time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(poll):
		}
	}
}

// Restart stops the daemon and then starts it again. Down no-ops cleanly when
// the daemon is already stopped (prints "not running", returns nil), so Restart
// also works from a stopped state — it simply starts. Errors from Down or
// EnsureUp are returned unchanged.
//
// On the SIGKILL-escalation path (graceful SIGTERM timed out) Down returns
// right after sending SIGKILL, without waiting for the kernel to finish
// teardown. The old process's socket can still momentarily answer, and
// EnsureUp gates on alive() over that socket — so a naive Down-then-EnsureUp
// could see the dying daemon as "up" and skip starting a replacement, leaving
// Restart reporting success while the daemon ends up down. To close that gap we
// capture the old pid before Down and confirm it is truly gone before EnsureUp,
// so EnsureUp always observes a dead daemon and starts a fresh one.
func Restart(ctx context.Context, cfg Config, out io.Writer) error {
	oldPid, hadPid := readPid(cfg)
	if err := Down(ctx, cfg, out); err != nil {
		return err
	}
	if hadPid {
		waitPidGone(ctx, oldPid, cfg.ReadyTimeout, cfg.PollInterval)
	}
	_, err := EnsureUp(ctx, cfg, out)
	return err
}

// TailLog writes the last n lines of the log, then (if follow) streams appends
// until ctx is cancelled.
func TailLog(ctx context.Context, cfg Config, n int, follow bool, out io.Writer) error {
	f, err := os.Open(cfg.LogFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no log at %s (has the daemon ever started?)", cfg.LogFile)
		}
		return err
	}
	defer f.Close()
	if s := tail(cfg.LogFile, n); s != "" {
		fmt.Fprintln(out, s)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if !follow {
		return nil
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			io.WriteString(out, line)
		}
		if err == io.EOF {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		if err != nil {
			return err
		}
	}
}

// tail returns the last n lines of path (best-effort; empty on error).
func tail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
