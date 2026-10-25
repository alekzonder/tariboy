package commands

import (
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/registry"
)

var updateAgentsGolden = flag.Bool("update-agents-golden", false, "rewrite testdata/agents_responses.golden.json")

// agentsGoldenVolatile names keys whose values depend on the wall clock.
var agentsGoldenVolatile = map[string]bool{"server_now": true, "schedule_next_at": true, "status_updated": true}

// Catches a typed agents response changing the JSON a client sees: the set of
// keys, their JSON types, and their values must match the recorded responses.
func TestAgentsResponsesMatchGolden(t *testing.T) {
	c, as, fc := ctxWithStore(t)
	fixed := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	ref := image.Ref{Name: "reviewer", Tag: "latest"}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "prompt.md"), []byte("prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := &imagefile.V2{SchemaVersion: 2, ImageVersion: "1.0.0", Dir: src, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}
	manifest, err := image.BuildV2(spec, imagefile.ResolveRoots{}, ref, imageStore(c), func() time.Time { return fixed }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := as.Create(agent.Agent{
		Name: "alpha", ImageRef: ref.String(), ImageDigest: manifest.Digest, Cwd: "/srv/alpha",
		HarnessType: "stub", Model: "m1", Effort: "high", Interactive: true, Enabled: true,
		IntervalS: 30, TimeoutS: 60, HardTimeoutS: 120, OnTimeout: "stop", OnError: "restart",
		UserPrompt: "do work", Env: map[string]string{"A": "1"}, Plugins: []string{"context"},
		MessagesBatch: 5, MessagesMaxQueue: 50, GoalEnabled: true, GoalWaitCustomerTimeoutS: 300,
		GoalDeliveryCooldownS: 60, MaxIdleIterations: 3, AIStallTimeoutS: 1800,
	}); err != nil {
		t.Fatal(err)
	}
	if err := as.SetError("alpha", "halted: boom"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetSchedule("alpha", "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0"); err != nil {
		t.Fatal(err)
	}
	if err := as.CreateIteration(agent.Iteration{ID: "alpha-1", Agent: "alpha", Status: "running", StartedAt: "2026-07-14T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := as.InitializeIterationTimeout("alpha-1", 60, 120, fixed); err != nil {
		t.Fatal(err)
	}
	if err := aiproxy.NewStore(c.Store, time.Now).SetAgentBudget("alpha", aiproxy.AgentBudget{HourUSD: 1, DayUSD: 2, WeekUSD: 3, MonthUSD: 4}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(agent.Agent{Name: "beta", ImageRef: "missing:latest", MessagesMaxQueue: 10}); err != nil {
		t.Fatal(err)
	}
	if err := as.SetIdleStopped("beta", agent.IdleStopPrefix+": 3 idle iterations", "2026-07-14T09:30:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := as.CreateIteration(agent.Iteration{ID: "beta-1", Agent: "beta", Status: "done", StartedAt: "2026-07-14T09:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(agent.Agent{Name: "gamma", AIStallTimeoutS: 300}); err != nil {
		t.Fatal(err)
	}

	type call struct {
		label, path string
		params      registry.Params
		live        string
	}
	calls := []call{
		{label: "agent.ps", path: "agent.ps", params: registry.Params{}},
		{label: "agent.inspect alpha", path: "agent.inspect", params: registry.Params{"name": "alpha"}},
		{label: "agent.inspect beta", path: "agent.inspect", params: registry.Params{"name": "beta"}},
		{label: "agent.inspect gamma stalled", path: "agent.inspect", params: registry.Params{"name": "gamma"}, live: "error"},
		{label: "agent.status.show alpha", path: "agent.status.show", params: registry.Params{"name": "alpha"}},
		{label: "agent.status.show beta", path: "agent.status.show", params: registry.Params{"name": "beta"}},
		{label: "agent.budget.get alpha", path: "agent.budget.get", params: registry.Params{"name": "alpha"}},
		{label: "agent.budget.set beta", path: "agent.budget.set", params: registry.Params{"name": "beta", "hour_usd": "1.5", "day_usd": "2", "week_usd": "3", "month_usd": "4"}},
		{label: "agent.image.status alpha", path: "agent.image.status", params: registry.Params{"name": "alpha"}},
		{label: "agent.image.status beta", path: "agent.image.status", params: registry.Params{"name": "beta"}},
		{label: "agent.image.set alpha", path: "agent.image.set", params: registry.Params{"name": "alpha", "image": ref.String()}},
		{label: "agent.image.cancel alpha", path: "agent.image.cancel", params: registry.Params{"name": "alpha"}},
		{label: "agent.goal-enabled alpha", path: "agent.goal-enabled", params: registry.Params{"name": "alpha", "enabled": false}},
		{label: "agent.goal-wait-customer-timeout alpha", path: "agent.goal-wait-customer-timeout", params: registry.Params{"name": "alpha", "seconds": float64(120)}},
		{label: "agent.goal-delivery-cooldown alpha", path: "agent.goal-delivery-cooldown", params: registry.Params{"name": "alpha", "seconds": float64(90)}},
		{label: "agent.run", path: "agent.run", params: registry.Params{"image": "basic:latest", "name": "delta", "goal_enabled": false}},
		{label: "agent.stop", path: "agent.stop", params: registry.Params{"name": "alpha"}},
		{label: "agent.start", path: "agent.start", params: registry.Params{"name": "alpha"}},
		{label: "agent.restart", path: "agent.restart", params: registry.Params{"name": "alpha"}},
		{label: "agent.kill", path: "agent.kill", params: registry.Params{"name": "alpha"}},
		{label: "agent.reprovision", path: "agent.reprovision", params: registry.Params{"name": "alpha"}},
		{label: "agent.rm", path: "agent.rm", params: registry.Params{"name": "beta", "purge": true}},
	}
	got := map[string]any{}
	for _, call := range calls {
		fc.live = call.live
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		data = []byte(strings.ReplaceAll(string(data), manifest.Digest, "$DIGEST"))
		data = []byte(strings.ReplaceAll(string(data), c.BaseDir, "$BASE"))
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		got[call.label] = maskAgentsGoldenVolatile(decoded)
	}

	golden := filepath.Join("testdata", "agents_responses.golden.json")
	if *updateAgentsGolden {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	for label := range want {
		if !reflect.DeepEqual(got[label], want[label]) {
			gotJSON, _ := json.MarshalIndent(got[label], "", "  ")
			wantJSON, _ := json.MarshalIndent(want[label], "", "  ")
			t.Errorf("%s response changed:\n got: %s\nwant: %s", label, gotJSON, wantJSON)
		}
	}
	if len(got) != len(want) {
		t.Errorf("recorded %d responses, golden has %d", len(got), len(want))
	}
}

func maskAgentsGoldenVolatile(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if agentsGoldenVolatile[key] && item != "" {
				v[key] = "$VOLATILE"
			} else {
				v[key] = maskAgentsGoldenVolatile(item)
			}
		}
	case []any:
		for i, item := range v {
			v[i] = maskAgentsGoldenVolatile(item)
		}
	}
	return value
}

// Catches an agents.go HTTP command publishing a free-form result in the
// OpenAPI spec, or a result reference that resolves to no component.
func TestAgentsOpenAPIResultsReferenceComponents(t *testing.T) {
	t.Parallel()
	server := api.NewServer(BuildRegistry(), &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/openapi.json", nil))
	var spec struct {
		Result struct {
			Paths map[string]map[string]struct {
				Responses map[string]struct {
					Content map[string]struct {
						Schema struct{ Properties map[string]map[string]any }
					}
				}
			}
			Components struct{ Schemas map[string]any }
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	routes := map[string][]string{
		"/api/agents":                                   {"get", "post"},
		"/api/agents/{name}":                            {"get", "delete"},
		"/api/agents/{name}/image":                      {"get", "post", "delete"},
		"/api/agents/{name}/goal-enabled":               {"post"},
		"/api/agents/{name}/goal-wait-customer-timeout": {"post"},
		"/api/agents/{name}/goal-delivery-cooldown":     {"post"},
		"/api/agents/{name}/status":                     {"get"},
		"/api/agents/{name}/budget":                     {"get", "post"},
		"/api/agents/{name}/stop":                       {"post"},
		"/api/agents/{name}/start":                      {"post"},
		"/api/agents/{name}/restart":                    {"post"},
		"/api/agents/{name}/kill":                       {"post"},
		"/api/agents/{name}/reprovision":                {"post"},
	}
	for path, methods := range routes {
		for _, method := range methods {
			result := spec.Result.Paths[path][method].Responses["200"].Content["application/json"].Schema.Properties["result"]
			ref, _ := result["$ref"].(string)
			if !strings.HasPrefix(ref, "#/components/schemas/") {
				t.Errorf("%s %s result = %#v, want a component $ref", method, path, result)
			}
		}
	}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				if _, found := spec.Result.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")]; !found {
					t.Errorf("unresolved $ref %s", ref)
				}
			}
			for _, item := range v {
				walk(item)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	var whole any
	if err := json.Unmarshal(w.Body.Bytes(), &whole); err != nil {
		t.Fatal(err)
	}
	walk(whole)
}
