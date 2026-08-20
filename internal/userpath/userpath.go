package userpath

import (
	"bytes"
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
)

const (
	probeTimeout   = 5 * time.Second
	pipeWaitDelay  = 100 * time.Millisecond
	maxProbeOutput = 64 << 10
	probeMarker    = "TARIBOY_PATH"
	probeCommand   = "printf '\\000%s\\000%s\\000%s\\000' 'TARIBOY_PATH' \"$PATH\" 'TARIBOY_PATH'"
)

var (
	errInvalidShell    = errors.New("invalid shell")
	errProbeFailed     = errors.New("probe failed")
	errProbeTimedOut   = errors.New("probe timed out")
	errOutputTooLarge  = errors.New("output too large")
	errDelimiterAbsent = errors.New("delimiter missing")
	errEmptyResult     = errors.New("empty result")
	errInvalidResult   = errors.New("invalid result")
)

// Reason names the class of a Resolve failure in one stable, disclosure-free
// token, so an operator log can say why the account PATH was not adopted without
// carrying the shell's startup output, the candidate path, or the raw message.
// It returns "" for a nil error and "unknown" for anything unrecognised.
func Reason(err error) string {
	for _, known := range []struct {
		sentinel error
		reason   string
	}{
		{errInvalidShell, "invalid_shell"},
		{errProbeTimedOut, "timeout"},
		{errProbeFailed, "probe_failed"},
		{errOutputTooLarge, "output_too_large"},
		{errDelimiterAbsent, "delimiter_missing"},
		{errEmptyResult, "empty_result"},
		{errInvalidResult, "invalid_result"},
	} {
		if errors.Is(err, known.sentinel) {
			return known.reason
		}
	}
	if err == nil {
		return ""
	}
	return "unknown"
}

// Resolve returns the PATH emitted by shell when run as an interactive login
// shell. Selecting a fallback when resolution fails is the caller's
// responsibility.
func Resolve(ctx context.Context, shell string) (string, error) {
	if err := validateShell(shell); err != nil {
		return "", fmt.Errorf("resolve user path: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	output := newBoundedOutput(maxProbeOutput)
	cmd := exec.CommandContext(probeCtx, shell, "-ilc", probeCommand)
	cmd.Stdin = nil
	cmd.Stdout = output.writer(true)
	cmd.Stderr = output.writer(false)
	cmd.WaitDelay = pipeWaitDelay
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return signalProbeProcessGroup(cmd.Process.Pid)
	}

	runErr := cmd.Start()
	cleanupErr := error(nil)
	if runErr == nil {
		runErr = cmd.Wait()
		cleanupErr = cleanupProbeProcessGroup(cmd.Process.Pid)
	}
	if output.exceeded() {
		return "", fmt.Errorf("resolve user path: %w", errOutputTooLarge)
	}
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) || errors.Is(runErr, exec.ErrWaitDelay) {
		return "", fmt.Errorf("resolve user path: %w", errProbeTimedOut)
	}
	if runErr != nil || cleanupErr != nil {
		return "", fmt.Errorf("resolve user path: %w", errProbeFailed)
	}

	return parseOutput(output.bytes())
}

func signalProbeProcessGroup(pgid int) error {
	if pgid <= 1 || pgid == syscall.Getpgrp() {
		return errors.New("unsafe probe process group")
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}

func cleanupProbeProcessGroup(pgid int) error {
	err := signalProbeProcessGroup(pgid)
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}

	deadline := time.Now().Add(pipeWaitDelay)
	for {
		err = syscall.Kill(-pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("probe process group cleanup timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

func validateShell(shell string) error {
	if !filepath.IsAbs(shell) {
		return errInvalidShell
	}

	info, err := os.Stat(shell)
	if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return errInvalidShell
	}
	return nil
}

func parseOutput(output []byte) (string, error) {
	delimiter := append(append([]byte{0}, probeMarker...), 0)
	start := bytes.Index(output, delimiter)
	if start < 0 {
		return "", fmt.Errorf("resolve user path: %w", errDelimiterAbsent)
	}

	valueAndSuffix := output[start+len(delimiter):]
	end := bytes.Index(valueAndSuffix, delimiter)
	if end < 0 {
		return "", fmt.Errorf("resolve user path: %w", errDelimiterAbsent)
	}

	value := valueAndSuffix[:end]
	if len(value) == 0 {
		return "", fmt.Errorf("resolve user path: %w", errEmptyResult)
	}
	if bytes.IndexByte(value, 0) >= 0 {
		return "", fmt.Errorf("resolve user path: %w", errInvalidResult)
	}
	return string(value), nil
}

type boundedOutput struct {
	mu        sync.Mutex
	remaining int
	tooLarge  bool
	stdout    bytes.Buffer
}

func newBoundedOutput(limit int) *boundedOutput {
	return &boundedOutput{remaining: limit}
}

func (o *boundedOutput) writer(capture bool) io.Writer {
	return boundedWriter{output: o, capture: capture}
}

func (o *boundedOutput) exceeded() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.tooLarge
}

func (o *boundedOutput) bytes() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Clone(o.stdout.Bytes())
}

type boundedWriter struct {
	output  *boundedOutput
	capture bool
}

func (w boundedWriter) Write(p []byte) (int, error) {
	w.output.mu.Lock()
	defer w.output.mu.Unlock()

	if len(p) > w.output.remaining {
		w.output.tooLarge = true
		return 0, errOutputTooLarge
	}

	w.output.remaining -= len(p)
	if w.capture {
		_, _ = w.output.stdout.Write(p)
	}
	return len(p), nil
}
