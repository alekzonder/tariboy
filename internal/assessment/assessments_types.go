package assessment

// Results of the assessment operations. The operator commands and the agent
// tool routes return the same shapes, so `tariboy assessment --json` reads
// one format in both modes; the names are the OpenAPI component names.

// Axes of an assessment. Every assessment has exactly one row per axis.
const (
	// AxisImageProcess is conformance to the image's process.
	AxisImageProcess = "image_process"
	// AxisRole is conformance to the image's ROLE.md.
	AxisRole = "role"
	// AxisTaskFit is fit to the task and to what the customer expected,
	// including resisting the request because of the agent's own process.
	AxisTaskFit = "task_fit"
)

// Axes lists the three axes in their display order.
var Axes = []string{AxisImageProcess, AxisRole, AxisTaskFit}

// Verdicts of one axis.
const (
	VerdictScored        = "scored"
	VerdictNotApplicable = "not_applicable"
)

// Evidence kinds of a violation.
const (
	EvidenceTranscript = "transcript"
	EvidenceTaskEvent  = "task_event"
	EvidenceScriptRun  = "script_run"
)

// AssessmentAxis is the verdict on one axis. Score is set, 0..100, only when
// Verdict is scored.
type AssessmentAxis struct {
	Axis    string `json:"axis"`
	Verdict string `json:"verdict"`
	Score   *int   `json:"score,omitempty"`
	Note    string `json:"note"`
}

// AssessmentEvidence locates the evidence of a violation. The fields that
// apply depend on Kind: transcript uses IterationID and Locator, task_event
// uses TaskKey and Sequence (task_events.sequence), script_run uses RunID.
type AssessmentEvidence struct {
	Kind        string `json:"kind"`
	IterationID string `json:"iteration_id"`
	Locator     string `json:"locator"`
	TaskKey     string `json:"task_key"`
	Sequence    int64  `json:"sequence"`
	RunID       string `json:"run_id"`
}

// AssessmentViolation is one violation found on an axis.
type AssessmentViolation struct {
	Axis        string             `json:"axis"`
	Description string             `json:"description"`
	Quote       string             `json:"quote"`
	Evidence    AssessmentEvidence `json:"evidence"`
}

// AssessmentRecord is the fields of an assessment without its violations.
// The assessor and the snapshot fields are written by the daemon, never by
// the request.
type AssessmentRecord struct {
	ID                   string           `json:"id"`
	CreatedAt            string           `json:"created_at"`
	AssessorAgent        string           `json:"assessor_agent"`
	AssessorImageRef     string           `json:"assessor_image_ref"`
	AssessorImageVersion string           `json:"assessor_image_version"`
	AssessorImageDigest  string           `json:"assessor_image_digest"`
	RubricVersion        string           `json:"rubric_version"`
	IterationID          string           `json:"iteration_id"`
	TaskKey              string           `json:"task_key"`
	SubjectAgent         string           `json:"subject_agent"`
	ImageRef             string           `json:"image_ref"`
	ImageVersion         string           `json:"image_version"`
	ImageDigest          string           `json:"image_digest"`
	WorkflowDigest       string           `json:"workflow_digest"`
	WorkflowVersion      string           `json:"workflow_version"`
	Summary              string           `json:"summary"`
	Axes                 []AssessmentAxis `json:"axes"`
}

// AssessmentSummary is one row of assessment.ls: the record and the number
// of its violations.
type AssessmentSummary struct {
	AssessmentRecord
	ViolationCount int `json:"violation_count"`
}

// Assessment is one whole assessment with its violations.
type Assessment struct {
	AssessmentRecord
	Violations []AssessmentViolation `json:"violations"`
}

// AssessmentList is the result of assessment.ls, newest first.
type AssessmentList struct {
	Assessments []AssessmentSummary `json:"assessments"`
}

// AssessmentShowResult is the result of assessment.show and of a write.
type AssessmentShowResult struct {
	Assessment Assessment `json:"assessment"`
}

// AssessmentInput is the body of a write. The assessor and the snapshots
// are not part of it: the daemon takes them from the socket identity and the
// iteration and task rows.
type AssessmentInput struct {
	RubricVersion string                `json:"rubric_version"`
	IterationID   string                `json:"iteration_id"`
	TaskKey       string                `json:"task_key"`
	Summary       string                `json:"summary"`
	Axes          []AssessmentAxis      `json:"axes"`
	Violations    []AssessmentViolation `json:"violations"`
}

// AssessmentFilter selects the assessments of List. Empty fields match
// everything; Limit defaults to 100 and is at most 1000. Without History,
// List takes only the latest assessment of each iteration and task
// (IsLatest) and then applies the other fields.
type AssessmentFilter struct {
	Iteration    string
	Task         string
	Agent        string
	Assessor     string
	Image        string
	ImageVersion string
	Limit        int
	History      bool
}
