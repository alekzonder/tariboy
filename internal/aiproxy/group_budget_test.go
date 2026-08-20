package aiproxy

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/store"
)

func newAIStore(t *testing.T) (*store.Store, *Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, NewStore(s, func() time.Time { return time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC) })
}

func seedReq(t *testing.T, s *Store, ag string, cost float64) {
	t.Helper()
	if err := s.Insert(AIRequest{
		ID: NewRequestID(nil), TS: time.Date(2026, 7, 5, 11, 30, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Agent: ag, Iteration: "i1", CostUSD: cost, Status: "ok",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGroupBudgetAggregatesMembers(t *testing.T) {
	base, ai := newAIStore(t)
	as := agent.NewStore(base)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"})
	as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research"})

	groups, err := ai.AgentGroups()
	if err != nil || groups["scout"] != "research" || groups["writer"] != "research" {
		t.Fatalf("AgentGroups = %v err=%v", groups, err)
	}
	seedReq(t, ai, "scout", 4)
	seedReq(t, ai, "writer", 5)
	spent, err := ai.GroupCostSince([]string{"scout", "writer"},
		time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC))
	if err != nil || spent != 9 {
		t.Fatalf("GroupCostSince = %v err=%v", spent, err)
	}
	// Empty membership sums to zero, not an error.
	if v, err := ai.GroupCostSince(nil, time.Time{}); err != nil || v != 0 {
		t.Fatalf("empty group cost = %v err=%v", v, err)
	}
}

func TestBudgetCacheGroupDecision(t *testing.T) {
	base, ai := newAIStore(t)
	as := agent.NewStore(base)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"})
	as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research"})
	seedReq(t, ai, "scout", 6)
	seedReq(t, ai, "writer", 6) // group total 12

	ai.SetBudget(Budget{Scope: "group:research", LimitUSD: 10, PeriodS: 86400, Mode: "block"})
	cache := NewBudgetCache(ai, func() time.Time { return time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC) })
	if err := cache.Refresh(); err != nil {
		t.Fatal(err)
	}
	// GREEN before the group limit: an agent with no group budget passes...
	as.Create(agent.Agent{Name: "loner", ImageRef: "i:latest"})
	if d := cache.Check("loner"); d.Over {
		t.Fatalf("ungrouped agent should not be blocked: %+v", d)
	}
	// RED: a research member is blocked because the GROUP is over 10.
	d := cache.Check("scout")
	if !d.Over || d.Mode != "block" || d.Scope != "group:research" {
		t.Fatalf("group member should be blocked by group budget: %+v", d)
	}
}
