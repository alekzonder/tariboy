// Package judge persists immutable LLM-as-Judge run selections.
package judge

import "errors"

var (
	ErrNotFound             = errors.New("judge: not found")
	ErrEmptySelection       = errors.New("judge: empty target selection")
	ErrNonTerminalIteration = errors.New("judge: target iteration is not terminal")
	ErrInsufficientJudges   = errors.New("judge: insufficient eligible judges")
	ErrNoAssignment         = errors.New("judge: no claimable assignment")
	ErrLeaseNotOwned        = errors.New("judge: assignment lease is not owned by caller")
	ErrInvalidSubmission    = errors.New("judge: invalid submission")
	ErrInvalidAnalysis      = errors.New("judge: invalid analysis")
	ErrInvalidSummary       = errors.New("judge: invalid summary")
	ErrUnauthorized         = errors.New("judge: unauthorized")
	ErrCapabilityDisabled   = errors.New("judge: llm-as-judge capability is disabled")
	ErrStaleIteration       = errors.New("judge: caller iteration is not active")
	ErrInvalidAction        = errors.New("judge: invalid action")
)

type RunStatus string

const (
	RunSnapshotting RunStatus = "snapshotting"
	RunRunning      RunStatus = "running"
	RunSummarizing  RunStatus = "summarizing"
	RunCompleted    RunStatus = "completed"
	RunPartial      RunStatus = "partial"
	RunCancelled    RunStatus = "cancelled"
)

type Selector struct {
	ExplicitIDs []string `json:"iteration_ids,omitempty"`
	Agents      []string `json:"agents,omitempty"`
	Group       string   `json:"group,omitempty"`
	Since       string   `json:"since,omitempty"`
	Until       string   `json:"until,omitempty"`
	Statuses    []string `json:"statuses,omitempty"`
	Order       string   `json:"order,omitempty"`
	Limit       int      `json:"limit,omitempty"`
}

type CreateRunRequest struct {
	OriginalRequest                                       string
	Selector                                              Selector
	JudgeGroup, LeadAgent, SummaryAgent, CreatorIteration string
	JudgeAgents                                           []string
	JudgesPerIteration, MaxAttempts                       int
}

type Run struct {
	ID                    string    `json:"id"`
	CreatedAt             string    `json:"created_at"`
	UpdatedAt             string    `json:"updated_at"`
	CreatorIteration      string    `json:"creator_iteration"`
	OriginalRequest       string    `json:"original_request"`
	Spec                  Selector  `json:"spec"`
	JudgeGroup            string    `json:"judge_group"`
	LeadAgent             string    `json:"lead_agent"`
	SummaryAgent          string    `json:"summary_agent"`
	JudgeAgents           []string  `json:"judge_agents"`
	JudgesPerIteration    int       `json:"judges_per_iteration"`
	MaxAttempts           int       `json:"max_attempts"`
	Status                RunStatus `json:"status"`
	TargetsTotal          int       `json:"targets_total"`
	TargetsReady          int       `json:"targets_ready"`
	AssignmentsTotal      int       `json:"assignments_total"`
	AssignmentsCompleted  int       `json:"assignments_completed"`
	ManifestHash          string    `json:"manifest_hash"`
	CurrentSummaryVersion int       `json:"current_summary_version"`
	LastError             string    `json:"last_error"`
}

type Target struct {
	ID                   string   `json:"id"`
	RunID                string   `json:"run_id"`
	Iteration            string   `json:"iteration"`
	Agent                string   `json:"agent"`
	Sequence             int      `json:"sequence"`
	BundlePath           string   `json:"bundle_path"`
	BundleHash           string   `json:"bundle_hash"`
	BundleBytes          int64    `json:"bundle_bytes"`
	SnapshotStatus       string   `json:"snapshot_status"`
	TargetState          string   `json:"target_state"`
	ConsensusVerdict     string   `json:"consensus_verdict"`
	ConsensusScore       *float64 `json:"consensus_score,omitempty"`
	AssignmentsCompleted int      `json:"assignments_completed"`
	AssignmentsFailed    int      `json:"assignments_failed"`
	AssignmentsPending   int      `json:"assignments_pending"`
}

type ListFilter struct {
	Statuses []RunStatus
	Limit    int
}

// Assignment is a durable unit of independent judge work. A claimed assignment
// is owned by one ordinary agent iteration until its lease expires.
type Assignment struct {
	ID, RunID, TargetID                                    string
	ReplicaIndex                                           int
	State                                                  string
	JudgeAgent, JudgeIteration, LeaseOwner, LeaseExpiresAt string
	AttemptCount                                           int
	LastError, AnalysisID                                  string
}

type ClaimRequest struct {
	RunID, Agent, Iteration, LeaseOwner string
	LeaseDuration                       int // seconds; zero selects the safe default
}

type Citation struct {
	BundleHash string `json:"bundle_hash,omitempty"`
	Artifact   string `json:"artifact,omitempty"`
	Locator    string `json:"locator,omitempty"`
}
type Violation struct {
	Criterion   string     `json:"criterion,omitempty"`
	Severity    string     `json:"severity,omitempty"`
	Description string     `json:"description,omitempty"`
	Citations   []Citation `json:"citations,omitempty"`
}
type Strength struct {
	Description string     `json:"description,omitempty"`
	Citations   []Citation `json:"citations,omitempty"`
}
type Recommendation struct {
	Description string `json:"description,omitempty"`
}

// AnalysisResult is the fixed v1 normalized model output schema.
type AnalysisResult struct {
	SchemaVersion   int              `json:"schema_version"`
	Verdict         string           `json:"verdict"`
	Score           float64          `json:"score"`
	Confidence      float64          `json:"confidence"`
	Summary         string           `json:"summary"`
	Violations      []Violation      `json:"violations"`
	Strengths       []Strength       `json:"strengths"`
	Recommendations []Recommendation `json:"recommendations"`
	EvidenceGaps    []string         `json:"evidence_gaps"`
}

type Analysis struct {
	ID             string         `json:"id"`
	RunID          string         `json:"run_id"`
	TargetID       string         `json:"target_id"`
	AssignmentID   string         `json:"assignment_id"`
	JudgeAgent     string         `json:"judge_agent"`
	JudgeIteration string         `json:"judge_iteration"`
	RawSubmission  string         `json:"raw_submission"`
	CreatedAt      string         `json:"created_at"`
	SchemaVersion  int            `json:"schema_version"`
	Result         AnalysisResult `json:"result"`
}
type SubmitAnalysisRequest struct {
	AssignmentID, Agent, Iteration, RawSubmission string
	Result                                        AnalysisResult
	Resolve                                       CitationResolver
}

type TargetConsensus struct {
	Verdict           string
	Score, Confidence float64
}

type SummaryResult struct {
	SchemaVersion          int            `json:"schema_version"`
	ExecutiveConclusion    string         `json:"executive_conclusion"`
	Coverage               map[string]int `json:"coverage"`
	CrossIterationPatterns []string       `json:"cross_iteration_patterns"`
	RecurringViolations    []string       `json:"recurring_violations"`
	Strengths              []string       `json:"strengths"`
	DisputedCases          []string       `json:"disputed_cases"`
	Recommendations        []string       `json:"recommendations"`
	FollowUpEvaluations    []string       `json:"follow_up_evaluations"`
	TargetIDs              []string       `json:"target_ids"`
	AnalysisIDs            []string       `json:"analysis_ids"`
}
type Summary struct {
	ID               string         `json:"id"`
	RunID            string         `json:"run_id"`
	SummaryAgent     string         `json:"summary_agent"`
	SummaryIteration string         `json:"summary_iteration"`
	RawSubmission    string         `json:"raw_submission"`
	CreatedAt        string         `json:"created_at"`
	Version          int            `json:"version"`
	Coverage         map[string]int `json:"coverage"`
	Result           SummaryResult  `json:"result"`
}
type SubmitSummaryRequest struct {
	RunID, Agent, Iteration, RawSubmission string
	Result                                 SummaryResult
}
