package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

// fakeRefresher records Refresh calls so tests can assert the immediate-
// refresh seam is invoked exactly when expected, without needing a live
// aiproxy.PolicyCache.
type fakeRefresher struct{ calls int }

func (f *fakeRefresher) Refresh() error {
	f.calls++
	return nil
}

func TestRuleSetValidation(t *testing.T) {
	c, _, _ := ctxWithStore(t)

	countRules := func() int {
		ls, err := h(t, "rule.ls")(c, registry.Params{})
		if err != nil {
			t.Fatal(err)
		}
		return ls.(map[string]any)["count"].(int)
	}

	// Bad scope (injection attempt via agent: suffix) is rejected.
	_, err := h(t, "rule.set")(c, registry.Params{"scope": "agent:../evil", "kind": "model-policy", "deny": "*"})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for bad scope, got %v", err)
	}
	if got := countRules(); got != 0 {
		t.Fatalf("bad scope: expected 0 rules stored, got %d", got)
	}

	// Bad kind is rejected.
	_, err = h(t, "rule.set")(c, registry.Params{"scope": "global", "kind": "bogus"})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for bad kind, got %v", err)
	}
	if got := countRules(); got != 0 {
		t.Fatalf("bad kind: expected 0 rules stored, got %d", got)
	}

	// rate-limit with no window is rejected.
	_, err = h(t, "rule.set")(c, registry.Params{"scope": "global", "kind": "rate-limit", "max-requests": 10})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for missing window-s, got %v", err)
	}
	if got := countRules(); got != 0 {
		t.Fatalf("missing window: expected 0 rules stored, got %d", got)
	}

	// rate-limit with a window but no limit is rejected.
	_, err = h(t, "rule.set")(c, registry.Params{"scope": "global", "kind": "rate-limit", "window-s": 60})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for missing max-requests/max-tokens, got %v", err)
	}
	if got := countRules(); got != 0 {
		t.Fatalf("missing limit: expected 0 rules stored, got %d", got)
	}

	// model-policy with no allow/deny/route is rejected.
	_, err = h(t, "rule.set")(c, registry.Params{"scope": "global", "kind": "model-policy"})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for empty model-policy, got %v", err)
	}
	if got := countRules(); got != 0 {
		t.Fatalf("empty model-policy: expected 0 rules stored, got %d", got)
	}

	// A valid rule is accepted and persisted.
	if _, err := h(t, "rule.set")(c, registry.Params{
		"scope": "agent:bob", "kind": "model-policy", "deny": "claude-opus-*",
	}); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	if got := countRules(); got != 1 {
		t.Fatalf("valid rule: expected 1 rule stored, got %d", got)
	}
}

func TestRuleLsOrder(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	mustSet := func(id string, priority int) {
		if _, err := h(t, "rule.set")(c, registry.Params{
			"id": id, "priority": priority, "scope": "global", "kind": "model-policy", "deny": "*",
		}); err != nil {
			t.Fatalf("rule.set %s: %v", id, err)
		}
	}
	mustSet("b", 5)
	mustSet("a", 1)
	mustSet("c", 5)

	res, err := h(t, "rule.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	rows := m["rules"].([]map[string]any)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rows))
	}
	got := []string{rows[0]["id"].(string), rows[1]["id"].(string), rows[2]["id"].(string)}
	want := []string{"a", "b", "c"} // priority asc, then id
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestRuleSetAndRmSkipRefreshWhenPolicyNil(t *testing.T) {
	c, _, _ := ctxWithStore(t) // Policy left nil (proxy unconfigured).
	if c.Policy != nil {
		t.Fatal("test setup: expected nil Policy")
	}

	// Must not panic and must still persist at the store level.
	res, err := h(t, "rule.set")(c, registry.Params{
		"id": "r1", "scope": "global", "kind": "model-policy", "deny": "*",
	})
	if err != nil {
		t.Fatalf("rule.set with nil Policy: %v", err)
	}
	if m := res.(map[string]any); m["id"] != "r1" {
		t.Fatalf("rule.set result = %v", m)
	}

	if _, err := h(t, "rule.rm")(c, registry.Params{"id": "r1"}); err != nil {
		t.Fatalf("rule.rm with nil Policy: %v", err)
	}
}

func TestRuleSetAndRmInvokePolicyRefreshWhenPresent(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	fr := &fakeRefresher{}
	c.Policy = fr

	if _, err := h(t, "rule.set")(c, registry.Params{
		"id": "r1", "scope": "global", "kind": "model-policy", "deny": "*",
	}); err != nil {
		t.Fatalf("rule.set: %v", err)
	}
	if fr.calls != 1 {
		t.Fatalf("rule.set: Policy.Refresh calls = %d, want 1", fr.calls)
	}

	if _, err := h(t, "rule.rm")(c, registry.Params{"id": "r1"}); err != nil {
		t.Fatalf("rule.rm: %v", err)
	}
	if fr.calls != 2 {
		t.Fatalf("rule.rm: Policy.Refresh calls = %d, want 2", fr.calls)
	}
}
