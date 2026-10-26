package commands

import (
	"encoding/json"

	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// Response types of the workflow image, script and schedule.ls commands.
// Each command declares its type with registry.WithResult, which derives the
// OpenAPI result schema from it. The workflow manifest types mirror the
// workflowimage and workflowfile types field for field, so their components
// carry a WorkflowImage prefix instead of generic names such as Status.

// WorkflowImageValidationError is one problem workflow.validate found in a source.
type WorkflowImageValidationError struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// WorkflowImageValidation is the workflow.validate result: the source's name,
// version, pools and files, and every error a build would refuse.
type WorkflowImageValidation struct {
	Valid   bool                           `json:"valid"`
	Name    string                         `json:"name"`
	Version string                         `json:"version"`
	Pools   []string                       `json:"pools"`
	Files   []string                       `json:"files"`
	Errors  []WorkflowImageValidationError `json:"errors"`
}

// WorkflowImageBuildResult is the workflow.build result; Created is false when
// the same content was already published.
type WorkflowImageBuildResult struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Digest  string   `json:"digest"`
	Tags    []string `json:"tags"`
	Created bool     `json:"created"`
}

// WorkflowImageListing is one tag of a built workflow image in workflow.ls.
type WorkflowImageListing struct {
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	BuiltAt string `json:"built_at"`
}

// WorkflowImageList is the workflow.ls result.
type WorkflowImageList struct {
	Workflows []WorkflowImageListing `json:"workflows"`
	Count     int                    `json:"count"`
}

// WorkflowImageRemoveResult is the workflow.rm result; ContentRemoved is true when
// the removed tag was the content's last one.
type WorkflowImageRemoveResult struct {
	Name           string `json:"name"`
	Tag            string `json:"tag"`
	Removed        bool   `json:"removed"`
	ContentRemoved bool   `json:"content_removed"`
}

// WorkflowImageFile is one stored file of a workflow image.
type WorkflowImageFile struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Executable bool   `json:"executable"`
	Size       int64  `json:"size"`
}

// WorkflowImageLimits are the idle, rejection, failure and availability limits of
// a workflow or one of its statuses.
type WorkflowImageLimits struct {
	IdleIterations   *int   `json:"idle_iterations,omitempty"`
	RejectedRequests *int   `json:"rejected_requests,omitempty"`
	ScriptFailures   *int   `json:"script_failures,omitempty"`
	UnavailableGrace string `json:"unavailable_grace,omitempty"`
}

// WorkflowImageArtifact is a named artifact a workflow's tasks record.
type WorkflowImageArtifact struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// WorkflowImageSource is a script that creates tasks on an interval.
type WorkflowImageSource struct {
	Name        string `json:"name"`
	Script      string `json:"script"`
	Every       string `json:"every"`
	Timeout     string `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
}

// WorkflowImageOwner owns a status: a pool, the customer, a script, or nobody
// (empty kind).
type WorkflowImageOwner struct {
	Kind string `json:"kind"`
	Pool string `json:"pool,omitempty"`
}

// WorkflowImageWatch is the script that drives a script-owned status.
type WorkflowImageWatch struct {
	Script      string `json:"script"`
	Every       string `json:"every"`
	Timeout     string `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
}

// WorkflowImageCheck is a script a transition runs before it is taken.
type WorkflowImageCheck struct {
	Script      string `json:"script"`
	RunAs       string `json:"run_as,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
}

// WorkflowImageTransition leads from a status to another on an outcome.
type WorkflowImageTransition struct {
	On       string               `json:"on"`
	To       string               `json:"to"`
	Requires []string             `json:"requires,omitempty"`
	Checks   []WorkflowImageCheck `json:"checks,omitempty"`
}

// WorkflowImageStatus is one status of a workflow definition.
type WorkflowImageStatus struct {
	ID           string                    `json:"id"`
	Owner        WorkflowImageOwner        `json:"owner"`
	Instructions string                    `json:"instructions,omitempty"`
	Watch        *WorkflowImageWatch       `json:"watch,omitempty"`
	Transitions  []WorkflowImageTransition `json:"transitions,omitempty"`
	Limits       *WorkflowImageLimits      `json:"limits,omitempty"`
	Terminal     bool                      `json:"terminal,omitempty"`
	Cancelled    bool                      `json:"cancelled,omitempty"`
}

// WorkflowImageDefinition is the parsed Workflowfile.yaml of a workflow image.
type WorkflowImageDefinition struct {
	SchemaVersion   int                     `json:"schema_version"`
	Name            string                  `json:"name"`
	WorkflowVersion string                  `json:"workflow_version"`
	InitialStatus   string                  `json:"initial_status"`
	RequiresSecrets []string                `json:"requires_secrets,omitempty"`
	Env             map[string]string       `json:"env,omitempty"`
	Limits          *WorkflowImageLimits    `json:"limits,omitempty"`
	WatchSpacing    string                  `json:"watch_spacing,omitempty"`
	Artifacts       []WorkflowImageArtifact `json:"artifacts,omitempty"`
	Sources         []WorkflowImageSource   `json:"sources,omitempty"`
	Statuses        []WorkflowImageStatus   `json:"statuses"`
}

// WorkflowImageManifest is a built workflow image (workflow.inspect).
type WorkflowImageManifest struct {
	SchemaVersion int                     `json:"schema_version"`
	Name          string                  `json:"name"`
	Version       string                  `json:"version"`
	Digest        string                  `json:"digest"`
	BuiltAt       string                  `json:"built_at"`
	Definition    WorkflowImageDefinition `json:"definition"`
	Files         []WorkflowImageFile     `json:"files"`
}

// workflowManifestFrom copies m through its JSON encoding: the mirror types
// carry the same JSON tags, so the encoding is unchanged.
func workflowManifestFrom(m workflowimage.Manifest) (WorkflowImageManifest, error) {
	var view WorkflowImageManifest
	data, err := json.Marshal(m)
	if err == nil {
		err = json.Unmarshal(data, &view)
	}
	return view, err
}

func workflowValidationErrors(errs []workflowfile.ValidationError) []WorkflowImageValidationError {
	out := make([]WorkflowImageValidationError, 0, len(errs))
	for _, e := range errs {
		out = append(out, WorkflowImageValidationError(e))
	}
	return out
}

// AgentScriptRun is one run of an agent's local script; PID and ExitCode appear
// once the run has them.
type AgentScriptRun struct {
	ID              string `json:"id"`
	ScriptID        string `json:"script_id"`
	Agent           string `json:"agent"`
	Status          string `json:"status"`
	CancelRequested bool   `json:"cancel_requested"`
	PID             *int   `json:"pid,omitempty"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	CreatedAt       string `json:"created_at"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	LogPath         string `json:"log_path"`
}

// AgentScript is an agent's local script with its latest run, if any.
type AgentScript struct {
	ID              string          `json:"id"`
	Agent           string          `json:"agent"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Command         string          `json:"command"`
	Mode            string          `json:"mode"`
	IntervalSeconds int             `json:"interval_seconds"`
	State           string          `json:"state"`
	CreatedAt       string          `json:"created_at"`
	NextRunAt       string          `json:"next_run_at"`
	LatestRun       *AgentScriptRun `json:"latest_run,omitempty"`
}

// AgentScriptList is the script.ls result.
type AgentScriptList struct {
	Scripts []AgentScript `json:"scripts"`
	Count   int           `json:"count"`
}

// AgentScriptStarted is a created script with its first run (script.run,
// script.schedule).
type AgentScriptStarted struct {
	Script AgentScript    `json:"script"`
	Run    AgentScriptRun `json:"run"`
}

// AgentScriptRunList is the script.runs result.
type AgentScriptRunList struct {
	Runs  []AgentScriptRun `json:"runs"`
	Count int              `json:"count"`
}

// AgentScriptRunLog is a run with the bounded tail of its log (script.logs).
type AgentScriptRunLog struct {
	Run AgentScriptRun `json:"run"`
	Log string         `json:"log"`
}

// AgentScriptCancelResult is the script.cancel result.
type AgentScriptCancelResult struct {
	ID        string `json:"id"`
	Cancelled bool   `json:"cancelled"`
}

// AgentScriptRemoveResult is the script.rm result.
type AgentScriptRemoveResult struct {
	ID      string `json:"id"`
	Removed bool   `json:"removed"`
}

func scriptRunFrom(run script.Run) AgentScriptRun {
	return AgentScriptRun{ID: run.ID, ScriptID: run.ScriptID, Agent: run.Agent, Status: run.Status, CancelRequested: run.CancelRequested,
		PID: run.PID, ExitCode: run.ExitCode, CreatedAt: run.CreatedAt, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, LogPath: run.LogPath}
}

func scriptDefinitionFrom(definition script.Definition) AgentScript {
	view := AgentScript{ID: definition.ID, Agent: definition.Agent, Name: definition.Name, Description: definition.Description,
		Command: definition.Command, Mode: definition.Mode, IntervalSeconds: definition.IntervalSeconds, State: definition.State,
		CreatedAt: definition.CreatedAt, NextRunAt: definition.NextRunAt}
	if definition.LatestRun != nil {
		latest := scriptRunFrom(*definition.LatestRun)
		view.LatestRun = &latest
	}
	return view
}

// AgentSchedule is one of an agent's schedules in schedule.ls.
type AgentSchedule struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Spec       string `json:"spec"`
	To         string `json:"to"`
	NextFireAt string `json:"next_fire_at"`
	Enabled    bool   `json:"enabled"`
}

// AgentScheduleList is the schedule.ls result.
type AgentScheduleList struct {
	Schedules []AgentSchedule `json:"schedules"`
	Count     int             `json:"count"`
}

func agentScheduleFrom(s schedule.Schedule) AgentSchedule {
	return AgentSchedule{ID: s.ID, Kind: s.Kind, Spec: s.Spec, To: s.Recipient, NextFireAt: s.NextFireAt, Enabled: s.Enabled}
}
