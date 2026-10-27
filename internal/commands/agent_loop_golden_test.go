package commands

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
)

var updateAgentLoopGolden = flag.Bool("update-agent-loop-golden", false, "rewrite testdata/agent_loop_responses.golden.json")

// agentLoopGoldenVolatile names keys whose values depend on the wall clock:
// file modification times, audit timestamps and the next scheduled start.
var agentLoopGoldenVolatile = map[string]bool{"mtime": true, "ts": true, "schedule_next_at": true}

// agentLoopSchedule is a schedule with an explicit DTSTART, so the stored rule
// does not depend on the time of the call.
const agentLoopSchedule = "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0"

// agentLoopFixture stores the agents the agent settings, loop, prompt and
// file commands read: alpha has a cwd with files, a v1 image with layers,
// a context, a schedule and status history; beta has a v1 image without
// layers; bare runs the bare image; v2 has a v2 prompt template. It returns
// the context, the fake control and alpha's cwd.
func agentLoopFixture(t *testing.T) (*registry.Ctx, *fakeControl, string) {
	t.Helper()
	c, as, fc := ctxWithStore(t)
	cwd := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(cwd, "notes.txt"), "hello file")
	write(filepath.Join(cwd, "blob.bin"), "a\x00b")
	for _, a := range []agent.Agent{
		{
			Name: "alpha", ImageRef: "basic:latest", ImageDigest: "digest-alpha", Cwd: cwd, HarnessType: "stub",
			Model: "m1", Effort: "high", Interactive: true, IntervalS: 30, TimeoutS: 60, HardTimeoutS: 120,
			OnTimeout: "stop", OnError: "restart", UserPrompt: "do work", MaxIdleIterations: 3, AIStallTimeoutS: 1800,
		},
		{Name: "beta", ImageRef: "basic:latest", OnTimeout: "restart", OnError: "restart"},
		{Name: "bare", ImageRef: image.BareRef.String()},
		{Name: "v2", ImageRef: "flow:latest", ImageDigest: "digest-v2", UserPrompt: "USER"},
	} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := as.SetAlias("alpha", "Alpha"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetNotes("alpha", "some notes"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetColor("alpha", "#4f8cff"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetSchedule("alpha", agentLoopSchedule); err != nil {
		t.Fatal(err)
	}
	dir := func(name string) agentdir.Layout { return agentdir.New(agentsDir(c), name) }
	write(dir("alpha").ContextPath(), "remember X")
	write(filepath.Join(dir("alpha").ImageDir(), "PROMPT.md"), "# image prompt")
	write(filepath.Join(dir("alpha").ImageDir(), "manifest.json"), `{"schema_version":1,"name":"basic","tag":"latest","layers":[{"name":"system","sha256":"abc"}]}`)
	write(filepath.Join(dir("beta").ImageDir(), "PROMPT.md"), "# beta prompt")
	write(filepath.Join(dir("beta").ImageDir(), "manifest.json"), `{"schema_version":1,"name":"basic","tag":"latest"}`)
	write(filepath.Join(dir("bare").ImageDir(), "manifest.json"), `{"schema_version":2,"name":"bare","tag":"latest"}`)
	write(filepath.Join(dir("v2").ImageDir(), "manifest.json"), `{"schema_version":2,"name":"flow","tag":"latest"}`)
	write(filepath.Join(dir("v2").ImageDir(), "prompt", "layers", "000.md"), "STATIC")
	entries := []image.TemplateEntry{
		{Kind: "file", Source: "./prompt.md", Category: "prompt", ArchivePath: "prompt/layers/000.md", Size: 6, SHA256: imageSHA256("STATIC")},
		{Kind: "runtime", Runtime: "user-prompt"},
	}
	templateSHA, err := image.PromptTemplateHash(entries)
	if err != nil {
		t.Fatal(err)
	}
	template, err := json.Marshal(image.PromptTemplate{SchemaVersion: 2, Entries: entries, SHA256: templateSHA})
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(dir("v2").ImageDir(), "prompt", "template.json"), string(template))
	log := audit.Open(dir("alpha").AuditLog(), nil)
	log.Record("status", "agent", "alpha-1", map[string]any{"message": "first"})
	log.Record("iteration", "system", "", map[string]any{"foo": "bar"})
	log.Record("status", "agent", "", map[string]any{"message": "second"})
	return c, fc, cwd
}

// Catches a typed agent settings, loop, prompt or file response changing the
// JSON a client sees: the set of keys, their JSON types, and their values
// must match the recorded responses.
func TestAgentLoopResponsesMatchGolden(t *testing.T) {
	c, _, cwd := agentLoopFixture(t)
	calls := []struct {
		label, path string
		params      registry.Params
	}{
		{"agent.cwd read", "agent.cwd", registry.Params{"name": "alpha"}},
		{"agent.model read", "agent.model", registry.Params{"name": "alpha"}},
		{"agent.model set", "agent.model", registry.Params{"name": "alpha", "value": "m2"}},
		{"agent.effort read", "agent.effort", registry.Params{"name": "alpha"}},
		{"agent.effort set", "agent.effort", registry.Params{"name": "alpha", "value": "low"}},
		{"agent.harness read", "agent.harness", registry.Params{"name": "alpha"}},
		{"agent.harness set", "agent.harness", registry.Params{"name": "alpha", "value": "codex"}},
		{"agent.interactive read", "agent.interactive", registry.Params{"name": "alpha"}},
		{"agent.interactive set", "agent.interactive", registry.Params{"name": "alpha", "value": false}},
		{"agent.env read", "agent.env", registry.Params{"name": "alpha"}},
		{"agent.env set", "agent.env", registry.Params{"name": "alpha", "value": map[string]any{"MODE": "test"}}},
		{"agent.alias.get", "agent.alias.get", registry.Params{"name": "alpha"}},
		{"agent.alias.set", "agent.alias.set", registry.Params{"name": "alpha", "value": "A"}},
		{"agent.alias.set clear", "agent.alias.set", registry.Params{"name": "beta", "value": ""}},
		{"agent.notes.get", "agent.notes.get", registry.Params{"name": "alpha"}},
		{"agent.notes.set", "agent.notes.set", registry.Params{"name": "alpha", "value": "new notes"}},
		{"agent.color.get", "agent.color.get", registry.Params{"name": "alpha"}},
		{"agent.color.set", "agent.color.set", registry.Params{"name": "alpha", "value": "#00ff00"}},
		{"loop.interval read", "loop.interval", registry.Params{"name": "alpha"}},
		{"loop.interval set", "loop.interval", registry.Params{"name": "alpha", "value": float64(45)}},
		{"loop.timeout", "loop.timeout", registry.Params{"name": "alpha", "value": float64(90)}},
		{"loop.hard-timeout", "loop.hard-timeout", registry.Params{"name": "alpha", "value": float64(180)}},
		{"loop.max-idle", "loop.max-idle", registry.Params{"name": "alpha", "value": float64(4)}},
		{"loop.ai-stall-timeout", "loop.ai-stall-timeout", registry.Params{"name": "alpha", "value": float64(600)}},
		{"loop.on-timeout read", "loop.on-timeout", registry.Params{"name": "alpha"}},
		{"loop.on-timeout set", "loop.on-timeout", registry.Params{"name": "alpha", "value": "restart"}},
		{"loop.on-error", "loop.on-error", registry.Params{"name": "alpha", "value": "stop"}},
		{"loop.schedule read", "loop.schedule", registry.Params{"name": "alpha"}},
		{"loop.schedule none", "loop.schedule", registry.Params{"name": "beta"}},
		{"loop.schedule set", "loop.schedule", registry.Params{"name": "beta", "value": agentLoopSchedule}},
		{"loop.schedule clear", "loop.schedule", registry.Params{"name": "beta", "clear": true}},
		{"user-prompt.get", "user-prompt.get", registry.Params{"name": "alpha"}},
		{"user-prompt.set", "user-prompt.set", registry.Params{"name": "alpha", "text": "new prompt"}},
		{"context.get", "context.get", registry.Params{"name": "alpha"}},
		{"context.get missing", "context.get", registry.Params{"name": "beta"}},
		{"context.set", "context.set", registry.Params{"name": "beta", "text": "beta context"}},
		{"prompt.get v1", "prompt.get", registry.Params{"name": "alpha"}},
		{"prompt.get v1 without layers", "prompt.get", registry.Params{"name": "beta"}},
		{"prompt.get bare", "prompt.get", registry.Params{"name": "bare"}},
		{"prompt.get v2", "prompt.get", registry.Params{"name": "v2"}},
		{"agent.exec", "agent.exec", registry.Params{"name": "alpha", "prompt": "now"}},
		{"agent.screen", "agent.screen", registry.Params{"name": "alpha"}},
		{"agent.send-keys line", "agent.send-keys", registry.Params{"name": "alpha", "keys": "ls"}},
		{"agent.send-keys items", "agent.send-keys", registry.Params{"name": "alpha", "items": []any{map[string]any{"text": "hi"}, map[string]any{"key": "Enter"}}}},
		{"agent.shell-script.get empty", "agent.shell-script.get", registry.Params{"name": "alpha"}},
		{"agent.shell-script.set", "agent.shell-script.set", registry.Params{"name": "alpha", "script": "echo hi\n"}},
		{"agent.shell-script.get", "agent.shell-script.get", registry.Params{"name": "alpha"}},
		{"agent.status.history", "agent.status.history", registry.Params{"name": "alpha"}},
		{"agent.status.history limit", "agent.status.history", registry.Params{"name": "alpha", "limit": "1"}},
		{"agent.status.history empty", "agent.status.history", registry.Params{"name": "beta"}},
		{"agent.pull", "agent.pull", registry.Params{"name": "alpha", "path": "notes.txt"}},
		{"agent.file.list", "agent.file.list", registry.Params{"name": "alpha"}},
		{"agent.file.read text", "agent.file.read", registry.Params{"name": "alpha", "path": "notes.txt"}},
		{"agent.file.read binary", "agent.file.read", registry.Params{"name": "alpha", "path": "blob.bin"}},
		{"agent.file.write", "agent.file.write", registry.Params{"name": "alpha", "path": "notes.txt", "content": "changed"}},
		{"agent.file.create file", "agent.file.create", registry.Params{"name": "alpha", "path": "new.txt"}},
		{"agent.file.create dir", "agent.file.create", registry.Params{"name": "alpha", "path": "sub", "type": "dir"}},
		{"agent.file.rename", "agent.file.rename", registry.Params{"name": "alpha", "from": "new.txt", "to": "sub/moved.txt"}},
		{"agent.file.delete", "agent.file.delete", registry.Params{"name": "alpha", "path": "sub/moved.txt"}},
		{"agent.file.list empty", "agent.file.list", registry.Params{"name": "alpha", "path": "sub"}},
		{"agent.cwd set", "agent.cwd", registry.Params{"name": "alpha", "value": filepath.Join(cwd, "made"), "create": true}},
		{"agent.cwd clear", "agent.cwd", registry.Params{"name": "alpha", "value": ""}},
	}
	recorder := newGoldenRecorder(t, agentLoopGoldenVolatile, cwd, "$CWD", c.BaseDir, "$BASE")
	for _, call := range calls {
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, value)
	}
	recorder.check("agent_loop_responses.golden.json", *updateAgentLoopGolden)
}

func init() { typedCallSets["agent-loop"] = agentLoopTypedCalls }

// agentLoopTypedCalls covers every agent setting, loop, prompt, file browser,
// interactive session and shell script route, with the states that encode a
// null or an absent key: a binary file read, a v1 prompt without layers, a
// bare prompt, and both send-keys result shapes.
func agentLoopTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, _, cwd := agentLoopFixture(t)
	return c, []typedCall{
		{"POST", "/api/agents/{name}/cwd", "/api/agents/alpha/cwd", map[string]any{}},
		{"POST", "/api/agents/{name}/model", "/api/agents/alpha/model", map[string]any{"value": "m2"}},
		{"POST", "/api/agents/{name}/effort", "/api/agents/alpha/effort", map[string]any{"value": "low"}},
		{"POST", "/api/agents/{name}/harness", "/api/agents/alpha/harness", map[string]any{"value": "codex"}},
		{"POST", "/api/agents/{name}/interactive", "/api/agents/alpha/interactive", map[string]any{"value": false}},
		{"POST", "/api/agents/{name}/env", "/api/agents/alpha/env", map[string]any{"value": map[string]any{"MODE": "test"}}},
		{"GET", "/api/agents/{name}/alias", "/api/agents/alpha/alias", nil},
		{"POST", "/api/agents/{name}/alias", "/api/agents/alpha/alias", map[string]any{"value": "A"}},
		{"GET", "/api/agents/{name}/notes", "/api/agents/alpha/notes", nil},
		{"POST", "/api/agents/{name}/notes", "/api/agents/alpha/notes", map[string]any{"value": "n"}},
		{"GET", "/api/agents/{name}/color", "/api/agents/alpha/color", nil},
		{"POST", "/api/agents/{name}/color", "/api/agents/alpha/color", map[string]any{"value": ""}},
		{"POST", "/api/agents/{name}/loop/interval", "/api/agents/alpha/loop/interval", map[string]any{"value": 45}},
		{"POST", "/api/agents/{name}/loop/timeout", "/api/agents/alpha/loop/timeout", map[string]any{"value": 90}},
		{"POST", "/api/agents/{name}/loop/hard-timeout", "/api/agents/alpha/loop/hard-timeout", map[string]any{"value": 180}},
		{"POST", "/api/agents/{name}/loop/max-idle", "/api/agents/alpha/loop/max-idle", map[string]any{"value": 4}},
		{"POST", "/api/agents/{name}/loop/ai-stall-timeout", "/api/agents/alpha/loop/ai-stall-timeout", map[string]any{"value": 600}},
		{"POST", "/api/agents/{name}/loop/on-timeout", "/api/agents/alpha/loop/on-timeout", map[string]any{"value": "restart"}},
		{"POST", "/api/agents/{name}/loop/on-error", "/api/agents/alpha/loop/on-error", map[string]any{}},
		{"POST", "/api/agents/{name}/loop/schedule", "/api/agents/alpha/loop/schedule", map[string]any{}},
		{"POST", "/api/agents/{name}/loop/schedule", "/api/agents/beta/loop/schedule", map[string]any{"clear": true}},
		{"GET", "/api/agents/{name}/user-prompt", "/api/agents/alpha/user-prompt", nil},
		{"POST", "/api/agents/{name}/user-prompt", "/api/agents/alpha/user-prompt", map[string]any{"text": "p"}},
		{"GET", "/api/agents/{name}/context", "/api/agents/alpha/context", nil},
		{"POST", "/api/agents/{name}/context", "/api/agents/beta/context", map[string]any{"text": "c"}},
		{"GET", "/api/agents/{name}/prompt", "/api/agents/alpha/prompt", nil},
		{"GET", "/api/agents/{name}/prompt", "/api/agents/beta/prompt", nil},
		{"GET", "/api/agents/{name}/prompt", "/api/agents/bare/prompt", nil},
		{"GET", "/api/agents/{name}/prompt", "/api/agents/v2/prompt", nil},
		{"POST", "/api/agents/{name}/exec", "/api/agents/alpha/exec", map[string]any{"prompt": "now"}},
		{"GET", "/api/agents/{name}/screen", "/api/agents/alpha/screen", nil},
		{"POST", "/api/agents/{name}/send-keys", "/api/agents/alpha/send-keys", map[string]any{"keys": "ls"}},
		{"POST", "/api/agents/{name}/send-keys", "/api/agents/alpha/send-keys", map[string]any{"items": []any{map[string]any{"key": "Enter"}}}},
		{"GET", "/api/agents/{name}/shell-script", "/api/agents/alpha/shell-script", nil},
		{"POST", "/api/agents/{name}/shell-script", "/api/agents/alpha/shell-script", map[string]any{"script": "echo hi\n"}},
		{"GET", "/api/agents/{name}/status/history", "/api/agents/alpha/status/history", nil},
		{"GET", "/api/agents/{name}/status/history", "/api/agents/beta/status/history", nil},
		{"GET", "/api/agents/{name}/files", "/api/agents/alpha/files?path=notes.txt", nil},
		{"GET", "/api/agents/{name}/file/list", "/api/agents/alpha/file/list", nil},
		{"GET", "/api/agents/{name}/file", "/api/agents/alpha/file?path=notes.txt", nil},
		{"GET", "/api/agents/{name}/file", "/api/agents/alpha/file?path=blob.bin", nil},
		{"PUT", "/api/agents/{name}/file", "/api/agents/alpha/file", map[string]any{"path": "notes.txt", "content": "changed"}},
		{"POST", "/api/agents/{name}/file", "/api/agents/alpha/file", map[string]any{"path": "new.txt"}},
		{"POST", "/api/agents/{name}/file/rename", "/api/agents/alpha/file/rename", map[string]any{"from": "new.txt", "to": "moved.txt"}},
		{"DELETE", "/api/agents/{name}/file", "/api/agents/alpha/file?path=moved.txt", nil},
		{"POST", "/api/agents/{name}/cwd", "/api/agents/alpha/cwd", map[string]any{"value": filepath.Join(cwd, "made"), "create": true}},
	}
}
