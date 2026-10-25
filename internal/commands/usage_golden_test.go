package commands

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/registry"
)

var updateUsageGolden = flag.Bool("update-usage-golden", false, "rewrite testdata/usage_responses.golden.json")

// usageFixedDay is the day of the fixture's recorded AI requests.
var usageFixedDay = time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)

// usageTranscriptLine is one proxy transcript entry with a text response.
const usageTranscriptLine = `{"meta":{"TS":"2026-07-06T09:00:00Z","Provider":"anthropic","Model":"m","InputTokens":3},"request":"eyJzeXN0ZW0iOiJTMSIsIm1lc3NhZ2VzIjpbeyJyb2xlIjoidXNlciIsImNvbnRlbnQiOiJoaSJ9XX0=","response":"eyJzdG9wX3JlYXNvbiI6ImVuZF90dXJuIiwiY29udGVudCI6W3sidHlwZSI6InRleHQiLCJ0ZXh0Ijoib2sifV19"}` + "\n"

// usageFixture returns a context with agents alice and bob. alice has a done
// iteration alice-1 (exit code, resource use, harness logs, a transcript and
// audit events) and a running alice-2 without them. AI requests on
// usageFixedDay carry task, group and model keys; bob has one request an hour
// ago so budget.status reports spend. The budgets cover the agent, group and
// global scopes.
func usageFixture(t *testing.T) *registry.Ctx {
	t.Helper()
	c, as, fc := ctxWithStore(t)
	fc.extendResult = registry.IterationTimeoutExtension{TimeoutDeadline: "2026-07-06T10:00:00Z", HardTimeoutDeadline: "2026-07-06T11:00:00Z", TimeoutExtensions: 1, ShimSync: "pending"}
	for _, name := range []string{"alice", "bob"} {
		if err := as.Create(agent.Agent{Name: name, ImageRef: "basic:latest"}); err != nil {
			t.Fatal(err)
		}
	}
	exit, cpu, mem := 0, 1200, 4096
	iterations := []agent.Iteration{
		{ID: "alice-1", Agent: "alice", Trigger: "interval", Status: "done", StartedAt: "2026-07-06T09:00:00Z",
			EndedAt: "2026-07-06T09:05:00Z", ExitCode: &exit, DoneFlag: true, Productive: true, PromptPath: "prompt.md",
			CPUMs: &cpu, MemPeakKB: &mem, ImageRef: "basic:latest", ImageVersion: "1.2.3", ImageDigest: "sha256:abc",
			PromptTemplateSHA256: "def"},
		{ID: "alice-2", Agent: "alice", Trigger: "message", Status: "running", StartedAt: "2026-07-06T10:00:00Z"},
	}
	for _, it := range iterations {
		if err := as.CreateIteration(it); err != nil {
			t.Fatal(err)
		}
	}
	l := agentdir.New(agentsDir(c), "alice")
	if err := l.EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{
		l.HarnessStdout("alice-1"): "out-line",
		l.HarnessStderr("alice-1"): "err-line",
		filepath.Join(l.IterationDir("alice-1"), "proxy-transcript.jsonl"): usageTranscriptLine,
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := audit.Open(l.AuditLog(), func() time.Time { return usageFixedDay })
	log.Record("iteration_started", "system", "alice-1", nil)
	log.Record("shim", "shim", "alice-1", map[string]any{"line": "boom"})
	log.Record("status", "agent", "", map[string]any{"text": "building"})
	log.Record("iteration_started", "system", "alice-2", nil)

	ap := aiproxy.NewStore(c.Store, nil)
	requests := []aiproxy.AIRequest{
		{ID: "r1", TS: usageFixedDay.Format(time.RFC3339Nano), Agent: "alice", Iteration: "alice-1", ImageName: "basic",
			Provider: "anthropic", Model: "claude-opus-4-8", InputTokens: 100, OutputTokens: 50, CacheWriteTokens: 10,
			CacheReadTokens: 5, CostUSD: 0.10, Status: "ok", TaskID: "USE-2", EpicID: "USE-1", GroupID: "g1", GroupName: "ops"},
		{ID: "r2", TS: usageFixedDay.Add(time.Hour).Format(time.RFC3339Nano), Agent: "alice", Iteration: "alice-2", ImageName: "basic",
			Provider: "openai", Model: "gpt-5", InputTokens: 20, OutputTokens: 10, CostUSD: 0.02, Status: "upstream_error"},
		{ID: "r3", TS: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), Agent: "bob", ImageName: "basic",
			Provider: "anthropic", Model: "claude-opus-4-8", InputTokens: 1, OutputTokens: 1, CostUSD: 0.25, Status: "ok"},
	}
	for _, r := range requests {
		if err := ap.Insert(r); err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range []aiproxy.Budget{
		{Scope: "agent:bob", LimitUSD: 0.20, PeriodS: 86400, Mode: "block"},
		{Scope: "group:ops", LimitUSD: 5, PeriodS: 3600, Mode: "warn"},
		{Scope: "global", LimitUSD: 10, PeriodS: 86400, Mode: "warn"},
	} {
		if err := ap.SetBudget(b); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// Catches a typed usage, budget, audit log or iteration response changing
// the JSON a client sees.
func TestUsageResponsesMatchGolden(t *testing.T) {
	c := usageFixture(t)
	empty, _, _ := ctxWithStore(t)
	window := registry.Params{"since": "2026-07-06T00:00:00Z", "until": "2026-07-07T00:00:00Z"}
	with := func(p registry.Params, extra registry.Params) registry.Params {
		out := registry.Params{}
		for k, v := range p {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	type call struct {
		label, path string
		c           *registry.Ctx
		params      registry.Params
	}
	calls := []call{
		{"usage window", "usage", c, window},
		{"usage group", "usage", c, with(window, registry.Params{"group": "g1"})},
		{"usage empty", "usage", empty, registry.Params{}},
		{"agent.usage iteration", "agent.usage", c, with(window, registry.Params{"name": "alice", "bucket": "1d"})},
		{"agent.usage task", "agent.usage", c, with(window, registry.Params{"name": "alice", "group_by": "task", "bucket": "1d"})},
		{"agent.usage model", "agent.usage", c, with(window, registry.Params{"name": "alice", "group_by": "model", "bucket": "1h"})},
		{"budget.set", "budget.set", c, registry.Params{"scope": "agent:alice", "limit-usd": "1.5", "period": "12h", "mode": "block"}},
		{"budget.set defaults", "budget.set", empty, registry.Params{"scope": "global", "limit-usd": "3"}},
		{"budget.ls", "budget.ls", c, registry.Params{}},
		{"budget.ls defaults", "budget.ls", empty, registry.Params{}},
		{"budget.status", "budget.status", c, registry.Params{}},
		{"budget.status defaults", "budget.status", empty, registry.Params{}},
		{"logs recent", "logs", c, registry.Params{"name": "alice"}},
		{"logs since", "logs", c, registry.Params{"name": "alice", "since": "2"}},
		{"logs before", "logs", c, registry.Params{"name": "alice", "before": "3", "limit": "1"}},
		{"logs iteration", "logs", c, registry.Params{"name": "alice", "iteration": "alice-1"}},
		{"logs type", "logs", c, registry.Params{"name": "alice", "type": "status"}},
		{"logs query", "logs", c, registry.Params{"name": "alice", "q": "BOOM"}},
		{"logs distinct", "logs", c, registry.Params{"name": "alice", "distinct": "types"}},
		{"logs distinct empty", "logs", c, registry.Params{"name": "bob", "distinct": "types"}},
		{"logs empty", "logs", c, registry.Params{"name": "bob"}},
		{"transcript", "transcript", c, registry.Params{"name": "alice", "id": "alice-1"}},
		{"transcript raw", "transcript", c, registry.Params{"name": "alice", "id": "alice-1", "raw": "true"}},
		{"transcript missing", "transcript", c, registry.Params{"name": "alice", "id": "alice-2"}},
		{"transcript raw missing", "transcript", c, registry.Params{"name": "alice", "id": "alice-2", "raw": "true"}},
		{"iteration.extend-timeout", "iteration.extend-timeout", c, registry.Params{"name": "alice", "id": "alice-2"}},
		{"iteration.tag.add", "iteration.tag.add", c, registry.Params{"name": "alice", "id": []string{"alice-1", "alice-2"}, "tag": []string{"keep", "review"}}},
		{"iteration.tag.rm", "iteration.tag.rm", c, registry.Params{"name": "alice", "id": []string{"alice-2"}, "tag": []string{"review"}}},
		{"iteration.tag.set clear", "iteration.tag.set", c, registry.Params{"name": "alice", "id": []string{"alice-2"}}},
		{"iteration.ls", "iteration.ls", c, registry.Params{"name": "alice"}},
		{"iteration.ls filtered", "iteration.ls", c, registry.Params{"name": "alice", "tag": "keep"}},
		{"iteration.ls empty", "iteration.ls", c, registry.Params{"name": "bob"}},
		{"iteration.inspect done", "iteration.inspect", c, registry.Params{"name": "alice", "id": "alice-1"}},
		{"iteration.inspect running", "iteration.inspect", c, registry.Params{"name": "alice", "id": "alice-2"}},
		{"iteration.logs", "iteration.logs", c, registry.Params{"name": "alice", "id": "alice-1"}},
		{"iteration.logs missing", "iteration.logs", c, registry.Params{"name": "alice", "id": "alice-2"}},
	}
	recorder := newGoldenRecorder(t, map[string]bool{}, c.BaseDir, "$BASE", empty.BaseDir, "$EMPTY")
	for _, call := range calls {
		value, err := h(t, call.path)(call.c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, value)
	}
	recorder.check("usage_responses.golden.json", *updateUsageGolden)
}
