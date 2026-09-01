package loop

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
)

// Missing required direct scripts skip only the affected agent; a stale shim is
// safer than replacing it with a dead one, and another agent may still refresh.
func TestRefreshShimsSkipsAgentMissingRequiredDirectScript(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	var logs bytes.Buffer
	m.cfg.Log = slog.New(slog.NewTextHandler(&logs, nil))
	m.cfg.SkillsDir = testSkillsDir(t)
	if err := os.Remove(filepath.Join(m.cfg.SkillsDir, "loop", "scripts", "loop.sh")); err != nil {
		t.Fatal(err)
	}

	stale := agent.Agent{
		Name: "stale", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: false, LoopEnabled: false, Plugins: []string{"loop", "tasks"},
	}
	healthy := agent.Agent{
		Name: "healthy", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: false, LoopEnabled: false, Plugins: []string{"tasks"},
	}
	for _, a := range []agent.Agent{stale, healthy} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	l := writeStaleShims(t, agentsDir, stale.Name)
	before := map[string][]byte{}
	for _, f := range []string{"tools", "i-am-done", "tasks"} {
		before[f] = []byte(readShim(t, l, f))
	}
	lHealthy := writeStaleShims(t, agentsDir, healthy.Name)

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll failed over a missing direct skill script: %v", err)
	}

	for f, want := range before {
		if got := []byte(readShim(t, l, f)); !bytes.Equal(got, want) {
			t.Fatalf("%s/%s was rewritten against a missing client:\nbefore: %s\nafter:  %s", l.Name, f, want, got)
		}
	}
	if got := readShim(t, lHealthy, "tasks"); !strings.Contains(got, filepath.Join(m.cfg.SkillsDir, "tasks/scripts/tasks.sh")) {
		t.Fatalf("healthy agent did not refresh against its direct script: %s", got)
	}
	if _, err := os.Stat(filepath.Join(lHealthy.BinDir(), "tools")); !os.IsNotExist(err) {
		t.Fatalf("healthy agent kept managed legacy tools shim: %v", err)
	}
	if !strings.Contains(logs.String(), stale.Name) {
		t.Fatalf("missing direct script was not logged with its agent name: %s", logs.String())
	}
}

func TestStartAllSkipsEnabledAgentMissingRequiredDirectScript(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	m.cfg.SkillsDir = testSkillsDir(t)
	if err := os.Remove(filepath.Join(m.cfg.SkillsDir, "loop", "scripts", "loop.sh")); err != nil {
		t.Fatal(err)
	}

	blocked := agent.Agent{Name: "blocked", ImageRef: "basic:latest", HarnessType: "stub", Enabled: true, Plugins: []string{"loop"}}
	healthy := agent.Agent{Name: "healthy", ImageRef: "basic:latest", HarnessType: "stub", Enabled: true, Plugins: []string{"tasks"}}
	for _, a := range []agent.Agent{blocked, healthy} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
		writeStaleShims(t, agentsDir, a.Name)
	}

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	_, blockedStarted := m.runs[blocked.Name]
	_, healthyStarted := m.runs[healthy.Name]
	m.mu.Unlock()
	if blockedStarted {
		t.Fatal("enabled agent started without its required direct launcher")
	}
	if !healthyStarted {
		t.Fatal("healthy enabled agent did not start")
	}
}

// Direct scripts, not the removed central dispatcher, are the startup
// prerequisite for a stale shim refresh.
func TestRefreshShimsRewritesShimsWithDirectScripts(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	live := testSkillsDir(t)
	m.cfg.SkillsDir = live

	a := agent.Agent{
		Name: "stale", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: false, LoopEnabled: false, Plugins: []string{"loop", "tasks"},
	}
	if err := as.Create(a); err != nil {
		t.Fatal(err)
	}
	l := writeStaleShims(t, agentsDir, a.Name)

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"i-am-done", "tasks"} {
		got := readShim(t, l, f)
		if strings.Contains(got, "0.21.6") {
			t.Fatalf("%s/%s still pinned to the provisioning release: %s", l.Name, f, got)
		}
		if !strings.Contains(got, filepath.Join(live, map[string]string{
			"i-am-done": "loop/scripts/loop.sh", "tasks": "tasks/scripts/tasks.sh",
		}[f])) {
			t.Fatalf("%s/%s does not exec a script from %q: %s", l.Name, f, live, got)
		}
	}
	if _, err := os.Stat(filepath.Join(l.BinDir(), "tools")); !os.IsNotExist(err) {
		t.Fatalf("managed legacy tools shim survived refresh: %v", err)
	}
}
