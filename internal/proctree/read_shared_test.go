//go:build linux || darwin

package proctree

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// testReadFindsShellCommand starts a real shell child of the test process and
// expects the rule to find it, then a real non-shell child and expects the
// rule to ignore it.
func testReadFindsShellCommand(t *testing.T, shellScript string) {
	self := os.Getpid()

	shell := startChild(t, "sh", "-c", shellScript)
	if !eventually(func() bool { return agentCommandOf(t, self) }) {
		t.Fatal("live sh child of the test process is not an agent command")
	}
	procs, err := Read([]int{self})
	if err != nil {
		t.Fatal(err)
	}
	if args := argsOf(procs, shell.Process.Pid); len(args) < 3 || args[1] != "-c" || args[2] != shellScript {
		t.Fatalf("shell args = %q, want [sh -c %q]", args, shellScript)
	}
	stopChild(shell)

	startChild(t, "sleep", "30")
	if agentCommandOf(t, self) {
		t.Fatal("non-shell sleep child counted as an agent command")
	}
}

func agentCommandOf(t *testing.T, pid int) bool {
	t.Helper()
	procs, err := Read([]int{pid})
	if err != nil {
		t.Fatal(err)
	}
	return AgentCommand(procs, pid, nil)
}

func argsOf(procs []Proc, pid int) []string {
	for _, p := range procs {
		if p.PID == pid {
			return p.Args
		}
	}
	return nil
}

func startChild(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopChild(cmd) })
	return cmd
}

func stopChild(cmd *exec.Cmd) {
	if cmd.ProcessState == nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// eventually waits for exec to replace the forked test binary image.
func eventually(cond func() bool) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return true
		}
	}
	return false
}
