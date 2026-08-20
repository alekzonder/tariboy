package groups

import (
	"os"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
)

func subChannels(t *testing.T, b *bus.Bus, ag string) map[string]bool {
	t.Helper()
	subs, err := b.ListSubscriptions(ag)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, s := range subs {
		set[s.Channel] = true
	}
	return set
}

func TestReconcileSubscribesMembersAndLead(t *testing.T) {
	p, as, b, _ := newProvisioner(t)
	if err := as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research"}); err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureGroup("research", "scout"); err != nil {
		t.Fatal(err)
	}
	if err := p.Reconcile("research"); err != nil {
		t.Fatal(err)
	}
	lead := subChannels(t, b, "scout")
	if !lead["group:research:broadcast"] || !lead["agent:scout:inbox"] || !lead["group:research:inbox"] {
		t.Fatalf("lead subs = %v", lead)
	}
	member := subChannels(t, b, "writer")
	if !member["group:research:broadcast"] || !member["agent:writer:inbox"] {
		t.Fatalf("member subs = %v", member)
	}
	if member["group:research:inbox"] {
		t.Fatal("non-lead must NOT be subscribed to the group inbox")
	}
	for _, name := range []string{"scout", "writer"} {
		ag, err := as.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := ag.Env["BE"+"ADS_DIR"]; ok {
			t.Fatalf("retired environment variable injected for %s: %v", name, ag.Env)
		}
	}
}

func TestReconcileLeadChangeMovesInbox(t *testing.T) {
	p, as, b, _ := newProvisioner(t)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"})
	as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research"})
	p.EnsureGroup("research", "scout")
	p.Reconcile("research")
	// Promote writer to lead.
	p.groups.SetLead("research", "writer")
	if err := p.Reconcile("research"); err != nil {
		t.Fatal(err)
	}
	if subChannels(t, b, "scout")["group:research:inbox"] {
		t.Fatal("old lead still subscribed to the group inbox")
	}
	if !subChannels(t, b, "writer")["group:research:inbox"] {
		t.Fatal("new lead not subscribed to the group inbox")
	}
}

// TestReconcileIsIdempotent proves a second Reconcile call never adds
// duplicate subscriptions for either the lead or a plain member — bus.Subscribe
// is SELECT-first, so the subscription counts must be identical before and
// after the repeat call.
func TestReconcileIsIdempotent(t *testing.T) {
	p, as, b, _ := newProvisioner(t)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"})
	as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research"})
	if err := p.EnsureGroup("research", "scout"); err != nil {
		t.Fatal(err)
	}
	if err := p.Reconcile("research"); err != nil {
		t.Fatal(err)
	}
	leadSubs, err := b.ListSubscriptions("scout")
	if err != nil {
		t.Fatal(err)
	}
	memberSubs, err := b.ListSubscriptions("writer")
	if err != nil {
		t.Fatal(err)
	}
	leadCount, memberCount := len(leadSubs), len(memberSubs)

	if err := p.Reconcile("research"); err != nil {
		t.Fatal(err)
	}

	if got, err := b.ListSubscriptions("scout"); err != nil {
		t.Fatal(err)
	} else if len(got) != leadCount {
		t.Fatalf("lead subscription count changed on repeat Reconcile: %d -> %d (%+v)", leadCount, len(got), got)
	}
	if got, err := b.ListSubscriptions("writer"); err != nil {
		t.Fatal(err)
	} else if len(got) != memberCount {
		t.Fatalf("member subscription count changed on repeat Reconcile: %d -> %d (%+v)", memberCount, len(got), got)
	}
}

// TestUnsubscribeIsScopedToLeavingAgent proves that when one group member
// leaves, only that member's own subscriptions are removed. It would catch a
// regression where Unsubscribe deleted by channel alone (e.g. "DELETE ...
// WHERE channel=?") instead of being scoped to (agent, id): with such a bug,
// the remaining member would lose its group:research:broadcast subscription
// too, since it shares that exact channel string with the leaver.
func TestUnsubscribeIsScopedToLeavingAgent(t *testing.T) {
	p, as, b, _ := newProvisioner(t)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"})
	as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research"})
	if err := p.EnsureGroup("research", "scout"); err != nil {
		t.Fatal(err)
	}
	if err := p.Reconcile("research"); err != nil {
		t.Fatal(err)
	}
	// Sanity: both members share the same broadcast channel before the leave.
	if !subChannels(t, b, "scout")["group:research:broadcast"] || !subChannels(t, b, "writer")["group:research:broadcast"] {
		t.Fatal("setup: both members must be subscribed to broadcast before leave")
	}

	// writer leaves via the same path TestAssignAgentJoinAndLeave uses.
	if err := p.AssignAgent("writer", ""); err != nil {
		t.Fatal(err)
	}

	scoutSubs := subChannels(t, b, "scout")
	if !scoutSubs["group:research:broadcast"] {
		t.Fatal("scout lost the group broadcast subscription after writer left (Unsubscribe not scoped to the leaving agent)")
	}
	if !scoutSubs["agent:scout:inbox"] {
		t.Fatal("scout lost its own inbox subscription after writer left")
	}
	if subChannels(t, b, "writer")["group:research:broadcast"] {
		t.Fatal("writer still subscribed to broadcast after leaving")
	}
}

func TestAssignAgentJoinAndLeave(t *testing.T) {
	p, as, b, _ := newProvisioner(t)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest"})
	if err := p.AssignAgent("scout", "research"); err != nil {
		t.Fatal(err)
	}
	if g, _ := as.Get("scout"); g.Group != "research" {
		t.Fatalf("join did not persist: %+v", g)
	}
	if !subChannels(t, b, "scout")["group:research:broadcast"] {
		t.Fatal("join did not subscribe to broadcast")
	}
	if err := p.AssignAgent("scout", ""); err != nil {
		t.Fatal(err)
	}
	if g, _ := as.Get("scout"); g.Group != "" {
		t.Fatalf("leave did not clear: %+v", g)
	}
	if subChannels(t, b, "scout")["group:research:broadcast"] {
		t.Fatal("leave did not unsubscribe from broadcast")
	}
}

// TestAssignAgentRejectsInvalidName proves the agent-name guard fires BEFORE any
// path/channel is built or any subscription created (path/channel-traversal
// guard — the recurring Critical bug class in this project).
func TestAssignAgentRejectsInvalidName(t *testing.T) {
	p, _, b, _ := newProvisioner(t)
	bad := "../evil"
	if err := p.AssignAgent(bad, "research"); err == nil {
		t.Fatal("invalid agent name must be rejected")
	}
	// No subscription may have been created for the traversing name.
	if len(subChannels(t, b, bad)) != 0 {
		t.Fatalf("subscription created for invalid agent name: %v", subChannels(t, b, bad))
	}
	// The group must not have been provisioned as a side effect either.
	if _, ok, _ := p.groups.Get("research"); ok {
		t.Fatal("group provisioned despite invalid agent name")
	}
}

func TestRemoveGroup(t *testing.T) {
	p, as, b, _ := newProvisioner(t)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"})
	p.EnsureGroup("research", "scout")
	p.Reconcile("research")
	if err := p.RemoveGroup("research", true); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := p.groups.Get("research"); ok {
		t.Fatal("group row survived removal")
	}
	if g, _ := as.Get("scout"); g.Group != "" {
		t.Fatalf("member not detached: %+v", g)
	}
	if subChannels(t, b, "scout")["group:research:broadcast"] {
		t.Fatal("member still subscribed after removal")
	}
	if _, err := os.Stat(p.SharedDir("research")); err == nil {
		t.Fatal("shared dir not removed with --volumes")
	}
}
