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

// SUPER-224 made daemon startup rewrite *every* agent's shims, so a ToolsBin
// that does not exist turns one start into a fleet-wide breakage that only
// surfaces at exec time inside an iteration. A missing client must leave the
// existing shims byte-for-byte alone: a working old shim beats a certainly
// dead new one. Startup itself still succeeds.
func TestRefreshShimsKeepsShimsWhenToolsBinIsMissing(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	var logs bytes.Buffer
	m.cfg.Log = slog.New(slog.NewTextHandler(&logs, nil))
	gone := filepath.Join(t.TempDir(), "tariboy-tools")
	m.cfg.ToolsBin = gone

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
		t.Fatalf("StartAll failed over a missing tools binary: %v", err)
	}

	for f, want := range before {
		if got := []byte(readShim(t, l, f)); !bytes.Equal(got, want) {
			t.Fatalf("%s/%s was rewritten against a missing client:\nbefore: %s\nafter:  %s", l.Name, f, want, got)
		}
	}
	if !strings.Contains(logs.String(), gone) {
		t.Fatalf("missing tools binary was not logged with its path %q: %s", gone, logs.String())
	}
}

// The guard above must not disable the SUPER-224 fix itself: when the client is
// there, a stale shim is still repointed at it.
func TestRefreshShimsRewritesShimsWhenToolsBinExists(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	live := filepath.Join(t.TempDir(), "tariboy-tools")
	if err := os.WriteFile(live, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	m.cfg.ToolsBin = live

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
		if !strings.Contains(got, live) {
			t.Fatalf("%s/%s does not exec the live client %q: %s", l.Name, f, live, got)
		}
	}
}
