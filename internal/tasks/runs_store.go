package tasks

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// The SQL of workflow script runs: the run rows, their queries, and the checks
// a completion makes that its run still applies. workflow_runs.go holds what a
// run does when it completes.

// scriptRunRecord is a run with the references the engine follows.
type scriptRunRecord struct {
	ScriptRun
	taskID          int64
	visitID         int64
	requestID       int64 // 0 for a watch run
	checkIndex      int
	cancelRequested bool
}

const scriptRunSelect = `
	SELECT r.id, t.task_key, r.kind, r.script, r.run_as, r.state, r.verdict, r.exit_code, r.message,
	       r.created_at, r.started_at, r.finished_at, r.log_path, r.holder, r.pid,
	       r.task_id, r.visit_id, COALESCE(r.request_id, 0), r.check_index, r.cancel_requested
	FROM task_script_runs r JOIN tasks t ON t.id = r.task_id`

func scanScriptRun(row interface{ Scan(...any) error }) (scriptRunRecord, error) {
	var r scriptRunRecord
	var exit, pid sql.NullInt64
	if err := row.Scan(&r.ID, &r.TaskKey, &r.Kind, &r.Script, &r.RunAs, &r.State, &r.Verdict, &exit, &r.Message,
		&r.CreatedAt, &r.StartedAt, &r.FinishedAt, &r.LogPath, &r.Holder, &pid,
		&r.taskID, &r.visitID, &r.requestID, &r.checkIndex, &r.cancelRequested); err != nil {
		return scriptRunRecord{}, err
	}
	if exit.Valid {
		code := int(exit.Int64)
		r.ExitCode = &code
	}
	if pid.Valid {
		p := int(pid.Int64)
		r.PID = &p
	}
	r.RequestID = r.requestID
	return r, nil
}

// queryScriptRuns reads the runs matching where, which follows scriptRunSelect.
func queryScriptRuns(ctx context.Context, q queryer, where string, args ...any) ([]scriptRunRecord, error) {
	rows, err := q.QueryContext(ctx, scriptRunSelect+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scriptRunRecord
	for rows.Next() {
		run, err := scanScriptRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// Predicates of the run queries the worker and the readers repeat; each
// follows scriptRunSelect. A state list repeats the WHERE clause of the partial
// index idx_task_script_runs_active word for word, which is what lets SQLite
// use that index.
const (
	taskRunsWhere        = ` WHERE r.task_id = ? ORDER BY r.id DESC`
	pendingRunsWhere     = ` WHERE r.state IN ('pending', 'running') AND r.state = 'pending' ORDER BY r.id`
	cancelRequestedWhere = ` WHERE r.state IN ('pending', 'running') AND r.state = 'running' AND r.cancel_requested = 1 ORDER BY r.id`
	runningRunsWhere     = ` WHERE r.state IN ('pending', 'running') AND r.state = 'running' ORDER BY r.id`
)

// scriptRunsTx returns a task's runs, newest first; limit 0 returns all.
func scriptRunsTx(ctx context.Context, q queryer, taskID int64, limit int) ([]ScriptRun, error) {
	where := taskRunsWhere
	args := []any{taskID}
	if limit > 0 {
		where += ` LIMIT ?`
		args = append(args, limit)
	}
	records, err := queryScriptRuns(ctx, q, where, args...)
	if err != nil {
		return nil, err
	}
	runs := make([]ScriptRun, 0, len(records))
	for _, record := range records {
		runs = append(runs, record.ScriptRun)
	}
	return runs, nil
}

// insertScriptRunTx records a pending run. requestID is 0 for a watch run.
func insertScriptRunTx(ctx context.Context, tx *sql.Tx, taskID, visitID, requestID int64, kind RunKind, script, runAs, holder string, checkIndex int, now string) error {
	var request any
	if requestID != 0 {
		request = requestID
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_script_runs(task_id, visit_id, request_id, kind, script, run_as, holder, check_index, state, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
		taskID, visitID, request, kind, script, runAs, holder, checkIndex, now)
	return err
}

// stopVisitScriptsTx stops the scripts of the task's open visit: it cancels
// the pending transition request with why, clears the watch schedule, cancels
// pending runs, and asks the worker to kill a running one. Every route that
// closes a visit calls it first.
func stopVisitScriptsTx(ctx context.Context, tx *sql.Tx, task Task, why, now string) error {
	if err := cancelPendingRequestTx(ctx, tx, task, why, now); err != nil {
		return err
	}
	taskID := task.ID
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_status_visits SET next_watch_at = '' WHERE task_id = ? AND left_at = ''`, taskID); err != nil {
		return err
	}
	const openVisit = `visit_id IN (SELECT id FROM task_status_visits WHERE task_id = ? AND left_at = '')`
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_script_runs SET state = 'cancelled', finished_at = ?
		WHERE task_id = ? AND state = 'pending' AND `+openVisit, now, taskID, taskID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE task_script_runs SET cancel_requested = 1
		WHERE task_id = ? AND state = 'running' AND `+openVisit, taskID, taskID)
	return err
}

// checkRequest is the transition request a check run belongs to.
type checkRequest struct {
	id      int64
	visitID int64
	outcome string
	message string
	actor   string
}

// liveCheckRequestTx returns the request of a check run when the request is
// still pending and its visit is still the task's open visit; ok is false
// otherwise, and the run then changes nothing.
func liveCheckRequestTx(ctx context.Context, tx *sql.Tx, run scriptRunRecord) (checkRequest, bool, error) {
	var request checkRequest
	var state RequestState
	err := tx.QueryRowContext(ctx, `
		SELECT id, visit_id, outcome, message, actor, state FROM task_transition_requests WHERE id = ?`, run.requestID).
		Scan(&request.id, &request.visitID, &request.outcome, &request.message, &request.actor, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return checkRequest{}, false, nil
	}
	if err != nil {
		return checkRequest{}, false, err
	}
	if state != RequestStatePending {
		return checkRequest{}, false, nil
	}
	open, err := visitIsOpenTx(ctx, tx, run.taskID, request.visitID)
	return request, open, err
}

// visitIsOpenTx reports whether visitID is the task's open visit.
func visitIsOpenTx(ctx context.Context, tx *sql.Tx, taskID, visitID int64) (bool, error) {
	var open int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM task_status_visits WHERE task_id = ? AND left_at = ''`, taskID).Scan(&open)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && open == visitID, err
}

// storeScriptArtifactsTx stores the artifacts a script returned, authored by
// the script, in name order.
func storeScriptArtifactsTx(ctx context.Context, tx *sql.Tx, task Task, manifest workflowimage.Manifest, script string, artifacts map[string]string, now string) error {
	for _, name := range sortedKeys(artifacts) {
		if _, err := setArtifactTx(ctx, tx, task, manifest, name, artifacts[name], scriptAuthor(script), now); err != nil {
			return err
		}
	}
	return nil
}

// liveWatchTx reports whether a watch run of visitID still speaks for the
// task: the visit is open and the task is neither closed nor paused.
func liveWatchTx(ctx context.Context, tx *sql.Tx, task Task, visitID int64) (bool, error) {
	if task.Status == StatusDone || task.Status == StatusCancelled || task.WorkflowPausedReason != "" {
		return false, nil
	}
	return visitIsOpenTx(ctx, tx, task.ID, visitID)
}

// dueWatchVisits selects the open visits whose watch is due at the bound time
// and may run: the task is open and unpaused and has no active run.
const dueWatchVisits = `
	SELECT v.id FROM task_status_visits v JOIN tasks t ON t.id = v.task_id
	WHERE v.left_at = '' AND v.next_watch_at <> '' AND v.next_watch_at <= ?
	  AND t.status NOT IN ('done', 'cancelled') AND t.workflow_paused_reason = ''
	  AND NOT EXISTS (
		SELECT 1 FROM task_script_runs r WHERE r.task_id = t.id AND r.state IN ('pending', 'running'))`
