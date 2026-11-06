package tasks

// Task budgets: a customer's USD limit on the AI spending of a task and its
// subtree. The AI proxy refuses a request of a task under an exhausted limit
// and calls HandleBudgetExhausted, which pauses a workflow task or blocks a
// flexible one.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
)

// PauseBudgetExhausted is the pause reason of a workflow task whose budget,
// or the budget of an ancestor, is spent.
const PauseBudgetExhausted = "budget_exhausted"

// budgetActor authors the comment that tells the customer a flexible task
// was blocked by its budget.
const budgetActor = "system:budget"

// BudgetBlock is the task whose limit refuses a request: its key, its limit
// and what its subtree spent.
type BudgetBlock struct {
	Key       string
	BudgetUSD float64
	SpentUSD  float64
}

// TaskBudget is the limit of a task and what the task and its subtree spent.
type TaskBudget struct {
	Key       string  `json:"key"`
	BudgetUSD float64 `json:"budget_usd" jsonschema:"description=The limit in USD; 0 means no limit"`
	SpentUSD  float64 `json:"spent_usd" jsonschema:"description=The AI spending of the task and its subtree in USD"`
	Exhausted bool    `json:"exhausted" jsonschema:"description=True when the limit is set and the spending reached it"`
}

// budgetBlockPrefix starts the block reason that names key; a raised limit of
// key lifts exactly the blocks that start with it.
func budgetBlockPrefix(key string) string { return "budget exhausted: task " + key + " spent " }

func (b BudgetBlock) text() string {
	return fmt.Sprintf("%s$%.2f of $%.2f", budgetBlockPrefix(b.Key), b.SpentUSD, b.BudgetUSD)
}

func isBudgetBlock(reason string) bool { return strings.HasPrefix(reason, "budget exhausted: task ") }

func validBudget(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return newError(http.StatusBadRequest, "invalid_budget", "a task budget is a finite USD amount of 0 or more")
	}
	return nil
}

// subtreeSpend sums the cost of every AI request of the task, its
// descendants, and their old keys.
func subtreeSpend(ctx context.Context, q queryer, taskID int64) (float64, error) {
	var spent float64
	err := q.QueryRowContext(ctx, `
		WITH RECURSIVE sub(id) AS (
			SELECT ? UNION ALL SELECT t.id FROM tasks t JOIN sub ON t.parent_id = sub.id)
		SELECT COALESCE(SUM(r.cost_usd), 0) FROM ai_requests r WHERE r.task_id IN (
			SELECT t.task_key FROM tasks t JOIN sub ON sub.id = t.id
			UNION SELECT a.old_key FROM task_key_aliases a JOIN sub ON sub.id = a.task_id)`, taskID).Scan(&spent)
	return math.Round(spent*1e6) / 1e6, err
}

// exhaustedBudget returns the nearest task, from the task of key up to its
// root, whose limit its subtree spent.
func exhaustedBudget(ctx context.Context, q queryer, key string) (BudgetBlock, bool, error) {
	key = NormalizeKey(key)
	rows, err := q.QueryContext(ctx, `
		WITH RECURSIVE up(id, depth) AS (
			SELECT id, 0 FROM tasks
			WHERE task_key = ? OR id = (SELECT task_id FROM task_key_aliases WHERE old_key = ?)
			UNION ALL SELECT t.parent_id, up.depth + 1 FROM tasks t JOIN up ON t.id = up.id
			WHERE t.parent_id IS NOT NULL)
		SELECT t.id, t.task_key, t.budget_usd FROM up JOIN tasks t ON t.id = up.id
		WHERE t.budget_usd > 0 ORDER BY up.depth`, key, key)
	if err != nil {
		return BudgetBlock{}, false, err
	}
	type limited struct {
		id    int64
		block BudgetBlock
	}
	var limits []limited
	for rows.Next() {
		var l limited
		if err := rows.Scan(&l.id, &l.block.Key, &l.block.BudgetUSD); err != nil {
			rows.Close()
			return BudgetBlock{}, false, err
		}
		limits = append(limits, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return BudgetBlock{}, false, err
	}
	for _, l := range limits {
		if l.block.SpentUSD, err = subtreeSpend(ctx, q, l.id); err != nil {
			return BudgetBlock{}, false, err
		}
		if l.block.SpentUSD >= l.block.BudgetUSD {
			return l.block, true, nil
		}
	}
	return BudgetBlock{}, false, nil
}

// ExhaustedBudget is the proxy's check for a request of the task key.
func (s *core) ExhaustedBudget(ctx context.Context, key string) (BudgetBlock, bool, error) {
	return exhaustedBudget(ctx, s.reader(), key)
}

// TaskBudget returns the limit and the spending of a task the actor can read.
func (s *TaskService) TaskBudget(ctx context.Context, actor Actor, key string) (TaskBudget, error) {
	if err := validateActor(actor); err != nil {
		return TaskBudget{}, err
	}
	q := s.reader()
	task, err := taskByKey(ctx, q, strings.TrimSpace(key))
	if err != nil {
		return TaskBudget{}, err
	}
	if access, err := taskAccess(ctx, q, actor, task.ID); err != nil {
		return TaskBudget{}, err
	} else if access == "" {
		return TaskBudget{}, taskNotFound(key)
	}
	spent, err := subtreeSpend(ctx, q, task.ID)
	if err != nil {
		return TaskBudget{}, err
	}
	return TaskBudget{Key: task.Key, BudgetUSD: task.BudgetUSD, SpentUSD: spent,
		Exhausted: task.BudgetUSD > 0 && spent >= task.BudgetUSD}, nil
}

// guardBudgetInput refuses a budget change by anyone but the customer, an
// invalid limit, and an agent lifting a budget block.
func guardBudgetInput(actor Actor, task Task, in UpdateTaskInput) error {
	if in.BudgetUSD != nil {
		if !actor.IsCustomer {
			return newError(http.StatusForbidden, "forbidden", "only the customer can change a task budget")
		}
		if err := validBudget(*in.BudgetUSD); err != nil {
			return err
		}
	}
	if in.ManualBlockReason != nil && !actor.IsCustomer && isBudgetBlock(task.ManualBlockReason) &&
		strings.TrimSpace(*in.ManualBlockReason) != task.ManualBlockReason {
		return newError(http.StatusForbidden, "forbidden", "only the customer can lift a budget block")
	}
	return nil
}

// liftBudgetBlocksTx runs after the customer changed the limit of task: when
// the limit is gone or above the spending, it lifts the blocks that name task
// in its subtree. The caller saves task itself; a block with other text stays.
// shortcut: a block written under a key the task had before a requeue is not
// lifted by the new key; the customer clears it with --manual-block-reason "".
func liftBudgetBlocksTx(ctx context.Context, tx *sql.Tx, task *Task, actor Actor, now string) error {
	if task.BudgetUSD > 0 {
		spent, err := subtreeSpend(ctx, tx, task.ID)
		if err != nil || spent >= task.BudgetUSD {
			return err
		}
	}
	prefix := budgetBlockPrefix(task.Key)
	if strings.HasPrefix(task.ManualBlockReason, prefix) {
		task.ManualBlockReason = ""
	}
	rows, err := tx.QueryContext(ctx, `
		WITH RECURSIVE sub(id) AS (
			SELECT id FROM tasks WHERE parent_id = ? UNION ALL SELECT t.id FROM tasks t JOIN sub ON t.parent_id = sub.id)
		SELECT t.id FROM tasks t JOIN sub ON sub.id = t.id WHERE substr(t.manual_block_reason, 1, ?) = ?`,
		task.ID, len(prefix), prefix)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks SET manual_block_reason = '', revision = revision + 1, updated_at = ? WHERE id = ?`, now, id); err != nil {
			return err
		}
		lifted, err := taskByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := appendEventTx(ctx, tx, lifted, "task.updated", actor,
			map[string]any{"status": lifted.Status, "manual_block_reason": ""}, now); err != nil {
			return err
		}
	}
	return nil
}

// HandleBudgetExhausted reacts to a request the proxy refused for the task
// key: a workflow task is paused, a flexible one is blocked and its customer
// told. A task already paused, blocked, closed, or no longer over a limit is
// left as it is.
func (s *WorkflowEngine) HandleBudgetExhausted(ctx context.Context, key string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	task, err := taskByKey(ctx, tx, key)
	var taskErr *Error
	if errors.As(err, &taskErr) && taskErr.Status == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if task.Status == StatusDone || task.Status == StatusCancelled || task.WorkflowPausedReason != "" ||
		(task.WorkflowDigest == "" && task.ManualBlockReason != "") {
		return nil
	}
	block, over, err := exhaustedBudget(ctx, tx, task.Key)
	if err != nil || !over {
		return err
	}
	now := s.now()
	if task.WorkflowDigest != "" {
		manifest, err := loadManifestTx(ctx, tx, task.WorkflowDigest)
		if err != nil {
			return err
		}
		if err := s.pauseTx(ctx, tx, &task, manifest, PauseBudgetExhausted, block.text()+"."); err != nil {
			return err
		}
	} else if err := blockForBudgetTx(ctx, tx, &task, block, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.signal()
	return nil
}

// blockForBudgetTx blocks a flexible task and tells its customer in a comment.
func blockForBudgetTx(ctx context.Context, tx *sql.Tx, task *Task, block BudgetBlock, now string) error {
	task.ManualBlockReason = block.text()
	task.Revision++
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks SET manual_block_reason = ?, revision = ?, updated_at = ? WHERE id = ?`,
		task.ManualBlockReason, task.Revision, now, task.ID); err != nil {
		return err
	}
	actor := Actor{Principal: budgetActor}
	if _, err := appendEventTx(ctx, tx, *task, "task.updated", actor,
		map[string]any{"status": task.Status, "manual_block_reason": task.ManualBlockReason}, now); err != nil {
		return err
	}
	body := fmt.Sprintf("@%s The AI proxy refuses requests of this task: %s.\n\n"+
		"The task is blocked. Raise or remove the limit with `ttasks update %s --budget-usd AMOUNT`; "+
		"the block is lifted when the limit is above the spending.", task.Customer, task.ManualBlockReason, block.Key)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_comments(task_id, author, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, task.ID, budgetActor, body, now, now)
	if err != nil {
		return err
	}
	commentID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	sequence, err := appendEventTx(ctx, tx, *task, "task.comment_added", actor,
		map[string]any{"comment_id": commentID, "mentions": []string{task.Customer}, "resolved": []string{}}, now)
	if err != nil {
		return err
	}
	return enqueueNotificationTx(ctx, tx, sequence, task.Customer, budgetActor,
		"task.budget_exhausted", *task, body, now)
}
