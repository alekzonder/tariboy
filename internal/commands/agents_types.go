package commands

import (
	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/invopop/jsonschema"
)

// Response types of the agents.go commands. Each command declares its type
// with registry.WithResult, which derives the OpenAPI result schema from it.

// AgentBudget is an agent's calendar USD limits and current spend.
type AgentBudget struct {
	HourUSD       float64  `json:"hour_usd"`
	DayUSD        float64  `json:"day_usd"`
	WeekUSD       float64  `json:"week_usd"`
	MonthUSD      float64  `json:"month_usd"`
	HourSpentUSD  float64  `json:"hour_spent_usd"`
	DaySpentUSD   float64  `json:"day_spent_usd"`
	WeekSpentUSD  float64  `json:"week_spent_usd"`
	MonthSpentUSD float64  `json:"month_spent_usd"`
	Exhausted     []string `json:"exhausted" jsonschema_extras:"nullable=true"`
}

// AgentHalt carries the derived halt keys; each appears only when non-empty.
type AgentHalt struct {
	ErrorReason string `json:"error_reason,omitempty"`
	HaltKind    string `json:"halt_kind,omitempty"`
	HaltReason  string `json:"halt_reason,omitempty"`
}

// AgentView is one agent's full configuration (agent.inspect).
type AgentView struct {
	Name                     string            `json:"name"`
	Image                    string            `json:"image"`
	Digest                   string            `json:"digest"`
	State                    string            `json:"state"`
	Cwd                      string            `json:"cwd"`
	ConfiguredCwd            string            `json:"configured_cwd"`
	Harness                  string            `json:"harness"`
	Model                    string            `json:"model"`
	Effort                   string            `json:"effort"`
	Interactive              bool              `json:"interactive"`
	Enabled                  bool              `json:"enabled"`
	IntervalS                int               `json:"interval_s"`
	TimeoutS                 int               `json:"timeout_s"`
	HardTimeoutS             int               `json:"hard_timeout_s"`
	OnTimeout                string            `json:"on_timeout"`
	OnError                  string            `json:"on_error"`
	UserPrompt               string            `json:"user_prompt"`
	Env                      map[string]string `json:"env"`
	Plugins                  []string          `json:"plugins"`
	MessagesBatch            int               `json:"messages_batch"`
	MessagesMaxQueue         int               `json:"messages_max_queue"`
	GoalEnabled              bool              `json:"goal_enabled"`
	GoalWaitCustomerTimeoutS int               `json:"goal_wait_customer_timeout_s"`
	GoalDeliveryCooldownS    int               `json:"goal_delivery_cooldown_s"`
	CurrentGoalTaskKey       string            `json:"current_goal_task_key"`
	Group                    string            `json:"group"`
	Alias                    string            `json:"alias"`
	Notes                    string            `json:"notes"`
	Color                    string            `json:"color"`
	MaxIdleIterations        int               `json:"max_idle_iterations"`
	AIStallTimeoutS          int               `json:"ai_stall_timeout_s"`
	Schedule                 string            `json:"schedule"`
	ScheduleNextAt           string            `json:"schedule_next_at"`
	Budget                   AgentBudget       `json:"budget"`
	AgentHalt
}

// AgentRow is one agent in the agent.ps list.
type AgentRow struct {
	Name                     string            `json:"name"`
	Image                    string            `json:"image"`
	State                    string            `json:"state"`
	Alias                    string            `json:"alias"`
	Harness                  string            `json:"harness"`
	Enabled                  bool              `json:"enabled"`
	Group                    string            `json:"group"`
	Color                    string            `json:"color"`
	Cwd                      string            `json:"cwd"`
	TimeoutS                 int               `json:"timeout_s"`
	IntervalS                int               `json:"interval_s"`
	OnTimeout                string            `json:"on_timeout"`
	OnError                  string            `json:"on_error"`
	MaxIdleIterations        int               `json:"max_idle_iterations"`
	AIStallTimeoutS          int               `json:"ai_stall_timeout_s"`
	Interactive              bool              `json:"interactive"`
	Env                      map[string]string `json:"env"`
	GoalEnabled              bool              `json:"goal_enabled"`
	GoalWaitCustomerTimeoutS int               `json:"goal_wait_customer_timeout_s"`
	GoalDeliveryCooldownS    int               `json:"goal_delivery_cooldown_s"`
	CurrentGoalTaskKey       string            `json:"current_goal_task_key"`
	Schedule                 string            `json:"schedule"`
	ScheduleNextAt           string            `json:"schedule_next_at"`
	Budget                   AgentBudget       `json:"budget"`
	AgentHalt
}

type AgentListResult struct {
	Agents []AgentRow `json:"agents"`
	Count  int        `json:"count"`
}

type AgentRunResult struct {
	Name                     string `json:"name"`
	State                    string `json:"state"`
	GoalEnabled              bool   `json:"goal_enabled"`
	GoalWaitCustomerTimeoutS int    `json:"goal_wait_customer_timeout_s"`
	GoalDeliveryCooldownS    int    `json:"goal_delivery_cooldown_s"`
}

// AgentGoalSettings is the result of the agent.goal-* setting commands.
type AgentGoalSettings struct {
	Name                     string `json:"name"`
	GoalEnabled              bool   `json:"goal_enabled"`
	GoalWaitCustomerTimeoutS int    `json:"goal_wait_customer_timeout_s"`
	GoalDeliveryCooldownS    int    `json:"goal_delivery_cooldown_s"`
}

// AgentStatus is one agent's runtime status (agent.status.show).
type AgentStatus struct {
	Name              string                `json:"name"`
	State             string                `json:"state"`
	Enabled           bool                  `json:"enabled"`
	Iterations        int                   `json:"iterations"`
	LastIteration     string                `json:"last_iteration"`
	LastIterationID   string                `json:"last_iteration_id"`
	StatusMessage     string                `json:"status_message"`
	StatusUpdated     string                `json:"status_updated"`
	MessagesPending   int                   `json:"messages_pending"`
	MessagesMaxQueue  int                   `json:"messages_max_queue"`
	MessagesQueueFull bool                  `json:"messages_queue_full"`
	ServerNow         string                `json:"server_now"`
	Budget            AgentBudget           `json:"budget"`
	ActiveIteration   *AgentActiveIteration `json:"active_iteration,omitempty"`
	AgentHalt
}

// AgentActiveIteration is the running iteration and its deadlines; deadlines
// are absent for rows written before timeout snapshots.
type AgentActiveIteration struct {
	ID                  string  `json:"id"`
	StartedAt           string  `json:"started_at"`
	TimeoutExtensions   int     `json:"timeout_extensions"`
	TimeoutPeriodS      *int    `json:"timeout_period_s,omitempty"`
	TimeoutDeadline     *string `json:"timeout_deadline,omitempty"`
	HardTimeoutDeadline *string `json:"hard_timeout_deadline,omitempty"`
	EffectiveDeadline   string  `json:"effective_deadline,omitempty"`
	// RateLimitedSince is the time of the newest AI request of this iteration
	// when the upstream refused it with a rate limit (a usage limit).
	RateLimitedSince string `json:"rate_limited_since,omitempty"`
}

// AgentAction is the lifecycle verb a lifecycle command applied.
type AgentAction string

const (
	AgentActionStop    AgentAction = "stop"
	AgentActionStart   AgentAction = "start"
	AgentActionRestart AgentAction = "restart"
	AgentActionKill    AgentAction = "kill"
)

func (AgentAction) JSONSchemaExtend(s *jsonschema.Schema) {
	s.Enum = []any{AgentActionStop, AgentActionStart, AgentActionRestart, AgentActionKill}
}

type AgentActionResult struct {
	Name   string      `json:"name"`
	Action AgentAction `json:"action"`
}

type AgentRemoveResult struct {
	Removed string `json:"removed"`
	Purged  bool   `json:"purged"`
}

type AgentStateResult struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// AgentImageReason says why an image is the next one; empty on current.
type AgentImageReason string

const (
	AgentImageReasonCurrent  AgentImageReason = "current"
	AgentImageReasonPending  AgentImageReason = "pending"
	AgentImageReasonRefMoved AgentImageReason = "ref_moved"
)

func (AgentImageReason) JSONSchemaExtend(s *jsonschema.Schema) {
	s.Enum = []any{AgentImageReasonCurrent, AgentImageReasonPending, AgentImageReasonRefMoved}
}

type AgentImageProjection struct {
	agent.ImageAssignment
	ImageVersion string           `json:"image_version,omitempty"`
	Reason       AgentImageReason `json:"reason,omitempty"`
}

// AgentImageStatus is the result of the agent.image.* commands.
type AgentImageStatus struct {
	Name    string                `json:"name"`
	Current AgentImageProjection  `json:"current"`
	Pending agent.ImageAssignment `json:"pending"`
	Next    AgentImageProjection  `json:"next"`
}
