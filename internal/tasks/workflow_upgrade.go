package tasks

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// UpgradeWorkflow pins a task to the workflow version its queue is bound to
// now and restarts it at that version's initial status. The current status,
// a pending request, a pause, and the status's scripts end as a move ends
// them; artifacts and pool holders stay. Customer only.
func (s *WorkflowEngine) UpgradeWorkflow(ctx context.Context, actor Actor, key, reason string) (Task, error) {
	if err := requireWorkflowOperator(actor); err != nil {
		return Task{}, err
	}
	reason = strings.TrimSpace(reason)
	global, err := s.admin.globalEnvNames()
	if err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	task, from, err := artifactTaskTx(ctx, tx, actor, key)
	if err != nil {
		return Task{}, err
	}
	if task.Status == StatusDone || task.Status == StatusCancelled {
		return Task{}, newError(http.StatusConflict, "workflow_closed", "task "+task.Key+" is closed")
	}
	to, err := s.upgradeTargetTx(ctx, tx, task, global)
	if err != nil {
		return Task{}, err
	}
	now := s.now()
	pausedReason, err := overrideWorkflowTx(ctx, tx, &task, "workflow upgraded by "+actor.Principal, now)
	if err != nil {
		return Task{}, err
	}
	// The old version knows who holds its status: its holder's questions end
	// with it. enterStatusTx reads the new version, which may not declare it.
	holder := ""
	if previous, ok := taskStatus(from, task); ok && !previous.Terminal && previous.Owner.Kind == workflowfile.OwnerPool {
		holder = task.Assignee
	}
	if err := resolveWorkflowWaitsTx(ctx, tx, task, holder, now); err != nil {
		return Task{}, err
	}
	fromDigest, status := task.WorkflowDigest, task.WorkflowStatus
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET workflow_digest = ? WHERE id = ?`, to.Digest, task.ID); err != nil {
		return Task{}, err
	}
	task.WorkflowDigest = to.Digest
	message := "workflow upgraded from " + from.Version + " to " + to.Version
	if reason != "" {
		message += ": " + reason
	}
	if err := s.enterStatusTx(ctx, tx, &task, to, to.Definition.InitialStatus, actor.Principal, "", message); err != nil {
		return Task{}, err
	}
	if err := recordOverrideResumeTx(ctx, tx, task, actor, status, "upgrade", pausedReason, now); err != nil {
		return Task{}, err
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.upgraded", actor, map[string]any{
		"from_version": from.Version, "to_version": to.Version, "from_digest": fromDigest, "to_digest": to.Digest,
		"status": status, "actor": actor.Principal, "reason": reason,
	}, now); err != nil {
		return Task{}, err
	}
	upgraded, err := taskByID(ctx, tx, task.ID)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	s.signal()
	upgraded.Access = "write"
	return upgraded, nil
}

// upgradeTargetTx returns the manifest the task's queue is bound to now,
// following a tag binding as a new task does. It refuses an unbound queue, a
// task already on that version, and a version whose pools or secrets the
// queue does not provide.
func (s *WorkflowEngine) upgradeTargetTx(ctx context.Context, tx *sql.Tx, task Task, global map[string]bool) (workflowimage.Manifest, error) {
	binding, bound, err := queueWorkflowTx(ctx, tx, task.Queue)
	if err != nil {
		return workflowimage.Manifest{}, err
	}
	if !bound {
		return workflowimage.Manifest{}, newError(http.StatusConflict, "workflow_unbound",
			"queue "+task.Queue+" has no workflow binding")
	}
	if binding, err = s.followQueueWorkflowTx(ctx, tx, binding); err != nil {
		return workflowimage.Manifest{}, err
	}
	if binding.Digest == task.WorkflowDigest {
		return workflowimage.Manifest{}, newError(http.StatusConflict, "workflow_current",
			"task "+task.Key+" already follows "+binding.Name+"@"+binding.Version+", the version its queue is bound to")
	}
	manifest, err := loadManifestTx(ctx, tx, binding.Digest)
	if err != nil {
		return workflowimage.Manifest{}, err
	}
	pools, err := workflowPoolsTx(ctx, tx, task.Queue, manifest)
	if err != nil {
		return workflowimage.Manifest{}, err
	}
	if missing, err := emptyPools(ctx, tx, task.Queue, pools); err != nil {
		return workflowimage.Manifest{}, err
	} else if len(missing) > 0 {
		return workflowimage.Manifest{}, poolsEmptyError(missing)
	}
	if missing, err := missingQueueSecrets(ctx, tx, task.Queue, manifest.Definition.RequiresSecrets, global); err != nil {
		return workflowimage.Manifest{}, err
	} else if len(missing) > 0 {
		return workflowimage.Manifest{}, queueSecretsMissingError(missing)
	}
	return manifest, nil
}
