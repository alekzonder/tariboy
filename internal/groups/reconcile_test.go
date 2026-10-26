package groups

import (
	"os"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
)

func TestReconcileLeavesMembersUntouched(t *testing.T) {
	p, as, _ := newProvisioner(t)
	if err := as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"}); err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureGroup("research", "scout"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := p.Reconcile("research"); err != nil {
			t.Fatal(err)
		}
	}
	ag, err := as.Get("scout")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ag.Env["BE"+"ADS_DIR"]; ok || ag.Group != "research" {
		t.Fatalf("member after reconcile = %+v", ag)
	}
}

func TestAssignAgentJoinAndLeave(t *testing.T) {
	p, as, _ := newProvisioner(t)
	if err := as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := p.AssignAgent("scout", "research"); err != nil {
		t.Fatal(err)
	}
	if g, _ := as.Get("scout"); g.Group != "research" {
		t.Fatalf("join did not persist: %+v", g)
	}
	if _, ok, _ := p.groups.Get("research"); !ok {
		t.Fatal("join did not provision the group")
	}
	if err := p.AssignAgent("scout", ""); err != nil {
		t.Fatal(err)
	}
	if g, _ := as.Get("scout"); g.Group != "" {
		t.Fatalf("leave did not clear: %+v", g)
	}
}

// TestAssignAgentRejectsInvalidName proves the agent-name guard fires before
// any group is provisioned (path-traversal guard).
func TestAssignAgentRejectsInvalidName(t *testing.T) {
	p, _, _ := newProvisioner(t)
	if err := p.AssignAgent("../evil", "research"); err == nil {
		t.Fatal("invalid agent name must be rejected")
	}
	if _, ok, _ := p.groups.Get("research"); ok {
		t.Fatal("group provisioned despite invalid agent name")
	}
}

func TestRemoveGroup(t *testing.T) {
	p, as, _ := newProvisioner(t)
	if err := as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"}); err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureGroup("research", "scout"); err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveGroup("research", true); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := p.groups.Get("research"); ok {
		t.Fatal("group row survived removal")
	}
	if g, _ := as.Get("scout"); g.Group != "" {
		t.Fatalf("member not detached: %+v", g)
	}
	if _, err := os.Stat(p.SharedDir("research")); err == nil {
		t.Fatal("shared dir not removed with --volumes")
	}
}
