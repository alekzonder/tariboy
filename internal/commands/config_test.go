package commands

import (
	"context"
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
	t.Parallel()
	c, _ := seedAgent(t)
	res, err := registry.Call(c, context.Background(), agentModel(), registry.Params{"name": "a1", "value": "opus"})
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
	t.Parallel()
	c, _ := seedAgent(t)
	// seed a value, then read with no value
	if _, err := registry.Call(c, context.Background(), agentModel(), registry.Params{"name": "a1", "value": "sonnet"}); err != nil {
		t.Fatal(err)
	}
	res, err := registry.Call(c, context.Background(), agentModel(), registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["model"] != "sonnet" {
		t.Fatalf("read = %v", res)
	}
}

func TestAgentEffortSet(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	if _, err := registry.Call(c, context.Background(), agentEffort(), registry.Params{"name": "a1", "value": "high"}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Effort != "high" {
		t.Fatalf("effort=%q", a.Effort)
	}
}

func TestAgentInteractiveSet(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	if _, err := registry.Call(c, context.Background(), agentInteractive(), registry.Params{"name": "a1", "value": true}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if !a.Interactive {
		t.Fatalf("interactive=%v", a.Interactive)
	}
}

func TestAgentInteractiveRejectsNonBool(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	if _, err := registry.Call(c, context.Background(), agentInteractive(), registry.Params{"name": "a1", "value": "yes"}); err == nil {
		t.Fatal("string value should be rejected")
	}
}

func TestAgentHarnessSet(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	res, err := registry.Call(c, context.Background(), agentHarness(), registry.Params{"name": "a1", "value": "codex"})
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
	t.Parallel()
	c, _ := seedAgent(t)
	// seed a valid value first
	if _, err := registry.Call(c, context.Background(), agentHarness(), registry.Params{"name": "a1", "value": "claude"}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Call(c, context.Background(), agentHarness(), registry.Params{"name": "a1", "value": "bogus"})
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
	t.Parallel()
	c, _ := seedAgent(t)
	if _, err := registry.Call(c, context.Background(), agentHarness(), registry.Params{"name": "a1", "value": "opencode"}); err != nil {
		t.Fatal(err)
	}
	res, err := registry.Call(c, context.Background(), agentHarness(), registry.Params{"name": "a1"})
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
	t.Parallel()
	r := BuildRegistry()
	want := map[string]string{
		"agent.model":              "POST /api/agents/{name}/model",
		"agent.effort":             "POST /api/agents/{name}/effort",
		"agent.interactive":        "POST /api/agents/{name}/interactive",
		"agent.harness":            "POST /api/agents/{name}/harness",
		"agent.messages.batch":     "POST /api/agents/{name}/messages/batch",
		"agent.messages.max-queue": "POST /api/agents/{name}/messages/max-queue",
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

func TestAgentMessageSettingsValidateAndPersist(t *testing.T) {
	t.Parallel()
	c, agents := seedAgent(t)
	for path, field := range map[string]string{
		"agent.messages.batch":     "messages_batch",
		"agent.messages.max-queue": "messages_max_queue",
	} {
		res, err := h(t, path)(c, registry.Params{"name": "a1", "value": float64(23)})
		got := -1
		switch v := res.(type) {
		case AgentMessagesBatch:
			if field == "messages_batch" {
				got = v.MessagesBatch
			}
		case AgentMessagesMaxQueue:
			if field == "messages_max_queue" {
				got = v.MessagesMaxQueue
			}
		}
		if err != nil || got != 23 {
			t.Fatalf("%s result=%#v err=%v", path, res, err)
		}
		if _, err := h(t, path)(c, registry.Params{"name": "a1", "value": float64(0)}); err == nil {
			t.Fatalf("%s accepted zero", path)
		}
	}
	a, err := agents.Get("a1")
	if err != nil || a.MessagesBatch != 23 || a.MessagesMaxQueue != 23 {
		t.Fatalf("stored message settings = %#v err=%v", a, err)
	}
}

func TestAgentCwdSetReadClear(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	dir := t.TempDir()
	// set
	res, err := registry.Call(c, context.Background(), agentCwd(), registry.Params{"name": "a1", "value": dir})
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
	res, err = registry.Call(c, context.Background(), agentCwd(), registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["cwd"] != dir {
		t.Fatalf("read = %v", res)
	}
	// relative -> bad_cwd error, value unchanged
	if _, err := registry.Call(c, context.Background(), agentCwd(), registry.Params{"name": "a1", "value": "rel/path"}); err == nil {
		t.Fatal("want error for relative cwd")
	}
	if a, _ := getAgent(c, "a1"); a.Cwd != dir {
		t.Fatalf("cwd changed on invalid set: %q", a.Cwd)
	}
	// clear (empty value)
	if _, err := registry.Call(c, context.Background(), agentCwd(), registry.Params{"name": "a1", "value": ""}); err != nil {
		t.Fatal(err)
	}
	if a, _ := getAgent(c, "a1"); a.Cwd != "" {
		t.Fatalf("cwd not cleared: %q", a.Cwd)
	}
}
