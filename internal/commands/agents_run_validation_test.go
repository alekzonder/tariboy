package commands

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

// Catches a change in which invalid agent.run argument is reported first:
// clients show that single error, so the order is observable. Each step
// removes the argument that was just reported.
func TestAgentRunReportsValidationErrorsInOrder(t *testing.T) {
	t.Parallel()
	c, _, fc := ctxWithStore(t)
	params := registry.Params{
		"image": "", "cwd": "relative\x00path", "mail_instant_senders": []any{"user:x"},
		"goal_wait_customer_timeout_s": float64(0), "goal_delivery_cooldown_s": float64(0),
		"timeout": "1m", "timeout_s": float64(-1),
		"env": map[string]any{"COUNT": float64(3)}, "plugins": []any{float64(3)},
		"interval_s": float64(-1), "hard_timeout_s": float64(-1), "max_idle_iterations": float64(-1),
		"ai_stall_timeout_s": float64(0), "messages_batch": float64(0), "messages_max_queue": float64(0),
		"on_timeout": "continue", "on_error": "continue", "color": "red",
	}
	steps := []struct{ code, remove string }{
		{"bad_goal_wait_customer_timeout", "goal_wait_customer_timeout_s"},
		{"bad_goal_delivery_cooldown", "goal_delivery_cooldown_s"},
		{"ambiguous_timeout", "timeout"},
		{"bad_env", "env"},
		{"bad_plugins", "plugins"},
		{"bad_interval", "interval_s"},
		{"bad_timeout", "timeout_s"},
		{"bad_hard_timeout", "hard_timeout_s"},
		{"bad_max_idle", "max_idle_iterations"},
		{"bad_ai_stall_timeout", "ai_stall_timeout_s"},
		{"bad_messages_batch", "messages_batch"},
		{"bad_messages_max_queue", "messages_max_queue"},
		{"bad_on_timeout", "on_timeout"},
		{"bad_on_error", "on_error"},
		{"bad_color", "color"},
	}
	for _, step := range steps {
		if _, err := h(t, "agent.run")(c, params); !isCode(err, step.code) {
			t.Fatalf("err = %#v, want %s", err, step.code)
		}
		delete(params, step.remove)
	}
	if _, err := h(t, "agent.run")(c, params); !isCode(err, "missing_image") {
		t.Fatalf("err = %#v, want missing_image", err)
	}
	params["image"] = "basic:latest"
	if _, err := h(t, "agent.run")(c, params); !isCode(err, "bad_mail_instant_senders") {
		t.Fatalf("err = %#v, want bad_mail_instant_senders", err)
	}
	delete(params, "mail_instant_senders")
	if _, err := h(t, "agent.run")(c, params); !isCode(err, "bad_cwd") {
		t.Fatalf("err = %#v, want bad_cwd", err)
	}
	if fc.ran.ImageRef != "" {
		t.Fatalf("Control.Run called for invalid request: %#v", fc.ran)
	}
}

// Catches the duration timeout losing its place before interval_s, which a
// whole-seconds timeout_s does not have.
func TestAgentRunBadDurationPrecedesInterval(t *testing.T) {
	t.Parallel()
	c, _, _ := ctxWithStore(t)
	_, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "timeout": "soon", "interval_s": float64(-1)})
	if !isCode(err, "bad_timeout") || !strings.Contains(err.Error(), `invalid timeout "soon"`) {
		t.Fatalf("err = %#v, want bad_timeout for the duration", err)
	}
}

// Catches a default of an omitted argument changing or being lost between
// validation and the spec.
func TestAgentRunSpecDefaults(t *testing.T) {
	t.Parallel()
	c, _, fc := ctxWithStore(t)
	value, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "name": "w1"})
	if err != nil {
		t.Fatal(err)
	}
	got := fc.ran
	if !got.Interactive || got.GoalEnabled == nil || !*got.GoalEnabled || got.GoalWaitCustomerTimeoutS != 300 || got.GoalDeliveryCooldownS != 60 ||
		got.AIStallTimeoutS != 1800 || got.MessagesBatch != 10 || got.MessagesMaxQueue != 100 ||
		got.TimeoutS != 0 || got.IntervalS != 0 || got.HardTimeoutS != 0 || got.MaxIdleIterations != 0 ||
		got.OnTimeout != "" || got.OnError != "" || got.Color != "" || got.MailInstantSenders != nil {
		t.Fatalf("spec = %#v", got)
	}
	result := value.(AgentRunResult)
	if result.Name != "w1" || !result.GoalEnabled || result.GoalWaitCustomerTimeoutS != 300 || result.GoalDeliveryCooldownS != 60 {
		t.Fatalf("result = %#v", result)
	}
}

// Catches a supplied argument not reaching the spec, or the result reporting
// a goal setting other than the one sent.
func TestAgentRunSpecCarriesEveryArgument(t *testing.T) {
	t.Parallel()
	c, _, fc := ctxWithStore(t)
	cwd := filepath.Join(t.TempDir(), "made")
	value, err := h(t, "agent.run")(c, registry.Params{
		"image": "basic:latest", "name": "w1", "cwd": cwd, "create_cwd": true, "harness": "stub", "model": "m1", "effort": "high",
		"interactive": false, "env": map[string]any{"A": "1"}, "plugins": []any{"context"}, "group": "g", "timeout": "2h",
		"interval_s": float64(5), "hard_timeout_s": float64(9), "on_timeout": "stop", "on_error": "restart", "max_idle_iterations": float64(3),
		"ai_stall_timeout_s": float64(7), "user_prompt": "go", "messages_batch": float64(2), "messages_max_queue": float64(4),
		"goal_enabled": false, "goal_wait_customer_timeout_s": float64(11), "goal_delivery_cooldown_s": float64(13),
		"alias": "al", "notes": "n", "color": "#ABCDEF",
	})
	if err != nil {
		t.Fatal(err)
	}
	goalEnabled := false
	want := registry.RunSpec{
		ImageRef: "basic:latest", Name: "w1", Cwd: cwd, Harness: "stub", Model: "m1", Effort: "high",
		Env: map[string]string{"A": "1"}, Plugins: []string{"context"}, Group: "g", TimeoutS: 7200,
		IntervalS: 5, HardTimeoutS: 9, OnTimeout: "stop", OnError: "restart", MaxIdleIterations: 3,
		AIStallTimeoutS: 7, UserPrompt: "go", MessagesBatch: 2, MessagesMaxQueue: 4,
		GoalEnabled: &goalEnabled, GoalWaitCustomerTimeoutS: 11, GoalDeliveryCooldownS: 13,
		Alias: "al", Notes: "n", Color: "#abcdef",
	}
	if !reflect.DeepEqual(fc.ran, want) {
		t.Fatalf("spec:\n got %#v\nwant %#v", fc.ran, want)
	}
	result := value.(AgentRunResult)
	if result.GoalEnabled || result.GoalWaitCustomerTimeoutS != 11 || result.GoalDeliveryCooldownS != 13 {
		t.Fatalf("result = %#v", result)
	}
}

// Catches timeout_s losing to the empty duration default.
func TestAgentRunTimeoutSeconds(t *testing.T) {
	t.Parallel()
	c, _, fc := ctxWithStore(t)
	if _, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "timeout_s": float64(45)}); err != nil {
		t.Fatal(err)
	}
	if fc.ran.TimeoutS != 45 {
		t.Fatalf("timeout = %d", fc.ran.TimeoutS)
	}
}

// Catches a failed Control.Run losing its stable code.
func TestAgentRunMapsControlFailure(t *testing.T) {
	t.Parallel()
	c, _, fc := ctxWithStore(t)
	c.Control = failingRunControl{fakeControl: fc, err: errors.New("no provision")}
	_, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest"})
	if !isCode(err, "run_failed") || !strings.Contains(err.Error(), "no provision") {
		t.Fatalf("err = %#v, want run_failed", err)
	}
}
