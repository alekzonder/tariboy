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
