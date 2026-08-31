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
	"github.com/alekzonder/tariboy/internal/agentdir"
)

// writeStaleShims fakes an agent dir provisioned by an older daemon: the bin
// shims are pinned to a release path that no longer exists.
func writeStaleShims(t *testing.T, agentsDir, name string) agentdir.Layout {
	t.Helper()
	l := agentdir.New(agentsDir, name)
	if err := os.MkdirAll(l.BinDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"tools", "i-am-done", "tasks"} {
		body := "#!/usr/bin/env bash\nexec python3 \"/opt/tariboy/0.21.6/skills/agent-tools/scripts/tools.py\" \"$@\"\n"
		if err := os.WriteFile(filepath.Join(l.BinDir(), f), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func readShim(t *testing.T, l agentdir.Layout, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(l.BinDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The frozen-client bug (SUPER-224): a daemon upgrade left every agent execing
// the tools binary of whatever release provisioned it. StartAll must repoint
// them at the running daemon's client — including agents that are disabled.
func TestStartAllRefreshesShimsForEveryAgent(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)

	on := agent.Agent{
		Name: "runner", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: true, LoopEnabled: false, Plugins: []string{"loop", "tasks"},
	}
	off := agent.Agent{
		Name: "parked", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: false, LoopEnabled: false, Plugins: []string{"loop", "tasks"},
	}
	for _, a := range []agent.Agent{on, off} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	lOn := writeStaleShims(t, agentsDir, on.Name)
	lOff := writeStaleShims(t, agentsDir, off.Name)

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, l := range []agentdir.Layout{lOn, lOff} {
		for _, f := range []string{"tools", "i-am-done", "tasks"} {
			got := readShim(t, l, f)
			if strings.Contains(got, "0.21.6") {
				t.Fatalf("%s/%s still pinned to the provisioning release: %s", l.Name, f, got)
			}
			if !strings.Contains(got, m.cfg.SkillsDir) {
				t.Fatalf("%s/%s does not exec a live skill script from %q: %s", l.Name, f, m.cfg.SkillsDir, got)
			}
		}
	}
}

func TestStartAllDoesNotStartPersistedAgentsWithoutPython3(t *testing.T) {
	runner := &fakeRunner{}
	m, as, agentsDir, _ := newManager(t, runner)
	t.Cleanup(m.Shutdown)
	a := agent.Agent{
		Name: "runner", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: true, LoopEnabled: false, Plugins: []string{"loop", "tasks"},
	}
	if err := as.Create(a); err != nil {
		t.Fatal(err)
	}
	writeStaleShims(t, agentsDir, a.Name)
	t.Setenv("PATH", t.TempDir())

	err := m.StartAll(context.Background())
	if err == nil || !strings.Contains(err.Error(), "python3") {
		t.Fatalf("StartAll error = %v, want missing python3", err)
	}
	m.mu.Lock()
	_, started := m.runs[a.Name]
	m.mu.Unlock()
	if started {
		t.Fatal("persisted agent started without python3")
	}
}

// One unwritable agent dir must not take the daemon down or stop the other
// agents from being refreshed; the failure is logged against the agent name.
func TestStartAllSurvivesOneUnwritableAgentDir(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	var logs bytes.Buffer
	m.cfg.Log = slog.New(slog.NewTextHandler(&logs, nil))

	broken := agent.Agent{
		Name: "broken", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: false, Plugins: []string{"loop"},
	}
	fine := agent.Agent{
		Name: "fine", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: false, Plugins: []string{"loop"},
	}
	for _, a := range []agent.Agent{broken, fine} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	// "broken" has a *file* where its bin dir belongs, so every shim write fails.
	lBroken := agentdir.New(agentsDir, broken.Name)
	if err := os.MkdirAll(lBroken.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lBroken.BinDir(), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	lFine := writeStaleShims(t, agentsDir, fine.Name)

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll failed because of one bad agent dir: %v", err)
	}
	if got := readShim(t, lFine, "tools"); !strings.Contains(got, m.cfg.SkillsDir) {
		t.Fatalf("healthy agent was skipped after the broken one: %s", got)
	}
	if !strings.Contains(logs.String(), "broken") {
		t.Fatalf("shim refresh failure was not logged with the agent name: %s", logs.String())
	}
}
