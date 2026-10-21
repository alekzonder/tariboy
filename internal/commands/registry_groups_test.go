package commands

import "testing"

func TestBuildRegistryValidates(t *testing.T) {
	t.Parallel()
	r := BuildRegistry()
	if err := r.Validate(); err != nil {
		t.Fatalf("BuildRegistry did not validate: %v", err)
	}
}

func TestRenamedDualNodePaths(t *testing.T) {
	t.Parallel()
	r := BuildRegistry()
	for _, p := range []string{"agent.status.show", "agent.alias.set", "agent.notes.set"} {
		if _, ok := r.Get(p); !ok {
			t.Errorf("expected command %s to exist", p)
		}
	}
	for _, p := range []string{"agent.status", "agent.alias", "agent.notes"} {
		if _, ok := r.Get(p); ok {
			t.Errorf("old dual-node command %s should no longer be a command", p)
		}
		if _, ok := r.Group(p); !ok {
			t.Errorf("expected %s to be a registered group", p)
		}
	}
}

// TestDaemonStatusRunsLocallyAndKeepsItsRoute pins that the CLI answers
// `daemon status` itself while the API keeps GET /api/daemon/status.
func TestDaemonStatusRunsLocallyAndKeepsItsRoute(t *testing.T) {
	t.Parallel()
	r := BuildRegistry()
	c, ok := r.Get("daemon.status")
	if !ok {
		t.Fatal("daemon.status should still exist for its HTTP route")
	}
	if c.CLIHidden || c.Local == nil || c.Handler == nil || c.HTTP == nil || c.ExitCodeFrom == nil {
		t.Fatalf("daemon.status = %+v, want a visible Local command with its HTTP route", c)
	}
}

// TestLocalOnlyCommandsHaveNoRoute pins that the daemon never serves or runs
// the CLI's own verbs.
func TestLocalOnlyCommandsHaveNoRoute(t *testing.T) {
	t.Parallel()
	r := BuildRegistry()
	for _, path := range []string{"daemon.start", "daemon.stop", "daemon.restart", "daemon.logs", "update", "compose.up", "compose.exec", "compose.import"} {
		c, ok := r.Get(path)
		if !ok {
			t.Fatalf("%s is not registered", path)
		}
		if c.Local == nil || c.Handler != nil || c.HTTP != nil {
			t.Fatalf("%s: Local=%v Handler=%v HTTP=%v, want Local only", path, c.Local != nil, c.Handler != nil, c.HTTP)
		}
	}
}
