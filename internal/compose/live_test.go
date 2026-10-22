package compose

import (
	"strings"
	"testing"
)

func TestCheckAgentListContractRejectsMissingAndMistypedFields(t *testing.T) {
	full := map[string]any{"name": "a", "state": "running", "image": "i:latest", "harness": "claude", "enabled": true,
		"group": "", "cwd": "/w", "timeout_s": 0, "interval_s": 0, "on_timeout": "", "on_error": "", "max_idle_iterations": 0,
		"goal_enabled": true, "goal_wait_customer_timeout_s": 0, "goal_delivery_cooldown_s": 0}
	if err := CheckAgentListContract(mustJSON(map[string]any{"agents": []any{full}})); err != nil {
		t.Fatalf("complete row rejected: %v", err)
	}
	missing := map[string]any{}
	for k, v := range full {
		if k != "harness" {
			missing[k] = v
		}
	}
	if err := CheckAgentListContract(mustJSON(map[string]any{"agents": []any{missing}})); err == nil || !strings.Contains(err.Error(), "harness") {
		t.Fatalf("missing harness accepted: %v", err)
	}
	full["timeout_s"] = "60"
	if err := CheckAgentListContract(mustJSON(map[string]any{"agents": []any{full}})); err == nil {
		t.Fatal("string timeout_s accepted")
	}
}

// A file the daemon already matches plans no action, so up makes no mutating
// call and reports every resource ok.
func TestUpConvergedFileMakesNoMutatingCall(t *testing.T) {
	fc := newFake()
	var out strings.Builder
	r := NewRunner(fc, "", &out)
	f, _ := Parse([]byte(goodYAML))
	upNoBuild(t, r, f)
	before, outBefore := len(fc.calls), out.Len()
	upNoBuild(t, r, f)
	for _, c := range fc.calls[before:] {
		if !strings.HasPrefix(c, "GET ") {
			t.Fatalf("converged up made a mutating call %s: %v", c, fc.calls[before:])
		}
	}
	if got := out.String()[outBefore:]; !strings.Contains(got, "up: 5 ok, 0 changed, 0 failed, 0 skipped") {
		t.Fatalf("report:\n%s", got)
	}
}

// A converged file with subscriptions reports every agent ok: the only
// non-GET calls are the subscription POSTs, and the daemon reports each one
// unchanged.
func TestUpConvergedSubscriptionsReportOK(t *testing.T) {
	const y = `
version: 1
agents:
  a:
    image: img:latest
    subscribe:
      - group:dev:broadcast
      - channel: issue-provider:query
        params:
          query: "Q1"
`
	fc := newFake()
	var out strings.Builder
	r := NewRunner(fc, "", &out)
	f, _ := Parse([]byte(y))
	upNoBuild(t, r, f)
	before, outBefore := len(fc.calls), out.Len()
	upNoBuild(t, r, f)
	for _, c := range fc.calls[before:] {
		if !strings.HasPrefix(c, "GET ") && c != "POST /api/agents/a/subscriptions" {
			t.Fatalf("converged up made a mutating call %s: %v", c, fc.calls[before:])
		}
	}
	got := out.String()[outBefore:]
	if !strings.Contains(got, "ok       agent:a") || !strings.Contains(got, "up: 1 ok, 0 changed, 0 failed, 0 skipped") {
		t.Fatalf("report:\n%s", got)
	}
	if strings.Contains(got, "subscribing") {
		t.Fatalf("unchanged subscriptions printed progress lines:\n%s", got)
	}
}
