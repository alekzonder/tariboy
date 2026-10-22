package tasks

// The task repository: the SQL that reads and writes a task row and the
// visibility join, as functions over a queryer or a transaction, so a service
// method holds authorization, validation, and events and calls these for
// storage.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// taskColumns, taskFrom, and scanTask read one task; taskSelect joins them.
// visibleTaskSelect also returns the access rank and needs visibleCTE.
const taskColumns = `
SELECT t.id, t.task_key, t.queue_prefix, COALESCE(p.task_key, ''),
       t.position, t.priority, t.title, t.description, t.status, t.pull_request, t.author, t.customer,
       t.group_name, t.assignee, t.manual_block_reason,
       t.open_blockers > 0 OR t.manual_block_reason <> '',
       t.revision, t.created_at, t.started_at, t.updated_at, t.completed_at,` + TaskWorkflowColumns

const taskFrom = `
FROM tasks t
LEFT JOIN tasks p ON p.id = t.parent_id` + TaskWorkflowJoin

const taskSelect = taskColumns + taskFrom

const visibleTaskSelect = taskColumns + `, v.access_rank` + taskFrom + `
JOIN visible_access v ON v.id = t.id`

type rowScanner interface {
	Scan(dest ...any) error
}

// scanTask scans the columns of taskColumns, then extra.
func scanTask(row rowScanner, extra ...any) (Task, error) {
	var t Task
	var blocked bool
	workflow, finish := WorkflowScanTargets(&t)
	dest := []any{
		&t.ID, &t.Key, &t.Queue, &t.ParentKey,
		&t.Position, &t.Priority, &t.Title, &t.Description, &t.Status, &t.PullRequest, &t.Author, &t.Customer,
		&t.Group, &t.Assignee, &t.ManualBlockReason, &blocked,
		&t.Revision, &t.CreatedAt, &t.StartedAt, &t.UpdatedAt, &t.CompletedAt,
	}
	err := row.Scan(append(append(dest, workflow...), extra...)...)
	t.Blocked = blocked
	finish()
	return t, err
}

// scanVisibleTask scans a row of visibleTaskSelect and sets Access.
func scanVisibleTask(row rowScanner) (Task, error) {
	var rank int
	t, err := scanTask(row, &rank)
	t.Access = accessName(rank)
	return t, err
}

// taskByKey is the single key lookup for the whole service, so key spelling is
// normalized here rather than at every caller, and a key retired by the random
// key migration keeps resolving through its alias.
func taskByKey(ctx context.Context, q queryer, key string) (Task, error) {
	normalized := NormalizeKey(key)
	t, err := scanTask(q.QueryRowContext(ctx, taskSelect+`
		WHERE t.task_key = ?
		   OR t.id = (SELECT task_id FROM task_key_aliases WHERE old_key = ?)`,
		normalized, normalized))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, taskNotFound(key)
	}
	return t, err
}

func taskByID(ctx context.Context, q queryer, id int64) (Task, error) {
	t, err := scanTask(q.QueryRowContext(ctx, taskSelect+` WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, taskNotFound("")
	}
	return t, err
}

// visibleCTE returns a WITH clause defining visible_access(id, access_rank):
// write-visible tasks (rank 2), tasks waiting on the actor (rank 1), and the
// read-only ancestor chain required to place them in the tree (rank 0). Its
// arguments do not depend on the number of tasks, so a query joins it instead
// of binding a variable per visible id. The recursive CTE has no depth limit.
func visibleCTE(actor Actor) (string, []any) {
	if actor.IsCustomer {
		return `WITH visible_access(id, access_rank) AS (SELECT id, 2 FROM tasks) `, nil
	}
	agent := strings.TrimPrefix(actor.Principal, "agent:")
	return `
		WITH RECURSIVE writeable(id) AS (
			-- Authorship carries write access inside a tree the agent is already responsible
			-- for, but not to a root: a root it authored in a queue it does not run is a
			-- filed report, and holding on to it would turn filing into self-assignment.
			SELECT id FROM tasks
			WHERE assignee = ? OR (author = ? AND parent_id IS NOT NULL)
			UNION
			SELECT t.id
			FROM tasks t
			WHERE EXISTS (
				SELECT 1
				FROM task_queues q
				LEFT JOIN task_queue_owners qo
				  ON qo.queue_prefix = q.prefix AND qo.agent = ?
				WHERE q.prefix = t.queue_prefix
				  AND (qo.agent IS NOT NULL OR q.responsible_agent = ?)
			)
			UNION
			SELECT t.id
			FROM tasks t
			WHERE t.group_name <> ''
			  AND EXISTS (
				SELECT 1 FROM agents ag
				WHERE ag.name = ? AND ag."group" = t.group_name
			)
			UNION
			SELECT child.id
			FROM tasks child
			JOIN writeable parent ON child.parent_id = parent.id
		),
		respondable(id) AS (
			SELECT DISTINCT t.id
			FROM tasks t
			WHERE EXISTS (
				SELECT 1 FROM task_waiting_for wf
				WHERE wf.task_id = t.id
				  AND wf.expected_principal = ?
				  AND wf.resolved_at = ''
			)
		),
		direct(id, access_rank) AS (
			SELECT id, 2 FROM writeable
			UNION
			SELECT id, 1 FROM respondable
		),
		visible(id, access_rank) AS (
			SELECT id, access_rank FROM direct
			UNION
			SELECT parent.parent_id, 0
			FROM visible
			JOIN tasks parent ON parent.id = visible.id
			WHERE parent.parent_id IS NOT NULL
		),
		visible_access(id, access_rank) AS (
			SELECT id, MAX(access_rank) FROM visible GROUP BY id
		) `, []any{actor.Principal, actor.Principal, agent, agent, agent, actor.Principal}
}

// accessName names an access_rank of visible_access.
func accessName(rank int) string {
	switch rank {
	case 2:
		return "write"
	case 1:
		return "respond"
	default:
		return "context"
	}
}

// saveTaskStateTx is the one write of a task's state: status, workflow
// status, assignee, pause reason, and completed_at as task holds them. It
// bumps the stored revision into task.Revision and mirrors the first start
// the tasks_started_at trigger stores. With guard the write applies only while
// the stored revision is still task.Revision, and saved reports whether it did.
func saveTaskStateTx(ctx context.Context, tx *sql.Tx, task *Task, previousCategory, now string, guard bool) (saved bool, err error) {
	query := `
		UPDATE tasks
		SET status = ?, workflow_status = NULLIF(?, ''), assignee = ?, workflow_paused_reason = ?,
		    completed_at = ?, revision = revision + 1, updated_at = ?
		WHERE id = ?`
	args := []any{task.Status, task.WorkflowStatus, task.Assignee, task.WorkflowPausedReason, task.CompletedAt, now, task.ID}
	if guard {
		query += ` AND revision = ?`
		args = append(args, task.Revision)
	}
	err = tx.QueryRowContext(ctx, query+` RETURNING revision`, args...).Scan(&task.Revision)
	if guard && errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if task.Status == StatusInProgress && previousCategory != StatusInProgress && task.StartedAt == "" {
		task.StartedAt = now
	}
	task.UpdatedAt = now
	return true, nil
}
