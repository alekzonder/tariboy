package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// Verdicts of a finished run. They are the verdict names of
// internal/workflowrun, which this package cannot import: its worker imports
// this package.
const (
	verdictPass    = "pass"    // check: the condition holds
	verdictReject  = "reject"  // check: the condition does not hold
	verdictOutcome = "outcome" // watch: an outcome is ready
	verdictQuiet   = "quiet"   // watch or source: nothing changed
	verdictItems   = "items"   // source: the items it found
	verdictFailure = "failure" // anything else
)

// maxRunMessageBytes bounds the message stored from a run.
const maxRunMessageBytes = 4 << 10

// workflowViewRuns is how many recent runs the workflow view shows.
const workflowViewRuns = 20

// ScriptRun is the durable record of one workflow script run.
type ScriptRun struct {
	ID         int64  `json:"id"`
	TaskKey    string `json:"task_key"`
	Kind       string `json:"kind"`
	Script     string `json:"script"`
	RunAs      string `json:"run_as"`
	State      string `json:"state"`
	Verdict    string `json:"verdict,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	Message    string `json:"message,omitempty"`
	CreatedAt  string `json:"created_at"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	LogPath    string `json:"log_path,omitempty"`
	// Holder is the agent a run_as agent check ran as, fixed when the run was
	// created; "" for queue and watch runs.
	Holder string `json:"holder,omitempty"`
	// PID is the process a running run recorded, for the worker's recovery
	// after a restart; it is never marshalled.
	PID *int `json:"-"`
	// RequestID is the transition request a check belongs to, 0 for a watch;
	// the Goal block and Desktop match a failed request to its run by it.
	RequestID int64 `json:"request_id,omitempty"`
}

// RunJob is everything the worker needs to execute one pending run.
type RunJob struct {
	Run             ScriptRun
	Queue           string
	WorkflowName    string
	WorkflowVersion string
	WorkflowDigest  string
	Status          string
	Outcome         string // requested outcome; checks only
	Timeout         time.Duration
	Holder          string // agent name for run_as agent, else ""
	// WorkflowEnv, QueueEnv, and QueueSecrets reach the process environment
	// only; they are never marshalled. Only QueueSecrets is redacted.
	WorkflowEnv  map[string]string `json:"-"`
	QueueEnv     map[string]string `json:"-"`
	QueueSecrets map[string]string `json:"-"`
	Snapshot     []byte            // task.json content, no secrets
	Outcomes     []string          // declared outcomes of the status
	Artifacts    []string          // declared artifact names
}

// RunCompletion is the result of a finished run as the worker reports it.
type RunCompletion struct {
	// Verdict is one of pass, reject, outcome, quiet, or failure, the verdict
	// names of internal/workflowrun. CompleteScriptRun refuses any other value.
	Verdict   string
	Outcome   string
	Message   string
	Artifacts map[string]string
	ExitCode  *int
	LogPath   string
	// FinishedAt is when the run finished; CompleteScriptRun refuses the zero
	// time.
	FinishedAt time.Time
}

// invalidCompletion refuses a run result without a finish time.
func invalidCompletion(kind string, id int64) error {
	return domainError(http.StatusBadRequest, "invalid_completion",
		fmt.Sprintf("%s %d: the completion has no finish time", kind, id))
}

// scriptTimeout parses a declared timeout, falling back to fallback when it
// is empty or invalid.
func scriptTimeout(declared string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(declared); err == nil && d > 0 {
		return d
	}
	return fallback
}

// watchEvery is the pause after a watch run of status before the next one.
func watchEvery(status workflowfile.Status) time.Duration {
	if status.Watch == nil {
		return workflowfile.MinWatchEvery
	}
	return scriptTimeout(status.Watch.Every, workflowfile.MinWatchEvery)
}

// maxEchoedNameBytes bounds a name taken from script output that goes into a
// message or an event.
const maxEchoedNameBytes = 64

// boundRunMessage cuts message to maxRunMessageBytes on a rune boundary.
func boundRunMessage(message string) string {
	message, _ = CutBytes(message, maxRunMessageBytes)
	return message
}

func scriptAuthor(script string) string { return "script:" + script }

// IsTaskDirKey reports whether a task key may name the task's directory under
// <base-dir>/tasks: a single local path element, and not "." or "..".
func IsTaskDirKey(key string) bool {
	return key != "" && key != "." && key != ".." && filepath.IsLocal(key) && !strings.ContainsAny(key, `/\`)
}

func scriptRunNotFound(id int64) error {
	return domainError(http.StatusNotFound, "run_not_found", fmt.Sprintf("script run %d not found", id))
}

// PendingRunJobs returns every pending run, oldest first, with what the worker
// needs to execute it. A run whose job cannot be built is logged and skipped,
// so it does not hold back the others.
func (s *RunService) PendingRunJobs(ctx context.Context) ([]RunJob, error) {
	runs, err := queryScriptRuns(ctx, s.db, pendingRunsWhere)
	if err != nil {
		return nil, err
	}
	jobs := make([]RunJob, 0, len(runs))
	for _, run := range runs {
		job, err := s.runJob(ctx, run)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			s.logger().Error("skip workflow script run whose job cannot be built",
				"run_id", run.ID, "task", run.TaskKey, "err", err)
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// runJob builds the job of one run. It reads through s.db one query at a time:
// the database has a single connection.
func (s *RunService) runJob(ctx context.Context, run scriptRunRecord) (RunJob, error) {
	task, err := taskByID(ctx, s.db, run.taskID)
	if err != nil {
		return RunJob{}, err
	}
	manifest, err := loadManifestTx(ctx, s.db, task.WorkflowDigest)
	if err != nil {
		return RunJob{}, err
	}
	var statusID string
	if err := s.db.QueryRowContext(ctx, `SELECT status_id FROM task_status_visits WHERE id = ?`, run.visitID).
		Scan(&statusID); err != nil {
		return RunJob{}, err
	}
	status, _ := currentStatus(manifest, statusID)
	job := RunJob{
		Run: run.ScriptRun, Queue: task.Queue,
		WorkflowName: manifest.Name, WorkflowVersion: manifest.Version, WorkflowDigest: manifest.Digest,
		Status: statusID, WorkflowEnv: maps.Clone(manifest.Definition.Env),
		Outcomes: statusOutcomes(status), Artifacts: []string{},
	}
	if job.WorkflowEnv == nil {
		job.WorkflowEnv = map[string]string{}
	}
	for _, declared := range manifest.Definition.Artifacts {
		job.Artifacts = append(job.Artifacts, declared.Name)
	}
	var message string
	switch run.Kind {
	case "check":
		if err := s.db.QueryRowContext(ctx, `SELECT outcome, message FROM task_transition_requests WHERE id = ?`,
			run.requestID).Scan(&job.Outcome, &message); err != nil {
			return RunJob{}, err
		}
		job.Timeout = workflowfile.DefaultCheckTimeout
		if transition, ok := statusTransition(status, job.Outcome); ok && run.checkIndex < len(transition.Checks) {
			job.Timeout = scriptTimeout(transition.Checks[run.checkIndex].Timeout, workflowfile.DefaultCheckTimeout)
		}
		if run.RunAs == workflowfile.RunAsAgent {
			job.Holder = run.Holder
		}
	default:
		job.Timeout = workflowfile.DefaultWatchTimeout
		if status.Watch != nil {
			job.Timeout = scriptTimeout(status.Watch.Timeout, workflowfile.DefaultWatchTimeout)
		}
	}
	if job.Snapshot, err = taskSnapshotJSON(ctx, s.db, task, manifest, run.visitID, job.Outcome, message); err != nil {
		return RunJob{}, err
	}
	if job.QueueEnv, job.QueueSecrets, err = s.admin.queueEnvironment(ctx, task.Queue); err != nil {
		return RunJob{}, err
	}
	return job, nil
}

// ClaimScriptRun moves a pending run to running. It returns false when the run
// is no longer pending or its cancellation was requested.
func (s *RunService) ClaimScriptRun(ctx context.Context, id int64, startedAt time.Time, logPath string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE task_script_runs SET state = 'running', started_at = ?, log_path = ?
		WHERE id = ? AND state = 'pending' AND cancel_requested = 0`, store.FormatTime(startedAt), logPath, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// SetScriptRunPID records the process of a running run.
func (s *RunService) SetScriptRunPID(ctx context.Context, id int64, pid int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE task_script_runs SET pid = ? WHERE id = ? AND state = 'running'`, pid, id)
	return err
}

// CancelRequestedRuns lists the running runs whose cancellation was requested;
// the worker kills them.
func (s *RunService) CancelRequestedRuns(ctx context.Context) ([]ScriptRun, error) {
	return s.listScriptRuns(ctx, cancelRequestedWhere)
}

// RunningScriptRuns lists every running run with its recorded PID. At daemon
// start the worker uses it to find the scripts a previous daemon left behind.
func (s *RunService) RunningScriptRuns(ctx context.Context) ([]ScriptRun, error) {
	return s.listScriptRuns(ctx, runningRunsWhere)
}

func (s *RunService) listScriptRuns(ctx context.Context, where string) ([]ScriptRun, error) {
	records, err := queryScriptRuns(ctx, s.db, where)
	if err != nil {
		return nil, err
	}
	runs := make([]ScriptRun, 0, len(records))
	for _, record := range records {
		runs = append(runs, record.ScriptRun)
	}
	return runs, nil
}

// ListScriptRuns returns the runs of a task the actor may read, newest first.
func (s *RunService) ListScriptRuns(ctx context.Context, actor Actor, key string) ([]ScriptRun, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	task, _, err := workflowReadTaskTx(ctx, s.db, actor, key)
	if err != nil {
		return nil, err
	}
	return scriptRunsTx(ctx, s.db, task.ID, 0)
}

// GetScriptRun returns one run of a task the actor may read.
func (s *RunService) GetScriptRun(ctx context.Context, actor Actor, key string, id int64) (ScriptRun, error) {
	if err := validateActor(actor); err != nil {
		return ScriptRun{}, err
	}
	task, _, err := workflowReadTaskTx(ctx, s.db, actor, key)
	if err != nil {
		return ScriptRun{}, err
	}
	run, err := scanScriptRun(s.db.QueryRowContext(ctx, scriptRunSelect+` WHERE r.task_id = ? AND r.id = ?`, task.ID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ScriptRun{}, scriptRunNotFound(id)
	}
	if err != nil {
		return ScriptRun{}, err
	}
	return run.ScriptRun, nil
}

// CompleteScriptRun records the result of a run and applies what it means: a
// check moves its transition request on, a watch run reschedules itself or
// applies an outcome. Completing a run that is no longer pending or running is
// a no-op. A run whose cancellation was requested is recorded as cancelled and
// has no further effect.
func (s *RunService) CompleteScriptRun(ctx context.Context, id int64, done RunCompletion) error {
	switch done.Verdict {
	case verdictPass, verdictReject, verdictOutcome, verdictQuiet, verdictFailure:
	default:
		return fmt.Errorf("script run %d: unknown verdict %q", id, done.Verdict)
	}
	if done.FinishedAt.IsZero() {
		return invalidCompletion("script run", id)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	run, err := scanScriptRun(tx.QueryRowContext(ctx, scriptRunSelect+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return scriptRunNotFound(id)
	}
	if err != nil {
		return err
	}
	if run.State != "pending" && run.State != "running" {
		return nil
	}
	now := s.now()
	finished := done.FinishedAt.UTC()
	finishedAt := store.FormatTime(finished)
	logPath := done.LogPath
	if logPath == "" {
		logPath = run.LogPath
	}
	message := boundRunMessage(done.Message)
	state := "finished"
	if run.cancelRequested {
		state = "cancelled"
	}
	var exit any
	if done.ExitCode != nil {
		exit = *done.ExitCode
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_script_runs SET state = ?, verdict = ?, exit_code = ?, message = ?, finished_at = ?, log_path = ?
		WHERE id = ?`, state, done.Verdict, exit, message, finishedAt, logPath, id); err != nil {
		return err
	}
	task, err := taskByID(ctx, tx, run.taskID)
	if err != nil {
		return err
	}
	if run.Kind == "watch" && done.Verdict == verdictQuiet && state == "finished" {
		// A quiet watch run says nothing happened: it leaves no event, and only
		// the newest quiet runs of the visit are kept. A run cancelled while it
		// went quiet is not one of them.
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM task_script_runs
			WHERE visit_id = ? AND kind = 'watch' AND verdict = 'quiet' AND state = 'finished' AND id NOT IN (
				SELECT id FROM task_script_runs
				WHERE visit_id = ? AND kind = 'watch' AND verdict = 'quiet' AND state = 'finished' ORDER BY id DESC LIMIT ?)`,
			run.visitID, run.visitID, workflowViewRuns); err != nil {
			return err
		}
	} else if _, err := appendEventTx(ctx, tx, task, "workflow.script_run", Actor{Principal: workflowActor}, map[string]any{
		"run_id": run.ID, "kind": run.Kind, "script": run.Script, "state": state,
		"verdict": done.Verdict, "exit_code": done.ExitCode,
	}, now); err != nil {
		return err
	}
	if state == "finished" {
		switch run.Kind {
		case "check":
			err = s.finishCheckTx(ctx, tx, &task, run, done.Verdict, done.Artifacts, message, logPath)
		default:
			err = s.finishWatchTx(ctx, tx, &task, run, done, message, logPath, finished)
		}
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.signal()
	return nil
}

// finishCheckTx applies the verdict of a finished check run to its request.
func (s *RunService) finishCheckTx(ctx context.Context, tx *sql.Tx, task *Task, run scriptRunRecord, verdict string, artifacts map[string]string, message, logPath string) error {
	request, live, err := liveCheckRequestTx(ctx, tx, run)
	if err != nil || !live {
		return err
	}
	manifest, err := loadManifestTx(ctx, tx, task.WorkflowDigest)
	if err != nil {
		return err
	}
	switch verdict {
	case verdictPass:
		status, _ := currentStatus(manifest, task.WorkflowStatus)
		transition, ok := statusTransition(status, request.outcome)
		if !ok {
			return s.failRequestTx(ctx, tx, *task, &manifest, run, request,
				"status "+task.WorkflowStatus+" no longer declares outcome "+request.outcome, "", false)
		}
		if err := validateScriptArtifacts(manifest, artifacts); err != nil {
			return s.failRequestTx(ctx, tx, *task, &manifest, run, request, err.Error(), logPath, true)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE task_status_visits SET script_failures = 0 WHERE id = ?`, request.visitID); err != nil {
			return err
		}
		if err := storeScriptArtifactsTx(ctx, tx, *task, manifest, run.Script, artifacts, s.now()); err != nil {
			return err
		}
		if next := run.checkIndex + 1; next < len(transition.Checks) {
			check := transition.Checks[next]
			return insertScriptRunTx(ctx, tx, task.ID, request.visitID, request.id, "check", check.Script,
				checkRunAs(check), checkHolder(check, task.Assignee), next, s.now())
		}
		// The last check passed. Required artifacts were present when the
		// request was made; confirm nothing removed one since.
		present, err := currentArtifactsTx(ctx, tx, task.ID)
		if err != nil {
			return err
		}
		if missing := missingArtifacts(transition, artifactNames(present)); len(missing) > 0 {
			return s.failRequestTx(ctx, tx, *task, &manifest, run, request,
				"outcome "+request.outcome+" requires artifacts with no value: "+strings.Join(missing, ", "), "", false)
		}
		return s.engine.applyTransitionTx(ctx, tx, task, manifest, request.id, transition, request.actor, request.message)
	case verdictReject:
		now := s.now()
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_transition_requests SET state = 'rejected', result_message = ?, finished_at = ? WHERE id = ?`,
			message, now, request.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_status_visits SET rejected_requests = rejected_requests + 1, script_failures = 0 WHERE id = ?`,
			request.visitID); err != nil {
			return err
		}
		if _, err := appendEventTx(ctx, tx, *task, "workflow.transition_rejected", Actor{Principal: workflowActor}, map[string]any{
			"request_id": request.id, "status": task.WorkflowStatus, "outcome": request.outcome,
			"script": run.Script, "message": message,
		}, now); err != nil {
			return err
		}
		return s.engine.pauseAtLimitTx(ctx, tx, task, manifest, request.visitID, PauseRejectedRequests, message)
	default:
		// A check never produces outcome or quiet; the protocol turns them into
		// failures, and so does this.
		if message == "" {
			message = "check " + run.Script + " failed"
		}
		return s.failRequestTx(ctx, tx, *task, &manifest, run, request, message, logPath, true)
	}
}

// failRequestTx closes a check's request as failed with message and, when
// set, the log path. scriptFailure counts it toward the visit's
// script_failures and pauses the task at the limit; with no manifest (nil)
// there is no limit to compare, and the failure is only counted.
func (s *RunService) failRequestTx(ctx context.Context, tx *sql.Tx, task Task, manifest *workflowimage.Manifest, run scriptRunRecord, request checkRequest, message, logPath string, scriptFailure bool) error {
	now := s.now()
	result := message
	if logPath != "" {
		result += "\nlog: " + logPath
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_transition_requests SET state = 'failed', result_message = ?, finished_at = ? WHERE id = ?`,
		result, now, request.id); err != nil {
		return err
	}
	if scriptFailure {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_status_visits SET script_failures = script_failures + 1 WHERE id = ?`, request.visitID); err != nil {
			return err
		}
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.transition_failed", Actor{Principal: workflowActor}, map[string]any{
		"request_id": request.id, "status": task.WorkflowStatus, "outcome": request.outcome,
		"script": run.Script, "message": message, "log_path": logPath,
	}, now); err != nil {
		return err
	}
	if !scriptFailure || manifest == nil {
		return nil
	}
	return s.engine.pauseAtLimitTx(ctx, tx, &task, *manifest, request.visitID, PauseScriptFailures, result)
}

// checkHolder is the agent a check runs as, recorded when the run is created:
// the task's assignee for a run_as agent check, "" for a queue check. The log
// of such a run may hold the agent's own secrets. A run_as agent check of a
// task no agent holds records no holder; the worker fails it.
func checkHolder(check workflowfile.Check, assignee string) string {
	agent, isAgent := strings.CutPrefix(assignee, "agent:")
	if check.RunAs != workflowfile.RunAsAgent || !isAgent {
		return ""
	}
	return agent
}

// checkRunAs is the run mode of a check; queue when it declares none.
func checkRunAs(check workflowfile.Check) string {
	if check.RunAs == workflowfile.RunAsAgent {
		return workflowfile.RunAsAgent
	}
	return workflowfile.RunAsQueue
}

// validateScriptArtifacts confirms every artifact a script returned is
// declared and storable, so a bad result fails the run instead of the
// transaction.
func validateScriptArtifacts(manifest workflowimage.Manifest, artifacts map[string]string) error {
	for _, name := range sortedKeys(artifacts) {
		value := artifacts[name]
		switch {
		case !artifactDeclared(manifest, name):
			// The name comes from the script; only a bounded part of it is echoed.
			echoed, _ := CutBytes(name, maxEchoedNameBytes)
			return fmt.Errorf("the script returned undeclared artifact %q", echoed)
		case value == "" || !utf8.ValidString(value):
			return fmt.Errorf("the script returned artifact %q that is not non-empty UTF-8 text", name)
		case len(value) > maxArtifactBytes:
			return fmt.Errorf("the script returned artifact %q larger than 64 KiB", name)
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// finishWatchTx applies the verdict of a finished watch run.
func (s *RunService) finishWatchTx(ctx context.Context, tx *sql.Tx, task *Task, run scriptRunRecord, done RunCompletion, message, logPath string, finished time.Time) error {
	live, err := liveWatchTx(ctx, tx, *task, run.visitID)
	if err != nil || !live {
		return err
	}
	manifest, err := loadManifestTx(ctx, tx, task.WorkflowDigest)
	if err != nil {
		return err
	}
	status, _ := currentStatus(manifest, task.WorkflowStatus)
	next := store.FormatTime(finished.Add(watchEvery(status)))
	switch done.Verdict {
	case verdictQuiet:
		_, err := tx.ExecContext(ctx, `
			UPDATE task_status_visits SET next_watch_at = ?, script_failures = 0 WHERE id = ?`, next, run.visitID)
		return err
	case verdictOutcome:
		transition, ok := statusTransition(status, done.Outcome)
		if !ok {
			return s.failWatchTx(ctx, tx, *task, manifest, run, done.Verdict,
				"status "+status.ID+" does not declare outcome "+done.Outcome, logPath, next)
		}
		if err := validateScriptArtifacts(manifest, done.Artifacts); err != nil {
			return s.failWatchTx(ctx, tx, *task, manifest, run, done.Verdict, err.Error(), logPath, next)
		}
		present, err := currentArtifactsTx(ctx, tx, task.ID)
		if err != nil {
			return err
		}
		names := artifactNames(present)
		for name := range done.Artifacts {
			names[name] = true
		}
		if missing := missingArtifacts(transition, names); len(missing) > 0 {
			return s.failWatchTx(ctx, tx, *task, manifest, run, done.Verdict,
				"outcome "+done.Outcome+" requires artifacts with no value: "+strings.Join(missing, ", "), logPath, next)
		}
		return s.applyWatchOutcomeTx(ctx, tx, task, manifest, run, transition, done.Artifacts, message)
	default:
		// A watch run never passes or rejects; those are failures too.
		if message == "" {
			message = "watch script " + run.Script + " failed"
		}
		return s.failWatchTx(ctx, tx, *task, manifest, run, done.Verdict, message, logPath, next)
	}
}

// applyWatchOutcomeTx stores the run's artifacts and applies the transition
// through an applied request authored by the script, for the audit trail.
func (s *RunService) applyWatchOutcomeTx(ctx context.Context, tx *sql.Tx, task *Task, manifest workflowimage.Manifest, run scriptRunRecord, transition workflowfile.Transition, artifacts map[string]string, message string) error {
	now := s.now()
	author := scriptAuthor(run.Script)
	if err := storeScriptArtifactsTx(ctx, tx, *task, manifest, run.Script, artifacts, now); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_transition_requests(task_id, visit_id, outcome, message, actor, state, created_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?)`, task.ID, run.visitID, transition.On, message, author, now)
	if err != nil {
		return err
	}
	requestID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, *task, "workflow.transition_requested", Actor{Principal: author}, map[string]any{
		"request_id": requestID, "status": task.WorkflowStatus, "outcome": transition.On, "actor": author, "state": "applied",
	}, now); err != nil {
		return err
	}
	return s.engine.applyTransitionTx(ctx, tx, task, manifest, requestID, transition, author, message)
}

// failWatchTx counts a failed watch run and schedules the next run at next.
// At the limit it pauses the task, which clears that schedule again.
func (s *RunService) failWatchTx(ctx context.Context, tx *sql.Tx, task Task, manifest workflowimage.Manifest, run scriptRunRecord, verdict, message, logPath, next string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_status_visits SET script_failures = script_failures + 1, next_watch_at = ? WHERE id = ?`,
		next, run.visitID); err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.script_failed", Actor{Principal: workflowActor}, map[string]any{
		"run_id": run.ID, "script": run.Script, "status": task.WorkflowStatus, "verdict": verdict,
		"message": message, "log_path": logPath,
	}, s.now()); err != nil {
		return err
	}
	detail := message
	if logPath != "" {
		detail += "\nlog: " + logPath
	}
	return s.engine.pauseAtLimitTx(ctx, tx, &task, manifest, run.visitID, PauseScriptFailures, detail)
}

// RecoverScriptRuns runs when the worker starts: every running run is recorded
// as interrupted, its check request fails, and its watch runs next after
// every. Pending runs stay pending. A run that cannot be interrupted is logged
// and forced to interrupted, so it does not hold back every other script;
// only a context error fails recovery.
func (s *RunService) RecoverScriptRuns(ctx context.Context) error {
	runs, err := queryScriptRuns(ctx, s.db, runningRunsWhere)
	if err != nil {
		return err
	}
	for _, run := range runs {
		err := s.interruptRun(ctx, run.ID)
		if err == nil {
			continue
		}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("script run %d: %w", run.ID, err)
		}
		s.logger().Error("force workflow script run to interrupted", "run_id", run.ID, "task", run.TaskKey, "err", err)
		if err := s.forceInterruptRun(ctx, run, err); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("script run %d: %w", run.ID, err)
			}
			s.logger().Error("force workflow script run to interrupted failed", "run_id", run.ID, "task", run.TaskKey, "err", err)
		}
	}
	if len(runs) > 0 {
		s.signal()
	}
	return nil
}

// forceInterruptRun records a running run as interrupted when interruptRun
// failed with cause, touching only the run row and, for a check, its pending
// request or, for a watch, the schedule of its open visit.
func (s *RunService) forceInterruptRun(ctx context.Context, run scriptRunRecord, cause error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now()
	message := boundRunMessage(fmt.Sprintf("the daemon restarted during %s %s; recording the interruption failed: %v",
		run.Kind, run.Script, cause))
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_script_runs SET state = 'interrupted', finished_at = ?, message = ? WHERE id = ? AND state = 'running'`,
		now, message, run.ID); err != nil {
		return err
	}
	if run.Kind == "check" && run.requestID != 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_transition_requests SET state = 'failed', result_message = ?, finished_at = ? WHERE id = ? AND state = 'pending'`,
			message, now, run.requestID); err != nil {
			return err
		}
	}
	// A watch of a visit still open runs again after the shortest every, so
	// the status is not left without its watch.
	if run.Kind == "watch" {
		if _, err := tx.ExecContext(ctx, `UPDATE task_status_visits SET next_watch_at = ? WHERE id = ? AND left_at = ''`,
			store.FormatTime(s.clock().Add(workflowfile.MinWatchEvery)), run.visitID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// interruptRun records one running run as interrupted in its own transaction.
func (s *RunService) interruptRun(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	run, err := scanScriptRun(tx.QueryRowContext(ctx, scriptRunSelect+` WHERE r.id = ?`, id))
	if err != nil {
		return err
	}
	if run.State != "running" {
		return nil
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_script_runs SET state = 'interrupted', finished_at = ? WHERE id = ?`, now, id); err != nil {
		return err
	}
	task, err := taskByID(ctx, tx, run.taskID)
	if err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.script_run", Actor{Principal: workflowActor}, map[string]any{
		"run_id": run.ID, "kind": run.Kind, "script": run.Script, "state": "interrupted",
		"verdict": "", "exit_code": nil,
	}, now); err != nil {
		return err
	}
	switch run.Kind {
	case "check":
		request, live, err := liveCheckRequestTx(ctx, tx, run)
		if err != nil {
			return err
		}
		if live {
			// The manifest serves only the limit comparison: without it the
			// request still fails and the failure still counts.
			var limits *workflowimage.Manifest
			manifest, err := loadManifestTx(ctx, tx, task.WorkflowDigest)
			switch {
			case err == nil:
				limits = &manifest
			case ctx.Err() != nil:
				return err
			default:
				s.logger().Warn("fail interrupted check without its manifest", "run_id", run.ID, "task", task.Key, "err", err)
			}
			if err := s.failRequestTx(ctx, tx, task, limits, run, request,
				"the daemon restarted during check "+run.Script+"; the check did not finish", run.LogPath, true); err != nil {
				return err
			}
		}
	default:
		live, err := liveWatchTx(ctx, tx, task, run.visitID)
		if err != nil {
			return err
		}
		if live {
			// The next run follows after every, as after any finished run; with
			// no readable manifest, after the shortest every.
			every := workflowfile.MinWatchEvery
			manifest, err := loadManifestTx(ctx, tx, task.WorkflowDigest)
			switch {
			case err == nil:
				status, _ := currentStatus(manifest, task.WorkflowStatus)
				every = watchEvery(status)
			case ctx.Err() != nil:
				return err
			default:
				s.logger().Warn("reschedule interrupted watch without its manifest", "run_id", run.ID, "task", task.Key, "err", err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE task_status_visits SET next_watch_at = ? WHERE id = ?`,
				store.FormatTime(s.clock().Add(every)), run.visitID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ScheduleDueWatches creates a pending watch run for every visit whose watch
// is due at now and clears its schedule; completing the run sets it again. It
// returns how many runs it created.
func (s *RunService) ScheduleDueWatches(ctx context.Context, now time.Time) (int, error) {
	cutoff := store.FormatTime(now)
	rows, err := s.db.QueryContext(ctx, dueWatchVisits+` ORDER BY v.next_watch_at, v.id`, cutoff)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	scheduled := 0
	var errs []error
	for _, id := range ids {
		created, err := s.scheduleWatch(ctx, id, cutoff)
		if err != nil {
			errs = append(errs, fmt.Errorf("visit %d: %w", id, err))
			continue
		}
		if created {
			scheduled++
		}
	}
	if scheduled > 0 {
		s.signal()
	}
	return scheduled, errors.Join(errs...)
}

// scheduleWatch creates the watch run of one due visit in its own transaction.
func (s *RunService) scheduleWatch(ctx context.Context, visitID int64, cutoff string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var still int64
	err = tx.QueryRowContext(ctx, dueWatchVisits+` AND v.id = ?`, cutoff, visitID).Scan(&still)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var taskID int64
	if err := tx.QueryRowContext(ctx, `SELECT task_id FROM task_status_visits WHERE id = ?`, visitID).Scan(&taskID); err != nil {
		return false, err
	}
	task, err := taskByID(ctx, tx, taskID)
	if err != nil {
		return false, err
	}
	manifest, err := loadManifestTx(ctx, tx, task.WorkflowDigest)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_status_visits SET next_watch_at = '' WHERE id = ?`, visitID); err != nil {
		return false, err
	}
	status, ok := currentStatus(manifest, task.WorkflowStatus)
	created := ok && !status.Terminal && status.Owner.Kind == workflowfile.OwnerScript && status.Watch != nil
	if created {
		if err := insertScriptRunTx(ctx, tx, task.ID, visitID, 0, "watch", status.Watch.Script,
			workflowfile.RunAsQueue, "", 0, s.now()); err != nil {
			return false, err
		}
	}
	return created, tx.Commit()
}
