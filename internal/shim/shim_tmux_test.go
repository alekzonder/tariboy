//go:build tmux

// Tests in this file drive a real tmux server. They compile only with
// `go test -tags tmux` and still skip when no tmux binary is installed.

package shim

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/testutil"
)

// buildShimBinary builds cmd/tariboy-shim into dir. The go command gets the
// real HOME back so it reuses the user's build and module caches.
func buildShimBinary(t *testing.T, dir string) string {
	t.Helper()
	shimBin := filepath.Join(dir, "tariboy-shim")
	build := exec.Command("go", "build", "-o", shimBin, "./cmd/tariboy-shim")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "HOME="+realHome)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build tariboy-shim: %v\n%s", err, output)
	}
	return shimBin
}

func TestKillTmuxSessionTerminatesAllPanesAndPreservesPrefixedSession(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	tmuxTmp, err := os.MkdirTemp("/tmp", "tariboy-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmuxTmp) })
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", tmuxTmp)

	dir := t.TempDir()
	shimBin := buildShimBinary(t, dir)
	harness := filepath.Join(dir, "bounded-harness.sh")
	// Each process ignores the signals before it writes its marker, so Kill
	// sent once all markers exist cannot end the leader through TERM.
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
trap '' TERM HUP INT
printf '%s\n' "$$" > "$1"
/bin/sh -c 'trap '\'''\'' TERM HUP INT; printf '\''%s\n'\'' "$$" > "$1"; i=0; while [ "$i" -lt 120 ]; do /bin/sleep 0.05; i=$((i + 1)); done' child "$2" &
wait
`), 0o700); err != nil {
		t.Fatal(err)
	}
	managed, decoy := "shim-kill", "shim-kill-decoy"
	markers := []string{
		filepath.Join(dir, "managed-1.pid"), filepath.Join(dir, "managed-1-child.pid"),
		filepath.Join(dir, "managed-2.pid"), filepath.Join(dir, "managed-2-child.pid"),
	}
	statuses := []string{filepath.Join(dir, "managed-1.status"), filepath.Join(dir, "managed-2.status")}
	runTmux := func(args ...string) error { return exec.Command(tmux, args...).Run() }
	supervisorBinDir := t.TempDir()
	supervisorTmuxCalled := filepath.Join(dir, "supervisor-tmux-called")
	if err := os.WriteFile(filepath.Join(supervisorBinDir, "tmux"), []byte("#!/bin/sh\nprintf called > \"$SHIM_TEST_SUPERVISOR_TMUX_CALLED\"\nexit 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	supervisorCommand := func(status string, paneMarkers []string) string {
		return shellJoin([]string{"/usr/bin/env", "PATH=" + supervisorBinDir, "SHIM_TEST_SUPERVISOR_TMUX_CALLED=" + supervisorTmuxCalled, shimBin, "__tmux-supervisor", managed, status, "--", harness, paneMarkers[0], paneMarkers[1]})
	}
	readPID := func(marker string) (int, bool) {
		data, err := os.ReadFile(marker)
		if err != nil {
			return 0, false
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		return pid, err == nil && pid > 1
	}
	var pids []int
	decoyPID := 0
	t.Cleanup(func() {
		_ = runTmux("kill-session", "-t", "="+managed)
		_ = runTmux("kill-session", "-t", "="+decoy)
		_ = runTmux("kill-server")
		cleanupPIDs := append([]int(nil), pids...)
		if decoyPID > 1 {
			cleanupPIDs = append(cleanupPIDs, decoyPID)
		}
		deadline := time.Now().Add(9 * time.Second)
		for _, pid := range cleanupPIDs {
			if !testutil.WaitFor(time.Until(deadline), func() bool { return syscall.Kill(pid, 0) != nil }) {
				t.Errorf("bounded owned process %d survived cleanup", pid)
			}
		}
	})
	if err := runTmux("new-session", "-d", "-s", managed, supervisorCommand(statuses[0], markers[:2])); err != nil {
		t.Fatal(err)
	}
	if err := runTmux("split-window", "-d", "-t", "="+managed+":", supervisorCommand(statuses[1], markers[2:])); err != nil {
		t.Fatal(err)
	}
	if err := runTmux("new-session", "-d", "-s", decoy, "/bin/sleep 8"); err != nil {
		t.Fatal(err)
	}

	for _, marker := range markers {
		var pid int
		if !testutil.WaitFor(3*time.Second, func() bool {
			var ok bool
			pid, ok = readPID(marker)
			return ok
		}) {
			t.Fatalf("pane did not write owned marker %s", marker)
		}
		pids = append(pids, pid)
	}
	output, err := exec.Command(tmux, "list-panes", "-t", "="+managed, "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range strings.Fields(string(output)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 1 {
			t.Fatalf("invalid supervisor pid %q", field)
		}
		pids = append(pids, pid)
	}
	decoyOutput, err := exec.Command(tmux, "list-panes", "-t", "="+decoy, "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	decoyPID, err = strconv.Atoi(strings.TrimSpace(string(decoyOutput)))
	if err != nil || decoyPID <= 1 {
		t.Fatalf("invalid decoy pid %q", decoyOutput)
	}

	if err := KillTmuxSession(managed); err != nil {
		t.Fatal(err)
	}
	for _, pid := range pids {
		if !testutil.WaitFor(3*time.Second, func() bool { return syscall.Kill(pid, 0) != nil }) {
			t.Fatalf("managed supervisor or harness process %d survived Kill", pid)
		}
	}
	for _, statusPath := range statuses {
		status, err := os.ReadFile(statusPath)
		if err != nil || string(status) != "137\n" {
			t.Fatalf("supervisor status = %q, error = %v; want 137", status, err)
		}
	}
	if _, err := os.Stat(supervisorTmuxCalled); !os.IsNotExist(err) {
		t.Fatalf("pane supervisor invoked tmux: %v", err)
	}
	if err := KillTmuxSession(managed); err != nil {
		t.Fatalf("second Kill of absent exact session: %v", err)
	}
	if err := runTmux("has-session", "-t", "="+decoy); err != nil {
		t.Fatal("Kill of absent managed session removed prefixed decoy")
	}
	if err := syscall.Kill(decoyPID, 0); err != nil {
		t.Fatal("prefixed decoy pane process was killed")
	}
}

func TestTmuxSupervisorGivesHarnessForegroundTerminal(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", t.TempDir())

	dir := t.TempDir()
	shimBin := buildShimBinary(t, dir)
	harness := filepath.Join(dir, "interactive-harness.sh")
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
printf '%s\n' "$$" > "$1"
trap 'printf int > "$3"; exit 23' INT
trap 'printf ttin > "$4"; exit 24' TTIN
IFS= read -r line
printf '%s\n' "$line" > "$2"
i=0
while [ "$i" -lt 120 ]; do /bin/sleep 0.05; i=$((i + 1)); done
`), 0o700); err != nil {
		t.Fatal(err)
	}
	session := "shim-foreground"
	pidPath := filepath.Join(dir, "harness.pid")
	inputPath := filepath.Join(dir, "input")
	intPath := filepath.Join(dir, "int")
	ttinPath := filepath.Join(dir, "ttin")
	statusPath := filepath.Join(dir, "harness.status")
	supervisorErrPath := filepath.Join(dir, "supervisor.stderr")
	runTmux := func(args ...string) error { return exec.Command(tmux, args...).Run() }

	var pids []int
	t.Cleanup(func() {
		if output, err := exec.Command(tmux, "list-panes", "-t", "="+session, "-F", "#{pane_pid}").Output(); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(output))); err == nil && pid > 1 {
				pids = append(pids, pid)
			}
		}
		_ = runTmux("kill-session", "-t", "="+session)
		_ = runTmux("kill-server")
		deadline := time.Now().Add(8 * time.Second)
		for _, pid := range pids {
			if !testutil.WaitFor(time.Until(deadline), func() bool { return syscall.Kill(pid, 0) != nil }) {
				t.Errorf("bounded interactive process %d survived cleanup", pid)
			}
		}
	})
	command := shellJoin([]string{shimBin, TmuxSupervisorMode, session, statusPath, "--", harness, pidPath, inputPath, intPath, ttinPath}) + " 2>" + shellQuote(supervisorErrPath)
	if err := runTmux("new-session", "-d", "-s", session, command); err != nil {
		t.Fatal(err)
	}

	readPID := func(path string) int {
		t.Helper()
		pid := waitForPIDFile(path, 2*time.Second)
		if pid <= 1 {
			t.Fatalf("missing valid pid marker %s", path)
		}
		return pid
	}
	harnessPID := readPID(pidPath)
	pids = append(pids, harnessPID)
	supervisorOutput, err := exec.Command(tmux, "list-panes", "-t", "="+session, "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	supervisorPID, err := strconv.Atoi(strings.TrimSpace(string(supervisorOutput)))
	if err != nil || supervisorPID <= 1 || supervisorPID == harnessPID {
		t.Fatalf("supervisor pid = %q, harness pid = %d", supervisorOutput, harnessPID)
	}
	pids = append(pids, supervisorPID)
	paneTarget := "=" + session + ":"

	sendErr := runTmux("send-keys", "-t", paneTarget, "-l", "--", "typed input")
	if sendErr == nil {
		sendErr = runTmux("send-keys", "-t", paneTarget, "Enter")
	}
	testutil.WaitFor(2*time.Second, func() bool {
		data, err := os.ReadFile(inputPath)
		return err == nil && string(data) == "typed input\n"
	})
	if data, err := os.ReadFile(inputPath); err != nil || string(data) != "typed input\n" {
		ttin, _ := os.ReadFile(ttinPath)
		supervisorErr, _ := os.ReadFile(supervisorErrPath)
		status, _ := os.ReadFile(statusPath)
		pane, _ := exec.Command(tmux, "list-panes", "-t", "="+session, "-F", "dead=#{pane_dead} status=#{pane_dead_status} signal=#{pane_dead_signal}").Output()
		t.Fatalf("harness did not read pane input (input=%q, ttin=%q, status=%q, read_error=%v, send_error=%v, supervisor_error=%q, pane=%q)", data, ttin, status, err, sendErr, supervisorErr, pane)
	}
	if syscall.Kill(supervisorPID, 0) != nil {
		t.Fatal("supervisor exited before its harness")
	}
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Fatalf("supervisor wrote status before harness exit: %v", err)
	}

	if err := runTmux("send-keys", "-t", paneTarget, "C-c"); err != nil {
		t.Fatal(err)
	}
	if testutil.WaitFor(2*time.Second, func() bool {
		data, statusErr := os.ReadFile(statusPath)
		_, intErr := os.Stat(intPath)
		return intErr == nil && statusErr == nil && string(data) == "23\n"
	}) {
		return
	}
	data, statusErr := os.ReadFile(statusPath)
	_, intErr := os.Stat(intPath)
	t.Fatalf("terminal C-c did not reach harness (status=%q, status_error=%v, int_error=%v)", data, statusErr, intErr)
}

func TestTmuxOperationsPreservePrefixedSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	root, err := os.MkdirTemp("/tmp", "tariboy-exact-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", root)
	t.Setenv("TARIBOY_BASE_DIR", filepath.Join(root, "base"))
	t.Setenv("TARIBOY_RUNTIME_DIR", filepath.Join(root, "runtime"))
	defer exec.Command("tmux", "kill-server").Run()
	managed, decoy := "image-creator", "image-creator-sam"
	if out, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-s", decoy, "printf DECOY; exec /bin/cat").CombinedOutput(); err != nil {
		t.Fatalf("decoy: %v: %s", err, out)
	}
	if TmuxHasSession(managed) || !TmuxHasSession(decoy) {
		t.Fatal("session lookup matched prefix or missed exact name")
	}
	s := &tmuxShim{session: managed, harnessWindow: true}
	t.Run("missing screen", func(t *testing.T) {
		if out, err := s.Screen(); err == nil {
			t.Fatalf("captured absent session: %q", out)
		}
	})
	t.Run("missing input", func(t *testing.T) {
		if err := s.SendKeys(SendKeysParams{Keys: "WRONG"}); err == nil {
			t.Fatal("sent input to absent session")
		}
	})
	t.Run("fallback", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		ticks := make(chan time.Time, 1)
		ticks <- time.Now()
		if got := waitTmuxEnd(ctx, managed, nil, ticks); got != tmuxEndSessionGone {
			t.Fatalf("fallback = %s, want session_gone", got)
		}
	})
	bin := buildShimBinary(t, root)
	iter := filepath.Join(root, "iteration")
	if err := os.MkdirAll(filepath.Join(iter, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--iteration-dir", iter, "--agent", managed, "--iteration-id", "exact-1", "--tmux-session", managed, "--", "/bin/sh", "-c", "printf MANAGED; IFS= read -r input; [ \"$input\" = FINISH ]")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill(); <-done }()
	if !testutil.WaitFor(3*time.Second, func() bool { out, err := s.Screen(); return err == nil && strings.Contains(out, "MANAGED") }) {
		t.Fatal("shim did not start beside prefixed session")
	}
	if err := s.SendKeys(SendKeysParams{Keys: "FINISH"}); err != nil {
		t.Fatal(err)
	}
	if !testutil.WaitFor(3*time.Second, func() bool { _, err := os.Stat(filepath.Join(iter, "result.json")); return err == nil }) {
		t.Fatal("shim did not finish while prefixed session remained")
	}
	if err := s.Kill(); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("tmux", "capture-pane", "-p", "-t", "=image-creator-sam:").Output()
	if err != nil || !strings.Contains(string(out), "DECOY") || strings.Contains(string(out), "WRONG") || strings.Contains(string(out), "FINISH") {
		t.Fatalf("decoy affected: %q, %v", out, err)
	}
}
