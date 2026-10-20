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
