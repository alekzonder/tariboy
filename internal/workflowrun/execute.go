package workflowrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/tidy"
)

// DefaultTimeout bounds a run whose Spec names no timeout.
const DefaultTimeout = 60 * time.Second

// killGrace is how long a process group has between SIGTERM and SIGKILL.
const killGrace = 2 * time.Second

// Spec describes one run of one workflow script.
type Spec struct {
	RunID      string
	Kind       tasks.RunKind
	ScriptPath string // absolute path inside the unpacked image
	Cwd        string
	Env        []string // complete environment, protocol entries included
	Timeout    time.Duration
	RunDir     string // <base>/tasks/<KEY>/runs/<run-id>, or the run directory of a source
	TaskDir    string // <base>/tasks/<KEY>/state, or the state directory of a source
	Snapshot   []byte // written to RunDir/task.json
	Declared   Declared
	OnStart    func(pid int) // called once the process has a PID
}

// Result is what one run produced.
type Result struct {
	Verdict  Verdict
	ExitCode *int
	TimedOut bool
	LogPath  string
	Started  time.Time
	Finished time.Time
}

// Execute prepares the directories and task.json, runs the script in its own
// process group with combined output in RunDir/run.log, enforces the timeout,
// reads RunDir/result.json, and classifies. Cancelling ctx kills the group.
func Execute(ctx context.Context, spec Spec) Result {
	e := &execution{spec: spec}
	if err := e.prepare(ctx); err != nil {
		return e.fail("%v", err)
	}

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancelRun := context.WithTimeout(ctx, timeout)
	defer cancelRun()

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return e.fail("cannot open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()

	cmd := e.command(devNull)
	e.res.Started = time.Now()
	if err := cmd.Start(); err != nil {
		return e.fail("cannot start the script %s: %v", spec.ScriptPath, err)
	}
	proc := startProcess(cmd.Process.Pid)
	if !callOnStart(spec.OnStart, proc.pid) {
		return e.abandon(cmd, proc)
	}
	killed := awaitProcessExit(runCtx, proc)
	e.reap(cmd, proc)
	tidy.Close(nil, e.logFile)
	return e.judge(ctx, runCtx, cmd, killed)
}

// execution is the state of one Execute call: the result so far and, once it
// is open, the run log.
type execution struct {
	spec    Spec
	res     Result
	logFile *os.File
}

func (e *execution) resultPath() string {
	return filepath.Join(e.spec.RunDir, "result.json")
}

// fail records a run that ended before or instead of a normal exit. The
// reason also goes to the log when one is open, so a reader sees it there.
func (e *execution) fail(format string, args ...any) Result {
	e.res.Verdict = failure(format, args...)
	now := time.Now()
	if e.res.Started.IsZero() {
		e.res.Started = now
	}
	e.res.Finished = now
	if e.logFile != nil {
		_, _ = fmt.Fprintf(e.logFile, "error: %s\n", e.res.Verdict.Message)
		tidy.Close(nil, e.logFile)
	}
	return e.res
}

// prepare does everything that must hold before the script starts. Its error
// is the failure message of the run.
func (e *execution) prepare(ctx context.Context) error {
	if err := e.openLog(); err != nil {
		return err
	}
	if err := e.writeInputs(); err != nil {
		return err
	}
	if err := e.checkRunnable(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return errors.New("the run was cancelled before it started")
	}
	return nil
}

// openLog creates the run directories and the run log with its header.
func (e *execution) openLog() error {
	spec := e.spec
	for _, dir := range []string{spec.RunDir, spec.TaskDir} {
		if err := ownerDir(dir); err != nil {
			return fmt.Errorf("cannot create directory %s: %v", dir, err)
		}
	}
	logPath, err := filepath.Abs(filepath.Join(spec.RunDir, "run.log"))
	if err != nil {
		return fmt.Errorf("cannot resolve the run log path: %v", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("cannot create the run log %s: %v", logPath, err)
	}
	e.logFile = logFile
	e.res.LogPath = logPath
	if _, err := fmt.Fprintf(logFile, "script: %s\ncwd: %s\n", spec.ScriptPath, spec.Cwd); err != nil {
		return fmt.Errorf("cannot write the run log %s: %v", logPath, err)
	}
	return nil
}

// writeInputs writes the task snapshot and removes any stale result file.
func (e *execution) writeInputs() error {
	// A source run has no task, so it has no snapshot.
	if e.spec.Kind != tasks.RunKindSource {
		taskFile := filepath.Join(e.spec.RunDir, "task.json")
		if err := writeOwnerFile(taskFile, e.spec.Snapshot); err != nil {
			return fmt.Errorf("cannot write %s: %v", taskFile, err)
		}
	}
	resultPath := e.resultPath()
	if err := os.Remove(resultPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot remove the stale result file %s: %v", resultPath, err)
	}
	return nil
}

// checkRunnable checks the working directory and the script.
func (e *execution) checkRunnable() error {
	spec := e.spec
	if info, err := os.Stat(spec.Cwd); err != nil {
		return fmt.Errorf("the working directory %s is not usable: %v", spec.Cwd, err)
	} else if !info.IsDir() {
		return fmt.Errorf("the working directory %s is not a directory", spec.Cwd)
	}
	if !filepath.IsAbs(spec.ScriptPath) {
		return fmt.Errorf("the script path %s is not absolute", spec.ScriptPath)
	}
	if info, err := os.Stat(spec.ScriptPath); err != nil {
		return fmt.Errorf("the script %s is not usable: %v", spec.ScriptPath, err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("the script %s is not a regular file", spec.ScriptPath)
	} else if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("the script %s is not executable", spec.ScriptPath)
	}
	return nil
}

// command builds the script's process. The worker signals the process group
// itself, never through os/exec's context handling, which may signal after the
// process was reaped.
func (e *execution) command(stdin *os.File) *exec.Cmd {
	cmd := exec.Command(e.spec.ScriptPath)
	cmd.Dir, cmd.Env = e.spec.Cwd, e.spec.Env
	if cmd.Env == nil {
		// A nil Env means "inherit" to os/exec; a run gets exactly spec.Env.
		cmd.Env = []string{}
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, e.logFile, e.logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// reap kills what the script left in its group while the exited leader, not
// yet reaped, still holds the group id, and then reaps the leader.
func (e *execution) reap(cmd *exec.Cmd, proc *process) {
	<-proc.exited
	if err := syscall.Kill(-proc.pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_, _ = fmt.Fprintf(e.logFile, "error: cannot kill the process group %d: %v\n", proc.pid, err)
	}
	_ = cmd.Wait() // reaper only: a non-zero exit is reported through the run result, not here
	e.res.Finished = time.Now()
}

// abandon kills a started script whose bookkeeping failed.
func (e *execution) abandon(cmd *exec.Cmd, proc *process) Result {
	proc.signal(syscall.SIGKILL)
	e.reap(cmd, proc)
	_, _ = fmt.Fprintln(e.logFile, "error: run bookkeeping failed")
	tidy.Close(nil, e.logFile)
	e.res.Verdict = failure("run bookkeeping failed")
	return e.res
}

// awaitProcessExit waits for the script to exit, and stops it when runCtx ends first.
// It reports whether the script was signalled.
func awaitProcessExit(runCtx context.Context, proc *process) (killed bool) {
	select {
	case <-proc.exited:
	case <-runCtx.Done():
		if testHookTimeout != nil {
			testHookTimeout(proc.exited)
		}
		// A script that has exited by itself is judged by its exit code, even
		// when the deadline passed at the same instant.
		if killed = proc.signal(syscall.SIGTERM); killed {
			select {
			case <-proc.exited:
			case <-time.After(killGrace):
				proc.signal(syscall.SIGKILL)
			}
		}
	}
	return killed
}

// judge turns a reaped script into the result of the run.
func (e *execution) judge(ctx, runCtx context.Context, cmd *exec.Cmd, killed bool) Result {
	if st := cmd.ProcessState; st != nil && st.Exited() {
		code := st.ExitCode()
		e.res.ExitCode = &code
	}
	if killed && ctx.Err() != nil {
		e.res.Verdict = failure("the run was cancelled")
		return e.res
	}
	e.res.TimedOut = killed && errors.Is(runCtx.Err(), context.DeadlineExceeded)
	var raw []byte
	var resultErr error
	if !e.res.TimedOut {
		raw, resultErr = readResultFile(e.resultPath())
	}
	e.res.Verdict = Classify(e.spec.Kind, e.res.ExitCode, e.res.TimedOut, raw, resultErr, e.spec.Declared)
	return e.res
}

// testHookTimeout, when set by a test, runs when the timeout fires, before the
// worker decides whether to signal; it receives the channel closed on exit.
var testHookTimeout func(exited <-chan struct{})

// process tracks whether a started child has exited, without reaping it, so
// its process group is signalled only while its id cannot have been reused.
type process struct {
	pid    int
	exited chan struct{} // closed once the child has exited

	mu   sync.Mutex
	done bool
}

// startProcess watches pid until it exits. The child stays a zombie, holding
// its pid and group id, until the caller reaps it after exited is closed.
func startProcess(pid int) *process {
	p := &process{pid: pid, exited: make(chan struct{})}
	go func() {
		waitExit(pid)
		p.mu.Lock()
		p.done = true
		p.mu.Unlock()
		close(p.exited)
	}()
	return p
}

// signal sends sig to the process group while the leader has not exited; it
// reports whether it did.
func (p *process) signal(sig syscall.Signal) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return false
	}
	tidy.Log(nil, "signal script group", syscall.Kill(-p.pid, sig), "pid", p.pid, "signal", sig.String())
	return true
}

// callOnStart runs onStart and reports false when it panicked.
func callOnStart(onStart func(int), pid int) (ok bool) {
	if onStart == nil {
		return true
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	onStart(pid)
	return true
}

// ownerDir creates dir, and narrows it to 0700 if it already existed.
func ownerDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// writeOwnerFile writes data to path with mode 0600, never through a symlink.
func writeOwnerFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		tidy.Close(nil, f)
		return err
	}
	if _, err := f.Write(data); err != nil {
		tidy.Close(nil, f)
		return err
	}
	return f.Close()
}

// readResultFile reads the result a script wrote. A missing file is no result;
// a symlink, a non-regular file, or one larger than MaxResultBytes is an error.
func readResultFile(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("result file is not a regular file (%v)", info.Mode().Type())
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxResultBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxResultBytes {
		return nil, fmt.Errorf("result file is larger than %d bytes", MaxResultBytes)
	}
	return raw, nil
}
