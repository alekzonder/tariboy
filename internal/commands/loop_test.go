package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestLoopIntervalGetSet(t *testing.T) {
	t.Parallel()
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
	if res.(LoopIntervalResult).IntervalS != 120 {
		t.Fatalf("get result: %v", res)
	}
}

func TestLoopAIStallTimeoutGetSet(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "smoke", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h(t, "loop.ai-stall-timeout")(c, registry.Params{"name": "smoke", "value": 420}); err != nil {
		t.Fatal(err)
	}
	got, _ := as.Get("smoke")
	if got.AIStallTimeoutS != 420 {
		t.Fatalf("AI stall timeout = %d, want 420", got.AIStallTimeoutS)
	}
	if _, err := h(t, "loop.ai-stall-timeout")(c, registry.Params{"name": "smoke", "value": 0}); err == nil {
		t.Fatal("zero AI stall timeout accepted")
	}
}

// agent.start and agent.stop are the only on/off switch; the second loop flag
// and its commands are gone.
func TestLoopEnableDisableRemoved(t *testing.T) {
	t.Parallel()
	r := BuildRegistry()
	for _, path := range []string{"loop.enable", "loop.disable"} {
		if _, ok := r.Get(path); ok {
			t.Fatalf("%s is still registered", path)
		}
	}
	cmd, _ := r.Get("agent.run")
	for _, arg := range cmd.Args {
		if arg.Name == "loop" {
			t.Fatal("agent.run still takes a loop argument")
		}
	}
}

func TestLoopOnErrorValidation(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	if _, err := h(t, "user-prompt.set")(c, registry.Params{"name": "smoke", "text": "focus on X"}); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "user-prompt.get")(c, registry.Params{"name": "smoke"})
	if err != nil || res.(AgentUserPromptResult).UserPrompt != "focus on X" {
		t.Fatalf("user-prompt: %v err=%v", res, err)
	}
}

func TestAgentExec(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	if _, err := h(t, "agent.exec")(c, registry.Params{"name": "smoke", "prompt": "do it now"}); err != nil {
		t.Fatal(err)
	}
}

func TestLoopScheduleSetReadClear(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "smoke", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	cmd := h(t, "loop.schedule")
	res, err := cmd(c, registry.Params{"name": "smoke", "value": "RRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := as.Get("smoke")
	if !strings.HasPrefix(got.ScheduleRRule, "DTSTART:") ||
		!strings.HasSuffix(got.ScheduleRRule, "\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0") {
		t.Fatalf("stored schedule = %q", got.ScheduleRRule)
	}
	view := res.(LoopScheduleResult)
	if view.Schedule != got.ScheduleRRule {
		t.Fatalf("set result = %v", view)
	}
	next, err := time.Parse(time.RFC3339, view.ScheduleNextAt)
	if err != nil || !next.After(time.Now()) || next.In(time.Local).Hour() != 9 || next.In(time.Local).Minute() != 30 {
		t.Fatalf("schedule_next_at = %v (%v)", view.ScheduleNextAt, err)
	}

	if _, err := cmd(c, registry.Params{"name": "smoke", "value": "FREQ=SECONDLY"}); err == nil {
		t.Fatal("second-precision schedule accepted")
	} else if ue, ok := err.(api.UserError); !ok || ue.Code != "bad_schedule" {
		t.Fatalf("err = %#v, want bad_schedule", err)
	}
	if after, _ := as.Get("smoke"); after.ScheduleRRule != got.ScheduleRRule {
		t.Fatalf("rejected rule changed the schedule to %q", after.ScheduleRRule)
	}

	read, err := cmd(c, registry.Params{"name": "smoke"})
	if err != nil || read.(LoopScheduleResult).Schedule != got.ScheduleRRule {
		t.Fatalf("read = %v, %v", read, err)
	}

	cleared, err := cmd(c, registry.Params{"name": "smoke", "clear": true})
	if err != nil {
		t.Fatal(err)
	}
	if v := cleared.(LoopScheduleResult); v.Schedule != "" || v.ScheduleNextAt != "" {
		t.Fatalf("clear result = %v", v)
	}
	if after, _ := as.Get("smoke"); after.ScheduleRRule != "" {
		t.Fatalf("schedule after clear = %q", after.ScheduleRRule)
	}
}
