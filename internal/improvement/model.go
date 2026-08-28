package improvement

import "errors"

var (
	ErrInvalidProposal   = errors.New("improvement: invalid proposal")
	ErrNotFound          = errors.New("improvement: not found")
	ErrRevisionMismatch  = errors.New("improvement: revision mismatch")
	ErrInvalidTransition = errors.New("improvement: invalid transition")
)

type Status string

const (
	StatusDraft                   Status = "draft"
	StatusAwaitingPlanApproval    Status = "awaiting_plan_approval"
	StatusApproved                Status = "approved"
	StatusImplementing            Status = "implementing"
	StatusPullRequestOpen         Status = "pull_request_open"
	StatusMerged                  Status = "merged"
	StatusImageBuilt              Status = "image_built"
	StatusAwaitingRolloutApproval Status = "awaiting_rollout_approval"
	StatusRolloutPending          Status = "rollout_pending"
	StatusRolledOut               Status = "rolled_out"
	StatusRejected                Status = "rejected"
	StatusCancelled               Status = "cancelled"
	StatusFailed                  Status = "failed"
)

type Citation struct {
	BundleHash string `json:"bundle_hash"`
	Artifact   string `json:"artifact"`
	Locator    string `json:"locator"`
}

type Finding struct {
	Severity    string     `json:"severity"`
	Criterion   string     `json:"criterion"`
	Observation string     `json:"observation"`
	Evidence    []Citation `json:"evidence"`
}

type Change struct {
	File   string `json:"file"`
	Intent string `json:"intent"`
}

type Target struct {
	Repository  string `json:"repository"`
	BaseCommit  string `json:"base_commit"`
	Image       string `json:"image"`
	ImageDigest string `json:"image_digest"`
}

type ProposalDraft struct {
	SubjectIDs    []string  `json:"subject_ids"`
	Target        Target    `json:"target"`
	Findings      []Finding `json:"findings"`
	Changes       []Change  `json:"changes"`
	Acceptance    []string  `json:"acceptance"`
	Risk          string    `json:"risk"`
	RollbackImage string    `json:"rollback_image"`
}

type CreateProposalRequest struct {
	JudgeRunID, SummaryID, CreatorAgent, CreatorIteration string
	Draft                                                 ProposalDraft
}

type Proposal struct {
	ID, JudgeRunID, SummaryID, CreatorAgent, CreatorIteration   string
	Draft                                                       ProposalDraft
	RevisionHash                                                string
	Status                                                      Status
	Branch, PullRequestURL, HeadCommit, MergedCommit, LastError string
	CreatedAt, UpdatedAt                                        string
}

type ApprovalPhase string
type ApprovalDecision string

const (
	PhasePlan    ApprovalPhase = "plan"
	PhaseRollout ApprovalPhase = "rollout"

	DecisionApprove ApprovalDecision = "approve"
	DecisionReject  ApprovalDecision = "reject"
)

type ApprovalRequest struct {
	ProposalID string
	ObjectHash string
	Decision   ApprovalDecision
	Actor      string
	Reason     string
}

type Approval struct {
	ID, ProposalID, ObjectHash, Actor, Reason, CreatedAt string
	Phase                                                ApprovalPhase
	Decision                                             ApprovalDecision
}
