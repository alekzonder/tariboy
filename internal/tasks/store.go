package tasks

import (
	"database/sql"
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
func taskByKey(q interface {
	QueryRow(query string, args ...any) *sql.Row
}, key string) (Task, error) {
	normalized := NormalizeKey(key)
	t, err := scanTask(q.QueryRow(taskSelect+`
		WHERE t.task_key = ?
		   OR t.id = (SELECT task_id FROM task_key_aliases WHERE old_key = ?)`,
		normalized, normalized))
	if err == sql.ErrNoRows {
		return Task{}, notFound(key)
	}
	return t, err
}

func taskByID(q interface {
	QueryRow(query string, args ...any) *sql.Row
}, id int64) (Task, error) {
	t, err := scanTask(q.QueryRow(taskSelect+` WHERE t.id = ?`, id))
	if err == sql.ErrNoRows {
		return Task{}, notFound("")
	}
	return t, err
}
