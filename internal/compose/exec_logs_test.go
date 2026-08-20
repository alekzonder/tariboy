package compose

import (
	"encoding/json"
	"io"
	"testing"
)

func TestExecTargetsNamedMember(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "running", "group": "research-team"}
	r := NewRunner(fc, "", "", io.Discard)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Exec(f, []string{"scout", "do", "the", "thing"}); err != nil {
		t.Fatal(err)
	}
	if countCalls(fc, "POST /api/agents/scout/exec") != 1 {
		t.Fatalf("exec not sent: %v", fc.calls)
	}
	if err := r.Exec(f, nil); err == nil {
		t.Fatal("exec with no member should error")
	}
}

func TestLogsAllMembers(t *testing.T) {
	fc := newFake()
	fc.logsFor = func(name string) json.RawMessage {
		return mustJSON(map[string]any{"lines": []string{name + ": hello"}, "count": 1})
	}
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "running", "group": "research-team"}
	fc.agents["writer"] = map[string]any{"name": "writer", "state": "running", "group": "research-team"}
	r := NewRunner(fc, "", "", io.Discard)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Logs(f, nil, 50); err != nil {
		t.Fatal(err)
	}
	if countCalls(fc, "GET /api/agents/scout/logs") != 1 || countCalls(fc, "GET /api/agents/writer/logs") != 1 {
		t.Fatalf("logs not fetched for all members: %v", fc.calls)
	}
}

func TestRmStoppedMember(t *testing.T) {
	fc := newFake()
	fc.agents["scout"] = map[string]any{"name": "scout", "state": "stopped", "group": "research-team"}
	fc.agents["writer"] = map[string]any{"name": "writer", "state": "running", "group": "research-team"}
	r := NewRunner(fc, "", "", io.Discard)
	f, _ := Parse([]byte(goodYAML))
	if err := r.Rm(f, nil); err != nil {
		t.Fatal(err)
	}
	// Only the stopped member is removed; the running one is skipped.
	if countCalls(fc, "DELETE /api/agents/scout") != 1 {
		t.Fatalf("stopped member not removed: %v", fc.calls)
	}
	if countCalls(fc, "DELETE /api/agents/writer") != 0 {
		t.Fatalf("running member should not be removed by rm: %v", fc.calls)
	}
}
