package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// workflowActor is the principal the engine acts as: it authors assignments,
// the customer-status comment, and its wait.
const workflowActor = "system:workflow"

// currentStatus finds a status of the manifest by ID.
func currentStatus(manifest workflowimage.Manifest, id string) (workflowfile.Status, bool) {
	for _, status := range manifest.Definition.Statuses {
		if status.ID == id {
			return status, true
		}
	}
	return workflowfile.Status{}, false
}

// statusOutcomes lists the outcome names a status declares, in order.
func statusOutcomes(status workflowfile.Status) []string {
	out := make([]string, 0, len(status.Transitions))
	for _, transition := range status.Transitions {
		out = append(out, transition.On)
	}
	return out
}

// workflowManagedErrorFor is workflowManagedError for a workflow task, with the
// current workflow status and its outcomes in Data.
func workflowManagedErrorFor(ctx context.Context, q queryer, task Task) error {
	manifest, err := loadManifestTx(ctx, q, task.WorkflowDigest)
	if err != nil {
		return err
	}
	outcomes := []string{}
	if status, ok := currentStatus(manifest, task.WorkflowStatus); ok {
		outcomes = statusOutcomes(status)
	}
	return workflowManagedError().WithData(map[string]any{"status": task.WorkflowStatus, "outcomes": outcomes})
}

// enterStatusTx closes the current visit, opens a visit for statusID, sets
// tasks.workflow_status, derives the category and assignee, and records the
// workflow.transitioned event and notification intents. via is the actor.
func (s *WorkflowEngine) enterStatusTx(ctx context.Context, tx *sql.Tx, task *Task, manifest workflowimage.Manifest, statusID, via, outcome, message string) error {
	next, ok := currentStatus(manifest, statusID)
	if !ok {
		return fmt.Errorf("workflow %s:%s has no status %q", manifest.Name, manifest.Version, statusID)
	}
	now := s.now()
	from := task.WorkflowStatus
	if from != "" {
		// Leaving any status by any route answers the workflow's own wait (a
		// customer status's question); leaving a pool status makes the holder's
		// questions to the customer moot.
		holder := ""
		if previous, ok := currentStatus(manifest, from); ok && !previous.Terminal && previous.Owner.Kind == workflowfile.OwnerPool {
			holder = task.Assignee
		}
		if err := resolveWorkflowWaitsTx(ctx, tx, *task, holder, now); err != nil {
			return err
		}
	}
	var last int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sequence), 0) FROM task_status_visits WHERE task_id = ?`, task.ID).Scan(&last); err != nil {
		return err
	}
	if err := stopVisitScriptsTx(ctx, tx, *task, "the task entered status "+statusID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_status_visits SET left_at = ?, outcome = ?, message = ?
		WHERE task_id = ? AND left_at = ''`, now, outcome, message, task.ID); err != nil {
		return err
	}
	// A script status runs its watch at once after it is entered.
	nextWatch := ""
	if !next.Terminal && next.Owner.Kind == workflowfile.OwnerScript && next.Watch != nil {
		nextWatch = store.FormatTime(s.clock())
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_status_visits(task_id, sequence, status_id, entered_at, entered_by, next_watch_at)
		VALUES (?, ?, ?, ?, ?, ?)`, task.ID, last+1, statusID, now, via, nextWatch); err != nil {
		return err
	}

	previousAssignee, previousCategory := task.Assignee, task.Status
	task.WorkflowStatus = statusID
	task.CompletedAt = ""
	if next.Terminal {
		task.CompletedAt = now
	}
	state := stateIn(*task, next)
	if err := s.pickStateHolderTx(ctx, tx, *task, &state); err != nil {
		return err
	}
	if err := applyState(task, state); err != nil {
		return err
	}
	if _, err := saveTaskStateTx(ctx, tx, task, previousCategory, now, false); err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, *task, "workflow.transitioned", Actor{Principal: via}, map[string]any{
		"from": from, "to": statusID, "outcome": outcome, "actor": via, "message": message,
	}, now); err != nil {
		return err
	}
	if !next.Terminal && next.Owner.Kind == workflowfile.OwnerCustomer {
		if err := s.openStatusWaitTx(ctx, tx, *task, next, now); err != nil {
			return err
		}
	}
	return recordAssignmentTx(ctx, tx, *task, previousAssignee, now)
}

// resolveWorkflowWaitsTx resolves open waits of a workflow task without an
// answering comment: always the workflow's own wait, and with holder set the
// questions holder asked the task's customer. Waits requested by anyone else,
// or expecting another principal, stay open.
func resolveWorkflowWaitsTx(ctx context.Context, tx *sql.Tx, task Task, holder, now string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE task_waiting_for SET resolved_at = ?
		WHERE task_id = ? AND resolved_at = '' AND (
			requesting_principal = ? OR
			(? <> '' AND requesting_principal = ? AND expected_principal = ?))`,
		now, task.ID, workflowActor, holder, holder, task.Customer)
	return err
}

// openStatusWaitTx asks the customer for an outcome the way a comment with a
// mention does: a comment by the workflow, an open wait, and a question
// notification.
func (s *WorkflowEngine) openStatusWaitTx(ctx context.Context, tx *sql.Tx, task Task, status workflowfile.Status, now string) error {
	body := fmt.Sprintf("@%s The task is waiting for you in workflow status %q.\nOutcomes: %s.",
		task.Customer, status.ID, strings.Join(statusOutcomes(status), ", "))
	if instructions := strings.TrimSpace(status.Instructions); instructions != "" {
		body += "\n\n" + instructions
	}
	return s.askCustomerTx(ctx, tx, task, body, now)
}

// askCustomerTx posts body as a comment by the workflow and opens the
// workflow's wait on the customer with a question notification.
func (s *WorkflowEngine) askCustomerTx(ctx context.Context, tx *sql.Tx, task Task, body, now string) error {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_comments(task_id, author, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, task.ID, workflowActor, body, now, now)
	if err != nil {
		return err
	}
	commentID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := upsertWait(ctx, tx, task, task.Customer, workflowActor, commentID, now); err != nil {
		return err
	}
	sequence, err := appendEventTx(ctx, tx, task, "task.comment_added", Actor{Principal: workflowActor},
		map[string]any{"comment_id": commentID, "mentions": []string{task.Customer}, "resolved": []string{}}, now)
	if err != nil {
		return err
	}
	return enqueueNotificationTx(ctx, tx, sequence, task.Customer, workflowActor,
		"task.question", task, workflowActor+" asked for an answer on "+task.Key, now)
}

// recordAssignmentTx writes what a manual assignment writes, by the workflow:
// the task.updated event and the assignment notification.
func recordAssignmentTx(ctx context.Context, tx *sql.Tx, task Task, previousAssignee, now string) error {
	if task.Assignee == "" || task.Assignee == previousAssignee {
		return nil
	}
	sequence, err := appendEventTx(ctx, tx, task, "task.updated", Actor{Principal: workflowActor},
		map[string]any{"status": task.Status, "pull_request": task.PullRequest, "assignee": task.Assignee, "priority": task.Priority}, now)
	if err != nil {
		return err
	}
	return enqueueNotificationTx(ctx, tx, sequence, task.Assignee, workflowActor,
		"task.assigned", task, "Task assigned: "+task.Key+" "+task.Title, now)
}
