package groups

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
)

func newProvisioner(t *testing.T) (*Provisioner, *agent.Store, *bus.Bus, string) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// Advancing clock (one ns per call), matching the bus test convention: a
	// fixed clock makes the bus mint colliding subscription ids when one agent
	// subscribes to several channels in the same tick. Production uses time.Now.
	tick := int64(0)
	clock := func() time.Time {
		tick++
		return time.Date(2026, 7, 5, 9, 0, 0, int(tick), time.UTC)
	}
	as := agent.NewStore(s)
	b := bus.New(s, clock)
	groupsDir := filepath.Join(t.TempDir(), "groups")
	p := NewProvisioner(ProvisionerConfig{
		Groups: NewStore(s, clock), Agents: as, Bus: b, GroupsDir: groupsDir, Clock: clock,
	})
	return p, as, b, groupsDir
}

func TestEnsureGroupCreatesRowAndDir(t *testing.T) {
	p, _, _, groupsDir := newProvisioner(t)
	if err := p.EnsureGroup("research", "scout"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(groupsDir, "research", "shared")
	if p.SharedDir("research") != want {
		t.Fatalf("SharedDir = %q want %q", p.SharedDir("research"), want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Fatalf("shared dir not created: %v", err)
	}
	g, ok, _ := p.groups.Get("research")
	if !ok || g.Lead != "scout" {
		t.Fatalf("group row = %+v ok=%v", g, ok)
	}
	// Idempotent, and a leaderless re-ensure keeps the existing lead.
	if err := p.EnsureGroup("research", ""); err != nil {
		t.Fatal(err)
	}
	if g, _, _ := p.groups.Get("research"); g.Lead != "scout" {
		t.Fatalf("lead clobbered by leaderless ensure: %+v", g)
	}
}

func TestEnsureGroupRejectsTraversingName(t *testing.T) {
	p, _, _, groupsDir := newProvisioner(t)
	if err := p.EnsureGroup("../evil", ""); err == nil {
		t.Fatal("traversing group name must be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(groupsDir), "evil")); err == nil {
		t.Fatal("a dir was materialised outside GroupsDir")
	}
}

func TestRenameAndRemoveMemberPreserveAgent(t *testing.T) {
	p, agents, _, groupsDir := newProvisioner(t)
	if err := agents.Create(agent.Agent{Name: "lead", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureGroup("old-team", "lead"); err != nil {
		t.Fatal(err)
	}
	if err := p.AssignAgent("lead", "old-team"); err != nil {
		t.Fatal(err)
	}
	if err := p.Rename("old-team", "new-team"); err != nil {
		t.Fatal(err)
	}
	got, err := agents.Get("lead")
	if err != nil || got.Group != "new-team" {
		t.Fatalf("agent after rename = %+v, err %v", got, err)
	}
	if _, ok, _ := p.groups.Get("old-team"); ok {
		t.Fatal("old group row remains")
	}
	if renamed, ok, _ := p.groups.Get("new-team"); !ok || renamed.Lead != "lead" {
		t.Fatalf("renamed group = %+v, ok %v", renamed, ok)
	}
	if _, err := os.Stat(filepath.Join(groupsDir, "new-team", "shared")); err != nil {
		t.Fatal(err)
	}
	if err := p.AssignAgent("lead", ""); err != nil {
		t.Fatal(err)
	}
	got, err = agents.Get("lead")
	if err != nil || got.Group != "" || got.ImageRef != "basic:latest" {
		t.Fatalf("detached agent = %+v, err %v", got, err)
	}
}
