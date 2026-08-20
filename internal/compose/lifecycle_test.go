package compose

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestLifecycleAllMembers(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "running", "group": "research-team"}
	fc.agents["writer"] = map[string]any{"name": "writer", "state": "running", "group": "research-team"}
	r := NewRunner(fc, "", "", io.Discard)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Lifecycle(f, "stop", nil); err != nil {
		t.Fatal(err)
	}
	if countCalls(fc, "POST /api/agents/scout/stop") != 1 || countCalls(fc, "POST /api/agents/writer/stop") != 1 {
		t.Fatalf("stop not applied to all members: %v", fc.calls)
	}
}

func TestLifecycleNamedMember(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "running", "group": "research-team"}
	fc.agents["writer"] = map[string]any{"name": "writer", "state": "running", "group": "research-team"}
	r := NewRunner(fc, "", "", io.Discard)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Lifecycle(f, "kill", []string{"scout"}); err != nil {
		t.Fatal(err)
	}
	if countCalls(fc, "POST /api/agents/scout/kill") != 1 || countCalls(fc, "POST /api/agents/writer/kill") != 0 {
		t.Fatalf("kill scoping wrong: %v", fc.calls)
	}
	// An agent not in the file is rejected.
	if err := r.Lifecycle(f, "kill", []string{"ghost"}); err == nil {
		t.Fatal("kill of an unknown member should error")
	}
}

func TestPsAndStatusRun(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "running", "group": "research-team"}
	r := NewRunner(fc, "", "", io.Discard)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Ps(f); err != nil {
		t.Fatalf("ps: %v", err)
	}
	if err := r.Status(f); err != nil {
		t.Fatalf("status: %v", err)
	}
	// writer is desired-but-missing → status must still succeed and note it.
	if countCalls(fc, "GET /api/agents") < 2 {
		t.Fatalf("ps/status should each read live agents: %v", fc.calls)
	}
}

// TestStatusReportsFieldDriftWithConvergedGroupAndBudget pins the M8 status
// drift model: group membership/lead and budgets converge on `up` and are
// shown as OK when they already match; a non-group field (image — the only
// per-agent field agent.ps's live view exposes besides state/harness/group)
// is reported as drift since there is no agent.update to converge it.
func TestStatusReportsFieldDriftWithConvergedGroupAndBudget(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "running", "group": "research-team", "image": "analyst:old"}
	fc.agents["writer"] = map[string]any{"name": "writer", "state": "running", "group": "research-team", "image": "analyst:latest"}
	fc.groups["research-team"] = map[string]any{"name": "research-team", "lead": "scout"}
	fc.budgets["group:research-team"] = map[string]any{"scope": "group:research-team", "limit_usd": 50.0, "period_s": 86400, "mode": "block"}
	fc.budgets["agent:scout"] = map[string]any{"scope": "agent:scout", "limit_usd": 10.0, "period_s": 86400, "mode": "warn"}
	var buf bytes.Buffer
	r := NewRunner(fc, "", "", &buf)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Status(f); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "image drift") || !strings.Contains(out, "have=analyst:old want=analyst:latest") {
		t.Fatalf("expected scout image drift reported:\n%s", out)
	}
	if !strings.Contains(out, "ok (lead=scout)") {
		t.Fatalf("expected group research-team to show converged:\n%s", out)
	}
	if strings.Count(out, "budget") < 2 || strings.Contains(out, "budget group:research-team") == false {
		t.Fatalf("expected both budgets reported as converged:\n%s", out)
	}
	if !strings.Contains(out, "drift: 1") {
		t.Fatalf("expected exactly 1 drifted item (the image), got:\n%s", out)
	}
}

// harnessYAML declares scout with a harness the live agent will not match, so
// TestStatusReportsHarnessDrift can assert `compose status` calls it out as
// drift rather than falsely claiming convergence.
const harnessYAML = `
version: 1
agents:
  scout:
    image: analyst:latest
    harness:
      type: claude
`

// TestStatusReportsHarnessDrift pins that a file declaring harness: claude
// against a live agent running codex is DRIFT, not ok — a drift/audit tool
// must not falsely claim convergence on a field it has the data to check.
func TestStatusReportsHarnessDrift(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{
		"name": "scout", "state": "running", "image": "analyst:latest", "harness": "codex",
	}
	var buf bytes.Buffer
	r := NewRunner(fc, "", "", &buf)
	f, err := Parse([]byte(harnessYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Status(f); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "harness drift: have=codex want=claude") {
		t.Fatalf("expected harness drift reported, got:\n%s", out)
	}
	if strings.Contains(out, "agent scout           ok") {
		t.Fatalf("harness drift must not be reported as ok:\n%s", out)
	}
	if !strings.Contains(out, "drift: 1") {
		t.Fatalf("expected exactly 1 drifted item (the harness), got:\n%s", out)
	}
}

// TestStatusDisclosesUncheckedFields pins that Status always prints a
// one-line disclaimer noting model/env/plugins drift is not checked, so an
// operator reading "ok"/"drift: N" is not misled into a full convergence
// guarantee.
func TestStatusDisclosesUncheckedFields(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "running", "group": "research-team"}
	var buf bytes.Buffer
	r := NewRunner(fc, "", "", &buf)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Status(f); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if strings.Count(out, "note: model/env/plugins drift not checked") != 1 {
		t.Fatalf("expected exactly one disclaimer line, got:\n%s", out)
	}
}
