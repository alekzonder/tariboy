package tasks

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// Why a pool member cannot work, as holderUnavailableCauseSQL names it.
const (
	causeDeleted  = "deleted"
	causeDisabled = "disabled"
	causeBare     = "bare"
	causeHalted   = "halted"
	causeLeftPool = "left_pool"
)

// holderUnavailableCauseSQL is the one reading of whether an agent can work on
// pool work: it yields "" when it can, or one of the cause constants. It reads
// the agents row as a and the pool membership row as m, either of which may be
// missing (NULL) under a LEFT JOIN, and takes agent.IdleStopPrefix twice. The
// halt test mirrors agent.HaltReason: an error reason, or a status message
// that starts with the idle-stop prefix. The terminal-only bare image runs no
// iterations. The dispatcher's eligibility and the
// holder check both use it; Goal settings and the current Goal are not part of
// it.
const holderUnavailableCauseSQL = `CASE
		WHEN a.name IS NULL THEN '` + causeDeleted + `'
		WHEN a.enabled = 0 THEN '` + causeDisabled + `'
		WHEN a.error_reason <> '' OR substr(a.status_message, 1, length(?)) = ? THEN '` + causeHalted + `'
		WHEN a.image_ref = 'bare:latest' THEN '` + causeBare + `'
		WHEN m.agent IS NULL THEN '` + causeLeftPool + `'
		ELSE '' END`

// Why pickHolderTx chose its agent, as the workflow.dispatched event names it.
const (
	dispatchSticky         = "sticky"
	dispatchPreviousHolder = "previous_holder"
	dispatchFreeMember     = "free_member"
)

// holderPick is the result of pickHolderTx. Agent is "" when nobody was chosen.
type holderPick struct {
	Agent               string
	Reason              string // dispatchSticky, dispatchPreviousHolder, or dispatchFreeMember
	PreviousHolder      string // the pool's previous holder: a member, not released; or ""
	HolderFree          bool   // the chosen agent is a free member
	FreeMemberAvailable bool   // a free member other than the chosen agent exists
}

// pickHolderTx chooses the agent for a task entering or waiting in a status
// owned by pool. A member is free when it can work, has Goal enabled, has no
// current Goal other than this task, has no other assigned task that Goal
// could select, and is not in busy. The pool's previous holder, while it is
// still a member and not released, wins when sticky is set, whatever it is
// doing, and otherwise only when it is free. Without it the first free member
// wins: ties go to the member dispatched least recently in this queue, then to
// pool order. A released holder is neither sticky nor free for this task and
// pool until another agent replaces its row or an operator move clears it.
// Agent is "" when nobody is chosen; a later DispatchPending pass applies the
// same rule again.
func pickHolderTx(ctx context.Context, tx *sql.Tx, task Task, pool string, sticky bool, busy map[string]bool) (holderPick, error) {
	var pick holderPick
	err := tx.QueryRowContext(ctx, `
		SELECT h.agent FROM task_workflow_holders h
		JOIN task_agent_pools p ON p.queue_prefix = ? AND p.name = h.pool
		JOIN task_agent_pool_members m ON m.pool_id = p.id AND m.agent = h.agent
		WHERE h.task_id = ? AND h.pool = ? AND h.released = 0`, task.Queue, task.ID, pool).Scan(&pick.PreviousHolder)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return holderPick{}, err
	}
	free, err := freeMembersTx(ctx, tx, task, pool, busy)
	if err != nil {
		return holderPick{}, err
	}
	isFree := func(name string) bool {
		for _, f := range free {
			if f == name {
				return true
			}
		}
		return false
	}
	switch {
	case pick.PreviousHolder != "" && sticky:
		pick.Agent, pick.Reason = pick.PreviousHolder, dispatchSticky
	case pick.PreviousHolder != "" && isFree(pick.PreviousHolder):
		pick.Agent, pick.Reason = pick.PreviousHolder, dispatchPreviousHolder
	case len(free) > 0:
		pick.Agent, pick.Reason = free[0], dispatchFreeMember
	}
	pick.HolderFree = pick.Agent != "" && isFree(pick.Agent)
	for _, f := range free {
		if f != pick.Agent {
			pick.FreeMemberAvailable = true
		}
	}
	return pick, nil
}

// freeMembersTx returns the free members of pool for task in dispatch order.
// The assigned-work test is the one taskgoal's reconcileAgent selects a Goal
// by, read in this transaction: current_goal_task_key is filled only after the
// reconciler runs, so without it one free agent would collect every new task.
// A current Goal on this task itself does not make its holder busy: the holder
// asked for a move between two statuses of one pool.
func freeMembersTx(ctx context.Context, tx *sql.Tx, task Task, pool string, busy map[string]bool) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT m.agent
		FROM task_agent_pools p
		JOIN task_agent_pool_members m ON m.pool_id = p.id
		JOIN agents a ON a.name = m.agent
		WHERE p.queue_prefix = ? AND p.name = ?
		  AND (`+holderUnavailableCauseSQL+`) = '' AND a.goal_enabled = 1
		  AND a.current_goal_task_key IN ('', ?)
		  AND NOT EXISTS (
			SELECT 1 FROM task_workflow_holders r
			WHERE r.task_id = ? AND r.pool = p.name AND r.agent = m.agent AND r.released = 1
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM tasks t
			WHERE t.assignee = 'agent:' || m.agent AND t.id <> ?
			  AND t.status IN ('in_progress', 'open')
			  AND t.manual_block_reason = ''
			  AND t.open_blockers = 0
		  )
		ORDER BY COALESCE((
			SELECT MAX(h.dispatched_at) FROM task_workflow_holders h
			JOIN tasks ht ON ht.id = h.task_id
			WHERE h.agent = m.agent AND ht.queue_prefix = p.queue_prefix
		), ''), m.position`,
		task.Queue, pool, agent.IdleStopPrefix, agent.IdleStopPrefix, task.Key, task.ID, task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var free []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !busy[name] {
			free = append(free, name)
		}
	}
	return free, rows.Err()
}

// appendDispatchedEventTx records the workflow.dispatched event of pick: the
// agent dispatched into a pool status and why.
func appendDispatchedEventTx(ctx context.Context, tx *sql.Tx, task Task, statusID, pool string, sticky bool, pick holderPick, now string) error {
	_, err := appendEventTx(ctx, tx, task, "workflow.dispatched", Actor{Principal: workflowActor}, map[string]any{
		"status": statusID, "pool": pool, "agent": pick.Agent, "reason": pick.Reason, "sticky_holder": sticky,
		"previous_holder": pick.PreviousHolder, "holder_free": pick.HolderFree,
		"free_member_available": pick.FreeMemberAvailable,
	}, now)
	return err
}

// recordHolderTx records name as the task's holder for pool, dispatched now,
// replacing a released holder. It is the only writer of
// task_workflow_holders.dispatched_at; a dispatch starts the holder's
// availability afresh, so it clears unavailable_since.
func (s *WorkflowEngine) recordHolderTx(ctx context.Context, tx *sql.Tx, task Task, pool, name string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_workflow_holders(task_id, pool, agent, dispatched_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(task_id, pool) DO UPDATE SET agent = excluded.agent, dispatched_at = excluded.dispatched_at,
			released = 0, unavailable_since = ''`,
		task.ID, pool, name, store.FormatTime(s.clock()))
	return err
}

// DispatchPending assigns holders to workflow tasks waiting in a pool status
// without one. It returns how many tasks it assigned.
func (s *WorkflowEngine) DispatchPending(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id FROM tasks t
		WHERE t.workflow_digest IS NOT NULL AND t.status = 'open' AND t.workflow_paused_reason = ''
		  AND t.manual_block_reason = '' AND t.open_blockers = 0
		ORDER BY t.priority, t.created_at, t.id`)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	// An agent assigned in this pass has work now and is not eligible again.
	busy := map[string]bool{}
	assigned := 0
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		name, err := s.dispatchTask(ctx, id, busy)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if name != "" {
			busy[name] = true
			assigned++
		}
	}
	if assigned > 0 {
		s.signal()
	}
	return assigned, errors.Join(errs...)
}

// dispatchTask assigns one waiting workflow task in its own transaction and
// returns the agent it assigned, or "" when the task needs nobody or nobody is
// eligible.
func (s *WorkflowEngine) dispatchTask(ctx context.Context, id int64, busy map[string]bool) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	task, err := taskByID(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if task.WorkflowDigest == "" || task.Status != StatusOpen || task.WorkflowPausedReason != "" || task.Blocked {
		return "", nil
	}
	manifest, err := loadManifestTx(ctx, tx, task.WorkflowDigest)
	if err != nil {
		return "", err
	}
	status, ok := taskStatus(manifest, task)
	if !ok || status.Terminal || status.Owner.Kind != workflowfile.OwnerPool {
		return "", nil
	}
	sticky := manifest.Definition.StatusStickyHolder(status.ID)
	pick, err := pickHolderTx(ctx, tx, task, status.Owner.Pool, sticky, busy)
	if err != nil || pick.Agent == "" {
		return "", err
	}
	now := s.now()
	if err := s.recordHolderTx(ctx, tx, task, status.Owner.Pool, pick.Agent); err != nil {
		return "", err
	}
	previousAssignee, previousCategory := task.Assignee, task.Status
	state := stateIn(task, status)
	state.Holder = agentPrincipal(pick.Agent)
	if err := applyState(&task, state); err != nil {
		return "", err
	}
	if saved, err := saveTaskStateTx(ctx, tx, &task, previousCategory, now, true); err != nil || !saved {
		return "", err
	}
	if err := recordAssignmentTx(ctx, tx, task, previousAssignee, now); err != nil {
		return "", err
	}
	if err := appendDispatchedEventTx(ctx, tx, task, status.ID, status.Owner.Pool, sticky, pick, now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return pick.Agent, nil
}
