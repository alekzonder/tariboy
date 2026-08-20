package aiproxy

import (
	"testing"
	"time"
)

func TestPolicyRuleCRUD(t *testing.T) {
	_, ai := newAIStore(t)
	if err := ai.SetRule(PolicyRule{
		ID: "r1", Priority: 5, Scope: "agent:bob", ModelGlob: "claude-*", Kind: "model-policy",
		Deny: []string{"claude-opus-*"}, Route: "", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	// Generated id when empty.
	if err := ai.SetRule(PolicyRule{Priority: 1, Scope: "global", Kind: "rate-limit",
		MaxRequests: 10, WindowS: 60, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	list, err := ai.ListRules()
	if err != nil || len(list) != 2 {
		t.Fatalf("ListRules = %+v err=%v", list, err)
	}
	// Ordered by priority: the global rate-limit (priority 1) precedes r1 (priority 5).
	if list[0].Scope != "global" || list[1].ID != "r1" {
		t.Fatalf("rule order = %+v", list)
	}
	if list[1].Deny[0] != "claude-opus-*" || !list[1].Enabled {
		t.Fatalf("r1 round-trip = %+v", list[1])
	}
	// Upsert (same id) mutates in place.
	if err := ai.SetRule(PolicyRule{ID: "r1", Priority: 5, Scope: "agent:bob", Kind: "model-policy",
		Deny: []string{"claude-opus-*"}, Route: "sonnet-routed", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ai.GetRule("r1")
	if err != nil || !ok || got.Route != "sonnet-routed" || got.Enabled {
		t.Fatalf("GetRule r1 = %+v ok=%v err=%v", got, ok, err)
	}
	if err := ai.DeleteRule("r1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ai.GetRule("r1"); ok {
		t.Fatal("DeleteRule did not remove r1")
	}
}

func TestRequestAndTokenCountSince(t *testing.T) {
	_, ai := newAIStore(t)
	seedReq(t, ai, "scout", 4) // seedReq inserts one ai_request row (M8 helper)
	seedReq(t, ai, "scout", 5)
	seedReq(t, ai, "writer", 1)
	since := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	// Per-agent count.
	n, err := ai.RequestCountSince([]string{"scout"}, since)
	if err != nil || n != 2 {
		t.Fatalf("RequestCountSince(scout) = %d err=%v", n, err)
	}
	// Group members aggregate.
	if n, _ := ai.RequestCountSince([]string{"scout", "writer"}, since); n != 3 {
		t.Fatalf("RequestCountSince(group) = %d", n)
	}
	// nil ⇒ global (all rows).
	if n, _ := ai.RequestCountSince(nil, since); n != 3 {
		t.Fatalf("RequestCountSince(global) = %d", n)
	}
	// Token sum over members (seedReq rows carry 0 tokens ⇒ 0; verifies the query runs).
	if tok, err := ai.TokenSumSince([]string{"scout"}, since); err != nil || tok != 0 {
		t.Fatalf("TokenSumSince = %d err=%v", tok, err)
	}
}
