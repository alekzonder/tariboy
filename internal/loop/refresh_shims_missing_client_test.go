package loop

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
)

// SUPER-224 made daemon startup rewrite *every* agent's shims, so a skills
// directory without the dispatcher turns one start into a fleet-wide breakage that only
// surfaces at exec time inside an iteration. A missing client must leave the
// existing shims byte-for-byte alone: a working old shim beats a certainly
// dead new one. Startup itself still succeeds.
func TestRefreshShimsKeepsShimsWhenDispatcherIsMissing(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	var logs bytes.Buffer
	m.cfg.Log = slog.New(slog.NewTextHandler(&logs, nil))
	gone := t.TempDir()
	m.cfg.SkillsDir = gone

	a := agent.Agent{
		Name: "stale", ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: false, LoopEnabled: false, Plugins: []string{"loop", "tasks"},
	}
	if err := as.Create(a); err != nil {
		t.Fatal(err)
	}
	l := writeStaleShims(t, agentsDir, a.Name)
	before := map[string][]byte{}
	for _, f := range []string{"tools", "i-am-done", "tasks"} {
		before[f] = []byte(readShim(t, l, f))
	}

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll failed over a missing tools dispatcher: %v", err)
	}

	for f, want := range before {
		if got := []byte(readShim(t, l, f)); !bytes.Equal(got, want) {
			t.Fatalf("%s/%s was rewritten against a missing client:\nbefore: %s\nafter:  %s", l.Name, f, want, got)
		}
	}
	if !strings.Contains(logs.String(), gone) {
		t.Fatalf("missing skills directory was not logged with its path %q: %s", gone, logs.String())
	}
}

// The guard above must not disable the SUPER-224 fix itself: when the client is
// there, a stale shim is still repointed at it.
func TestRefreshShimsRewritesShimsWhenDispatcherExists(t *testing.T) {
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

	for _, f := range []string{"tools", "i-am-done", "tasks"} {
		got := readShim(t, l, f)
		if strings.Contains(got, "0.21.6") {
			t.Fatalf("%s/%s still pinned to the provisioning release: %s", l.Name, f, got)
		}
		if !strings.Contains(got, filepath.Join(live, map[string]string{
			"tools": "agent-tools/scripts/tools.py", "i-am-done": "loop/scripts/loop.py", "tasks": "tasks/scripts/tasks.py",
		}[f])) {
			t.Fatalf("%s/%s does not exec a script from %q: %s", l.Name, f, live, got)
		}
	}
}
