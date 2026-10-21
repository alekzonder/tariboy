package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// One agent's create fails: the other agent still converges, the failed
// agent's dependent actions (start) and its budget are skipped, Up returns an
// error naming the failure, and the report shows failed and skipped.
func TestUpCreateFailureDoesNotBlockOtherAgents(t *testing.T) {
	fc := newFake()
	f := File{Version: 1, Agents: map[string]AgentSpec{
		"a": {Image: "img:latest", Budget: &BudgetSpec{LimitUSD: 5}},
		"b": {Image: "img:latest"},
	}}
	// Both creates share one route, so fail a's create by its body.
	caller := &failCreateCaller{fakeCaller: fc, failName: "a"}
	var out strings.Builder
	r := NewRunner(caller, "", &out)
	r.skipBuild = true
	err := r.Up(f)
	if err == nil || !strings.Contains(err.Error(), "create agent a") {
		t.Fatalf("Up = %v; want the create failure of a", err)
	}
	if fc.agents["b"] == nil || countCalls(fc, "POST /api/agents/b/start") != 1 {
		t.Fatalf("agent b did not converge after a failed: %v", fc.calls)
	}
	if n := countCalls(fc, "POST /api/agents/a/start"); n != 0 {
		t.Fatalf("start of a ran after its create failed: %v", fc.calls)
	}
	if n := countCalls(fc, "POST /api/budgets"); n != 0 {
		t.Fatalf("budget of the failed agent was applied: %v", fc.calls)
	}
	got := out.String()
	for _, want := range []string{"failed   agent:a", "skipped  budget:agent:a", "changed  agent:b"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}

// failCreateCaller fails POST /api/agents for one agent name.
type failCreateCaller struct {
	*fakeCaller
	failName string
}

func (c *failCreateCaller) Call(method, route string, body any) (json.RawMessage, error) {
	if method == "POST" && route == "/api/agents" {
		if m, ok := body.(map[string]any); ok && m["name"] == c.failName {
			c.calls = append(c.calls, method+" "+route)
			c.bodies = append(c.bodies, body)
			return nil, errors.New("boom")
		}
	}
	return c.fakeCaller.Call(method, route, body)
}

// A failed independent action on one resource does not stop the other actions
// of the same resource.
func TestUpIndependentAgentActionsContinueAfterFailure(t *testing.T) {
	fc := newFake()
	fc.fail = map[string]error{"POST /api/agents/a/cwd": errors.New("bad cwd")}
	fc.agents["a"] = map[string]any{"name": "a", "state": "running", "image": "img:latest", "harness": "claude", "cwd": "/old", "timeout_s": 1}
	dir := t.TempDir()
	r := NewRunner(fc, dir, io.Discard)
	r.skipBuild = true
	err := r.Up(File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "img:latest", Timeout: "1m"}}})
	if err == nil || !strings.Contains(err.Error(), "set cwd a") {
		t.Fatalf("Up = %v", err)
	}
	if countCalls(fc, "POST /api/agents/a/loop/timeout") != 1 {
		t.Fatalf("timeout was not converged after the cwd failure: %v", fc.calls)
	}
}

func TestUpImageDriftSetsPendingImage(t *testing.T) {
	fc := newFake()
	fc.agents["a"] = map[string]any{"name": "a", "state": "running", "enabled": true, "image": "img:old", "harness": "claude"}
	r := NewRunner(fc, "", io.Discard)
	f := File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "img:new"}}}
	upNoBuild(t, r, f)
	if got := bodyFor(fc, "POST /api/agents/a/image"); !reflect.DeepEqual(got, map[string]any{"image": "img:new"}) {
		t.Fatalf("image body = %#v (calls %v)", got, fc.calls)
	}
	for _, c := range []string{"POST /api/agents/a/stop", "POST /api/agents/a/start", "POST /api/agents/a/reprovision"} {
		if countCalls(fc, c) != 0 {
			t.Fatalf("image drift restarted the agent: %v", fc.calls)
		}
	}
	// The image is pending until the next iteration: a second up sees the
	// pending image and does not set it again.
	before := len(fc.calls)
	upNoBuild(t, r, f)
	if n := countCallsSince(fc.calls, before, "POST /api/agents/a/image"); n != 0 {
		t.Fatalf("second up set the pending image again: %v", fc.calls[before:])
	}
}

// A bare image name in the file means NAME:latest, as the daemon stores it.
func TestUpBareImageNameMatchesLatest(t *testing.T) {
	fc := newFake()
	fc.agents["a"] = map[string]any{"name": "a", "state": "running", "enabled": true, "image": "img:latest", "harness": "claude"}
	upNoBuild(t, NewRunner(fc, "", io.Discard), File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "img"}}})
	if n := countCalls(fc, "POST /api/agents/a/image"); n != 0 {
		t.Fatalf("img and img:latest treated as drift: %v", fc.calls)
	}
}

func TestUpHarnessDriftRestartsRunningAgent(t *testing.T) {
	fc := newFake()
	fc.agents["a"] = map[string]any{"name": "a", "state": "running", "enabled": true, "image": "img:latest", "harness": "codex"}
	r := NewRunner(fc, "", io.Discard)
	f := File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "img:latest", Harness: &HarnessSpec{Type: "claude"}}}}
	upNoBuild(t, r, f)
	if got := bodyFor(fc, "POST /api/agents/a/harness"); !reflect.DeepEqual(got, map[string]any{"value": "claude"}) {
		t.Fatalf("harness body = %#v (calls %v)", got, fc.calls)
	}
	set, stop, start := callIndex(fc.calls, "POST /api/agents/a/harness"), callIndex(fc.calls, "POST /api/agents/a/stop"), callIndex(fc.calls, "POST /api/agents/a/start")
	if set < 0 || stop < 0 || start < 0 || !(set < stop && stop < start) {
		t.Fatalf("want harness, stop, start in order: %v", fc.calls)
	}
	if fc.agents["a"]["state"] != "running" {
		t.Fatalf("agent not running after restart: %#v", fc.agents["a"])
	}
	before := len(fc.calls)
	upNoBuild(t, r, f)
	for _, c := range fc.calls[before:] {
		if strings.HasPrefix(c, "POST ") {
			t.Fatalf("converged harness made a mutating call: %v", fc.calls[before:])
		}
	}
}

// A stopped agent is re-provisioned by up, which starts it with the new
// harness: no separate restart.
func TestUpHarnessDriftOnStoppedAgentUsesReprovision(t *testing.T) {
	fc := newFake()
	fc.agents["a"] = map[string]any{"name": "a", "state": "stopped", "enabled": false, "image": "img:latest", "harness": "codex"}
	upNoBuild(t, NewRunner(fc, "", io.Discard), File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "img:latest", Harness: &HarnessSpec{Type: "claude"}}}})
	set, reprovision := callIndex(fc.calls, "POST /api/agents/a/harness"), callIndex(fc.calls, "POST /api/agents/a/reprovision")
	if set < 0 || reprovision < 0 || set > reprovision {
		t.Fatalf("want harness before reprovision: %v", fc.calls)
	}
	if countCalls(fc, "POST /api/agents/a/stop") != 0 || countCalls(fc, "POST /api/agents/a/start") != 0 {
		t.Fatalf("stopped agent restarted besides reprovision: %v", fc.calls)
	}
}

// A file that disables the loop does not restart the agent for a harness
// change; the loop step stops it and the next start uses the new harness.
func TestUpHarnessDriftWithLoopDisabledDoesNotRestart(t *testing.T) {
	off := false
	fc := newFake()
	fc.agents["a"] = map[string]any{"name": "a", "state": "running", "enabled": true, "image": "img:latest", "harness": "codex"}
	upNoBuild(t, NewRunner(fc, "", io.Discard), File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "img:latest", Harness: &HarnessSpec{Type: "claude"}, Loop: &LoopSpec{Enabled: &off}}}})
	if countCalls(fc, "POST /api/agents/a/start") != 0 || countCalls(fc, "POST /api/agents/a/stop") != 1 {
		t.Fatalf("want exactly one stop and no start: %v", fc.calls)
	}
}

// When the harness change fails, the restart that would apply it is skipped.
func TestUpHarnessFailureSkipsRestart(t *testing.T) {
	fc := newFake()
	fc.fail = map[string]error{"POST /api/agents/a/harness": errors.New("bad harness")}
	fc.agents["a"] = map[string]any{"name": "a", "state": "running", "enabled": true, "image": "img:latest", "harness": "codex"}
	var out strings.Builder
	r := NewRunner(fc, "", &out)
	r.skipBuild = true
	if err := r.Up(File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "img:latest", Harness: &HarnessSpec{Type: "claude"}}}}); err == nil {
		t.Fatal("Up succeeded despite the harness failure")
	}
	if countCalls(fc, "POST /api/agents/a/stop") != 0 || countCalls(fc, "POST /api/agents/a/start") != 0 {
		t.Fatalf("restart ran after a failed harness change: %v", fc.calls)
	}
	if !strings.Contains(out.String(), "failed   agent:a") {
		t.Fatalf("report:\n%s", out.String())
	}
}

func TestComposeUpExitsNonZeroAndPrintsReportOnFailure(t *testing.T) {
	t.Setenv("TARIBOY_NO_AUTOSTART", "1")
	dir := t.TempDir()
	path := filepath.Join(dir, "tariboy-compose.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nagents:\n  a: {image: img:latest}\n  b: {image: img:latest}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fc := newFake()
	caller := &failCreateCaller{fakeCaller: fc, failName: "a"}
	var out, errOut strings.Builder
	if code := Run(context.Background(), caller, Options{Verb: "up", File: path}, &out, &errOut); code != 1 {
		t.Fatalf("code = %d; want 1 (stderr %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "failed   agent:a") || !strings.Contains(out.String(), "changed  agent:b") {
		t.Fatalf("report:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "create agent a") {
		t.Fatalf("stderr = %s", errOut.String())
	}
}

func TestComposeImportWithoutYesReadsAnswerFromStdin(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "team.tar.gz")
	if err := os.WriteFile(archivePath, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		answer  string
		applied int
	}{{"y\n", 1}, {"n\n", 0}, {"", 0}} {
		caller := &archiveImportCaller{fakeCaller: newFake()}
		var stderr bytes.Buffer
		if code := Run(context.Background(), caller, Options{Verb: "import", Archive: archivePath, Stdin: strings.NewReader(tc.answer)}, io.Discard, &stderr); code != 0 {
			t.Fatalf("answer %q: code=%d stderr=%s", tc.answer, code, stderr.String())
		}
		if got := countCalls(caller.fakeCaller, "POST /api/team-imports/import-1/apply"); got != tc.applied {
			t.Fatalf("answer %q: apply calls = %d, want %d", tc.answer, got, tc.applied)
		}
	}
}
