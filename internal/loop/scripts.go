package loop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/store"
	"golang.org/x/sys/unix"
)

var (
	scriptPollInterval = 100 * time.Millisecond
	scriptCancelGrace  = 2 * time.Second
)

// scriptSupervisor runs agents' durable scripts: it starts claimed runs as
// their agent and records how each ended. Its mutex guards only the process
// maps below; no Store call or process operation runs under it.
type scriptSupervisor struct {
	cfg *ManagerConfig
	wg  *sync.WaitGroup
	mu  sync.Mutex
	// scriptsWake wakes the supervisor after a new record is created. It is
	// deliberately buffered: callers never wait for a process to start in
	// order to receive a script ID.
	scriptsWake   chan struct{}
	scriptRuns    map[string]*exec.Cmd
	scriptCancels map[string]bool
}

func newScriptSupervisor(cfg *ManagerConfig, wg *sync.WaitGroup) *scriptSupervisor {
	return &scriptSupervisor{
		cfg: cfg, wg: wg, scriptsWake: make(chan struct{}, 1),
		scriptRuns: map[string]*exec.Cmd{}, scriptCancels: map[string]bool{},
	}
}

func (s *scriptSupervisor) markScriptCancellation(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, tracked := s.scriptRuns[key]; !tracked {
		return false
	}
	s.scriptCancels[key] = true
	return true
}

func (s *scriptSupervisor) terminateScriptProcess(key string, pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.AfterFunc(scriptCancelGrace, func() {
		s.mu.Lock()
		cancelPending := s.scriptCancels[key]
		_, stillRunning := s.scriptRuns[key]
		s.mu.Unlock()
		if cancelPending && stillRunning {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
}

func (s *scriptSupervisor) wakeScripts() {
	select {
	case s.scriptsWake <- struct{}{}:
	default:
	}
}

func (s *scriptSupervisor) startScriptSupervisor(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(scriptPollInterval)
		defer ticker.Stop()
		for {
			s.superviseScripts(ctx)
			select {
			case <-ctx.Done():
				return
			case <-s.scriptsWake:
			case <-ticker.C:
			}
		}
	}()
}

func (s *scriptSupervisor) superviseScripts(ctx context.Context) {
	agents, err := s.cfg.Store.List()
	if err != nil {
		s.cfg.Log.Error("list scripts agents", "err", err)
		return
	}
	now := store.FormatTime(s.cfg.Clock())
	due, err := s.cfg.Scripts.DueDefinitions(now)
	if err != nil {
		s.cfg.Log.Error("list due scripts", "err", err)
		return
	}
	for _, definition := range due {
		if _, err := s.cfg.Scripts.ScheduleNext(definition.Agent, definition.ID); err != nil && !errors.Is(err, script.ErrConflict) {
			s.cfg.Log.Error("schedule next script run", "agent", definition.Agent, "script", definition.ID, "err", err)
		}
	}
	pending, err := s.cfg.Scripts.PendingRuns()
	if err != nil {
		s.cfg.Log.Error("list pending script runs", "err", err)
		return
	}
	agentsByName := make(map[string]agent.Agent, len(agents))
	for _, ag := range agents {
		agentsByName[ag.Name] = ag
	}
	for _, run := range pending {
		ag, ok := agentsByName[run.Agent]
		if !ok {
			continue
		}
		s.startScript(ctx, ag, run)
	}
}

func (s *scriptSupervisor) startScript(ctx context.Context, ag agent.Agent, r script.Run) {
	key := r.Agent + "/" + r.ID
	s.mu.Lock()
	if _, running := s.scriptRuns[key]; running {
		s.mu.Unlock()
		return
	}
	// A placeholder makes concurrent scan passes unable to start this record
	// twice before its shell has a PID.
	s.scriptRuns[key] = nil
	s.mu.Unlock()
	l := agentdir.New(s.cfg.AgentsDir, ag.Name).WithRuntime(s.cfg.RuntimeDir)
	definition, err := s.cfg.Scripts.GetDefinition(r.Agent, r.ScriptID)
	if err != nil {
		s.finishScript(key, r, -1, fmt.Errorf("load script definition: %w", err), "")
		return
	}
	logPath, err := filepath.Abs(filepath.Join(l.Root, "scripts", r.ID+".log"))
	if err != nil {
		s.finishScript(key, r, -1, fmt.Errorf("resolve script log path: %w", err), "")
		return
	}
	claimed, err := s.cfg.Scripts.ClaimRun(r.Agent, r.ID, store.FormatTime(s.cfg.Clock()), logPath)
	if err != nil {
		s.finishScript(key, r, -1, err, logPath)
		return
	}
	if !claimed {
		s.mu.Lock()
		delete(s.scriptRuns, key)
		s.mu.Unlock()
		return
	}

	secrets, _ := s.cfg.Store.SecretMap(ag.Name)
	env := agentScriptEnv(s.cfg.BaseEnv, ag, l, secrets, l.Sock())
	// Appended last: os/exec keeps the final value of a duplicated key, so an
	// agent's own environment cannot redefine the protocol.
	env = append(env, script.ProtocolEnv()...)
	runContext := ctx
	cancelRunContext := func() {}
	if s.cfg.ScriptTimeout > 0 {
		runContext, cancelRunContext = context.WithTimeout(ctx, s.cfg.ScriptTimeout)
	}
	bash, err := harness.FindExecutable("bash", env, cwdOf(ag, l))
	if err != nil {
		cancelRunContext()
		s.finishScript(key, r, -1, errors.New("bash is required for agent shell scripts"), logPath)
		return
	}
	argv := agentShellCommand(bash, filepath.Dir(s.cfg.AgentsDir), l, []string{"sh", "-c", definition.Command})
	cmd := exec.CommandContext(runContext, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = cwdOf(ag, l), env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		cancelRunContext()
		s.finishScript(key, r, -1, err, logPath)
		return
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		cancelRunContext()
		s.finishScript(key, r, -1, err, logPath)
		return
	}
	if _, err := fmt.Fprintf(logFile, "cwd: %s\n", cmd.Dir); err != nil {
		cancelRunContext()
		_ = logFile.Close()
		s.finishScript(key, r, -1, err, logPath)
		return
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		cancelRunContext()
		_ = logFile.Close()
		s.finishScript(key, r, -1, err, logPath)
		return
	}
	pidSet, err := s.cfg.Scripts.SetRunPID(r.Agent, r.ID, cmd.Process.Pid)
	if err != nil || !pidSet {
		cancelRunContext()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_ = logFile.Close()
		if err == nil {
			err = errors.New("script claim was canceled before process startup completed")
		}
		s.finishScript(key, r, -1, err, logPath)
		return
	}
	s.mu.Lock()
	s.scriptRuns[key] = cmd
	cancelRequested := s.scriptCancels[key]
	s.mu.Unlock()
	if cancelRequested {
		s.terminateScriptProcess(key, cmd.Process.Pid)
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancelRunContext()
		err := cmd.Wait()
		_ = logFile.Close()
		code := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				code = -1
			}
		}
		if runContext.Err() != nil {
			code, err = -1, runContext.Err()
		}
		s.finishScript(key, r, code, err, logPath)
	}()
}

func (s *scriptSupervisor) finishScript(key string, r script.Run, code int, runErr error, logPath string) {
	s.mu.Lock()
	cancelRequested := s.scriptCancels[key]
	delete(s.scriptRuns, key)
	delete(s.scriptCancels, key)
	s.mu.Unlock()
	current, err := s.cfg.Scripts.GetRun(r.Agent, r.ID)
	if err != nil {
		return
	}
	if cancelRequested || current.CancelRequested {
		if err := s.cfg.Scripts.CancelRun(r.Agent, r.ID, store.FormatTime(s.cfg.Clock())); err != nil && !errors.Is(err, script.ErrConflict) {
			s.cfg.Log.Error("cancel script run", "agent", r.Agent, "run", r.ID, "err", err)
			return
		}
		if s.cfg.ScriptResults != nil {
			s.cfg.ScriptResults.Wake()
		}
		return
	}
	// Cancellation wins over a late Wait result, so a killed recurring script
	// cannot resurrect itself as waiting.
	if current.Status == script.RunCancelled {
		return
	}
	if runErr != nil && code == -1 {
		s.cfg.Log.Error("script start/wait", "agent", r.Agent, "script", r.ID, "err", runErr)
	}
	status := script.RunFailed
	var exitCode *int
	if errors.Is(runErr, context.DeadlineExceeded) {
		status = script.RunTimedOut
	} else if errors.Is(runErr, context.Canceled) {
		status = script.RunInterrupted
	} else if code >= 0 {
		exitCode = &code
		if code == 0 {
			status = script.RunSucceeded
		}
	}
	if _, err := s.cfg.Scripts.CompleteRun(r.Agent, r.ID, script.Completion{Status: status, ExitCode: exitCode, FinishedAt: store.FormatTime(s.cfg.Clock()), LogPath: logPath}); err != nil {
		if !errors.Is(err, script.ErrConflict) {
			s.cfg.Log.Error("complete script run", "agent", r.Agent, "run", r.ID, "err", err)
		}
		return
	}
	if s.cfg.ScriptResults != nil {
		s.cfg.ScriptResults.Wake()
	}
}

func (s *scriptSupervisor) openScriptLog(agentName, runID string) (*os.File, string, error) {
	run, err := s.GetScriptRun(agentName, runID)
	if err != nil {
		return nil, "", err
	}
	if run.LogPath == "" {
		return nil, "", script.ErrNotFound
	}
	root, path, err := s.scriptLogLocation(agentName, run.LogPath)
	if err != nil {
		return nil, "", err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", script.ErrNotFound
		}
		return nil, "", err
	}
	defer unix.Close(rootFD)
	filename := filepath.Base(path)
	fd, err := unix.Openat(rootFD, filename, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", script.ErrNotFound
		}
		return nil, "", err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, "", errors.New("open script log")
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, "", err
	}
	stat, _ := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || stat == nil || stat.Nlink != 1 {
		file.Close()
		return nil, "", errors.New("script log is not a regular owner-only file")
	}
	return file, filename, nil
}

func (s *scriptSupervisor) scriptLogLocation(agentName, logPath string) (string, string, error) {
	root, err := filepath.Abs(filepath.Join(agentdir.New(s.cfg.AgentsDir, agentName).Root, "scripts"))
	if err != nil {
		return "", "", err
	}
	path, err := filepath.Abs(logPath)
	if err != nil {
		return "", "", err
	}
	if filepath.Dir(path) != root {
		return "", "", fmt.Errorf("script log path is outside the agent scripts directory")
	}
	return root, path, nil
}

// RerunScript queues an immediate attempt for an eligible stored definition.
func (s *scriptSupervisor) RerunScript(agentName, scriptID string) (script.Run, error) {
	if s.cfg.Scripts == nil {
		return script.Run{}, fmt.Errorf("script store not configured")
	}
	run, err := s.cfg.Scripts.Rerun(agentName, scriptID)
	if err == nil {
		s.wakeScripts()
	}
	return run, err
}

// RunOnce creates one local command definition and one immediate asynchronous run.
func (s *scriptSupervisor) RunOnce(agentName string, in script.CreateOnce) (script.Definition, script.Run, error) {
	if s.cfg.Scripts == nil {
		return script.Definition{}, script.Run{}, fmt.Errorf("script store not configured")
	}
	if _, err := s.cfg.Store.Get(agentName); err != nil {
		return script.Definition{}, script.Run{}, err
	}
	definition, run, err := s.cfg.Scripts.CreateOnce(agentName, in)
	if err == nil {
		s.wakeScripts()
	}
	return definition, run, err
}

// ScheduleScript creates a fixed-delay recurring local command and its first
// immediate run. Later runs are queued only after the previous run finishes.
func (s *scriptSupervisor) ScheduleScript(agentName string, in script.CreateSchedule) (script.Definition, script.Run, error) {
	if s.cfg.Scripts == nil {
		return script.Definition{}, script.Run{}, fmt.Errorf("script store not configured")
	}
	if _, err := s.cfg.Store.Get(agentName); err != nil {
		return script.Definition{}, script.Run{}, err
	}
	definition, run, err := s.cfg.Scripts.CreateSchedule(agentName, in)
	if err == nil {
		s.wakeScripts()
	}
	return definition, run, err
}

func (s *scriptSupervisor) ListScripts(agentName string) ([]script.Definition, error) {
	if s.cfg.Scripts == nil {
		return nil, fmt.Errorf("script store not configured")
	}
	return s.cfg.Scripts.ListDefinitions(agentName)
}

func (s *scriptSupervisor) ListScriptRuns(agentName, scriptID string) ([]script.Run, error) {
	if s.cfg.Scripts == nil {
		return nil, fmt.Errorf("script store not configured")
	}
	return s.cfg.Scripts.ListRuns(agentName, scriptID)
}

func (s *scriptSupervisor) GetScriptRun(agentName, runID string) (script.Run, error) {
	if s.cfg.Scripts == nil {
		return script.Run{}, fmt.Errorf("script store not configured")
	}
	return s.cfg.Scripts.GetRun(agentName, runID)
}

// CancelScriptTarget records cancellation intent for a running attempt before
// terminating its process group. The run remains active until Wait confirms
// process exit, preserving the per-definition no-overlap invariant.
func (s *scriptSupervisor) CancelScriptTarget(agentName, id string) error {
	if s.cfg.Scripts == nil {
		return fmt.Errorf("script store not configured")
	}
	at := store.FormatTime(s.cfg.Clock())
	if run, err := s.cfg.Scripts.GetRun(agentName, id); err == nil {
		if run.Status == script.RunRunning {
			run, err = s.cfg.Scripts.RequestRunCancellation(agentName, id)
			if err != nil {
				return err
			}
			key := run.Agent + "/" + run.ID
			tracked := s.markScriptCancellation(key)
			if run.PID != nil {
				if tracked {
					s.terminateScriptProcess(key, *run.PID)
				} else {
					_ = syscall.Kill(-*run.PID, syscall.SIGTERM)
				}
			}
			return nil
		}
		if err := s.cfg.Scripts.CancelRun(agentName, id, at); err != nil {
			return err
		}
		if s.cfg.ScriptResults != nil {
			s.cfg.ScriptResults.Wake()
		}
		return nil
	} else if !errors.Is(err, script.ErrNotFound) {
		return err
	}

	definition, err := s.cfg.Scripts.GetDefinition(agentName, id)
	if err != nil {
		return err
	}
	var running *script.Run
	if definition.LatestRun != nil && definition.LatestRun.Status == script.RunRunning {
		running = definition.LatestRun
	}
	if err := s.cfg.Scripts.CancelDefinition(agentName, id, at); err != nil {
		return err
	}
	if running != nil {
		key := running.Agent + "/" + running.ID
		tracked := s.markScriptCancellation(key)
		if running.PID != nil {
			if tracked {
				s.terminateScriptProcess(key, *running.PID)
			} else {
				_ = syscall.Kill(-*running.PID, syscall.SIGTERM)
			}
		}
	}
	if s.cfg.ScriptResults != nil {
		s.cfg.ScriptResults.Wake()
	}
	return nil
}

func (s *scriptSupervisor) RemoveScript(agentName, id string) error {
	if s.cfg.Scripts == nil {
		return fmt.Errorf("script store not configured")
	}
	definition, err := s.cfg.Scripts.GetDefinition(agentName, id)
	if err != nil {
		return err
	}
	if definition.State == script.StateActive {
		return fmt.Errorf("cannot remove active script %q; cancel it first: %w", id, script.ErrActive)
	}
	runs, err := s.cfg.Scripts.ListRuns(agentName, id)
	if err != nil {
		return err
	}
	logPaths := make([]string, 0, len(runs))
	for _, run := range runs {
		if run.Status == script.RunPending || run.Status == script.RunRunning {
			return fmt.Errorf("cannot remove script %q with an active run: %w", id, script.ErrActive)
		}
		if run.LogPath == "" {
			continue
		}
		_, path, err := s.scriptLogLocation(agentName, run.LogPath)
		if err != nil {
			return err
		}
		logPaths = append(logPaths, path)
	}
	type movedLog struct{ from, to string }
	moved := make([]movedLog, 0, len(logPaths))
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			if err := os.Rename(moved[i].to, moved[i].from); err != nil && !os.IsNotExist(err) {
				s.cfg.Log.Error("restore script log after failed removal", "from", moved[i].to, "to", moved[i].from, "err", err)
			}
		}
	}
	for index, path := range logPaths {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			rollback()
			return err
		}
		quarantine := fmt.Sprintf("%s.deleting-%d-%d", path, s.cfg.Clock().UnixNano(), index)
		if _, err := os.Lstat(quarantine); err == nil {
			rollback()
			return fmt.Errorf("script log quarantine path already exists: %s", quarantine)
		} else if !os.IsNotExist(err) {
			rollback()
			return err
		}
		if err := os.Rename(path, quarantine); err != nil {
			rollback()
			return err
		}
		moved = append(moved, movedLog{from: path, to: quarantine})
	}
	if err := s.cfg.Scripts.RemoveDefinition(agentName, id); err != nil {
		rollback()
		return err
	}
	for _, log := range moved {
		if err := os.Remove(log.to); err != nil && !os.IsNotExist(err) {
			s.cfg.Log.Error("remove quarantined script log", "path", log.to, "err", err)
		}
	}
	return nil
}

// LogScript returns a bounded tail after confirming the record belongs to the
// requested agent. Missing logs are represented by an empty string.
func (s *scriptSupervisor) LogScriptRun(agentName, id string) (string, error) {
	file, _, err := s.openScriptLog(agentName, id)
	if errors.Is(err, script.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	const maxLogBytes = 64 * 1024
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > maxLogBytes {
		if _, err := file.Seek(info.Size()-maxLogBytes, io.SeekStart); err != nil {
			return "", err
		}
	}
	b, err := io.ReadAll(io.LimitReader(file, maxLogBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *scriptSupervisor) OpenScriptLog(agentName, runID string) (io.ReadCloser, string, error) {
	return s.openScriptLog(agentName, runID)
}

// shellEnvTimeout bounds sourcing the shell scripts for a script environment:
// a bind, a secret list, and every workflow run wait for it.
var shellEnvTimeout = 30 * time.Second

// ScriptRuntime returns the working directory and environment a script run
// "as the agent" uses: the daemon baseline, the agent's environment and
// secrets, the agent bin directory on PATH, and no tools socket.
func (m *Manager) ScriptRuntime(agentName string) (cwd string, env []string, err error) {
	ag, err := m.cfg.Store.Get(agentName)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, agent.ErrNotFound) {
		return "", nil, fmt.Errorf("agent %q not found", agentName)
	}
	if err != nil {
		return "", nil, fmt.Errorf("load agent %q: %w", agentName, err)
	}
	secrets, err := m.cfg.Store.SecretMap(ag.Name)
	if err != nil {
		return "", nil, fmt.Errorf("load secrets of agent %q: %w", agentName, err)
	}
	l := agentdir.New(m.cfg.AgentsDir, ag.Name).WithRuntime(m.cfg.RuntimeDir)
	cwd = cwdOf(ag, l)
	env, err = agentShellEnv(filepath.Dir(m.cfg.AgentsDir), l.ShellScriptPath(), cwd, agentScriptEnv(m.cfg.BaseEnv, ag, l, secrets, ""))
	if err != nil {
		return "", nil, fmt.Errorf("agent shell scripts of agent %q: %w", agentName, err)
	}
	return cwd, env, nil
}

// agentScriptEnv is the environment of a script that runs as ag: the daemon
// baseline (base, or the process environment when nil), the agent's
// environment and secrets, and the agent bin directory on PATH. toolsSock is
// the agent tools socket, or "" for a script that must not reach it.
func agentScriptEnv(base []string, ag agent.Agent, l agentdir.Layout, secrets map[string]string, toolsSock string) []string {
	env := BuildEnv(baseEnv(base), l.BinDir(), ag.Name, "", toolsSock, false, "", "", ag.Env, secrets)
	if toolsSock != "" {
		return env
	}
	// A script without the tools socket gets none of the variables that reach
	// the socket or the daemon API.
	out := env[:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(script.DaemonAccessEnv, name) {
			out = append(out, kv)
		}
	}
	return out
}

// GlobalShellEnv returns the daemon environment as global-agent-shell.sh under
// baseDir leaves it: the environment of workflow scripts that run as the queue.
func GlobalShellEnv(baseDir string) ([]string, error) {
	// The script sees none of the variables that reach the daemon API.
	base := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(script.DaemonAccessEnv, name)
	})
	env, err := agentShellEnv(baseDir, "", baseDir, base)
	if err != nil {
		return nil, fmt.Errorf("global agent shell script: %w", err)
	}
	return env, nil
}

// agentShellEnv returns env as the global and agent shell scripts leave it
// after they are sourced the way a harness launch sources them. An empty
// agentScript sources the global script only.
func agentShellEnv(baseDir, agentScript, cwd string, env []string) ([]string, error) {
	_, globalErr := os.Stat(filepath.Join(baseDir, "global-agent-shell.sh"))
	_, agentErr := os.Stat(agentScript)
	if globalErr != nil && (agentScript == "" || agentErr != nil) {
		return env, nil
	}
	bash, err := harness.FindExecutable("bash", env, cwd)
	if err != nil {
		return nil, errors.New("bash is required for agent shell scripts")
	}
	argv := []string{bash, "-c", agentShellPrelude, "tariboy-agent-shell", filepath.Join(baseDir, "global-agent-shell.sh"), agentScript,
		bash, "-c", `for n in $(compgen -e); do printf '%s=%s\0' "$n" "${!n}"; done`}
	ctx, cancel := context.WithTimeout(context.Background(), shellEnvTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = cwd, env
	// A timeout kills the scripts' children too, and stops waiting for a pipe
	// one of them still holds.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("shell scripts did not finish within %v", shellEnvTimeout)
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00"), nil
}
