package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestLoopIntervalGetSet(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", IntervalS: 60, OnTimeout: "restart", OnError: "restart"})

	// set
	if _, err := h(t, "loop.interval")(c, registry.Params{"name": "smoke", "value": 120}); err != nil {
		t.Fatal(err)
	}
	got, _ := as.Get("smoke")
	if got.IntervalS != 120 {
		t.Fatalf("interval = %d", got.IntervalS)
	}
	// get (no value)
	res, err := h(t, "loop.interval")(c, registry.Params{"name": "smoke"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["interval_s"] != 120 {
		t.Fatalf("get result: %v", res)
	}
}

func TestLoopEnableDisable(t *testing.T) {
	c, as, control := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", LoopEnabled: true, OnTimeout: "restart", OnError: "restart"})
	if _, err := h(t, "loop.disable")(c, registry.Params{"name": "smoke"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := as.Get("smoke"); got.LoopEnabled {
		t.Fatal("loop not disabled")
	}
	if _, err := h(t, "loop.enable")(c, registry.Params{"name": "smoke"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := as.Get("smoke"); !got.LoopEnabled {
		t.Fatal("loop not enabled")
	}
	if len(control.loopUpdates) != 2 || control.loopUpdates[0] || !control.loopUpdates[1] {
		t.Fatalf("runtime loop updates = %v, want [false true]", control.loopUpdates)
	}
}

func TestLoopOnErrorValidation(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	if _, err := h(t, "loop.on-error")(c, registry.Params{"name": "smoke", "value": "explode"}); err == nil {
		t.Fatal("invalid on-error accepted")
	}
	if _, err := h(t, "loop.on-error")(c, registry.Params{"name": "smoke", "value": "stop"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := as.Get("smoke"); got.OnError != "stop" {
		t.Fatalf("on_error = %q", got.OnError)
	}
}

func TestUserPromptGetSet(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	if _, err := h(t, "user-prompt.set")(c, registry.Params{"name": "smoke", "text": "focus on X"}); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "user-prompt.get")(c, registry.Params{"name": "smoke"})
	if err != nil || res.(map[string]any)["user_prompt"] != "focus on X" {
		t.Fatalf("user-prompt: %v err=%v", res, err)
	}
}

func TestAgentExec(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	if _, err := h(t, "agent.exec")(c, registry.Params{"name": "smoke", "prompt": "do it now"}); err != nil {
		t.Fatal(err)
	}
}
