package taskcli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

type call struct {
	method, route string
	body          any
}
type recorder struct {
	calls  []call
	result json.RawMessage
	err    error
}

func (r *recorder) Call(method, route string, body any) (json.RawMessage, error) {
	r.calls = append(r.calls, call{method, route, body})
	return r.result, r.err
}

func mapEnv(values ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i < len(values); i += 2 {
		m[values[i]] = values[i+1]
	}
	return func(key string) string { return m[key] }
}

func TestParseCurrentTasksSurface(t *testing.T) {
	tests := []struct {
		name, action string
		argv         []string
		want         map[string]any
	}{
		{"mine", "mine", []string{"mine", "--queue=DEV"}, map[string]any{"queue": "DEV"}},
		{"ready", "ready", []string{"ready", "--claim"}, map[string]any{"claim": true}},
		{"show", "show", []string{"show", "DEV-1"}, map[string]any{"key": "DEV-1"}},
		{"create", "create", []string{"create", "--queue", "DEV", "--title", "new"}, map[string]any{"queue": "DEV", "title": "new"}},
		{"update", "update", []string{"update", "DEV-1", "--pull-request="}, map[string]any{"key": "DEV-1", "pull_request": ""}},
		{"assign", "assign", []string{"assign", "DEV-1", "worker"}, map[string]any{"key": "DEV-1", "assignee": "worker"}},
		{"comment", "comment", []string{"comment", "DEV-1", "hello"}, map[string]any{"key": "DEV-1", "body": "hello"}},
		{"legacy ask", "ask", []string{"ask", "DEV-1", "user:me", "why"}, map[string]any{"key": "DEV-1", "principal": "user:me", "body": "why"}},
		{"workflow ask", "workflow_ask", []string{"ask", "A-1", "--question", "why"}, map[string]any{"assignment_id": "A-1", "question": "why"}},
		{"move", "move", []string{"move", "DEV-1", "--to-root"}, map[string]any{"key": "DEV-1", "parent_key": ""}},
		{"block", "block", []string{"block", "DEV-1", "--by", "DEV-2"}, map[string]any{"key": "DEV-1", "blocker_key": "DEV-2"}},
		{"relate", "relate", []string{"relate", "DEV-1", "DEV-2"}, map[string]any{"key": "DEV-1", "target_key": "DEV-2"}},
		{"done", "done", []string{"done", "DEV-1"}, map[string]any{"key": "DEV-1"}},
		{"work next", "work_next", []string{"work", "next", "--idempotency-key", "id"}, map[string]any{"idempotency_key": "id"}},
		{"artifacts", "artifact_add", []string{"artifacts", "add", "A-1", "--name", "report", "--type", "text", "--content="}, map[string]any{"assignment_id": "A-1", "name": "report", "type": "text", "content": ""}},
		{"questions", "questions", []string{"questions", "A-1"}, map[string]any{"assignment_id": "A-1"}},
		{"answer", "workflow_answer", []string{"answer", "1", "--assignment", "A-1", "--answer", "yes"}, map[string]any{"question_id": "1", "assignment_id": "A-1", "answer": "yes"}},
		{"observe", "observe_list", []string{"observe", "list", "A-1"}, map[string]any{"assignment_id": "A-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(tt.argv)
			if err != nil {
				t.Fatal(err)
			}
			if got.action != tt.action || !sameJSON(got.payload, tt.want) {
				t.Fatalf("parse(%q) = %#v, want action %q payload %#v", tt.argv, got, tt.action, tt.want)
			}
		})
	}
}

func TestRunUsageErrorsDoNotCall(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	recorded := &recorder{}
	newCaller = func(string) Caller { return recorded }
	for _, argv := range [][]string{{"unknown"}, {"show"}, {"mine", "--unknown"}} {
		if code := Run(context.Background(), argv, mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), io.Discard, io.Discard); code != 2 {
			t.Fatalf("Run(%q) = %d, want 2", argv, code)
		}
	}
	if len(recorded.calls) != 0 {
		t.Fatalf("calls = %#v, want none", recorded.calls)
	}
}

func TestRunJSONAndVersion(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	recorded := &recorder{result: json.RawMessage(`{"ok":true}`)}
	newCaller = func(string) Caller { return recorded }
	var out strings.Builder
	if code := Run(context.Background(), []string{"--version"}, mapEnv(), &out, io.Discard); code != 0 || strings.TrimSpace(out.String()) != version.Version {
		t.Fatalf("version = %d %q, want 0 %q", code, out.String(), version.Version)
	}
	out.Reset()
	if code := Run(context.Background(), []string{"mine", "--json"}, mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), &out, io.Discard); code != 0 || strings.TrimSpace(out.String()) != `{"ok":true}` {
		t.Fatalf("json = %d %q", code, out.String())
	}
}

func TestAgentModeNeverFallsBack(t *testing.T) {
	env := mapEnv("TARIBOY_TOOLS_SOCKET", "/missing/agent.sock", "TARIBOY_RUNTIME_DIR", t.TempDir())
	code := Run(context.Background(), []string{"mine"}, env, io.Discard, io.Discard)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestOnlyToolsSocketSelectsAgentMode(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	var sockets []string
	newCaller = func(socket string) Caller {
		sockets = append(sockets, socket)
		return &recorder{result: json.RawMessage(`{}`)}
	}
	env := mapEnv("TARIBOY_BASE_DIR", t.TempDir(), "TARIBOY_RUNTIME_DIR", t.TempDir())
	if code := Run(context.Background(), []string{"mine"}, env, io.Discard, io.Discard); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if len(sockets) != 1 || !strings.HasSuffix(sockets[0], "tariboyd.sock") {
		t.Fatalf("sockets = %q, want operator socket", sockets)
	}
}

func sameJSON(got, want any) bool {
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	return string(g) == string(w)
}
