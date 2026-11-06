package tasks

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"strings"
)

// DismissWaitsResult is the task after the customer dismissed its open
// questions, and the questions that closed.
type DismissWaitsResult struct {
	Task          Task         `json:"task"`
	ResolvedWaits []WaitingFor `json:"resolved_waits"`
}

// DismissWaits closes the customer's open questions on a task in any status
// without an answer. A workflow's own question stays: an outcome answers it.
// Customer only; with no open question it changes nothing.
func (s *TaskService) DismissWaits(ctx context.Context, actor Actor, key string) (DismissWaitsResult, error) {
	if err := validateActor(actor); err != nil {
		return DismissWaitsResult{}, err
	}
	if !actor.IsCustomer {
		return DismissWaitsResult{}, newError(http.StatusForbidden, "forbidden", "only the customer can dismiss a question")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DismissWaitsResult{}, err
	}
	defer tx.Rollback()
	task, err := taskByKey(ctx, tx, strings.TrimSpace(key))
	if err != nil {
		return DismissWaitsResult{}, err
	}
	open, err := openWaitsForPrincipal(ctx, tx, task, task.Customer)
	if err != nil {
		return DismissWaitsResult{}, err
	}
	resolved := withoutWorkflowWaits(open)
	task.Access = "write"
	if len(resolved) == 0 {
		return DismissWaitsResult{Task: task, ResolvedWaits: resolved}, nil
	}
	now := s.now()
	askers := []string{}
	for i := range resolved {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_waiting_for SET resolved_at = ?, resolving_comment_id = NULL
			WHERE id = ? AND resolved_at = ''`, now, resolved[i].ID); err != nil {
			return DismissWaitsResult{}, err
		}
		resolved[i].ResolvedAt = now
		if !slices.Contains(askers, resolved[i].RequestingPrincipal) {
			askers = append(askers, resolved[i].RequestingPrincipal)
		}
	}
	if err := closeQuestionLettersTx(ctx, tx, task, task.Customer, askers, "task-wait-dismissed", now); err != nil {
		return DismissWaitsResult{}, err
	}
	previousStatus := task.Status
	if err := dismissStatusTx(ctx, tx, &task); err != nil {
		return DismissWaitsResult{}, err
	}
	if _, err := saveTaskStateTx(ctx, tx, &task, previousStatus, now, false); err != nil {
		return DismissWaitsResult{}, err
	}
	if task.Status != previousStatus {
		if _, err := appendEventTx(ctx, tx, task, "task.updated", actor,
			map[string]any{"status": task.Status, "pull_request": task.PullRequest, "assignee": task.Assignee, "priority": task.Priority}, now); err != nil {
			return DismissWaitsResult{}, err
		}
	}
	sequence, err := appendEventTx(ctx, tx, task, "task.waits_dismissed", actor,
		map[string]any{"resolved": waitPrincipals(resolved), "askers": askers}, now)
	if err != nil {
		return DismissWaitsResult{}, err
	}
	for _, asker := range askers {
		if asker == actor.Principal {
			continue
		}
		if err := enqueueNotificationTx(ctx, tx, sequence, asker, actor.Principal, "task.answered", task,
			actor.Principal+" dismissed the question on "+task.Key+"; no answer is needed", now); err != nil {
			return DismissWaitsResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DismissWaitsResult{}, err
	}
	s.signal()
	return DismissWaitsResult{Task: task, ResolvedWaits: resolved}, nil
}

// dismissStatusTx moves the task's status as the customer's answer would: a
// task waiting for the customer goes back to its assignee once no question
// to the customer stays open. A closed task keeps its status.
func dismissStatusTx(ctx context.Context, tx *sql.Tx, task *Task) error {
	if task.Status == StatusDone || task.Status == StatusCancelled {
		return nil
	}
	left, err := openWaitsForPrincipal(ctx, tx, *task, task.Customer)
	if err != nil {
		return err
	}
	answered := task.Status == StatusWaitCustomer && len(left) == 0
	if task.WorkflowDigest != "" {
		// The answer lets the holder work again: idle iterations restart.
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_status_visits SET idle_iterations = 0 WHERE task_id = ? AND left_at = ''`, task.ID); err != nil {
			return err
		}
		return commentWorkflowStateTx(ctx, tx, task, false, answered)
	}
	if answered {
		task.Status = StatusInProgress
	}
	return nil
}
