package shim

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const userHZ = 100 // Linux USER_HZ; cpu_ms = ticks * 1000 / userHZ.

const (
	tmuxExitStatusFilename  = "harness.exit-status"
	defaultTmuxHistoryLimit = 10000
)

var (
	errTmuxExitStatusMissing    = errors.New("missing")
	errTmuxExitStatusUnreadable = errors.New("unreadable")
	errTmuxExitStatusInvalid    = errors.New("invalid")
)

// execCommand is the seam for spawning tmux (and the harness) so tests can
// stub it (e.g. replacing `tmux attach-session` with a plain `cat` echo).
// Defaults to exec.Command; production behaviour is unchanged.
var execCommand = exec.Command

// activePTYs maps a tmux session to the SET of live ptmx of its in-progress
// web attaches, so the separate `resize` control dial (a different
// connection dispatched to tmuxShim.Resize) can find and pty.Setsize them.
// A session can have multiple concurrent attaches (e.g. two browser tabs on
// one agent), so this must be a set keyed by *os.File, not a single value:
// each Attach call registers/deregisters only its own ptmx, so one client
// detaching never clobbers another's live entry. Registered on Attach,
// deleted when that Attach returns; guarded by activePTYsMu.
var (
	activePTYsMu sync.Mutex
	activePTYs   = map[string]map[*os.File]struct{}{}
)

type Options struct {
	IterationDir string
	Agent        string
	IterationID  string
	HardTimeoutS int
	// HardDeadline is an absolute RFC3339 deadline. It supersedes
	// HardTimeoutS when supplied and is used by restart/adoption resync.
	HardDeadline string
	TmuxSession  string
	HarnessArgv  []string
	// ShimSock is the RPC socket path. Empty falls back to the legacy
	// <IterationDir>/shim.sock for backward compatibility.
	ShimSock string
}

// sockPath resolves the RPC socket, preferring the explicit ShimSock.
func (o Options) sockPath() string {
	if o.ShimSock != "" {
		return o.ShimSock
	}
	return filepath.Join(o.IterationDir, "shim.sock")
}

// Run executes one iteration and blocks until it finishes. Grace/sample periods
// use production defaults.
func Run(o Options) error {
	if o.TmuxSession != "" {
		return runTmux(o, 2*time.Second)
	}
	forceStop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	runDone := make(chan struct{})
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			close(forceStop)
		case <-runDone:
		}
	}()
	err := runProcessWithStop(o, 10*time.Second, 2*time.Second, forceStop)
	close(runDone)
	return err
}

// runWith exposes the SIGTERM grace and sampling periods for tests.
func runWith(o Options, grace, sample time.Duration) error {
	if o.TmuxSession != "" {
		return runTmux(o, sample)
	}
	return runProcessWithStop(o, grace, sample, nil)
}

type procShim struct {
	mu                sync.Mutex
	pid               int
	startedAt         string
	finished          bool
	result            *IterationResult
	cmd               *exec.Cmd
	watchdog          *deadlineWatchdog
	terminationReason string
}

func (s *procShim) SetHardDeadline(deadline string) error {
	if s.watchdog == nil {
		return fmt.Errorf("watchdog is not running")
	}
	return s.watchdog.Set(deadline)
}

func (s *procShim) Status() StatusResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StatusResult{Running: !s.finished, PID: s.pid, StartedAt: s.startedAt}
}

func (s *procShim) Kill() error {
	s.mu.Lock()
	pid := s.pid
	s.mu.Unlock()
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func (s *procShim) Screen() (string, error) {
	return "", fmt.Errorf("screen is only available in interactive (tmux) mode")
}
func (s *procShim) SendKeys(SendKeysParams) error {
	return fmt.Errorf("send-keys is only available in interactive (tmux) mode")
}
func (s *procShim) Attach(net.Conn, AttachParams) error {
	return fmt.Errorf("attach is only available in interactive (tmux) mode")
}
func (s *procShim) Resize(ResizeParams) error {
	return fmt.Errorf("resize is only available in interactive (tmux) mode")
}

func (s *procShim) Report() ReportResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finished {
		return ReportResult{Finished: false}
	}
	return ReportResult{Finished: true, Result: s.result}
}

func runProcessWithStop(
	o Options,
	grace, sample time.Duration,
	forceStop <-chan struct{},
) error {
	stdout, err := os.OpenFile(filepath.Join(o.IterationDir, "logs", "harness.stdout.log"),
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(filepath.Join(o.IterationDir, "logs", "harness.stderr.log"),
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer stderr.Close()

	logShim(o.IterationDir, "launch agent=%s iteration=%s argv=%q", o.Agent, o.IterationID, redactHarnessArgv(o.HarnessArgv))
	cmd := exec.Command(o.HarnessArgv[0], o.HarnessArgv[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logShim(o.IterationDir, "ERROR start harness: %v", err)
		return err
	}
	pid := cmd.Process.Pid
	logShim(o.IterationDir, "started pid=%d", pid)

	s := &procShim{pid: pid, startedAt: time.Now().UTC().Format(time.RFC3339), cmd: cmd}
	sockPath := o.sockPath()
	stopRPC := serveRPC(o.IterationDir, sockPath, s)
	defer stopRPC()

	// Resource sampler.
	var peakRSS int
	var lastCPUms int
	sampleStop := make(chan struct{})
	var sampleWG sync.WaitGroup
	sampleWG.Add(1)
	go func() {
		defer sampleWG.Done()
		tk := time.NewTicker(sample)
		defer tk.Stop()
		for {
			select {
			case <-sampleStop:
				return
			case <-tk.C:
				if cpu, ok := readCPUms(pid); ok {
					lastCPUms = cpu
				}
				if rss, ok := readVmRSS(pid); ok && rss > peakRSS {
					peakRSS = rss
				}
			}
		}
	}()

	deadline := initialDeadline(o)
	processDone := make(chan struct{})
	var forceStopDone chan struct{}
	if forceStop != nil {
		forceStopDone = make(chan struct{})
		go func() {
			defer close(forceStopDone)
			select {
			case <-forceStop:
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			case <-processDone:
			}
		}()
	}
	s.watchdog = newDeadlineWatchdog(deadline, func() {
		s.mu.Lock()
		s.terminationReason = "hard_timeout"
		s.mu.Unlock()
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		select {
		case <-processDone:
		case <-time.After(grace):
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	})

	waitErr := cmd.Wait()
	close(processDone)
	if forceStopDone != nil {
		<-forceStopDone
	}
	s.watchdog.Stop()
	close(sampleStop)
	sampleWG.Wait()
	// Final sample (best-effort; the process may already be reaped).
	if cpu, ok := readCPUms(pid); ok && cpu > lastCPUms {
		lastCPUms = cpu
	}

	res := &IterationResult{
		ExitCode:  exitCode(waitErr),
		EndedAt:   time.Now().UTC().Format(time.RFC3339),
		CPUMs:     lastCPUms,
		MemPeakKB: peakRSS,
		OOM:       false,
	}
	s.mu.Lock()
	res.TerminationReason = s.terminationReason
	s.finished = true
	s.result = res
	s.mu.Unlock()

	return writeResult(o.IterationDir, res)
}

func runTmux(o Options, sample time.Duration) error {
	statusPath := filepath.Join(o.IterationDir, "logs", tmuxExitStatusFilename)
	if err := os.Remove(statusPath); err != nil && !os.IsNotExist(err) {
		return errors.New("prepare tmux exit status")
	}
	cmdStr := shellJoin(tmuxHarnessCommand(o.HarnessArgv, statusPath))
	logShim(o.IterationDir, "launch tmux agent=%s iteration=%s session=%s argv=%q", o.Agent, o.IterationID, o.TmuxSession, redactHarnessArgv(o.HarnessArgv))
	bootstrapWindowID, err := execCommand("tmux", tmuxBootstrapSessionArgs(o.TmuxSession, os.Environ())...).Output()
	if err != nil {
		// Surface the failure (e.g. "duplicate session") to shim.log before
		// returning, otherwise only the pre-flight "launch tmux" line is left and
		// the error is invisible to the operator.
		logShim(o.IterationDir, "ERROR tmux new-session session=%s: %v", o.TmuxSession, err)
		return fmt.Errorf("tmux new-session: %w", err)
	}
	bootstrapWindow := strings.TrimSpace(string(bootstrapWindowID))
	if bootstrapWindow == "" {
		return errors.New("tmux new-session: empty bootstrap window id")
	}
	if err := execCommand("tmux", "set-option", "-t", o.TmuxSession, "history-limit", strconv.Itoa(defaultTmuxHistoryLimit)).Run(); err != nil {
		logShim(o.IterationDir, "ERROR tmux set-option session=%s: %v", o.TmuxSession, err)
		return fmt.Errorf("tmux set-option: %w", err)
	}
	if err := execCommand("tmux", "set-option", "-t", o.TmuxSession, "mouse", "off").Run(); err != nil {
		logShim(o.IterationDir, "ERROR tmux set-option session=%s: %v", o.TmuxSession, err)
		return fmt.Errorf("tmux set-option: %w", err)
	}
	if err := execCommand("tmux", "new-window", "-t", o.TmuxSession, cmdStr).Run(); err != nil {
		logShim(o.IterationDir, "ERROR tmux new-window session=%s: %v", o.TmuxSession, err)
		return fmt.Errorf("tmux new-window: %w", err)
	}
	if err := execCommand("tmux", "kill-window", "-t", bootstrapWindow).Run(); err != nil {
		logShim(o.IterationDir, "ERROR tmux kill-window session=%s: %v", o.TmuxSession, err)
		return fmt.Errorf("tmux kill-window: %w", err)
	}
	if err := pipeTmuxLogs(o.TmuxSession, filepath.Join(o.IterationDir, "logs", "harness.stdout.log")); err != nil {
		logShim(o.IterationDir, "WARN tmux pipe logs unavailable")
	}
	s := &tmuxShim{
		session:      o.TmuxSession,
		iterationDir: o.IterationDir,
		startedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	sockPath := o.sockPath()
	stopRPC := serveRPC(o.IterationDir, sockPath, s)
	defer stopRPC()
	s.watchdog = newDeadlineWatchdog(tmuxDeadline(o), func() {
		s.mu.Lock()
		s.terminationReason = "hard_timeout"
		s.mu.Unlock()
		_ = execCommand("tmux", "kill-session", "-t", o.TmuxSession).Run()
	})
	defer s.watchdog.Stop()

	tk := time.NewTicker(sample)
	defer tk.Stop()
	for range tk.C {
		if execCommand("tmux", "has-session", "-t", o.TmuxSession).Run() != nil {
			break // session gone
		}
	}
	exitCode, statusErr := readTmuxExitStatus(statusPath)
	if statusErr != nil {
		exitCode = -1
		logShim(o.IterationDir, "ERROR tmux exit status category=%s", statusErr)
	}
	if err := os.Remove(statusPath); err != nil && !os.IsNotExist(err) {
		logShim(o.IterationDir, "ERROR tmux exit status category=cleanup_failed")
	}
	res := &IterationResult{ExitCode: exitCode, EndedAt: time.Now().UTC().Format(time.RFC3339)}
	s.mu.Lock()
	res.TerminationReason = s.terminationReason
	s.finished = true
	s.result = res
	s.mu.Unlock()
	return writeResult(o.IterationDir, res)
}

// redactHarnessArgv returns a log-only copy with tokenized local proxy URL path
// credentials removed. The original argv remains untouched for process and
// tmux execution. Keeping the path marker and suffix visible preserves useful
// launch diagnostics without disclosing the attribution credential.
func redactHarnessArgv(argv []string) []string {
	redacted := append([]string(nil), argv...)
	for i, arg := range redacted {
		redacted[i] = redactProxyTokenSegments(arg)
	}
	return redacted
}

func redactProxyTokenSegments(s string) string {
	const (
		marker      = "/_tariboy/"
		tokenPrefix = "sk-tariboy-"
		hexLength   = 48 // TokenRegistry mints 24 random bytes as lowercase hex.
		placeholder = "***"
	)
	needle := marker + tokenPrefix
	for searchFrom := 0; searchFrom < len(s); {
		rel := strings.Index(s[searchFrom:], needle)
		if rel < 0 {
			break
		}
		tokenStart := searchFrom + rel + len(marker)
		payloadStart := tokenStart + len(tokenPrefix)
		end := payloadStart + hexLength
		if end > len(s) || !isLowerHex(s[payloadStart:end]) ||
			(end < len(s) && isTokenIdentifierByte(s[end])) {
			searchFrom = payloadStart
			continue
		}
		s = s[:tokenStart] + placeholder + s[end:]
		searchFrom = tokenStart + len(placeholder)
	}
	return s
}

func isLowerHex(s string) bool {
	for i := range s {
		if (s[i] < '0' || s[i] > '9') && (s[i] < 'a' || s[i] > 'f') {
			return false
		}
	}
	return true
}

func isTokenIdentifierByte(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' || b == '-' || b == '_'
}

// deadlineWatchdog owns one resettable timer. Updates are absolute so a daemon
// can safely replay the persisted value after a restart; the newest value wins.
type deadlineWatchdog struct {
	mu      sync.Mutex
	updates chan time.Time
	stop    chan struct{}
	once    sync.Once
}

func initialDeadline(o Options) time.Time {
	if o.HardDeadline != "" {
		if d, err := time.Parse(time.RFC3339Nano, o.HardDeadline); err == nil {
			return d
		}
	}
	hard := o.HardTimeoutS
	if hard <= 0 {
		hard = 60
	}
	return time.Now().Add(time.Duration(hard) * time.Second)
}

// tmuxDeadline is initialDeadline for the interactive (tmux) path, WITHOUT the
// 60s default. An interactive agent is a live TUI a human sits in; a headless
// iteration's hard-timeout watchdog must not kill it mid-use. So only an
// explicitly configured deadline/timeout arms the watchdog — otherwise the
// session runs until it exits or is stopped (returns the zero time = no
// deadline, which newDeadlineWatchdog treats as "never fire").
func tmuxDeadline(o Options) time.Time {
	if o.HardDeadline != "" {
		if d, err := time.Parse(time.RFC3339Nano, o.HardDeadline); err == nil {
			return d
		}
	}
	if o.HardTimeoutS > 0 {
		return time.Now().Add(time.Duration(o.HardTimeoutS) * time.Second)
	}
	return time.Time{} // no deadline
}

func newDeadlineWatchdog(deadline time.Time, fire func()) *deadlineWatchdog {
	w := &deadlineWatchdog{updates: make(chan time.Time, 1), stop: make(chan struct{})}
	go func() {
		// arm (re)sets the timer to fire at d. A zero d means "no deadline":
		// the timer is left stopped so it never fires, but the goroutine keeps
		// listening so a later Set() can still arm a real deadline. This is how
		// an interactive session opts out of the hard-timeout watchdog.
		arm := func(t *time.Timer, d time.Time) {
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			if d.IsZero() {
				return
			}
			wait := time.Until(d)
			if wait < 0 {
				wait = 0
			}
			t.Reset(wait)
		}
		t := time.NewTimer(time.Hour) // placeholder; immediately (re)armed by arm below
		defer t.Stop()
		arm(t, deadline)
		for {
			select {
			case <-w.stop:
				return
			case d := <-w.updates:
				arm(t, d)
			case <-t.C:
				fire()
				return
			}
		}
	}()
	return w
}

func (w *deadlineWatchdog) Set(deadline string) error {
	d, err := time.Parse(time.RFC3339Nano, deadline)
	if err != nil {
		return fmt.Errorf("parse hard deadline: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.stop:
		return fmt.Errorf("watchdog is stopped")
	default:
	}
	select {
	case <-w.updates:
	default:
	}
	w.updates <- d
	return nil
}

func (w *deadlineWatchdog) Stop() { w.once.Do(func() { close(w.stop) }) }

func tmuxBootstrapSessionArgs(session string, env []string) []string {
	args := []string{"new-session", "-d", "-P", "-F", "#{window_id}", "-s", session}
	for _, kv := range env {
		if kv == "" || strings.HasPrefix(kv, "=") {
			continue
		}
		args = append(args, "-e", kv)
	}
	return args
}

func shellJoin(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func tmuxHarnessCommand(argv []string, statusPath string) []string {
	const script = `umask 077
status_path=$1
shift
status_tmp=$status_path.tmp.$$
cleanup_status_tmp() {
	/bin/rm -f "$status_tmp" 2>/dev/null
}
trap cleanup_status_tmp EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
"$@"
status=$?
if { printf '%s\n' "$status" > "$status_tmp"; } 2>/dev/null; then
	/bin/mv -f "$status_tmp" "$status_path" 2>/dev/null || :
fi
exit "$status"`
	wrapper := []string{"/bin/sh", "-c", script, "tariboy-tmux-harness", statusPath}
	return append(wrapper, argv...)
}

func readTmuxExitStatus(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return -1, errTmuxExitStatusMissing
		}
		return -1, errTmuxExitStatusUnreadable
	}
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return -1, errTmuxExitStatusInvalid
	}
	for _, b := range data {
		if b < '0' || b > '9' {
			return -1, errTmuxExitStatusInvalid
		}
	}
	status, err := strconv.ParseUint(string(data), 10, 8)
	if err != nil {
		return -1, errTmuxExitStatusInvalid
	}
	return int(status), nil
}

func pipeTmuxLogs(session, path string) error {
	cmd := "cat >> " + shellQuote(path)
	return execCommand("tmux", "pipe-pane", "-o", "-t", session, cmd).Run()
}

type tmuxShim struct {
	mu                sync.Mutex
	session           string
	iterationDir      string
	startedAt         string
	finished          bool
	result            *IterationResult
	watchdog          *deadlineWatchdog
	terminationReason string
}

func (s *tmuxShim) SetHardDeadline(deadline string) error {
	if s.watchdog == nil {
		return fmt.Errorf("watchdog is not running")
	}
	return s.watchdog.Set(deadline)
}

func (s *tmuxShim) Status() StatusResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StatusResult{Running: !s.finished, PID: 0, StartedAt: s.startedAt}
}

func (s *tmuxShim) Kill() error {
	return execCommand("tmux", "kill-session", "-t", s.session).Run()
}

// Resize retargets ALL live PTYs of in-progress web attaches for this
// session. The resize arrives on a separate control connection, so it looks
// the ptmxes up in activePTYs rather than sharing state with Attach
// directly. tmux forces a session to the smallest attached client, and
// `window-size latest` (set in Attach) makes the most-recently-active client
// dictate; sizing every registered PTY keeps every client's own Resize call
// working instead of only the most recent attacher's.
//
// Setsize calls are made while holding activePTYsMu: resizes are rare and
// the set is small, so the simplicity of not snapshotting outweighs the
// (negligible) extra time other goroutines may wait on the lock.
func (s *tmuxShim) Resize(p ResizeParams) error {
	activePTYsMu.Lock()
	defer activePTYsMu.Unlock()
	set := activePTYs[s.session]
	if len(set) == 0 {
		return fmt.Errorf("no active terminal for session %s", s.session)
	}
	ws := &pty.Winsize{Cols: uint16(p.Cols), Rows: uint16(p.Rows)}
	var firstErr error
	for ptmx := range set {
		if err := pty.Setsize(ptmx, ws); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Attach runs `tmux attach-session` inside a real PTY and bridges bytes between
// the shim socket connection and the PTY until either side closes. It takes
// over conn and returns when the stream ends.
//
// Teardown detaches the client only: it kills the `tmux attach` client process
// and closes the ptmx, but NEVER runs `kill-session` — detaching a browser tab
// must not destroy the running agent's session.
func (s *tmuxShim) Attach(conn net.Conn, p AttachParams) error {
	// Harden against depending on the caller's teardown: if the session ends
	// on its own, the pane→conn copy unblocks (ptmx EOF) and this func
	// returns, but the conn→pty copy is still blocked on conn.Read. Closing
	// conn here unblocks it unconditionally instead of relying on
	// serveConn's own `defer conn.Close()` running after we return. Harmless
	// double-close alongside that defer.
	defer conn.Close()

	// The RPC protocol acknowledges the raw tunnel before Attach starts the
	// tmux client. Reject a session that is already gone before spawning
	// `attach-session`: otherwise tmux writes "no sessions" into the PTY, that
	// diagnostic leaks into xterm, and the browser mistakes the short-lived
	// tunnel for a transport failure. A session can still disappear after this
	// check, but the daemon reports the resulting PTY EOF as a terminal outcome,
	// so the frontend will not retry it.
	if err := execCommand("tmux", "has-session", "-t", s.session).Run(); err != nil {
		logShim(s.iterationDir, "ERROR terminal attach category=session_missing session=%s", s.session)
		return fmt.Errorf("tmux session %s is not running: %w", s.session, err)
	}

	// Most-recently-active client dictates geometry.
	_ = execCommand("tmux", "set-option", "-t", s.session, "window-size", "latest").Run()
	cmd := execCommand("tmux", "-u", "attach-session", "-t", s.session)
	cmd.Env = envWithOverride(os.Environ(), "TERM", "xterm-256color")
	// Keep attach-client diagnostics out of the terminal byte stream. Pane
	// output still arrives on the attach client's stdout through the PTY, but
	// tmux process errors (notably "no sessions" if the session disappears
	// after the preflight above) are written to stderr and discarded here.
	// Setting this before StartWithSize is intentional: the PTY helper only
	// assigns stderr to the tty when cmd.Stderr is nil.
	cmd.Stderr = io.Discard
	size := &pty.Winsize{Cols: uint16(p.Cols), Rows: uint16(p.Rows)}
	if size.Cols == 0 {
		size.Cols = 80
	}
	if size.Rows == 0 {
		size.Rows = 24
	}
	ptmx, err := pty.StartWithSize(cmd, size)
	if err != nil {
		logShim(s.iterationDir, "ERROR terminal attach category=start_failed session=%s", s.session)
		return err
	}
	activePTYsMu.Lock()
	if activePTYs[s.session] == nil {
		activePTYs[s.session] = map[*os.File]struct{}{}
	}
	activePTYs[s.session][ptmx] = struct{}{}
	activePTYsMu.Unlock()
	defer func() {
		activePTYsMu.Lock()
		delete(activePTYs[s.session], ptmx)
		if len(activePTYs[s.session]) == 0 {
			delete(activePTYs, s.session)
		}
		activePTYsMu.Unlock()
		// Closing the ptmx unblocks whichever io.Copy is still running so
		// neither goroutine leaks. Kill the attach client (detach); never
		// kill-session.
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()
	// Buffered so the second (still-blocked) goroutine can send without a
	// receiver and exit cleanly once its copy unblocks.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(conn, ptmx); done <- struct{}{} }() // pane → browser
	go func() { _, _ = io.Copy(ptmx, conn); done <- struct{}{} }() // browser → pane
	<-done                                                         // first side to close ends the stream
	return nil
}

func envWithOverride(base []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(base)+1)
	for _, item := range base {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return append(out, prefix+value)
}

// captureArgs builds `tmux capture-pane` args: -e keeps SGR escapes so the UI
// can render color; -p prints to stdout; -S -<n> includes n lines of scrollback.
func captureArgs(session string, scrollback int) []string {
	return []string{"capture-pane", "-e", "-p", "-S", "-" + strconv.Itoa(scrollback), "-t", session}
}

func (s *tmuxShim) Screen() (string, error) {
	out, err := execCommand("tmux", captureArgs(s.session, 200)...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// sendKeysCommands turns a SendKeysParams into the ordered list of tmux
// send-keys argument vectors. Items are sent raw (no trailing Enter): a text
// item via `-l -- <text>`, a named key via `-- <key>`. The legacy Keys string
// path preserves the old behavior: `-- <keys> Enter`.
func sendKeysCommands(session string, p SendKeysParams) [][]string {
	if len(p.Items) > 0 {
		out := make([][]string, 0, len(p.Items))
		for _, it := range p.Items {
			if it.Key != "" {
				out = append(out, []string{"send-keys", "-t", session, "--", it.Key})
			} else {
				out = append(out, []string{"send-keys", "-t", session, "-l", "--", it.Text})
			}
		}
		return out
	}
	return [][]string{{"send-keys", "-t", session, "--", p.Keys, "Enter"}}
}

func (s *tmuxShim) SendKeys(p SendKeysParams) error {
	for _, args := range sendKeysCommands(s.session, p) {
		if err := execCommand("tmux", args...).Run(); err != nil {
			return err
		}
	}
	return nil
}

func (s *tmuxShim) Report() ReportResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finished {
		return ReportResult{Finished: false}
	}
	return ReportResult{Finished: true, Result: s.result}
}

// serveRPC listens on sockPath and returns a stop func that closes the listener
// and removes the socket. A bind failure is not fatal (the daemon falls back to
// result.json for completion) but it costs live control (kill/screen/reattach),
// so it is logged loudly to logs/shim.log rather than silently swallowed.
func serveRPC(iterDir, sockPath string, h Handler) func() {
	_ = os.Remove(sockPath)
	l, err := netListenUnix(sockPath)
	if err != nil {
		logShim(iterDir, "ERROR bind shim rpc socket %q: %v (continuing without live control; completion via result.json)", sockPath, err)
		return func() {}
	}
	go Serve(l, h)
	return func() {
		l.Close()
		_ = os.Remove(sockPath)
	}
}

// logShim appends a timestamped line to <iterDir>/logs/shim.log. The shim's own
// stdout/stderr are detached (nil), so this file is the shim's audit trail:
// the harness command it launched and any RPC/bind problems.
func logShim(iterDir, format string, args ...any) {
	if iterDir == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(iterDir, "logs", "shim.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().UTC().Format(time.RFC3339)}, args...)...)
}

func writeResult(dir string, res *IterationResult) error {
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "result.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "result.json"))
}

func exitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	if ee, ok := waitErr.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return 128 + int(ws.Signal())
			}
			return ws.ExitStatus()
		}
	}
	return 1
}

// readCPUms returns cumulative utime+stime of pid in milliseconds.
func readCPUms(pid int) (int, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	// Fields 14 (utime) and 15 (stime) follow the comm field, which is
	// parenthesised and may contain spaces; split after the last ')'.
	s := string(data)
	close := strings.LastIndex(s, ")")
	if close < 0 {
		return 0, false
	}
	fields := strings.Fields(s[close+1:])
	// After ')', field index 0 is state; utime is index 11, stime index 12.
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.Atoi(fields[11])
	stime, err2 := strconv.Atoi(fields[12])
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return (utime + stime) * 1000 / userHZ, true
}

// readVmRSS returns the resident set size of pid in kB.
func readVmRSS(pid int) (int, bool) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.Atoi(fields[1]); err == nil {
					return kb, true
				}
			}
		}
	}
	return 0, false
}
