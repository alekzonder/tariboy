package loop

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/script"
)

// fifoShim replaces an agent's direct loop shim with a named pipe, so the shim
// refresh for that agent blocks until the test opens the other end. It is the
// lever that makes StartAll's ordering observable: whatever the daemon does
// after the refresh cannot happen while this agent is being refreshed.
func fifoShim(t *testing.T, agentsDir, name string) string {
	t.Helper()
	l := agentdir.New(agentsDir, name)
	if err := os.MkdirAll(l.BinDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(l.BinDir(), "i-am-done")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// unblockFifoShim releases a shim refresh parked on the pipe: writeShim first
// reads the current content (open for read parks until a writer shows up), then
// writes the new body (open for write parks until a reader shows up).
func unblockFifoShim(t *testing.T, path string) {
	t.Helper()
	w, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Error(err)
		return
	}
	w.Close()
	r, err := os.Open(path)
	if err != nil {
		t.Error(err)
		return
	}
	io.Copy(io.Discard, r)
	r.Close()
}

func waitForFile(path string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// Ordering half of SUPER-224: refreshing the shims must happen before the
// script supervisor is started, not merely before the engines. The supervisor's
// first scan runs immediately, ahead of its first tick, and startScript prepends
// the agent's bin dir to PATH — so a script already due at daemon start is the
// one client call that can still hit a frozen shim.
//
// "blocker" sorts before "worker" (Store.List is ORDER BY name), and its loop
// shim is a pipe, so refreshShims parks there with "worker" still stale. If the
// supervisor is started first, its due script runs during that window and copies
// the stale shim; with the refresh first, the supervisor cannot start until the
// test unparks the pipe, by which point "worker" is repointed at the live client.
func TestStartAllRefreshesShimsBeforeScriptSupervisorStarts(t *testing.T) {
	m, as, agentsDir, raw := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	st := script.NewStore(raw, time.Now)
	m.cfg.Scripts = st

	workdir := t.TempDir()
	for _, a := range []agent.Agent{
		{Name: "blocker", ImageRef: "basic:latest", Plugins: []string{"loop", "tasks"}},
		{Name: "worker", ImageRef: "basic:latest", Cwd: workdir, Plugins: []string{"loop", "tasks"}},
	} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	fifo := fifoShim(t, agentsDir, "blocker")
	lWorker := writeStaleShims(t, agentsDir, "worker")

	// The due script records the direct loop shim before its refresh.
	snapshot := filepath.Join(workdir, "seen-shim")
	_, rec, err := st.CreateOnce("worker", script.CreateOnce{
		Name: "due", Description: "test",
		Command: "cat " + filepath.Join(lWorker.BinDir(), "i-am-done") + " > " + snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Park until the script has had its chance to run (broken order) or until
	// the grace period is up (correct order: the supervisor is not running yet,
	// so nothing will ever appear and StartAll stays parked on the pipe).
	ranWhileRefreshing := make(chan bool, 1)
	go func() {
		ranWhileRefreshing <- waitForFile(snapshot, 2*time.Second)
		unblockFifoShim(t, fifo)
	}()

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if <-ranWhileRefreshing {
		t.Fatalf("script supervisor started a due script while the shims were still being refreshed")
	}
	awaitScriptRun(t, st, "worker", rec.ID, func(r script.Run) bool { return r.Status == script.RunSucceeded })

	seen, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(seen), m.cfg.SkillsDir) {
		t.Fatalf("due script ran against a shim that does not exec a live skill script from %q: %s", m.cfg.SkillsDir, seen)
	}
	if strings.Contains(string(seen), "0.21.6") {
		t.Fatalf("due script ran against the shim pinned to the provisioning release: %s", seen)
	}
}
