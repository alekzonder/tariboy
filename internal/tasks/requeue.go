package tasks

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

// RequeueInput names the queue a task moves to and, when not zero, the
// revision the caller read.
type RequeueInput struct {
	Queue    string `json:"queue"`
	Revision int64  `json:"revision"`
}

// RequeueTask moves an open task to another queue. The task's key belongs to
// its queue, so the move creates a new task there, which enters the target
// queue's workflow like any new task, and closes the source as cancelled,
// stopping its workflow. Customer only: an agent cannot stop a workflow.
func (s *TaskService) RequeueTask(ctx context.Context, actor Actor, key string, in RequeueInput) (Task, error) {
	if err := requireWorkflowOperator(actor); err != nil {
		return Task{}, err
	}
	queue := strings.ToUpper(strings.TrimSpace(in.Queue))
	if queue == "" {
		return Task{}, newError(http.StatusBadRequest, "missing_queue", "target queue is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	source, err := requeueSourceTx(ctx, tx, strings.TrimSpace(key), queue, in.Revision)
	if err != nil {
		return Task{}, err
	}
	moved, err := s.createTaskTx(ctx, tx, actor, CreateTaskInput{
		Queue: queue, Title: source.Title, Description: source.Description, Priority: source.Priority,
		PullRequest: source.PullRequest, Group: source.Group,
	})
	if err != nil {
		return Task{}, err
	}
	now := s.now()
	if source.WorkflowDigest != "" {
		err = cancelWorkflowTaskTx(ctx, tx, actor, source, now)
	} else {
		err = cancelFlexibleTaskTx(ctx, tx, source, now)
	}
	if err != nil {
		return Task{}, err
	}
	if err := linkRequeuedTasksTx(ctx, tx, actor, source, moved, now); err != nil {
		return Task{}, err
	}
	stored, err := taskByID(ctx, tx, moved.ID)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	s.signal()
	stored.Access = "write"
	return stored, nil
}

// requeueSourceTx reads the task to move and refuses a closed task, a move
// into its own queue, a stale revision, and a task with open children.
func requeueSourceTx(ctx context.Context, tx *sql.Tx, key, queue string, revision int64) (Task, error) {
	source, err := taskByKey(ctx, tx, key)
	if err != nil {
		return Task{}, err
	}
	if source.Status == StatusDone || source.Status == StatusCancelled {
		return Task{}, newError(http.StatusConflict, "task_closed", "task "+source.Key+" is closed")
	}
	if source.Queue == queue {
		return Task{}, newError(http.StatusConflict, "same_queue", "task "+source.Key+" is already in queue "+queue)
	}
	if revision != 0 && revision != source.Revision {
		return Task{}, &Error{Status: http.StatusConflict, Code: "revision_conflict",
			Msg: "task was changed by another actor", Data: map[string]any{
				"current_revision": source.Revision, "current": source,
			}}
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT task_key FROM tasks WHERE parent_id = ? AND status NOT IN ('done', 'cancelled') ORDER BY id`, source.ID)
	if err != nil {
		return Task{}, err
	}
	defer rows.Close()
	var children []string
	for rows.Next() {
		var child string
		if err := rows.Scan(&child); err != nil {
			return Task{}, err
		}
		children = append(children, child)
	}
	if err := rows.Err(); err != nil {
		return Task{}, err
	}
	if len(children) > 0 {
		return Task{}, &Error{Status: http.StatusConflict, Code: "requeue_has_children",
			Msg:  "task " + source.Key + " has open subtasks: " + strings.Join(children, ", "),
			Data: map[string]any{"children": children}}
	}
	return source, nil
}

// cancelFlexibleTaskTx closes a task without a workflow as cancelled.
func cancelFlexibleTaskTx(ctx context.Context, tx *sql.Tx, task Task, now string) error {
	if err := guardRevisionTx(ctx, tx, task); err != nil {
		return err
	}
	previousCategory := task.Status
	task.Status, task.CompletedAt = StatusCancelled, now
	_, err := saveTaskStateTx(ctx, tx, &task, previousCategory, now, false)
	return err
}

// linkRequeuedTasksTx copies the source's comments to the moved task, relates
// the two, and records the move on both.
func linkRequeuedTasksTx(ctx context.Context, tx *sql.Tx, actor Actor, source, moved Task, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_comments(task_id, author, body, created_at, updated_at)
		SELECT ?, author, body, created_at, updated_at FROM task_comments WHERE task_id = ? ORDER BY id`,
		moved.ID, source.ID); err != nil {
		return err
	}
	for _, note := range []struct {
		task Task
		body string
	}{
		{source, "Moved to queue " + moved.Queue + " as " + moved.Key + "."},
		{moved, "Moved from " + source.Key + "."},
	} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_comments(task_id, author, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			note.task.ID, actor.Principal, note.body, now, now); err != nil {
			return err
		}
	}
	// A related relation is stored from the older task to the newer one.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_relations(source_id, target_id, type, created_by, created_at)
		VALUES (?, ?, 'related', ?, ?)`, source.ID, moved.ID, actor.Principal, now); err != nil {
		return err
	}
	payload := map[string]any{
		"from_key": source.Key, "to_key": moved.Key, "from_queue": source.Queue, "to_queue": moved.Queue,
	}
	for _, task := range []Task{source, moved} {
		if _, err := appendEventTx(ctx, tx, task, "task.requeued", actor, payload, now); err != nil {
			return err
		}
	}
	return nil
}
