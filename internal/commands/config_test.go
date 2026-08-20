package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func seedAgent(t *testing.T) (*registry.Ctx, *agent.Store) {
	t.Helper()
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "a1", OnTimeout: "restart", OnError: "restart"})
	return c, as
}

func TestAgentModelSet(t *testing.T) {
	c, _ := seedAgent(t)
	res, err := agentModel().Handler(c, registry.Params{"name": "a1", "value": "opus"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["model"] != "opus" {
		t.Fatalf("echo: %v", res)
	}
	a, _ := getAgent(c, "a1")
	if a.Model != "opus" {
		t.Fatalf("model=%q", a.Model)
	}
}

func TestAgentModelReadOnlyWhenNoValue(t *testing.T) {
	c, _ := seedAgent(t)
	// seed a value, then read with no value
	if _, err := agentModel().Handler(c, registry.Params{"name": "a1", "value": "sonnet"}); err != nil {
		t.Fatal(err)
	}
	res, err := agentModel().Handler(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["model"] != "sonnet" {
		t.Fatalf("read = %v", res)
	}
}

func TestAgentEffortSet(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentEffort().Handler(c, registry.Params{"name": "a1", "value": "high"}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Effort != "high" {
		t.Fatalf("effort=%q", a.Effort)
	}
}

func TestAgentInteractiveSet(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentInteractive().Handler(c, registry.Params{"name": "a1", "value": true}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if !a.Interactive {
		t.Fatalf("interactive=%v", a.Interactive)
	}
}

func TestAgentInteractiveRejectsNonBool(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentInteractive().Handler(c, registry.Params{"name": "a1", "value": "yes"}); err == nil {
		t.Fatal("string value should be rejected")
	}
}

func TestAgentHarnessSet(t *testing.T) {
	c, _ := seedAgent(t)
	res, err := agentHarness().Handler(c, registry.Params{"name": "a1", "value": "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["harness"] != "codex" {
		t.Fatalf("echo: %v", res)
	}
	a, _ := getAgent(c, "a1")
	if a.HarnessType != "codex" {
		t.Fatalf("harness=%q", a.HarnessType)
	}
}

func TestAgentHarnessRejectsInvalidAndDoesNotPersist(t *testing.T) {
	c, _ := seedAgent(t)
	// seed a valid value first
	if _, err := agentHarness().Handler(c, registry.Params{"name": "a1", "value": "claude"}); err != nil {
		t.Fatal(err)
	}
	_, err := agentHarness().Handler(c, registry.Params{"name": "a1", "value": "bogus"})
	if err == nil {
		t.Fatal("invalid harness should be rejected")
	}
	if ue, ok := err.(api.UserError); !ok || ue.Code != "bad_value" {
		t.Fatalf("want bad_value UserError, got %#v", err)
	}
	if a, _ := getAgent(c, "a1"); a.HarnessType != "claude" {
		t.Fatalf("harness changed on invalid set: %q", a.HarnessType)
	}
}

func TestAgentHarnessReadOnlyWhenNoValue(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentHarness().Handler(c, registry.Params{"name": "a1", "value": "opencode"}); err != nil {
		t.Fatal(err)
	}
	res, err := agentHarness().Handler(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["harness"] != "opencode" {
		t.Fatalf("read = %v", res)
	}
}

// TestConfigCommandsRegistered locks in that the setters are wired into the
// daemon registry with the expected HTTP routes (so they are actually served).
func TestConfigCommandsRegistered(t *testing.T) {
	r := BuildRegistry()
	want := map[string]string{
		"agent.model":       "POST /api/agents/{name}/model",
		"agent.effort":      "POST /api/agents/{name}/effort",
		"agent.interactive": "POST /api/agents/{name}/interactive",
		"agent.harness":     "POST /api/agents/{name}/harness",
	}
	for path, route := range want {
		cmd, ok := r.Get(path)
		if !ok {
			t.Fatalf("command %s not registered", path)
		}
		if cmd.HTTP == nil || cmd.HTTP.Method+" "+cmd.HTTP.Path != route {
			t.Fatalf("%s route = %v, want %q", path, cmd.HTTP, route)
		}
	}
}

func TestAgentCwdSetReadClear(t *testing.T) {
	c, _ := seedAgent(t)
	dir := t.TempDir()
	// set
	res, err := agentCwd().Handler(c, registry.Params{"name": "a1", "value": dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["cwd"] != dir {
		t.Fatalf("echo: %v", res)
	}
	if a, _ := getAgent(c, "a1"); a.Cwd != dir {
		t.Fatalf("cwd not persisted: %q", a.Cwd)
	}
	// read (no value) returns current
	res, err = agentCwd().Handler(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["cwd"] != dir {
		t.Fatalf("read = %v", res)
	}
	// relative -> bad_cwd error, value unchanged
	if _, err := agentCwd().Handler(c, registry.Params{"name": "a1", "value": "rel/path"}); err == nil {
		t.Fatal("want error for relative cwd")
	}
	if a, _ := getAgent(c, "a1"); a.Cwd != dir {
		t.Fatalf("cwd changed on invalid set: %q", a.Cwd)
	}
	// clear (empty value)
	if _, err := agentCwd().Handler(c, registry.Params{"name": "a1", "value": ""}); err != nil {
		t.Fatal(err)
	}
	if a, _ := getAgent(c, "a1"); a.Cwd != "" {
		t.Fatalf("cwd not cleared: %q", a.Cwd)
	}
}
