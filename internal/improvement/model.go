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
	ID               string        `json:"id"`
	JudgeRunID       string        `json:"judge_run_id"`
	SummaryID        string        `json:"summary_id"`
	CreatorAgent     string        `json:"creator_agent"`
	CreatorIteration string        `json:"creator_iteration"`
	Draft            ProposalDraft `json:"draft"`
	RevisionHash     string        `json:"revision_hash"`
	Status           Status        `json:"status"`
	Branch           string        `json:"branch"`
	PullRequestURL   string        `json:"pull_request_url"`
	HeadCommit       string        `json:"head_commit"`
	MergedCommit     string        `json:"merged_commit"`
	LastError        string        `json:"last_error"`
	CreatedAt        string        `json:"created_at"`
	UpdatedAt        string        `json:"updated_at"`
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
	ID         string           `json:"id"`
	ProposalID string           `json:"proposal_id"`
	ObjectHash string           `json:"object_hash"`
	Actor      string           `json:"actor"`
	Reason     string           `json:"reason"`
	CreatedAt  string           `json:"created_at"`
	Phase      ApprovalPhase    `json:"phase"`
	Decision   ApprovalDecision `json:"decision"`
}

type Release struct {
	ID                   string `json:"id"`
	ProposalID           string `json:"proposal_id"`
	RepositoryID         string `json:"repository_id"`
	GitCommit            string `json:"git_commit"`
	SourceName           string `json:"source_name"`
	SourceDigest         string `json:"source_digest"`
	LockDigest           string `json:"lock_digest"`
	PromptTemplateDigest string `json:"prompt_template_digest"`
	ImageRef             string `json:"image_ref"`
	ImageDigest          string `json:"image_digest"`
	BuilderVersion       string `json:"builder_version"`
	ReleaseHash          string `json:"release_hash"`
	CreatedAt            string `json:"created_at"`
	Status               Status `json:"status"`
}

type BuildRequest struct {
	ProposalID, RepositoryID, GitCommit, SourceDir, SourceName, ImageRef string
}

type Rollout struct {
	ID               string `json:"id"`
	ReleaseID        string `json:"release_id"`
	TargetAgent      string `json:"target_agent"`
	PriorImageRef    string `json:"prior_image_ref"`
	PriorImageDigest string `json:"prior_image_digest"`
	ImageRef         string `json:"image_ref"`
	ImageDigest      string `json:"image_digest"`
	CreatedAt        string `json:"created_at"`
	CompletedAt      string `json:"completed_at"`
	RollbackOf       string `json:"rollback_of"`
	Status           Status `json:"status"`
}
