package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/tidy"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// StatusVisit is one stay of a task in a workflow status.
type StatusVisit struct {
	ID        int64  `json:"id"`
	Sequence  int64  `json:"sequence"`
	Status    string `json:"status"`
	EnteredAt string `json:"entered_at"`
	EnteredBy string `json:"entered_by"`
	LeftAt    string `json:"left_at,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Message   string `json:"message,omitempty"`
	// WorkflowDigest and WorkflowVersion name the workflow image the task was
	// pinned to when it entered the status; an upgrade starts a new run of
	// visits under the new digest.
	WorkflowDigest  string `json:"workflow_digest,omitempty"`
	WorkflowVersion string `json:"workflow_version,omitempty"`
	// The visit's counters, compared with the status limits to pause the
	// task; see Limits in the documentation.
	IdleIterations   int `json:"idle_iterations,omitempty"`
	RejectedRequests int `json:"rejected_requests,omitempty"`
	ScriptFailures   int `json:"script_failures,omitempty"`
}

// TransitionRequest is a request to leave a status with an outcome.
type TransitionRequest struct {
	ID            int64        `json:"id"`
	TaskKey       string       `json:"task_key"`
	Outcome       string       `json:"outcome"`
	Message       string       `json:"message,omitempty"`
	Actor         string       `json:"actor"`
	State         RequestState `json:"state" jsonschema:"description=One of pending\\, applied\\, rejected\\, failed\\, cancelled"`
	ResultMessage string       `json:"result_message,omitempty"`
	CreatedAt     string       `json:"created_at"`
	FinishedAt    string       `json:"finished_at,omitempty"`
	// WaitSeconds is how long a caller may wait for a pending request: the
	// timeouts of the transition's checks plus a margin. It is computed from
	// the pinned manifest, never stored, and set only while pending.
	WaitSeconds int `json:"wait_seconds,omitempty" jsonschema:"description=Only while pending: how long to wait for the checks\\, in seconds"`
}

// Artifact is one stored version of a named task artifact.
type Artifact struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
}

// QueueWorkflow is the workflow image a queue is bound to.
type QueueWorkflow struct {
	Queue     string `json:"queue"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Digest    string `json:"digest"`
	Tag       string `json:"tag"`
	Revision  int64  `json:"revision"`
	UpdatedAt string `json:"updated_at"`
}

// TaskWorkflowColumns follow the stored task columns in every task select and
// need TaskWorkflowJoin. After the stored fields comes the stored owner kind
// of the current status, which the scan uses to derive WaitingOn, then two
// JSON columns, both NULL unless the task is an open workflow task, so a list
// query opens no manifest and reads no visit it does not need:
//
//   - the position of the current status in the manifest: its index, the
//     number of statuses, and whether it is the initial status;
//   - the open visit's counters, its latest transition request, and its
//     latest finished run, from which WorkflowAttention is derived, and the
//     pool it recorded in place of the customer.
//
// A task has at most one open visit: enterStatusTx closes the current one
// before it opens the next, and the newest by sequence is taken regardless.
// The request and run lookups go through the visit id, which both tables
// index. Messages are cut in SQL so a row never carries a whole log.
const TaskWorkflowColumns = `
       COALESCE(t.workflow_digest, ''), COALESCE(t.workflow_status, ''), t.workflow_paused_reason,
       COALESCE(wi.name, ''), COALESCE(wi.version, ''), t.workflow_owner_kind,
       CASE WHEN t.workflow_digest IS NOT NULL AND t.status NOT IN ('done', 'cancelled') THEN (
         SELECT json_object('index', s.key, 'total', json_array_length(wi.manifest, '$.definition.statuses'),
                            'initial', json_extract(wi.manifest, '$.definition.initial_status') = t.workflow_status,
                            'outcomes', CASE WHEN t.workflow_owner_kind = 'customer' THEN (
                              SELECT json_group_array(json_object(
                                'on', json_extract(tr.value, '$.on'), 'to', json_extract(tr.value, '$.to'),
                                'terminal', (SELECT COALESCE(json_extract(ts.value, '$.terminal'), 0)
                                             FROM json_each(wi.manifest, '$.definition.statuses') ts
                                             WHERE json_extract(ts.value, '$.id') = json_extract(tr.value, '$.to'))))
                              FROM json_each(s.value, '$.transitions') tr) END)
         FROM json_each(wi.manifest, '$.definition.statuses') s
         WHERE json_extract(s.value, '$.id') = t.workflow_status
       ) END,
       CASE WHEN t.workflow_digest IS NOT NULL AND t.status NOT IN ('done', 'cancelled') THEN (
         SELECT json_object(
           'idle', cv.idle_iterations, 'rejected', cv.rejected_requests, 'failures', cv.script_failures,
           'owner_pool', cv.owner_pool,
           'request', (
             SELECT json_object('id', r.id, 'state', r.state, 'message', substr(r.result_message, 1, 400))
             FROM task_transition_requests r WHERE r.visit_id = cv.id
             ORDER BY r.created_at DESC, r.id DESC LIMIT 1),
           'run', (
             SELECT json_object('id', x.id, 'kind', x.kind, 'verdict', x.verdict, 'request_id', x.request_id,
                                'message', substr(x.message, 1, 400))
             FROM task_script_runs x WHERE x.visit_id = cv.id AND x.state = 'finished'
             ORDER BY x.id DESC LIMIT 1))
         FROM task_status_visits cv WHERE cv.task_id = t.id AND cv.left_at = ''
         ORDER BY cv.sequence DESC LIMIT 1
       ) END`

// TaskWorkflowJoin joins the image of a workflow task.
const TaskWorkflowJoin = `
LEFT JOIN task_workflow_images wi ON wi.digest = t.workflow_digest`

// WorkflowScanTargets returns the scan destinations for TaskWorkflowColumns and
// a function to call after the scan, which derives Category, WaitingOn,
// WorkflowStep, and WorkflowAttention. It lets another package keep its own
// task select in step with scanTask.
func WorkflowScanTargets(t *Task) ([]any, func()) {
	var owner string
	var position, visit sql.NullString
	dest := []any{&t.WorkflowDigest, &t.WorkflowStatus, &t.WorkflowPausedReason, &t.WorkflowName, &t.WorkflowVersion, &owner, &position, &visit}
	return dest, func() {
		finishWorkflowFields(t, owner)
		var step scannedStep
		if position.Valid {
			tidy.Log(nil, "workflow position is not valid JSON", json.Unmarshal([]byte(position.String), &step), "workflow", t.WorkflowName)
		}
		if position.Valid && step.Total > 0 {
			t.WorkflowStep = &WorkflowStep{Index: step.Index + 1, Total: step.Total}
		}
		t.WorkflowInitial = position.Valid && step.Initial != 0
		t.WorkflowOutcomes = nil
		for _, outcome := range step.Outcomes {
			t.WorkflowOutcomes = append(t.WorkflowOutcomes, WorkflowOutcomeRef{On: outcome.On, To: outcome.To, Terminal: outcome.Terminal != 0})
		}
		var open scannedVisit
		if visit.Valid {
			tidy.Log(nil, "workflow visit is not valid JSON", json.Unmarshal([]byte(visit.String), &open), "workflow", t.WorkflowName)
		}
		t.workflowOwnerPool = open.OwnerPool
		t.WorkflowAttention = workflowAttention(t, open)
	}
}

// scannedStep and scannedVisit decode the JSON columns of TaskWorkflowColumns.
type scannedStep struct {
	Index int `json:"index"`
	Total int `json:"total"`
	// Initial is SQLite's 1 or 0 for the comparison with the initial status.
	Initial int `json:"initial"`
	// Outcomes are the transitions of a customer status, else null; Terminal
	// is SQLite's 1 or 0 for whether the target status is terminal.
	Outcomes []struct {
		On       string `json:"on"`
		To       string `json:"to"`
		Terminal int    `json:"terminal"`
	} `json:"outcomes"`
}

type scannedVisit struct {
	OwnerPool string `json:"owner_pool"`
	Idle      int    `json:"idle"`
	Rejected  int    `json:"rejected"`
	Failures  int    `json:"failures"`
	Request   *struct {
		ID      int64        `json:"id"`
		State   RequestState `json:"state"`
		Message string       `json:"message"`
	} `json:"request"`
	Run *struct {
		ID        int64   `json:"id"`
		Kind      RunKind `json:"kind"`
		Verdict   Verdict `json:"verdict"`
		RequestID int64   `json:"request_id"`
		Message   string  `json:"message"`
	} `json:"run"`
}

func finishWorkflowFields(t *Task, ownerKind string) {
	t.Category = t.Status
	if t.WorkflowDigest != "" {
		t.WaitingOn = waitingOn(t.Status, ownerKind, t.WorkflowPausedReason != "")
	}
}

// settleWorkflowSummary keeps WorkflowAttention and WorkflowStep in step with
// a change the engine made in memory to a task it had already read: a closed
// task has neither, and a paused task's attention is the pause. applyState
// calls it after every derived state change.
func settleWorkflowSummary(t *Task) {
	if t.WorkflowDigest == "" {
		return
	}
	if t.Status == StatusDone || t.Status == StatusCancelled {
		t.WorkflowAttention, t.WorkflowStep, t.WorkflowInitial = nil, nil, false
		t.WorkflowOutcomes = nil
		return
	}
	if t.WorkflowPausedReason != "" && (t.WorkflowAttention == nil || t.WorkflowAttention.Kind != AttentionPaused) {
		t.WorkflowAttention = &WorkflowAttention{Kind: AttentionPaused, Message: pauseReasonText(t.WorkflowPausedReason)}
	}
}

// The kinds of WorkflowAttention, from the most to the least urgent.
const (
	AttentionPaused       = "paused"        // the daemon paused the task and waits for a decision
	AttentionFailed       = "failed"        // the latest request of the visit failed: a check could not run
	AttentionRejected     = "rejected"      // the latest request of the visit was rejected by a check
	AttentionScriptFailed = "script_failed" // the latest finished run of the visit is a failed watch
	AttentionIdle         = "idle"          // the holder ended iterations without a transition request
)

// maxAttentionRunes bounds the message of a WorkflowAttention: one line for a
// list row, not the whole script output.
const maxAttentionRunes = 200

// WorkflowAttention says what, if anything, needs a person on an open workflow
// task, with the ids a reader needs to look further.
type WorkflowAttention struct {
	Kind string `json:"kind" jsonschema:"description=What needs a person on an open workflow task\\, the most urgent kind first: paused\\, failed\\, rejected\\, script_failed\\, idle"`
	// Message is the first line of the script's message, or what a pause
	// means, cut to one line.
	Message string `json:"message,omitempty" jsonschema:"description=The first line of the script's message\\, or what the pause means"`
	// Count is the visit counter behind the kind: rejected requests, script
	// failures, or idle iterations.
	Count     int   `json:"count,omitempty" jsonschema:"description=The visit counter behind the kind: rejected requests\\, script failures\\, or idle iterations"`
	RequestID int64 `json:"request_id,omitempty" jsonschema:"description=The failed or rejected transition request"`
	RunID     int64 `json:"run_id,omitempty" jsonschema:"description=The script run whose log explains it"`
}

// WorkflowStep is the position of the current status in the manifest,
// counted from 1.
type WorkflowStep struct {
	Index int `json:"index" jsonschema:"description=The position of the current status in the manifest\\, from 1"`
	Total int `json:"total" jsonschema:"description=The number of statuses the manifest declares"`
}

// WorkflowOutcomeRef is one outcome of a status: its name and the status it
// leads to.
type WorkflowOutcomeRef struct {
	On string `json:"on" jsonschema:"description=The outcome name"`
	To string `json:"to" jsonschema:"description=The status the outcome leads to"`
	// Terminal marks an outcome whose target status is terminal.
	Terminal bool `json:"terminal,omitempty" jsonschema:"description=True when the target status is terminal"`
}

// workflowAttention derives the attention summary of an open workflow task
// from its open visit. The first kind that applies wins.
func workflowAttention(t *Task, visit scannedVisit) *WorkflowAttention {
	if t.WorkflowDigest == "" || t.Status == StatusDone || t.Status == StatusCancelled {
		return nil
	}
	if t.WorkflowPausedReason != "" {
		return &WorkflowAttention{Kind: AttentionPaused, Message: pauseReasonText(t.WorkflowPausedReason)}
	}
	run := visit.Run
	if request := visit.Request; request != nil {
		attention := &WorkflowAttention{Message: attentionLine(request.Message), RequestID: request.ID}
		if run != nil && run.Kind == RunKindCheck && run.RequestID == request.ID {
			attention.RunID = run.ID
		}
		switch request.State {
		case RequestStateFailed:
			attention.Kind, attention.Count = AttentionFailed, visit.Failures
			return attention
		case RequestStateRejected:
			attention.Kind, attention.Count = AttentionRejected, visit.Rejected
			return attention
		}
	}
	if run != nil && run.Kind == RunKindWatch && run.Verdict == VerdictFailure {
		return &WorkflowAttention{Kind: AttentionScriptFailed, Message: attentionLine(run.Message), Count: visit.Failures, RunID: run.ID}
	}
	if visit.Idle > 0 {
		noun := "iterations"
		if visit.Idle == 1 {
			noun = "iteration"
		}
		return &WorkflowAttention{Kind: AttentionIdle, Count: visit.Idle,
			Message: fmt.Sprintf("the holder ended %d %s in a row without requesting a transition", visit.Idle, noun)}
	}
	return nil
}

// attentionLine keeps the first line of a message, cut to maxAttentionRunes.
func attentionLine(message string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	line = strings.TrimSpace(line)
	if cut, wasCut := CutRunes(line, maxAttentionRunes); wasCut {
		return cut + CutMarker
	}
	return line
}

// loadManifestTx reads a published manifest by digest from task_workflow_images.
func loadManifestTx(ctx context.Context, q queryer, digest string) (workflowimage.Manifest, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT manifest FROM task_workflow_images WHERE digest = ?`, digest).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return workflowimage.Manifest{}, newError(http.StatusConflict, "workflow_not_found", "workflow image "+digest+" is not published")
	}
	if err != nil {
		return workflowimage.Manifest{}, err
	}
	var m workflowimage.Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return workflowimage.Manifest{}, err
	}
	return m, nil
}
