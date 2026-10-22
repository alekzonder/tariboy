package tasks

// RunKind is the kind of script a workflow run executes.
type RunKind string

// The kinds of script run.
const (
	RunKindCheck  RunKind = "check"
	RunKindWatch  RunKind = "watch"
	RunKindSource RunKind = "source"
)

// RunState is the lifecycle state of a script run or source run.
type RunState string

// The states of a script run.
const (
	RunStatePending     RunState = "pending"
	RunStateRunning     RunState = "running"
	RunStateFinished    RunState = "finished"
	RunStateCancelled   RunState = "cancelled"
	RunStateInterrupted RunState = "interrupted"
)

// RequestState is the state of a transition request.
type RequestState string

// The states of a transition request.
const (
	RequestStatePending  RequestState = "pending"
	RequestStateApplied  RequestState = "applied"
	RequestStateFailed   RequestState = "failed"
	RequestStateRejected RequestState = "rejected"
)

// Verdict is the result a finished run reports.
type Verdict string

// The verdicts of a finished run.
const (
	VerdictPass    Verdict = "pass"    // check: the condition holds
	VerdictReject  Verdict = "reject"  // check: the condition does not hold
	VerdictOutcome Verdict = "outcome" // watch: an outcome is ready
	VerdictQuiet   Verdict = "quiet"   // watch or source: nothing changed
	VerdictItems   Verdict = "items"   // source: the items it found
	VerdictFailure Verdict = "failure" // anything else
)
