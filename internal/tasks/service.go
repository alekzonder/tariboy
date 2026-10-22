package tasks

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

var queuePrefixRE = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

func (s *TaskService) CreateTask(ctx context.Context, actor Actor, in CreateTaskInput) (Task, error) {
	if err := validateActor(actor); err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return Task{}, newError(http.StatusBadRequest, "missing_title", "task title is required")
	}
	if _, err := NormalizePriority(in.Priority); err != nil {
		return Task{}, err
	}
	if _, err := NormalizePullRequest(in.PullRequest); err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	if replayed, ok, err := readTaskIdempotency[Task](
		ctx, tx, actor.Principal, "create_task", in.IdempotencyKey,
	); err != nil {
		return Task{}, err
	} else if ok {
		return replayed, nil
	}
	task, err := s.createTaskTx(ctx, tx, actor, in)
	if err != nil {
		return Task{}, err
	}
	if err := writeTaskIdempotency(
		ctx, tx, actor.Principal, "create_task", in.IdempotencyKey, task, task.CreatedAt,
	); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	s.signal()
	return task, nil
}

// createTaskTx creates a task inside tx; a task of a bound queue enters the
// workflow's initial status. It neither reads nor writes idempotency.
func (s *TaskService) createTaskTx(ctx context.Context, tx *sql.Tx, actor Actor, in CreateTaskInput) (Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return Task{}, newError(http.StatusBadRequest, "missing_title", "task title is required")
	}
	priority, err := NormalizePriority(in.Priority)
	if err != nil {
		return Task{}, err
	}
	pullRequest, err := NormalizePullRequest(in.PullRequest)
	if err != nil {
		return Task{}, err
	}
	place, err := s.createTaskPlacementTx(ctx, tx, actor, in)
	if err != nil {
		return Task{}, err
	}
	queue, customer, group := place.queue, place.customer, place.group
	parentID, parentKey := place.parentID, place.parentKey
	binding, bound, err := s.createTaskQueueTx(ctx, tx, queue, in.Assignee)
	if err != nil {
		return Task{}, err
	}
	var workflowDigest any
	if bound {
		workflowDigest = binding.Digest
	}
	filed, crossQueueAssigned, err := createTaskReportTx(ctx, tx, actor, queue, group, parentKey, in.Assignee)
	if err != nil {
		return Task{}, err
	}
	var position int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position), -1) + 1 FROM tasks
		WHERE queue_prefix = ? AND parent_id IS ? AND priority = ?`, queue, parentID, priority).Scan(&position); err != nil {
		return Task{}, err
	}
	now := s.now()
	key, err := reserveTaskKey(ctx, tx, queue)
	if err != nil {
		return Task{}, err
	}
	assignee := normalizeAssignee(in.Assignee)
	status := StatusOpen
	res, err := tx.ExecContext(ctx, `
		INSERT INTO tasks(
			task_key, queue_prefix, parent_id, position, priority, title, description, status, pull_request,
			author, customer, group_name, assignee, created_at, updated_at, workflow_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key, queue, parentID, position, priority, in.Title, strings.TrimSpace(in.Description),
		status, pullRequest, actor.Principal, customer, group, assignee, now, now, workflowDigest)
	if err != nil {
		return Task{}, err
	}
	taskID, err := res.LastInsertId()
	if err != nil {
		return Task{}, err
	}
	task := Task{
		ID: taskID, Key: key, Queue: queue, ParentKey: parentKey, Position: position, Priority: priority,
		Title: in.Title, Description: strings.TrimSpace(in.Description), Status: status, PullRequest: pullRequest,
		Author: actor.Principal, Customer: customer, Group: group,
		Assignee: assignee, Revision: 1, CreatedAt: now, UpdatedAt: now,
		Access: "write",
	}
	if filed {
		task.Access, task.Filed = "", true
	}
	if bound {
		task.WorkflowDigest, task.WorkflowName, task.WorkflowVersion = binding.Digest, binding.Name, binding.Version
	}
	payload := map[string]any{"key": key, "parent_key": parentKey, "priority": priority, "pull_request": task.PullRequest}
	sequence, err := appendEventTx(ctx, tx, task, "task.created", actor, payload, now)
	if err != nil {
		return Task{}, err
	}
	if crossQueueAssigned {
		task.Access, err = taskAccess(ctx, tx, actor, task.ID)
		if err != nil {
			return Task{}, err
		}
	}
	if err := s.createTaskHandoffTx(ctx, tx, actor, &task, binding, bound, filed, sequence, now); err != nil {
		return Task{}, err
	}
	return task, nil
}

// createTaskPlacement is where createTaskTx puts a new task: a child takes its
// parent's queue, customer, and group.
type createTaskPlacement struct {
	queue, customer, group string
	parentID               any
	parentKey              string
}

// createTaskPlacementTx resolves the new task's parent, if any, and checks
// that the actor may write under it.
func (s *TaskService) createTaskPlacementTx(ctx context.Context, tx *sql.Tx, actor Actor, in CreateTaskInput) (createTaskPlacement, error) {
	place := createTaskPlacement{
		queue:    strings.ToUpper(strings.TrimSpace(in.Queue)),
		customer: userPrincipal(s.customer),
		group:    strings.TrimSpace(in.Group),
	}
	if strings.TrimSpace(in.ParentKey) != "" {
		parent, err := taskByKey(ctx, tx, strings.TrimSpace(in.ParentKey))
		if err != nil {
			return createTaskPlacement{}, err
		}
		if !actor.IsCustomer {
			if err := requireWrite(ctx, tx, actor, parent); err != nil {
				return createTaskPlacement{}, err
			}
		}
		place.queue, place.customer, place.group = parent.Queue, parent.Customer, parent.Group
		place.parentID, place.parentKey = parent.ID, parent.Key
	} else if place.queue == "" {
		return createTaskPlacement{}, newError(http.StatusBadRequest, "missing_queue", "queue is required for a root task")
	}
	return place, nil
}

// createTaskQueueTx checks that the queue exists and returns its workflow
// binding, following a tag binding to the version it names now.
func (s *TaskService) createTaskQueueTx(ctx context.Context, tx *sql.Tx, queue, assignee string) (QueueWorkflow, bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_queues WHERE prefix = ?`, queue).Scan(&exists); err != nil {
		return QueueWorkflow{}, false, err
	}
	if exists == 0 {
		return QueueWorkflow{}, false, notFound("queue_not_found", "queue not found")
	}
	// A task created in a bound queue pins the queue's workflow and the workflow
	// assigns it, so an explicit assignee is refused.
	binding, bound, err := queueWorkflowTx(ctx, tx, queue)
	if err != nil {
		return QueueWorkflow{}, false, err
	}
	if bound {
		if strings.TrimSpace(assignee) != "" {
			return QueueWorkflow{}, false, workflowManagedError()
		}
		// A binding made by a tag gives each new task the version the tag names now.
		if binding, err = s.engine.followQueueWorkflowTx(ctx, tx, binding); err != nil {
			return QueueWorkflow{}, false, err
		}
	}
	return binding, bound, nil
}

// createTaskReportTx reports whether a new root task is a filed report or a
// task assigned across a queue the actor does not run.
func createTaskReportTx(
	ctx context.Context,
	tx *sql.Tx,
	actor Actor,
	queue string,
	group string,
	parentKey string,
	assignee string,
) (filed, crossQueueAssigned bool, err error) {
	// An unassigned root task from an agent that does not run the queue is a filed report:
	// it is ungrouped and (through the visibility rules) invisible to its author afterwards,
	// so triage belongs to whoever runs the queue. An explicit assignee instead owns only
	// that task and its descendants; it does not gain access to the rest of the queue.
	if !actor.IsCustomer && parentKey == "" {
		runsQueue, err := agentRunsQueue(ctx, tx, actor, queue)
		if err != nil {
			return false, false, err
		}
		if !runsQueue {
			if group != "" {
				return false, false, newError(http.StatusBadRequest, "report_cannot_assign",
					"a root task created in a queue you do not run cannot carry a group")
			}
			if strings.TrimSpace(assignee) == "" {
				filed = true
			} else {
				crossQueueAssigned = true
			}
		}
	}
	return filed, crossQueueAssigned, nil
}

// createTaskHandoffTx hands a new task on: a bound task enters the workflow's
// initial status, an assigned one notifies its assignee, and an unassigned one
// notifies the queue's triager.
func (s *TaskService) createTaskHandoffTx(
	ctx context.Context,
	tx *sql.Tx,
	actor Actor,
	task *Task,
	binding QueueWorkflow,
	bound bool,
	filed bool,
	sequence int64,
	now string,
) error {
	if bound {
		// The workflow owns the task from here: entering the initial status assigns it.
		manifest, err := loadManifestTx(ctx, tx, binding.Digest)
		if err != nil {
			return err
		}
		if err := s.engine.enterStatusTx(ctx, tx, task, manifest, manifest.Definition.InitialStatus, actor.Principal, "", ""); err != nil {
			return err
		}
	} else if task.Assignee != "" && task.Assignee != actor.Principal {
		if err := enqueueNotificationTx(ctx, tx, sequence, task.Assignee, actor.Principal,
			"task.assigned", *task, "New task assigned: "+task.Key+" "+task.Title, now); err != nil {
			return err
		}
	} else if task.Assignee == "" {
		var responsible string
		if err := tx.QueryRowContext(ctx,
			`SELECT responsible_agent FROM task_queues WHERE prefix = ?`, task.Queue).Scan(&responsible); err != nil {
			return err
		}
		triager := ""
		if responsible != "" {
			triager = agentPrincipal(responsible)
		} else if filed {
			// A report nobody is on the hook for is a report nobody reads. With no responsible
			// agent on the queue, the customer is the only remaining triager.
			triager = task.Customer
		}
		if triager != "" {
			if err := enqueueNotificationTx(ctx, tx, sequence, triager, actor.Principal,
				"task.triage", *task, "New unassigned task: "+task.Key+" "+task.Title, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func readTaskIdempotency[T any](
	ctx context.Context,
	tx *sql.Tx,
	actor string,
	action string,
	key string,
) (T, bool, error) {
	var zero T
	key = strings.TrimSpace(key)
	if key == "" {
		return zero, false, nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, `
		SELECT response FROM task_idempotency
		WHERE actor = ? AND action = ? AND idempotency_key = ?`,
		actor, action, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return zero, false, err
	}
	return value, true, nil
}

func writeTaskIdempotency(
	ctx context.Context,
	tx *sql.Tx,
	actor string,
	action string,
	key string,
	value any,
	now string,
) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO task_idempotency(actor, action, idempotency_key, response, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		actor, action, key, string(raw), now)
	return err
}

func (s *TaskService) GetTask(ctx context.Context, actor Actor, key string) (TaskDetail, error) {
	if err := validateActor(actor); err != nil {
		return TaskDetail{}, err
	}
	task, err := taskByKey(ctx, s.db, strings.TrimSpace(key))
	if err != nil {
		return TaskDetail{}, err
	}
	access, holderOnly, err := holderReadAccess(ctx, s.db, actor, task.ID)
	if err != nil {
		return TaskDetail{}, err
	}
	if access == "" {
		return TaskDetail{}, taskNotFound(key)
	}
	task.Access = access
	comments, err := listComments(ctx, s.db, task)
	if err != nil {
		return TaskDetail{}, err
	}
	waits, err := listOpenWaits(ctx, s.db, task)
	if err != nil {
		return TaskDetail{}, err
	}
	relations, err := listRelations(ctx, s.db, actor, task)
	if err != nil {
		return TaskDetail{}, err
	}
	total, active, err := descendantCounts(ctx, s.db, task.ID)
	if err != nil {
		return TaskDetail{}, err
	}
	return TaskDetail{
		Task: task, Comments: comments,
		Descendants: total, ActiveDescendants: active,
		WaitingFor: waits, Relations: relations, HolderReadOnly: holderOnly,
	}, nil
}

// listOrders maps each ListFilter.Sort to its ORDER BY and to the keyset
// condition that keeps a page after the cursor task c in that order.
var listOrders = map[string]struct{ by, after string }{
	"tree": {
		by: `t.queue_prefix, t.parent_id, t.priority, t.position, t.task_key`,
		after: `(t.queue_prefix, COALESCE(t.parent_id, 0), t.priority, t.position, t.task_key) >
			(c.queue_prefix, COALESCE(c.parent_id, 0), c.priority, c.position, c.task_key)`,
	},
	"updated": {by: `t.updated_at DESC, t.task_key DESC`, after: `(t.updated_at, t.task_key) < (c.updated_at, c.task_key)`},
	"created": {by: `t.created_at DESC, t.task_key DESC`, after: `(t.created_at, t.task_key) < (c.created_at, c.task_key)`},
	"priority": {
		by:    `t.priority, t.updated_at DESC, t.task_key DESC`,
		after: `(t.priority > c.priority OR (t.priority = c.priority AND (t.updated_at, t.task_key) < (c.updated_at, c.task_key)))`,
	},
}

func (s *TaskService) ListTasks(ctx context.Context, actor Actor, filter ListFilter) (TaskPage, error) {
	if err := validateActor(actor); err != nil {
		return TaskPage{}, err
	}
	plan, err := listFilterPlan(actor, filter)
	if err != nil {
		return TaskPage{}, err
	}
	cte, cteArgs := visibleCTE(plan.visibility)
	q := listTasksQuery{sql: cte + visibleTaskSelect + ` WHERE 1=1`, args: cteArgs}
	listFilterClauses(&q, filter, plan)
	query, args := q.sql, q.args
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query += ` ORDER BY ` + plan.order.by
	query += ` LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.reader().QueryContext(ctx, query, args...)
	if err != nil {
		return TaskPage{}, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		task, err := scanVisibleTask(rows)
		if err != nil {
			return TaskPage{}, err
		}
		out = append(out, task)
	}
	if err := rows.Err(); err != nil {
		return TaskPage{}, err
	}
	nextCursor := ""
	if len(out) > limit {
		nextCursor = out[limit-1].Key
		out = out[:limit]
	}
	var sequence int64
	if err := s.reader().QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) FROM task_events`).Scan(&sequence); err != nil {
		return TaskPage{}, err
	}
	return TaskPage{Tasks: out, NextCursor: nextCursor, Sequence: sequence}, nil
}

// listTasksPlan is a validated ListFilter: the statuses and status view it
// selects, its sort order, and the actor whose visibility it lists.
type listTasksPlan struct {
	statuses   []string
	statusView string
	order      struct{ by, after string }
	visibility Actor
}

// listFilterPlan validates filter for ListTasks and resolves its defaults.
func listFilterPlan(actor Actor, filter ListFilter) (listTasksPlan, error) {
	statuses := filter.Statuses
	if filter.Status != "" {
		statuses = append(slices.Clone(statuses), filter.Status)
	}
	statusView := filter.StatusView
	if statusView == "" && len(statuses) == 0 {
		statusView = "active"
	}
	if statusView != "" && statusView != "active" && statusView != "closed" && statusView != "all" {
		return listTasksPlan{}, newError(http.StatusBadRequest, "invalid_status_view", "status_view must be active, closed, or all")
	}
	for _, category := range filter.Categories {
		switch category {
		case StatusOpen, StatusInProgress, StatusWaitCustomer, StatusDone, StatusCancelled:
		default:
			return listTasksPlan{}, newError(http.StatusBadRequest, "invalid_category", "category must be open, in_progress, wait_customer, done, or cancelled")
		}
	}
	for _, priority := range filter.Priorities {
		if _, err := NormalizePriority(priority); err != nil || priority == "" {
			return listTasksPlan{}, newError(http.StatusBadRequest, "invalid_priority", "priority must be P0, P1, P2, or P3")
		}
	}
	if filter.ParentKey != "" && filter.SubtreeKey != "" {
		return listTasksPlan{}, newError(http.StatusBadRequest, "invalid_filter", "parent and subtree cannot be combined")
	}
	sortName := filter.Sort
	if sortName == "" {
		sortName = "tree"
	}
	order, ok := listOrders[sortName]
	if !ok {
		return listTasksPlan{}, newError(http.StatusBadRequest, "invalid_sort", "sort must be tree, updated, created, or priority")
	}
	visibilityActor := actor
	if filter.ScopeAgent != "" {
		if !actor.IsCustomer &&
			actor.Principal != agentPrincipal(filter.ScopeAgent) {
			return listTasksPlan{}, newError(http.StatusForbidden, "forbidden", "agent scope cannot be expanded")
		}
		visibilityActor = AgentActor(filter.ScopeAgent)
	}
	return listTasksPlan{statuses: statuses, statusView: statusView, order: order, visibility: visibilityActor}, nil
}

// listTasksQuery accumulates the SQL text and arguments of a ListTasks query.
type listTasksQuery struct {
	sql  string
	args []any
}

// listFilterClauses appends the WHERE conditions of filter to q, ending with
// the keyset condition that resumes after the cursor task.
func listFilterClauses(q *listTasksQuery, filter ListFilter, plan listTasksPlan) {
	if filter.Queue != "" {
		q.sql += ` AND t.queue_prefix = ?`
		q.args = append(q.args, strings.ToUpper(filter.Queue))
	}
	listFilterStatusClauses(q, filter, plan)
	if filter.Customer != "" {
		customer := strings.TrimSpace(filter.Customer)
		if !strings.Contains(customer, ":") {
			customer = agentPrincipal(customer)
		}
		q.sql += ` AND t.customer = ?`
		q.args = append(q.args, customer)
	}
	if filter.ParentKey != "" {
		q.sql += ` AND t.parent_id = (SELECT id FROM tasks WHERE task_key = ?)`
		q.args = append(q.args, strings.TrimSpace(filter.ParentKey))
	}
	if filter.SubtreeKey != "" {
		q.sql += ` AND t.id IN (
			WITH RECURSIVE subtree(id) AS (
				SELECT child.id FROM tasks child
				JOIN tasks root ON root.id = child.parent_id WHERE root.task_key = ?
				UNION ALL
				SELECT child.id FROM tasks child JOIN subtree ON child.parent_id = subtree.id
			)
			SELECT id FROM subtree
		)`
		q.args = append(q.args, strings.TrimSpace(filter.SubtreeKey))
	}
	if filter.Assignee != "" {
		q.sql += ` AND t.assignee = ?`
		q.args = append(q.args, normalizeAssignee(filter.Assignee))
	}
	if filter.Author != "" {
		author := strings.TrimSpace(filter.Author)
		if !strings.Contains(author, ":") {
			author = agentPrincipal(author)
		}
		q.sql += ` AND t.author = ?`
		q.args = append(q.args, author)
	}
	if filter.Group != "" {
		q.sql += ` AND t.group_name = ?`
		q.args = append(q.args, filter.Group)
	}
	if filter.Text != "" {
		q.sql += ` AND (unicode_lower(t.title) LIKE ? OR unicode_lower(t.description) LIKE ? OR unicode_lower(t.task_key) LIKE ?)`
		pattern := "%" + strings.ToLower(strings.TrimSpace(filter.Text)) + "%"
		q.args = append(q.args, pattern, pattern, pattern)
	}
	if filter.WaitingFor != "" {
		q.sql += ` AND EXISTS (
			SELECT 1 FROM task_waiting_for wf
			WHERE wf.task_id = t.id AND wf.expected_principal = ? AND wf.resolved_at = ''
		)`
		q.args = append(q.args, filter.WaitingFor)
	}
	if filter.Blocked != nil {
		if *filter.Blocked {
			q.sql += ` AND (t.manual_block_reason <> '' OR t.open_blockers > 0)`
		} else {
			q.sql += ` AND t.manual_block_reason = '' AND t.open_blockers = 0`
		}
	}
	if filter.AfterKey != "" {
		q.sql += ` AND EXISTS (SELECT 1 FROM tasks c WHERE c.task_key = ? AND ` + plan.order.after + `)`
		q.args = append(q.args, strings.TrimSpace(filter.AfterKey))
	}
}

// listFilterStatusClauses appends the status, category, priority, and status
// view conditions to q.
func listFilterStatusClauses(q *listTasksQuery, filter ListFilter, plan listTasksPlan) {
	if len(plan.statuses) > 0 {
		marks := placeholders(len(plan.statuses))
		q.sql += ` AND (t.status IN (` + marks + `) OR t.workflow_status IN (` + marks + `))`
		for range 2 {
			for _, status := range plan.statuses {
				q.args = append(q.args, status)
			}
		}
	}
	if len(filter.Categories) > 0 {
		q.sql += ` AND t.status IN (` + placeholders(len(filter.Categories)) + `)`
		for _, category := range filter.Categories {
			q.args = append(q.args, category)
		}
	}
	if len(filter.Priorities) > 0 {
		q.sql += ` AND t.priority IN (` + placeholders(len(filter.Priorities)) + `)`
		for _, priority := range filter.Priorities {
			q.args = append(q.args, string(priority))
		}
	}
	switch plan.statusView {
	case "active":
		q.sql += ` AND t.status IN ('open', 'in_progress', 'wait_customer')`
	case "closed":
		q.sql += ` AND t.status IN ('done', 'cancelled')`
	}
}
