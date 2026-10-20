package tasks

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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

// agentRunsQueue reports whether the actor owns or triages the queue, which is what separates
// creating work in it from filing a report into it.
func agentRunsQueue(ctx context.Context, q queryer, actor Actor, queue string) (bool, error) {
	if actor.IsCustomer {
		return true, nil
	}
	agent := strings.TrimPrefix(actor.Principal, "agent:")
	var runs bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM task_queues q
			LEFT JOIN task_queue_owners qo
			  ON qo.queue_prefix = q.prefix AND qo.agent = ?
			WHERE q.prefix = ?
			  AND (q.responsible_agent = ? OR qo.agent IS NOT NULL)
		)`, agent, queue, agent).Scan(&runs)
	return runs, err
}

// taskAccess returns the actor's access to one task, or "" when it has none.
func taskAccess(ctx context.Context, q queryer, actor Actor, taskID int64) (string, error) {
	cte, args := visibleCTE(actor)
	var rank int
	err := q.QueryRowContext(ctx, cte+`SELECT access_rank FROM visible_access WHERE id = ?`,
		append(args, taskID)...).Scan(&rank)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return accessName(rank), nil
}

// readAccess is taskAccess for a read. An agent recorded in
// task_workflow_holders for the task reads it even with no other access, as
// "context": a holder whose transition moved the task into a script status or
// to another pool must still see its task and the result of its request. It
// grants no write; every write checks taskAccess.
func readAccess(ctx context.Context, q queryer, actor Actor, taskID int64) (string, error) {
	access, _, err := holderReadAccess(ctx, q, actor, taskID)
	return access, err
}

// holderReadAccess is readAccess that also reports whether the holder row is
// the reader's only access.
func holderReadAccess(ctx context.Context, q queryer, actor Actor, taskID int64) (access string, holderOnly bool, err error) {
	access, err = taskAccess(ctx, q, actor, taskID)
	if err != nil || access != "" || actor.IsCustomer {
		return access, false, err
	}
	agent, ok := strings.CutPrefix(actor.Principal, "agent:")
	if !ok {
		return "", false, nil
	}
	var holds bool
	if err := q.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM task_workflow_holders WHERE task_id = ? AND agent = ?)`, taskID, agent).Scan(&holds); err != nil {
		return "", false, err
	}
	if holds {
		return "context", true, nil
	}
	return "", false, nil
}

func requireRespond(ctx context.Context, q queryer, actor Actor, task Task) error {
	access, err := taskAccess(ctx, q, actor, task.ID)
	if err != nil {
		return err
	}
	if access != "write" && access != "respond" {
		return notFound(task.Key)
	}
	return nil
}

func requireWrite(ctx context.Context, q queryer, actor Actor, task Task) error {
	access, err := taskAccess(ctx, q, actor, task.ID)
	if err != nil {
		return err
	}
	if access != "write" {
		return notFound(task.Key)
	}
	return nil
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
