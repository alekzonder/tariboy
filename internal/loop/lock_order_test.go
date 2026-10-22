package loop

import (
	"fmt"
	"os"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// restartLaunchFixture drives an interactive agent through adoption and a
// Restart, and parks the post-adoption launch just before it takes its launch
// gate. proceed lets the launch continue.
type restartLaunchFixture struct {
	m       *Manager
	runner  *killBlockingRunner
	proceed chan struct{}
}

func newRestartLaunchFixture(t *testing.T) restartLaunchFixture {
	t.Helper()
	r := &killBlockingRunner{started: make(chan string, 1)}
	m, as, agentsDir, _ := newManager(t, r)
	killed := make(chan struct{}, 1)
	id := startPendingAdoption(t, m, as, agentsDir, attachRecordingShim{killed: killed})
	ag, err := as.Get("smoke")
	if err != nil {
		t.Fatal(err)
	}
	ag.IntervalS = 0
	if err := as.Update(ag); err != nil {
		t.Fatal(err)
	}
	launching := make(chan struct{})
	proceed := make(chan struct{})
	m.cfg.BeforeRestartLaunch = func() {
		close(launching)
		<-proceed
	}
	if err := m.Restart("smoke"); err != nil {
		t.Fatalf("Restart adopted agent: %v", err)
	}
	select {
	case <-killed:
	case <-time.After(time.Second):
		t.Fatal("Restart did not kill the adopted shim")
	}
	l := agentdir.New(agentsDir, "smoke").WithRuntime(m.cfg.RuntimeDir)
	if err := os.WriteFile(l.ResultPath(id), []byte(`{"exit_code":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-launching:
	case <-time.After(2 * time.Second):
		t.Fatal("post-adoption Restart did not reach the launch gate")
	}
	return restartLaunchFixture{m: m, runner: r, proceed: proceed}
}

// holdImageStoreLock holds the image store lock for writing until the returned
// release function runs. Release is idempotent.
func holdImageStoreLock(t *testing.T) (release func()) {
	t.Helper()
	held := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = image.WithStoreLock(func() error {
			close(held)
			<-done
			return nil
		})
	}()
	<-held
	var once sync.Once
	release = func() { once.Do(func() { close(done) }) }
	t.Cleanup(release)
	return release
}

// waitForBlockedIn waits until a goroutine whose stack holds every frame is
// blocked, which proves it reached a lock, gate or drain the test holds closed.
func waitForBlockedIn(t *testing.T, frames ...string) {
	t.Helper()
	buf := make([]byte, 1<<20)
	blocked := func() bool {
		n := goruntime.Stack(buf, true)
		for _, g := range strings.Split(string(buf[:n]), "\n\n") {
			header, _, _ := strings.Cut(g, "\n")
			if strings.Contains(header, "[running]") || strings.Contains(header, "[runnable]") {
				continue
			}
			all := true
			for _, frame := range frames {
				all = all && strings.Contains(g, frame)
			}
			if all {
				return true
			}
		}
		return false
	}
	if !testutil.WaitFor(5*time.Second, blocked) {
		t.Fatalf("no goroutine blocked in %s", strings.Join(frames, " > "))
	}
}

// exitOnDeadlock ends the test binary when the manager is deadlocked: a
// deadlocked Manager cannot be shut down, so the registered cleanup would hang
// until the go test timeout instead of reporting the failure.
func exitOnDeadlock(t *testing.T, what string) {
	t.Helper()
	buf := make([]byte, 1<<20)
	n := goruntime.Stack(buf, true)
	fmt.Fprintf(os.Stderr, "--- FAIL: %s: %s\n%s\n", t.Name(), what, buf[:n])
	os.Exit(1)
}

// The post-adoption Restart launch and Reprovision both hold an agent
// lifecycle lock and the image store lock at once. They must take them
// in one order: a launch that holds its lock while waiting for the gate must
// not meet a Reprovision that holds the gate while waiting for that lock.
func TestRestartAfterAdoptionAndReprovisionDoNotDeadlock(t *testing.T) {
	f := newRestartLaunchFixture(t)
	releaseGate := holdImageStoreLock(t)

	reprovisioned := make(chan error, 1)
	go func() { reprovisioned <- f.m.Reprovision("smoke", "") }()
	// Let Reprovision queue first, then the launch.
	waitForBlockedIn(t, "(*Manager).Reprovision")
	close(f.proceed)
	waitForBlockedIn(t, "(*Manager).acquireRestartLaunch")
	releaseGate()

	select {
	case err := <-reprovisioned:
		if err != nil {
			t.Fatalf("Reprovision: %v", err)
		}
	case <-time.After(5 * time.Second):
		exitOnDeadlock(t, "Reprovision deadlocked with the post-adoption Restart launch")
	}
	select {
	case <-f.runner.started:
	case <-time.After(5 * time.Second):
		exitOnDeadlock(t, "post-adoption Restart launch deadlocked with Reprovision")
	}
}

// Image activation can take long (unpack, skill bridge, harness probe). A
// launch that waits in it must not stall the manager's other APIs.
func TestManagerAPIsDoNotWaitForRestartImageActivation(t *testing.T) {
	f := newRestartLaunchFixture(t)
	releaseGate := holdImageStoreLock(t)
	close(f.proceed)
	// The launch now waits for the image store lock inside image activation.
	waitForBlockedIn(t, "(*Manager).activatePendingImage")

	calls := []struct {
		name string
		call func()
	}{
		{"Kill", func() { _ = f.m.Kill("smoke") }},
		{"LiveState", func() { _, _ = f.m.LiveState("smoke") }},
		{"WakeAgents", func() { f.m.WakeAgents([]string{"smoke", "other"}, WakeConfig) }},
		{"currentIterationID", func() { _ = f.m.currentIterationID("smoke") }},
		{"ActiveAgents", func() { _ = f.m.ActiveAgents() }},
	}
	for _, c := range calls {
		done := make(chan struct{})
		go func() {
			c.call()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			releaseGate()
			t.Fatalf("%s waited for the restart launch's image activation", c.name)
		}
	}
	releaseGate()
	select {
	case <-f.runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("post-adoption Restart did not launch after image activation")
	}
}
