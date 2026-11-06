package compose

import (
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
)

type AgentSpec struct {
	Image   string            `yaml:"image"`
	Group   string            `yaml:"group"`
	Cwd     string            `yaml:"cwd"`
	Harness *HarnessSpec      `yaml:"harness"`
	Env     map[string]string `yaml:"env"`
	Plugins []string          `yaml:"plugins"`
	Budget  *BudgetSpec       `yaml:"budget"`
	Goal    *GoalSpec         `yaml:"goal"`
	// Timeout is the soft iteration timeout as a Go duration string ("60m",
	// "2h", "90s"); a unit is required. Empty means no override (the daemon
	// default). It maps to the agent's timeout_s and, since the hard timeout
	// derives from it (timeout_s+60s), lifts the shim's 60s hard-kill above the
	// work — the knob that keeps long, subagent-spawning iterations alive.
	Timeout string `yaml:"timeout"`
	// Loop is the optional loop cadence + failure-policy block. See LoopSpec.
	Loop *LoopSpec `yaml:"loop"`
}

// LoopSpec is the per-agent loop cadence + failure policy (mirrors the old
// tariboys.yaml loop block). Every field is optional; an empty field leaves
// the daemon default untouched on converge. Enabled is a *bool so "unset" (nil,
// keep today's default: loop on at create) is distinct from explicit false.
type LoopSpec struct {
	Enabled   *bool  `yaml:"enabled"`
	Interval  string `yaml:"interval"`
	Timeout   string `yaml:"timeout"`
	OnTimeout string `yaml:"on_timeout"`
	OnError   string `yaml:"on_error"`
	// MaxIdleIterations is the idle-autostop threshold: the loop stops after this
	// many consecutive idle (self-declared non-productive) iterations. A *int so
	// "unset" (nil, leave the daemon default) is distinct from an explicit 0,
	// which means the feature is disabled. Negative is rejected by Validate.
	MaxIdleIterations *int `yaml:"max_idle_iterations"`
}

type GoalSpec struct {
	Enabled             *bool  `yaml:"enabled"`
	WaitCustomerTimeout string `yaml:"wait_customer_timeout"`
	DeliveryCooldown    string `yaml:"delivery_cooldown"`
}

func (a AgentSpec) goalWaitCustomerTimeoutSeconds() (int, bool, error) {
	if a.Goal == nil || a.Goal.WaitCustomerTimeout == "" {
		return 0, false, nil
	}
	d, err := time.ParseDuration(a.Goal.WaitCustomerTimeout)
	if err != nil {
		return 0, true, fmt.Errorf("invalid goal wait_customer_timeout %q: %w", a.Goal.WaitCustomerTimeout, err)
	}
	if d <= 0 || d%time.Second != 0 {
		return 0, true, fmt.Errorf("goal wait_customer_timeout %q must be a positive whole number of seconds", a.Goal.WaitCustomerTimeout)
	}
	return int(d / time.Second), true, nil
}

func (a AgentSpec) goalDeliveryCooldownSeconds() (int, bool, error) {
	if a.Goal == nil || a.Goal.DeliveryCooldown == "" {
		return 0, false, nil
	}
	d, err := time.ParseDuration(a.Goal.DeliveryCooldown)
	if err != nil {
		return 0, true, fmt.Errorf("invalid goal delivery_cooldown %q: %w", a.Goal.DeliveryCooldown, err)
	}
	if d <= 0 || d%time.Second != 0 {
		return 0, true, fmt.Errorf("goal delivery_cooldown %q must be a positive whole number of seconds", a.Goal.DeliveryCooldown)
	}
	return int(d / time.Second), true, nil
}

// parseDurationSeconds parses a Go duration string to whole seconds. Empty
// yields 0 (unset). A unit is required; a negative duration is rejected.
func parseDurationSeconds(what, s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q (want a duration like 60m/2h/90s): %w", what, s, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("negative %s %q", what, s)
	}
	return int(d.Seconds()), nil
}

// TimeoutSeconds parses the agent's flat Timeout duration string to whole
// seconds. Empty yields 0 (unset).
func (a AgentSpec) TimeoutSeconds() (int, error) {
	return parseDurationSeconds("timeout", a.Timeout)
}

// effectiveTimeout returns the timeout duration string to converge: loop.timeout
// when the block sets it, else the flat Timeout field (back-compat).
func (a AgentSpec) effectiveTimeout() string {
	if a.Loop != nil && a.Loop.Timeout != "" {
		return a.Loop.Timeout
	}
	return a.Timeout
}

// effectiveTimeoutSeconds parses effectiveTimeout to whole seconds.
func (a AgentSpec) effectiveTimeoutSeconds() (int, error) {
	return parseDurationSeconds("timeout", a.effectiveTimeout())
}

// intervalSeconds returns (seconds, set, err). set is false when no loop
// interval is configured, so the reconciler knows to leave the daemon alone.
func (a AgentSpec) intervalSeconds() (int, bool, error) {
	if a.Loop == nil || a.Loop.Interval == "" {
		return 0, false, nil
	}
	s, err := parseDurationSeconds("interval", a.Loop.Interval)
	return s, true, err
}

// validPolicy reports whether an on_timeout/on_error value is allowed (empty =
// unset). Mirrors the daemon's allowed set in internal/commands/loop.go.
func validPolicy(v string) bool { return v == "" || v == "restart" || v == "stop" }

// HarnessSpec is a per-agent harness config (spec §5.1). It is applied as a
// launch-time override on POST /api/agents; the image (built from a
// Tariboyfile, which no longer carries harness) supplies no harness of its
// own, so this block owns the agent's harness/model/effort/interactive.
type HarnessSpec struct {
	Type        string `yaml:"type"`
	Model       string `yaml:"model"`
	Effort      string `yaml:"effort"`
	Interactive bool   `yaml:"interactive"`
}

// Validate checks a per-agent harness block: type is required and must name a
// supported harness (mirrors internal/imagefile's set).
func (h HarnessSpec) Validate() error {
	if h.Type == "" {
		return fmt.Errorf("harness.type is required")
	}
	switch h.Type {
	case "claude", "codex", "opencode", "cursor":
		return nil
	default:
		return fmt.Errorf("harness.type %q is not supported (want claude|codex|opencode|cursor)", h.Type)
	}
}

func (f File) validateAgent(name string, a AgentSpec) error {
	if !agent.ValidName(name) {
		return fmt.Errorf("invalid agent name %q", name)
	}
	if a.Image == "" {
		return fmt.Errorf("agent %q has no image", name)
	}
	if a.Group != "" {
		if !agent.ValidName(a.Group) {
			return fmt.Errorf("agent %q references invalid group name %q", name, a.Group)
		}
		if _, ok := f.Groups[a.Group]; !ok {
			return fmt.Errorf("agent %q references undeclared group %q", name, a.Group)
		}
	}
	if a.Budget != nil {
		if _, err := a.Budget.NormalizedMode(); err != nil {
			return fmt.Errorf("agent %q budget: %w", name, err)
		}
	}
	if a.Harness != nil {
		if err := a.Harness.Validate(); err != nil {
			return fmt.Errorf("agent %q %w", name, err)
		}
	}
	if _, err := a.effectiveTimeoutSeconds(); err != nil {
		return fmt.Errorf("agent %q %w", name, err)
	}
	if _, _, err := a.goalWaitCustomerTimeoutSeconds(); err != nil {
		return fmt.Errorf("agent %q %w", name, err)
	}
	if _, _, err := a.goalDeliveryCooldownSeconds(); err != nil {
		return fmt.Errorf("agent %q %w", name, err)
	}
	if a.Loop != nil {
		return validateAgentLoop(name, a)
	}
	return nil
}

// validateAgentLoop checks the loop block of an agent that has one.
func validateAgentLoop(name string, a AgentSpec) error {
	if _, _, err := a.intervalSeconds(); err != nil {
		return fmt.Errorf("agent %q loop %w", name, err)
	}
	if !validPolicy(a.Loop.OnTimeout) {
		return fmt.Errorf("agent %q loop on_timeout %q must be restart|stop", name, a.Loop.OnTimeout)
	}
	if !validPolicy(a.Loop.OnError) {
		return fmt.Errorf("agent %q loop on_error %q must be restart|stop", name, a.Loop.OnError)
	}
	if a.Loop.MaxIdleIterations != nil && *a.Loop.MaxIdleIterations < 0 {
		return fmt.Errorf("agent %q loop max_idle_iterations %d must be >= 0", name, *a.Loop.MaxIdleIterations)
	}
	return nil
}
