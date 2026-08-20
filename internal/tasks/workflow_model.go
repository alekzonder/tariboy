package tasks

// Workflow definition DTOs are intentionally persistence-agnostic. Store and
// service layers translate them to normalized workflow records.

const (
	DispatchClaimOne   = "claim_one"
	DispatchRequireAll = "require_all"
)

const (
	AssignmentClaimable = "claimable"
	AssignmentLeased    = "leased"
	AssignmentCompleted = "completed"
	AssignmentReleased  = "released"
	AssignmentExpired   = "expired"
	AssignmentFailed    = "failed"
)

const (
	HoldNone        = "none"
	HoldAssignment  = "assignment"
	HoldRequirement = "requirement"
)

const (
	ArtifactMarkdown = "markdown"
	ArtifactJSON     = "json"
	ArtifactFile     = "file"
	ArtifactCommit   = "commit"
	ArtifactURL      = "url"
)

// WorkflowDefinition is the portable graph stored by a WorkflowVersion.
type WorkflowDefinition struct {
	Name          string                    `json:"name"`
	Version       int                       `json:"version"`
	InitialStatus string                    `json:"initial_status"`
	Statuses      []WorkflowStatus          `json:"statuses"`
	Budgets       WorkflowBudgetPolicy      `json:"budgets,omitempty"`
	Timeouts      WorkflowTimeoutPolicy     `json:"timeouts,omitempty"`
	Retries       WorkflowRetryPolicy       `json:"retries,omitempty"`
	Questions     WorkflowQuestionPolicy    `json:"questions,omitempty"`
	Observations  WorkflowObservationPolicy `json:"observations,omitempty"`
	Permissions   WorkflowPermissions       `json:"permissions,omitempty"`
}

// WorkflowBudgetPolicy bounds the work a workflow may consume before its
// declared exhausted outcome is applied.
type WorkflowBudgetPolicy struct {
	MaxCycles      int    `json:"max_cycles,omitempty"`
	MaxAssignments int    `json:"max_assignments,omitempty"`
	OnExhausted    string `json:"on_exhausted,omitempty"`
}

// WorkflowTimeoutPolicy declares bounded waiting without embedding executable
// timeout behavior in the definition DTO.
type WorkflowTimeoutPolicy struct {
	Assignment string `json:"assignment,omitempty"`
	Question   string `json:"question,omitempty"`
	OnTimeout  string `json:"on_timeout,omitempty"`
}

type WorkflowRetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty"`
	Backoff     string `json:"backoff,omitempty"`
	OnExhausted string `json:"on_exhausted,omitempty"`
}

type WorkflowQuestionPolicy struct {
	RouteTo              string   `json:"route_to,omitempty"`
	AllowedHolds         []string `json:"allowed_holds,omitempty"`
	MaxOpenPerAssignment int      `json:"max_open_per_assignment,omitempty"`
	Timeout              string   `json:"timeout,omitempty"`
}

type WorkflowObservationPolicy struct {
	OnLateEvent      string   `json:"on_late_event,omitempty"`
	AllowedReactions []string `json:"allowed_reactions,omitempty"`
}

// WorkflowPermissions is the declarative maximum capability set. WorkPacket
// narrows it to the current assignment's allowed operations.
type WorkflowPermissions struct {
	Tools    []string                   `json:"tools,omitempty"`
	Channels WorkflowChannelPermissions `json:"channels,omitempty"`
}

type WorkflowChannelPermissions struct {
	Subscribe []string `json:"subscribe,omitempty"`
	Reactions []string `json:"reactions,omitempty"`
}

type WorkflowVersion struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	Version     int                `json:"version"`
	State       string             `json:"state"`
	Definition  WorkflowDefinition `json:"definition"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
	PublishedAt string             `json:"published_at,omitempty"`
}

type WorkflowStatus struct {
	ID           string                `json:"id"`
	Instructions string                `json:"instructions,omitempty"`
	Requirements []WorkflowRequirement `json:"requirements"`
	Transitions  []WorkflowTransition  `json:"transitions"`
	Join         string                `json:"join,omitempty"`
	Terminal     bool                  `json:"terminal,omitempty"`
}

type WorkflowRequirement struct {
	ID       string   `json:"id"`
	Pool     string   `json:"pool"`
	Dispatch string   `json:"dispatch"`
	Inputs   []string `json:"inputs"`
	Produces []string `json:"produces"`
	Outcomes []string `json:"outcomes"`
	Optional bool     `json:"optional,omitempty"`
}

type WorkflowTransition struct {
	When string `json:"when"`
	To   string `json:"to"`
}

type QueueWorkflowBinding struct {
	Queue             string `json:"queue"`
	WorkflowVersionID int64  `json:"workflow_version_id"`
	WorkflowName      string `json:"workflow_name,omitempty"`
	WorkflowVersion   int    `json:"workflow_version,omitempty"`
	Revision          int64  `json:"revision"`
	BoundBy           string `json:"bound_by"`
	BoundAt           string `json:"bound_at"`
}

type AgentPool struct {
	ID        int64    `json:"id"`
	Queue     string   `json:"queue"`
	Name      string   `json:"name"`
	Agents    []string `json:"agents"`
	Revision  int64    `json:"revision"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// Runtime DTOs describe immutable workflow history and the active leases
// derived from it. They contain no database handles or persistence methods.

type StatusExecution struct {
	ID                int64  `json:"id"`
	TaskKey           string `json:"task_key"`
	WorkflowVersionID int64  `json:"workflow_version_id"`
	Status            string `json:"status"`
	Sequence          int64  `json:"sequence"`
	State             string `json:"state"`
	TransitionTo      string `json:"transition_to,omitempty"`
	TaskRevision      int64  `json:"task_revision"`
	CreatedAt         string `json:"created_at"`
	CompletedAt       string `json:"completed_at,omitempty"`
}

type RequirementExecution struct {
	ID                int64    `json:"id"`
	StatusExecutionID int64    `json:"status_execution_id"`
	RequirementID     string   `json:"requirement_id"`
	Pool              string   `json:"pool"`
	Dispatch          string   `json:"dispatch"`
	Optional          bool     `json:"optional"`
	PoolSnapshot      []string `json:"pool_snapshot"`
	Inputs            []string `json:"inputs"`
	Produces          []string `json:"produces"`
	Outcomes          []string `json:"outcomes"`
	State             string   `json:"state"`
	CreatedAt         string   `json:"created_at"`
	CompletedAt       string   `json:"completed_at,omitempty"`
}

type Assignment struct {
	ID                     int64  `json:"id"`
	RequirementExecutionID int64  `json:"requirement_execution_id"`
	Agent                  string `json:"agent"`
	Attempt                int    `json:"attempt"`
	State                  string `json:"state"`
	LeaseOwner             string `json:"lease_owner,omitempty"`
	LeaseIteration         string `json:"lease_iteration,omitempty"`
	LeaseExpiresAt         string `json:"lease_expires_at,omitempty"`
	Revision               int64  `json:"revision"`
	Outcome                string `json:"outcome,omitempty"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
	CompletedAt            string `json:"completed_at,omitempty"`
}

type Artifact struct {
	ID           int64          `json:"id"`
	TaskKey      string         `json:"task_key"`
	AssignmentID int64          `json:"assignment_id,omitempty"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Content      string         `json:"content,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Revision     int64          `json:"revision"`
	CreatedBy    string         `json:"created_by"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

type WorkflowQuestion struct {
	ID                     int64    `json:"id"`
	TaskKey                string   `json:"task_key"`
	AssignmentID           int64    `json:"assignment_id,omitempty"`
	RequirementExecutionID int64    `json:"requirement_execution_id,omitempty"`
	Question               string   `json:"question"`
	Context                string   `json:"context"`
	BlockingScope          string   `json:"blocking_scope"`
	Anchor                 string   `json:"anchor,omitempty"`
	Options                []string `json:"options,omitempty"`
	SuggestedAnswer        string   `json:"suggested_answer,omitempty"`
	ArtifactAttachments    []int64  `json:"artifact_attachments,omitempty"`
	State                  string   `json:"state"`
	DeadlineAt             string   `json:"deadline_at,omitempty"`
	Answer                 string   `json:"answer,omitempty"`
	AnsweredBy             string   `json:"answered_by,omitempty"`
	CreatedAt              string   `json:"created_at"`
	AnsweredAt             string   `json:"answered_at,omitempty"`
}

type WorkflowHold struct {
	ID                     int64  `json:"id"`
	TaskKey                string `json:"task_key"`
	AssignmentID           int64  `json:"assignment_id,omitempty"`
	RequirementExecutionID int64  `json:"requirement_execution_id,omitempty"`
	QuestionID             int64  `json:"question_id,omitempty"`
	Scope                  string `json:"scope"`
	Reason                 string `json:"reason,omitempty"`
	CreatedAt              string `json:"created_at"`
	ReleasedAt             string `json:"released_at,omitempty"`
}

type WorkflowObservation struct {
	ID             int64          `json:"id"`
	TaskKey        string         `json:"task_key"`
	SubscriptionID int64          `json:"subscription_id,omitempty"`
	AssignmentID   int64          `json:"assignment_id,omitempty"`
	Kind           string         `json:"kind"`
	Payload        map[string]any `json:"payload,omitempty"`
	ObservedAt     string         `json:"observed_at"`
}

type WorkflowSubscription struct {
	ID                    int64  `json:"id"`
	TaskKey               string `json:"task_key"`
	AssignmentID          int64  `json:"assignment_id,omitempty"`
	Pattern               string `json:"pattern"`
	CorrelationKey        string `json:"correlation_key,omitempty"`
	Reaction              string `json:"reaction"`
	State                 string `json:"state"`
	CreatedBy             string `json:"created_by"`
	CreatedAt             string `json:"created_at"`
	CancelledAt           string `json:"cancelled_at,omitempty"`
	CreatedAfterSequence  int64  `json:"-"`
	ActivationSequenceSet bool   `json:"-"`
}

// WorkPacket is the assignment-scoped task projection delivered to an agent.
// Future service code populates only policy-permitted inputs and observations.
type WorkPacket struct {
	TaskKey                     string                 `json:"task_key"`
	TaskRevision                int64                  `json:"task_revision"`
	Goal                        string                 `json:"goal,omitempty"`
	Status                      string                 `json:"status,omitempty"`
	StatusInstructions          string                 `json:"status_instructions,omitempty"`
	Assignment                  Assignment             `json:"assignment"`
	Requirement                 WorkflowRequirement    `json:"requirement"`
	Inputs                      []Artifact             `json:"inputs,omitempty"`
	AllowedOutcomes             []string               `json:"allowed_outcomes,omitempty"`
	AllowedActions              []string               `json:"allowed_actions,omitempty"`
	AllowedTools                []string               `json:"allowed_tools,omitempty"`
	AllowedChannelSubscriptions []string               `json:"allowed_channel_subscriptions,omitempty"`
	Questions                   []WorkflowQuestion     `json:"questions,omitempty"`
	Holds                       []WorkflowHold         `json:"holds,omitempty"`
	Observations                []WorkflowObservation  `json:"observations,omitempty"`
	Subscriptions               []WorkflowSubscription `json:"subscriptions,omitempty"`
}

type WorkflowExecutionView struct {
	Task         Task                   `json:"task"`
	Workflow     WorkflowVersion        `json:"workflow"`
	Statuses     []StatusExecution      `json:"status_executions"`
	Requirements []RequirementExecution `json:"requirement_executions"`
	Assignments  []Assignment           `json:"assignments"`
	Holds        []WorkflowHold         `json:"holds"`
	Observations []WorkflowObservation  `json:"observations"`
}
