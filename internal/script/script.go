// Package script owns durable, per-agent local script definitions and runs.
package script

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"time"
)

const killWaitDelay = 2 * time.Second

// Execute executes a command via sh -c, captures combined output, and
// kills the complete process group on cancellation.
func Execute(ctx context.Context, cwd string, env []string, command string, timeout time.Duration) (string, int, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	cmd.Dir = cwd
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = killWaitDelay
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return "", -1, err
	}
	err := cmd.Wait()
	if cctx.Err() == context.DeadlineExceeded {
		return output.String(), -1, cctx.Err()
	}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return output.String(), exitError.ExitCode(), nil
		}
		return output.String(), -1, err
	}
	return output.String(), 0, nil
}
