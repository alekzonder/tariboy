package tasks

import (
	"context"
	"net/http"

	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// RunWatchNow creates a pending run of the current status's watch at once
// instead of at the visit's next_watch_at, and clears that schedule as a due
// watch does; the run then completes like a scheduled one. Customer only. It
// refuses a closed or paused task, a status without a watch, and a task with
// an active run.
func (s *WorkflowEngine) RunWatchNow(ctx context.Context, actor Actor, key string) (ScriptRun, error) {
	if err := requireWorkflowOperator(actor); err != nil {
		return ScriptRun{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ScriptRun{}, err
	}
	defer tx.Rollback()
	task, manifest, err := artifactTaskTx(ctx, tx, actor, key)
	if err != nil {
		return ScriptRun{}, err
	}
	if err := requireOpenWorkflow(task); err != nil {
		return ScriptRun{}, err
	}
	status, ok := currentStatus(manifest, task.WorkflowStatus)
	if !ok || status.Terminal || status.Owner.Kind != workflowfile.OwnerScript || status.Watch == nil {
		return ScriptRun{}, newError(http.StatusConflict, "workflow_no_watch",
			"status "+task.WorkflowStatus+" of "+task.Key+" has no watch script")
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM task_script_runs WHERE task_id = ? AND state IN ('pending', 'running'))`,
		task.ID).Scan(&active); err != nil {
		return ScriptRun{}, err
	}
	if active {
		return ScriptRun{}, newError(http.StatusConflict, "script_running",
			"a script run of "+task.Key+" is still active")
	}
	var visitID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM task_status_visits WHERE task_id = ? AND left_at = ''`,
		task.ID).Scan(&visitID); err != nil {
		return ScriptRun{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE task_status_visits SET next_watch_at = '' WHERE id = ?`, visitID); err != nil {
		return ScriptRun{}, err
	}
	if err := insertScriptRunTx(ctx, tx, task.ID, visitID, 0, RunKindWatch, status.Watch.Script,
		workflowfile.RunAsQueue, "", 0, s.now()); err != nil {
		return ScriptRun{}, err
	}
	runs, err := scriptRunsTx(ctx, tx, task.ID, 1)
	if err != nil {
		return ScriptRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return ScriptRun{}, err
	}
	s.signal()
	return runs[0], nil
}
