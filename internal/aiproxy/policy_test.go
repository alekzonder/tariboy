package aiproxy

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
)

func policyClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC) }
}

func TestDecideModelPolicyDenyAllowRoute(t *testing.T) {
	_, ai := newAIStore(t)
	// Deny opus for bob; allowlist for carol; route for dave.
	ai.SetRule(PolicyRule{ID: "a", Priority: 1, Scope: "agent:bob", Kind: "model-policy",
		Deny: []string{"claude-opus-*"}, Enabled: true})
	ai.SetRule(PolicyRule{ID: "b", Priority: 1, Scope: "agent:carol", Kind: "model-policy",
		Allow: []string{"claude-sonnet-*"}, Enabled: true})
	ai.SetRule(PolicyRule{ID: "c", Priority: 1, Scope: "agent:dave", Kind: "model-policy",
		Route: "claude-sonnet-4", Enabled: true})
	c := NewPolicyCache(ai, policyClock())
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	if d := c.Decide("bob", "claude-opus-4-8"); !d.Deny {
		t.Fatalf("bob opus should be denied: %+v", d)
	}
	if d := c.Decide("bob", "claude-sonnet-4"); d.Deny {
		t.Fatalf("bob sonnet should pass: %+v", d)
	}
	if d := c.Decide("carol", "claude-opus-4-8"); !d.Deny {
		t.Fatalf("carol opus not in allowlist ⇒ deny: %+v", d)
	}
	if d := c.Decide("carol", "claude-sonnet-4"); d.Deny {
		t.Fatalf("carol sonnet in allowlist ⇒ pass: %+v", d)
	}
	if d := c.Decide("dave", "claude-opus-4-8"); d.RewriteModel != "claude-sonnet-4" {
		t.Fatalf("dave should be routed: %+v", d)
	}
	// Ungoverned agent ⇒ empty decision (allow).
	if d := c.Decide("mallory", "anything"); d.Deny || d.RateLimited || d.RewriteModel != "" {
		t.Fatalf("ungoverned agent ⇒ allow: %+v", d)
	}
}

func TestDecideRateLimitAggregated(t *testing.T) {
	base, ai := newAIStore(t)
	as := agent.NewStore(base)
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research"})
	as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research"})
	seedReq(t, ai, "scout", 1)
	seedReq(t, ai, "writer", 1) // group total 2 requests in-window
	// Group rate-limit: max 2 requests / 24h ⇒ over.
	ai.SetRule(PolicyRule{ID: "rl", Priority: 1, Scope: "group:research", Kind: "rate-limit",
		MaxRequests: 2, WindowS: 86400, Enabled: true})
	c := NewPolicyCache(ai, policyClock())
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	if d := c.Decide("scout", "claude-opus-4-8"); !d.RateLimited {
		t.Fatalf("group member should be rate-limited: %+v", d)
	}
	if d := c.Decide("loner", "claude-opus-4-8"); d.RateLimited {
		t.Fatalf("non-member should not be rate-limited: %+v", d)
	}
}

func TestDecideMalformedRuleSkipped(t *testing.T) {
	_, ai := newAIStore(t)
	ai.SetRule(PolicyRule{ID: "bad-kind", Priority: 1, Scope: "global", Kind: "bogus", Enabled: true})
	ai.SetRule(PolicyRule{ID: "bad-window", Priority: 2, Scope: "global", Kind: "rate-limit",
		MaxRequests: 1, WindowS: 0, Enabled: true})
	ai.SetRule(PolicyRule{ID: "disabled", Priority: 3, Scope: "global", Kind: "model-policy",
		Deny: []string{"*"}, Enabled: false})
	c := NewPolicyCache(ai, policyClock())
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	// None of the three take effect ⇒ everything allowed (no fail-open block).
	if d := c.Decide("anyone", "claude-opus-4-8"); d.Deny || d.RateLimited {
		t.Fatalf("malformed/disabled rules must not block: %+v", d)
	}
}

// TestDecideGroupRateLimitIgnoresGlobalTraffic is the discriminating test for the
// nil-vs-non-nil member carry-forward: a group scope must count ONLY its members,
// never global traffic. Here the group member has zero in-window requests while
// unrelated agents generate heavy global traffic; a bug that resolved the group
// scope to nil (global) would count that traffic and wrongly block.
func TestDecideGroupRateLimitIgnoresGlobalTraffic(t *testing.T) {
	base, ai := newAIStore(t)
	as := agent.NewStore(base)
	as.Create(agent.Agent{Name: "solo", ImageRef: "i:latest", Group: "team"})
	// Heavy GLOBAL traffic from agents NOT in the group; "solo" itself made none.
	seedReq(t, ai, "outsider1", 1)
	seedReq(t, ai, "outsider2", 1)
	seedReq(t, ai, "outsider3", 1)
	ai.SetRule(PolicyRule{ID: "rl", Priority: 1, Scope: "group:team", Kind: "rate-limit",
		MaxRequests: 1, WindowS: 86400, Enabled: true})
	c := NewPolicyCache(ai, policyClock())
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	if d := c.Decide("solo", "claude-opus-4-8"); d.RateLimited {
		t.Fatalf("group scope must count only its members (0), not global traffic: %+v", d)
	}
}

// TestDecideUnknownGroupCountsZero ensures a rate-limit rule for a group with no
// members resolves to ZERO (non-nil empty member set), never GLOBAL. Heavy global
// traffic is present; the rule must compile without marking anything over, and no
// agent is blocked by it.
func TestDecideUnknownGroupCountsZero(t *testing.T) {
	_, ai := newAIStore(t)
	seedReq(t, ai, "busy1", 1)
	seedReq(t, ai, "busy2", 1)
	seedReq(t, ai, "busy3", 1)
	ai.SetRule(PolicyRule{ID: "rl", Priority: 1, Scope: "group:ghost", Kind: "rate-limit",
		MaxRequests: 1, WindowS: 86400, Enabled: true})
	c := NewPolicyCache(ai, policyClock())
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	// No agent maps to the empty group, so nobody matches it; critically the rule
	// must NOT have folded global traffic into an over-limit state.
	if d := c.Decide("busy1", "claude-opus-4-8"); d.RateLimited {
		t.Fatalf("empty group must count zero, not global traffic: %+v", d)
	}
}

func TestDecideMalformedScopeSkipped(t *testing.T) {
	_, ai := newAIStore(t)
	// A path-traversal / invalid name in the scope must be skipped at refresh, not
	// applied. Even a Deny:* rule with a malformed scope must never block.
	ai.SetRule(PolicyRule{ID: "bad", Priority: 1, Scope: "agent:../evil", Kind: "model-policy",
		Deny: []string{"*"}, Enabled: true})
	c := NewPolicyCache(ai, policyClock())
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	if d := c.Decide("../evil", "claude-opus-4-8"); d.Deny {
		t.Fatalf("malformed scope must be skipped, not enforced: %+v", d)
	}
}

func TestRewriteModelPreservesOtherFields(t *testing.T) {
	out := rewriteModel([]byte(`{"model":"a","max_tokens":16,"messages":[]}`), "b")
	if want := `"model":"b"`; !containsJSON(out, want) {
		t.Fatalf("model not rewritten: %s", out)
	}
	if !containsJSON(out, `"max_tokens":16`) {
		t.Fatalf("other fields dropped: %s", out)
	}
}

func containsJSON(b []byte, sub string) bool {
	return len(b) > 0 && (string(b) == sub || indexOf(string(b), sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
