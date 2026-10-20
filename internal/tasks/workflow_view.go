package tasks

import (
	"context"
	"database/sql"
	"strings"

	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// OutcomeView is one transition of the current status as the task shows it.
type OutcomeView struct {
	On       string   `json:"on"`
	To       string   `json:"to"`
	Requires []string `json:"requires,omitempty"`
	Missing  []string `json:"missing,omitempty"` // required artifacts with no value
	Checks   []string `json:"checks,omitempty"`  // script paths
	// CheckDetails describe the same checks, in the same order, with the
	// manifest's description and the resolved run mode and timeout.
	CheckDetails []CheckView `json:"check_details,omitempty"`
}

// CheckView is one check script of a transition with its defaults resolved.
type CheckView struct {
	Script      string `json:"script"`
	Description string `json:"description,omitempty"`
	RunAs       string `json:"run_as"`  // queue or agent
	Timeout     string `json:"timeout"` // the effective timeout, as a duration
}

// checkView resolves a manifest check into the view's shape.
func checkView(check workflowfile.Check) CheckView {
	return CheckView{Script: check.Script, Description: check.Description, RunAs: checkRunAs(check),
		Timeout: scriptTimeout(check.Timeout, workflowfile.DefaultCheckTimeout).String()}
}

// DeclaredArtifact is one artifact the manifest declares.
type DeclaredArtifact struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// StatusView is one status the manifest declares, with what a reader needs to
// understand the step: who owns it, how it is left, and which scripts guard
// it. The graph fields are omitted when empty, and a terminal status has
// none of them.
type StatusView struct {
	ID       string `json:"id"`
	Owner    string `json:"owner"` // "pool:<name>", "customer", "script", or "" for terminal
	Terminal bool   `json:"terminal"`
	// Initial marks the status a new task enters; Cancelled marks a terminal
	// status whose result counts as cancelled.
	Initial          bool             `json:"initial,omitempty"`
	Cancelled        bool             `json:"cancelled,omitempty"`
	InstructionsPath string           `json:"instructions_path,omitempty"` // source-relative path inside the image
	Watch            *WatchView       `json:"watch,omitempty"`
	Transitions      []TransitionView `json:"transitions,omitempty"`
	Limits           *LimitsView      `json:"limits,omitempty"` // the effective limits; nil for a terminal status
}

// TransitionView is one way out of a status: the outcome, its target, and
// what the daemon requires before it applies.
type TransitionView struct {
	On       string      `json:"on"`
	To       string      `json:"to"`
	Requires []string    `json:"requires,omitempty"`
	Checks   []CheckView `json:"checks,omitempty"`
}

// WatchView is the script that owns a script status, with its defaults resolved.
type WatchView struct {
	Script      string `json:"script"`
	Description string `json:"description,omitempty"`
	Every       string `json:"every"`
	Timeout     string `json:"timeout"` // the effective timeout, as a duration
}

// LimitsView is the effective limits of one status, defaults and overrides
// resolved, as durations and counts a reader can compare with the visit's
// counters.
type LimitsView struct {
	IdleIterations   int    `json:"idle_iterations"`
	RejectedRequests int    `json:"rejected_requests"`
	ScriptFailures   int    `json:"script_failures"`
	UnavailableGrace string `json:"unavailable_grace"`
}

// statusView renders one manifest status with its graph.
func statusView(manifest workflowimage.Manifest, status workflowfile.Status) StatusView {
	view := StatusView{ID: status.ID, Owner: statusOwner(status), Terminal: status.Terminal,
		Initial: status.ID == manifest.Definition.InitialStatus, Cancelled: status.Cancelled,
		InstructionsPath: strings.TrimPrefix(status.Instructions, "./")}
	if status.Terminal {
		return view
	}
	if status.Watch != nil {
		view.Watch = &WatchView{Script: status.Watch.Script, Description: status.Watch.Description, Every: status.Watch.Every,
			Timeout: scriptTimeout(status.Watch.Timeout, workflowfile.DefaultWatchTimeout).String()}
	}
	for _, transition := range status.Transitions {
		t := TransitionView{On: transition.On, To: transition.To, Requires: transition.Requires}
		for _, check := range transition.Checks {
			t.Checks = append(t.Checks, checkView(check))
		}
		view.Transitions = append(view.Transitions, t)
	}
	limits := manifest.Definition.StatusLimits(status.ID)
	view.Limits = &LimitsView{IdleIterations: limits.IdleIterations, RejectedRequests: limits.RejectedRequests,
		ScriptFailures: limits.ScriptFailures, UnavailableGrace: limits.UnavailableGrace.String()}
	return view
}

// WorkflowView is the read model of one workflow task: where it is, who owns
// it, what it may do next, and its history.
type WorkflowView struct {
	Name             string             `json:"name"`
	Version          string             `json:"version"`
	Digest           string             `json:"digest"`
	Status           string             `json:"status"`
	Category         string             `json:"category"`
	WaitingOn        string             `json:"waiting_on,omitempty"`
	PausedReason     string             `json:"paused_reason,omitempty"`     // set while the task waits for the customer's decision
	Owner            string             `json:"owner"`                       // "pool:<name>", "customer", "script", or "" for terminal
	Holder           string             `json:"holder,omitempty"`            // "agent:<name>"
	InstructionsPath string             `json:"instructions_path,omitempty"` // source-relative path inside the image
	Outcomes         []OutcomeView      `json:"outcomes"`
	Artifacts        []Artifact         `json:"artifacts"`
	Visits           []StatusVisit      `json:"visits"`
	LastRequest      *TransitionRequest `json:"last_request,omitempty"`
	Runs             []ScriptRun        `json:"runs"` // the most recent script runs, newest first
	// DeclaredArtifacts and Statuses list the manifest's artifacts and
	// statuses in manifest order.
	DeclaredArtifacts []DeclaredArtifact `json:"declared_artifacts"`
	Statuses          []StatusView       `json:"statuses"`
}

// statusOwner renders a status's owner as the view shows it.
func statusOwner(status workflowfile.Status) string {
	switch {
	case status.Terminal:
		return ""
	case status.Owner.Kind == workflowfile.OwnerPool:
		return "pool:" + status.Owner.Pool
	default:
		return status.Owner.Kind
	}
}

// GetWorkflow returns the workflow view of a task the actor may read.
func (s *WorkflowEngine) GetWorkflow(ctx context.Context, actor Actor, key string) (WorkflowView, error) {
	if err := validateActor(actor); err != nil {
		return WorkflowView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowView{}, err
	}
	defer tx.Rollback()
	task, manifest, err := workflowReadTaskTx(ctx, tx, actor, key)
	if err != nil {
		return WorkflowView{}, err
	}
	view := WorkflowView{
		Name: manifest.Name, Version: manifest.Version, Digest: manifest.Digest,
		Status: task.WorkflowStatus, Category: task.Category, WaitingOn: task.WaitingOn, PausedReason: task.WorkflowPausedReason,
		Outcomes:          []OutcomeView{},
		DeclaredArtifacts: make([]DeclaredArtifact, 0, len(manifest.Definition.Artifacts)),
		Statuses:          make([]StatusView, 0, len(manifest.Definition.Statuses)),
	}
	for _, a := range manifest.Definition.Artifacts {
		view.DeclaredArtifacts = append(view.DeclaredArtifacts, DeclaredArtifact{Name: a.Name, Description: a.Description})
	}
	for _, s := range manifest.Definition.Statuses {
		view.Statuses = append(view.Statuses, statusView(manifest, s))
	}
	if view.Artifacts, err = currentArtifactsTx(ctx, tx, task.ID); err != nil {
		return WorkflowView{}, err
	}
	present := artifactNames(view.Artifacts)
	// A closed task has no owner and no exit: every advance would be refused
	// with workflow_closed.
	closed := task.Status == StatusDone || task.Status == StatusCancelled
	if status, ok := currentStatus(manifest, task.WorkflowStatus); ok && !closed {
		view.Owner = statusOwner(status)
		if !status.Terminal && status.Owner.Kind == workflowfile.OwnerPool {
			view.Holder = task.Assignee
		}
		view.InstructionsPath = strings.TrimPrefix(status.Instructions, "./")
		for _, transition := range status.Transitions {
			outcome := OutcomeView{On: transition.On, To: transition.To, Requires: transition.Requires,
				Missing: missingArtifacts(transition, present)}
			for _, check := range transition.Checks {
				outcome.Checks = append(outcome.Checks, check.Script)
				outcome.CheckDetails = append(outcome.CheckDetails, checkView(check))
			}
			view.Outcomes = append(view.Outcomes, outcome)
		}
	}
	if view.Visits, err = statusVisitsTx(ctx, tx, task.ID); err != nil {
		return WorkflowView{}, err
	}
	if view.Runs, err = scriptRunsTx(ctx, tx, task.ID, workflowViewRuns); err != nil {
		return WorkflowView{}, err
	}
	var last TransitionRequest
	err = tx.QueryRowContext(ctx, `
		SELECT id, outcome, message, actor, state, result_message, created_at, finished_at
		FROM task_transition_requests WHERE task_id = ? ORDER BY id DESC LIMIT 1`, task.ID).Scan(
		&last.ID, &last.Outcome, &last.Message, &last.Actor, &last.State, &last.ResultMessage, &last.CreatedAt, &last.FinishedAt)
	switch {
	case err == nil:
		last.TaskKey = task.Key
		view.LastRequest = &last
	case err != sql.ErrNoRows:
		return WorkflowView{}, err
	}
	return view, nil
}

// statusVisitsTx returns every visit of a task, oldest first.
func statusVisitsTx(ctx context.Context, q queryer, taskID int64) ([]StatusVisit, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, sequence, status_id, entered_at, entered_by, left_at, outcome, message,
		       idle_iterations, rejected_requests, script_failures
		FROM task_status_visits WHERE task_id = ? ORDER BY sequence`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	visits := []StatusVisit{}
	for rows.Next() {
		var v StatusVisit
		if err := rows.Scan(&v.ID, &v.Sequence, &v.Status, &v.EnteredAt, &v.EnteredBy, &v.LeftAt, &v.Outcome, &v.Message,
			&v.IdleIterations, &v.RejectedRequests, &v.ScriptFailures); err != nil {
			return nil, err
		}
		visits = append(visits, v)
	}
	return visits, rows.Err()
}
