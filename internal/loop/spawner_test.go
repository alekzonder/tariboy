package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// zombieChildren counts direct children of the test process left in state Z —
// exactly what `ps -o stat` reports for an exited-but-unreaped child.
func zombieChildren(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("ps", "-e", "-o", "pid=,ppid=,stat=").Output()
	if err != nil {
		t.Fatalf("ps: %v", err)
	}
	self := os.Getpid()
	n := 0
	for line := range strings.SplitSeq(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		if ppid, err := strconv.Atoi(f[1]); err != nil || ppid != self {
			continue
		}
		if strings.HasPrefix(f[2], "Z") {
			n++
		}
	}
	return n
}

func waitForCond(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A shim that exits must not linger as a zombie: without an explicit wait the
// daemon accumulates one dead child per iteration for its whole uptime.
func TestExecSpawnerReapsExitedShim(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("process states are POSIX-specific")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	baseline := zombieChildren(t)
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "exited")

	if _, err := (ExecSpawner{}).StartManaged(
		[]string{sh, "-c", "echo done > " + sentinel}, os.Environ(), dir); err != nil {
		t.Fatalf("StartManaged: %v", err)
	}

	waitForCond(t, "spawned process to run", func() bool {
		_, err := os.Stat(sentinel)
		return err == nil
	})
	waitForCond(t, "spawned process to be reaped", func() bool {
		return zombieChildren(t) <= baseline
	})
}

// Start (the unmanaged path) delegates to StartManaged and drops the terminate
// handle, so it must reap too.
func TestExecSpawnerStartReapsExitedShim(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("process states are POSIX-specific")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	baseline := zombieChildren(t)
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "exited")

	if err := (ExecSpawner{}).Start(
		[]string{sh, "-c", "echo done > " + sentinel}, os.Environ(), dir); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitForCond(t, "spawned process to run", func() bool {
		_, err := os.Stat(sentinel)
		return err == nil
	})
	waitForCond(t, "spawned process to be reaped", func() bool {
		return zombieChildren(t) <= baseline
	})
}

// terminate must not signal the process group once the shim has been reaped:
// the pgid is free for reuse then, and a stray kill would hit an unrelated
// group. It reports os.ErrProcessDone instead, as it already does for ESRCH.
func TestExecSpawnerTerminateAfterReapReportsDone(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("process states are POSIX-specific")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	baseline := zombieChildren(t)
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "exited")

	terminate, err := (ExecSpawner{}).StartManaged(
		[]string{sh, "-c", "echo done > " + sentinel}, os.Environ(), dir)
	if err != nil {
		t.Fatalf("StartManaged: %v", err)
	}
	waitForCond(t, "spawned process to run", func() bool {
		_, err := os.Stat(sentinel)
		return err == nil
	})
	waitForCond(t, "spawned process to be reaped", func() bool {
		return zombieChildren(t) <= baseline
	})

	if err := terminate(); err != os.ErrProcessDone {
		t.Fatalf("terminate after reap = %v, want %v", err, os.ErrProcessDone)
	}
}
