package cli_test

import "testing"

func TestRuleCommandsRoundTrip(t *testing.T) {
	_, _, c := startDaemon(t)

	// model-policy deny rule.
	mustCall(t, c, "POST", "/api/proxy-rules", map[string]any{
		"scope": "agent:bob", "kind": "model-policy", "deny": "claude-opus-*", "priority": "1",
	})
	// rate-limit rule.
	mustCall(t, c, "POST", "/api/proxy-rules", map[string]any{
		"scope": "global", "kind": "rate-limit", "max-requests": "100", "window-s": "60",
	})
	ls := mustCall(t, c, "GET", "/api/proxy-rules", map[string]string{})
	rows, _ := ls["rules"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rule ls rows = %v", ls["rules"])
	}
	var id string
	for _, r := range rows {
		m, _ := r.(map[string]any)
		if m["kind"] == "model-policy" {
			id, _ = m["id"].(string)
		}
	}
	if id == "" {
		t.Fatal("no model-policy rule id found")
	}
	mustCall(t, c, "DELETE", "/api/proxy-rules/"+id, map[string]string{})
	ls2 := mustCall(t, c, "GET", "/api/proxy-rules", map[string]string{})
	if rows2, _ := ls2["rules"].([]any); len(rows2) != 1 {
		t.Fatalf("after rm rows = %v", ls2["rules"])
	}
}

func TestRuleSetRejectsBadScope(t *testing.T) {
	_, _, c := startDaemon(t)
	if _, err := c.Call("POST", "/api/proxy-rules", map[string]any{
		"scope": "agent:../evil", "kind": "model-policy", "deny": "*",
	}); err == nil {
		t.Fatal("bad scope must be rejected")
	}
	if _, err := c.Call("POST", "/api/proxy-rules", map[string]any{
		"scope": "global", "kind": "bogus",
	}); err == nil {
		t.Fatal("bad kind must be rejected")
	}
}
