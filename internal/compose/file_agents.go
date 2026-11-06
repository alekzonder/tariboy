package compose

import (
	"fmt"
	"regexp"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/schedule"
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

	// EnvMode is merge (the default: keys the file lacks stay) or replace
	// (the agent env becomes exactly Env).
	EnvMode string `yaml:"env_mode,omitempty"`
	// Secrets names, for each agent secret, the variable of the compose
	// process that holds its value. The file never holds a secret value.
	Secrets map[string]SecretSpec `yaml:"secrets,omitempty"`
	// CreateCwd lets the daemon create Cwd when it does not exist.
	CreateCwd bool `yaml:"create_cwd,omitempty"`

	// A nil pointer or slice below leaves the live value untouched. An empty
	// alias, notes or color clears it; an empty mail_instant_senders refuses
	// every agent. omitempty keeps a team export, which does not set these
	// fields, from writing an empty mail_instant_senders.
	UserPrompt         *string        `yaml:"user_prompt,omitempty"`
	Alias              *string        `yaml:"alias,omitempty"`
	Notes              *string        `yaml:"notes,omitempty"`
	Color              *string        `yaml:"color,omitempty"`
	MessagesBatch      *int           `yaml:"messages_batch,omitempty"`
	MessagesMaxQueue   *int           `yaml:"messages_max_queue,omitempty"`
	MailInstantSenders []string       `yaml:"mail_instant_senders,omitempty"`
	Retention          *RetentionSpec `yaml:"retention,omitempty"`
}

// RetentionSpec is the agent's own retention override; a field the file
// omits keeps its live value, and 0 means the daemon default.
type RetentionSpec struct {
	KeepIterations *int   `yaml:"keep_iterations,omitempty"`
	KeepDays       *int   `yaml:"keep_days,omitempty"`
	MaxBytes       *int64 `yaml:"max_bytes,omitempty"`
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
	// HardTimeout and AIStallTimeout are durations like Timeout; empty leaves
	// the live value.
	HardTimeout    string `yaml:"hard_timeout,omitempty"`
	AIStallTimeout string `yaml:"ai_stall_timeout,omitempty"`
	// Schedule is an RFC 5545 RRULE with an optional DTSTART; empty clears
	// the schedule, nil leaves it.
	Schedule *string `yaml:"schedule,omitempty"`
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

// agentProblems adds the version 2 image reference to validateAgent: the
// agent's image must be declared in images.
func (f File) agentProblems(name string, a AgentSpec) []error {
	var out []error
	if err := f.validateAgent(name, a); err != nil {
		out = append(out, err)
	}
	if f.Version != 2 || a.Image == "" || !agent.ValidName(name) {
		return out
	}
	if ref, err := image.ParseRef(a.Image); err != nil {
		out = append(out, fmt.Errorf("agent %q image: %w", name, err))
	} else if _, ok := f.Images[ref.Name]; !ok {
		out = append(out, fmt.Errorf("agent %q references undeclared image %q", name, ref.Name))
	}
	return out
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
	if err := validateAgentSettings(a); err != nil {
		return fmt.Errorf("agent %q %w", name, err)
	}
	if a.Loop != nil {
		return validateAgentLoop(name, a)
	}
	return nil
}

var hexColorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// validateAgentSettings checks the settings the daemon would refuse, so a
// bad file fails before up changes anything.
func validateAgentSettings(a AgentSpec) error {
	if a.EnvMode != "" && a.EnvMode != "merge" && a.EnvMode != "replace" {
		return fmt.Errorf("env_mode %q must be merge|replace", a.EnvMode)
	}
	if a.Color != nil && *a.Color != "" && !hexColorRE.MatchString(*a.Color) {
		return fmt.Errorf("color %q must be a hex color like #4f8cff", *a.Color)
	}
	if err := validateAgentLimits(a); err != nil {
		return err
	}
	for _, sender := range a.MailInstantSenders {
		if !agent.ValidName(sender) {
			return fmt.Errorf("mail_instant_senders: invalid agent name %q", sender)
		}
	}
	// Agent secrets follow the queue secret rules; agent env keys keep the
	// daemon's own rules, so only the secrets are checked here.
	if err := validateQueueValues(TaskQueueSpec{Secrets: a.Secrets}); err != nil {
		return err
	}
	for key := range a.Secrets {
		if _, dup := a.Env[key]; dup {
			return fmt.Errorf("key %q is declared in env and secrets", key)
		}
	}
	if _, _, err := a.hardTimeoutSeconds(); err != nil {
		return err
	}
	if _, _, err := a.aiStallTimeoutSeconds(); err != nil {
		return err
	}
	if a.Loop != nil && a.Loop.Schedule != nil && *a.Loop.Schedule != "" {
		if _, err := schedule.NormalizeRRule(*a.Loop.Schedule, time.Now(), time.Local); err != nil {
			return fmt.Errorf("loop schedule: %w", err)
		}
	}
	return nil
}

// validateAgentLimits checks the mail limits and the retention fields.
func validateAgentLimits(a AgentSpec) error {
	for field, v := range map[string]*int{"messages_batch": a.MessagesBatch, "messages_max_queue": a.MessagesMaxQueue} {
		if v != nil && *v < 1 {
			return fmt.Errorf("%s %d must be >= 1", field, *v)
		}
	}
	if r := a.Retention; r != nil && (r.KeepIterations != nil && *r.KeepIterations < 0 || r.KeepDays != nil && *r.KeepDays < 0 || r.MaxBytes != nil && *r.MaxBytes < 0) {
		return fmt.Errorf("retention fields must be >= 0")
	}
	return nil
}

// hardTimeoutSeconds returns loop.hard_timeout in whole seconds; set is
// false when the file omits it.
func (a AgentSpec) hardTimeoutSeconds() (int, bool, error) {
	if a.Loop == nil || a.Loop.HardTimeout == "" {
		return 0, false, nil
	}
	s, err := parseDurationSeconds("loop hard_timeout", a.Loop.HardTimeout)
	return s, true, err
}

// aiStallTimeoutSeconds returns loop.ai_stall_timeout, a whole number of
// seconds of at least 1; set is false when the file omits it.
func (a AgentSpec) aiStallTimeoutSeconds() (int, bool, error) {
	if a.Loop == nil || a.Loop.AIStallTimeout == "" {
		return 0, false, nil
	}
	d, err := time.ParseDuration(a.Loop.AIStallTimeout)
	if err != nil || d < time.Second || d%time.Second != 0 {
		return 0, true, fmt.Errorf("loop ai_stall_timeout %q must be a whole number of seconds, at least 1s", a.Loop.AIStallTimeout)
	}
	return int(d / time.Second), true, nil
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
