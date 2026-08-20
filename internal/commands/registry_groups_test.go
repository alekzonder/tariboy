package commands

import "testing"

func TestBuildRegistryValidates(t *testing.T) {
	r := BuildRegistry()
	if err := r.Validate(); err != nil {
		t.Fatalf("BuildRegistry did not validate: %v", err)
	}
}

func TestRenamedDualNodePaths(t *testing.T) {
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

func TestDaemonStatusCLIHidden(t *testing.T) {
	r := BuildRegistry()
	c, ok := r.Get("daemon.status")
	if !ok {
		t.Fatal("daemon.status should still exist for its HTTP route")
	}
	if !c.CLIHidden {
		t.Error("daemon.status must be CLIHidden (shadowed by CLI-local daemon status)")
	}
}
