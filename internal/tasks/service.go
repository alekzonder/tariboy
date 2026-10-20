package tasks

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

var queuePrefixRE = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

type Service struct {
	db                                       *sql.DB
	read                                     *sql.DB
	customer                                 string
	clock                                    func() time.Time
	hub                                      *Hub
	goalSignal                               func()
	workflowResolver                         WorkflowResolver
	runBaseDir                               string
	globalEnv                                func() ([]string, error)
	log                                      *slog.Logger
	holderFailuresMu                         sync.Mutex
	holderFailures                           map[int64]string // task id -> logged manifest failure of CheckHolders
	workflowIngressEnabled                   atomic.Bool
	workflowIngressAfterTargetCount          func()
	workflowActivationAfterWriterReservation func()
}

func NewService(db *sql.DB, customer string, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	s := &Service{db: db, customer: strings.TrimPrefix(strings.TrimSpace(customer), "user:"), clock: clock}
	s.refreshWorkflowIngressEnabled(context.Background())
	return s
}

// SetReader makes ListTasks, which the UI polls, read through a read-only
// connection pool instead of the writer connection.
func (s *Service) SetReader(db *sql.DB) { s.read = db }

func (s *Service) reader() *sql.DB {
	if s.read != nil {
		return s.read
	}
	return s.db
}

func (s *Service) WorkflowIngressEnabled() bool { return s.workflowIngressEnabled.Load() }

func (s *Service) refreshWorkflowIngressEnabled(ctx context.Context) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_queue_workflow_triggers WHERE enabled=1`).Scan(&count); err == nil {
		s.workflowIngressEnabled.Store(count > 0)
	}
}

func (s *Service) CustomerLogin() string { return s.customer }

func (s *Service) SetHub(hub *Hub) { s.hub = hub }

// SetRunBaseDir sets the daemon base directory under which workflow script runs
// keep their files, so a run's log is read only from where the worker wrote it.
func (s *Service) SetRunBaseDir(dir string) { s.runBaseDir = dir }

func (s *Service) SetGoalSignal(signal func()) { s.goalSignal = signal }

// SetLogger sets where the service logs what it skips instead of failing a
// whole call, such as a pending script run whose job cannot be built.
func (s *Service) SetLogger(log *slog.Logger) { s.log = log }

func (s *Service) logger() *slog.Logger {
	if s.log == nil {
		return slog.Default()
	}
	return s.log
}

func (s *Service) signal() {
	if s.hub != nil {
		s.hub.Nudge()
	}
	if s.goalSignal != nil {
		s.goalSignal()
	}
}

func (s *Service) now() string {
	return store.FormatTime(s.clock())
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

func (s *Service) CreateQueue(ctx context.Context, actor Actor, in CreateQueueInput) (Queue, error) {
	if err := validateActor(actor); err != nil {
		return Queue{}, err
	}
	if !actor.IsCustomer || actor.Principal != userPrincipal(s.customer) {
		return Queue{}, domainError(http.StatusForbidden, "forbidden", "only the daemon customer can administer queues")
	}
	in.Prefix = strings.ToUpper(strings.TrimSpace(in.Prefix))
	in.Name = strings.TrimSpace(in.Name)
	if !queuePrefixRE.MatchString(in.Prefix) {
		return Queue{}, domainError(http.StatusBadRequest, "invalid_queue_prefix",
			"queue prefix must match ^[A-Z][A-Z0-9]{1,9}$")
	}
	if in.Name == "" {
		return Queue{}, domainError(http.StatusBadRequest, "missing_name", "queue name is required")
	}
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Queue{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_queues(prefix, name, description, responsible_agent, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		in.Prefix, in.Name, strings.TrimSpace(in.Description),
		strings.TrimPrefix(normalizeAssignee(in.ResponsibleAgent), "agent:"), now, now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Queue{}, domainError(http.StatusConflict, "queue_exists", "queue prefix already exists")
		}
		return Queue{}, err
	}
	owners := make([]string, 0, len(in.Owners))
	seen := map[string]bool{}
	for _, owner := range in.Owners {
		owner = strings.TrimPrefix(normalizeAssignee(owner), "agent:")
		if owner == "" || seen[owner] {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_queue_owners(queue_prefix, agent) VALUES (?, ?)`,
			in.Prefix, owner); err != nil {
			return Queue{}, err
		}
		seen[owner] = true
		owners = append(owners, owner)
	}
	queue := Queue{
		Prefix: in.Prefix, Name: in.Name, Description: strings.TrimSpace(in.Description),
		Owners: owners, ResponsibleAgent: strings.TrimPrefix(normalizeAssignee(in.ResponsibleAgent), "agent:"),
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := appendQueueEventTx(ctx, tx, queue, "task.queue_created", actor,
		map[string]any{"name": queue.Name}, now); err != nil {
		return Queue{}, err
	}
	if err := tx.Commit(); err != nil {
		return Queue{}, err
	}
	s.signal()
	return queue, nil
}

// EnsureDefaultQueue creates the TASK queue when it is absent, so every daemon
// starts with one queue. An existing TASK queue is left untouched.
func (s *Service) EnsureDefaultQueue(ctx context.Context) error {
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queue := Queue{Prefix: "TASK", Name: "Tasks", Owners: []string{}, Revision: 1, CreatedAt: now, UpdatedAt: now}
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO task_queues(prefix, name, description, responsible_agent, created_at, updated_at)
		VALUES (?, ?, '', '', ?, ?)`, queue.Prefix, queue.Name, now, now)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return err
	}
	if _, err := appendQueueEventTx(ctx, tx, queue, "task.queue_created",
		Actor{Principal: "system:tariboyd"}, map[string]any{"name": queue.Name}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.signal()
	return nil
}

func (s *Service) CreateTask(ctx context.Context, actor Actor, in CreateTaskInput) (Task, error) {
	if err := validateActor(actor); err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return Task{}, domainError(http.StatusBadRequest, "missing_title", "task title is required")
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
func (s *Service) createTaskTx(ctx context.Context, tx *sql.Tx, actor Actor, in CreateTaskInput) (Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return Task{}, domainError(http.StatusBadRequest, "missing_title", "task title is required")
	}
	priority, err := NormalizePriority(in.Priority)
	if err != nil {
		return Task{}, err
	}
	pullRequest, err := NormalizePullRequest(in.PullRequest)
	if err != nil {
		return Task{}, err
	}
	queue := strings.ToUpper(strings.TrimSpace(in.Queue))
	customer := userPrincipal(s.customer)
	group := strings.TrimSpace(in.Group)
	var parentID any
	var parentKey string
	if strings.TrimSpace(in.ParentKey) != "" {
		parent, err := taskByKey(tx, strings.TrimSpace(in.ParentKey))
		if err != nil {
			return Task{}, err
		}
		if !actor.IsCustomer {
			if err := requireWrite(ctx, tx, actor, parent); err != nil {
				return Task{}, err
			}
		}
		queue, customer, group = parent.Queue, parent.Customer, parent.Group
		parentID, parentKey = parent.ID, parent.Key
	} else if queue == "" {
		return Task{}, domainError(http.StatusBadRequest, "missing_queue", "queue is required for a root task")
	}
	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_queues WHERE prefix = ?`, queue).Scan(&exists); err != nil {
		return Task{}, err
	}
	if exists == 0 {
		return Task{}, domainError(http.StatusNotFound, "queue_not_found", "queue not found")
	}
	// A task created in a bound queue pins the queue's workflow and the workflow
	// assigns it, so an explicit assignee is refused.
	binding, bound, err := queueWorkflowTx(ctx, tx, queue)
	if err != nil {
		return Task{}, err
	}
	var workflowDigest any
	if bound {
		if strings.TrimSpace(in.Assignee) != "" {
			return Task{}, workflowManagedError()
		}
		// A binding made by a tag gives each new task the version the tag names now.
		if binding, err = s.followQueueWorkflowTx(ctx, tx, binding); err != nil {
			return Task{}, err
		}
		workflowDigest = binding.Digest
	}
	// An unassigned root task from an agent that does not run the queue is a filed report:
	// it is ungrouped and (through the visibility rules) invisible to its author afterwards,
	// so triage belongs to whoever runs the queue. An explicit assignee instead owns only
	// that task and its descendants; it does not gain access to the rest of the queue.
	filed := false
	crossQueueAssigned := false
	if !actor.IsCustomer && parentKey == "" {
		runsQueue, err := agentRunsQueue(ctx, tx, actor, queue)
		if err != nil {
			return Task{}, err
		}
		if !runsQueue {
			if group != "" {
				return Task{}, domainError(http.StatusBadRequest, "report_cannot_assign",
					"a root task created in a queue you do not run cannot carry a group")
			}
			if strings.TrimSpace(in.Assignee) == "" {
				filed = true
			} else {
				crossQueueAssigned = true
			}
		}
	}
	var position int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position), -1) + 1 FROM tasks
		WHERE queue_prefix = ? AND parent_id IS ? AND priority = ?`, queue, parentID, priority).Scan(&position); err != nil {
		return Task{}, err
	}
	now := s.now()
	key, err := reserveTaskKey(tx, queue)
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
	if bound {
		// The workflow owns the task from here: entering the initial status assigns it.
		manifest, err := loadManifestTx(ctx, tx, binding.Digest)
		if err != nil {
			return Task{}, err
		}
		if err := s.enterStatusTx(ctx, tx, &task, manifest, manifest.Definition.InitialStatus, actor.Principal, "", ""); err != nil {
			return Task{}, err
		}
	} else if task.Assignee != "" && task.Assignee != actor.Principal {
		if err := enqueueNotificationTx(ctx, tx, sequence, task.Assignee, actor.Principal,
			"task.assigned", task, "New task assigned: "+task.Key+" "+task.Title, now); err != nil {
			return Task{}, err
		}
	} else if task.Assignee == "" {
		var responsible string
		if err := tx.QueryRowContext(ctx,
			`SELECT responsible_agent FROM task_queues WHERE prefix = ?`, queue).Scan(&responsible); err != nil {
			return Task{}, err
		}
		triager := ""
		if responsible != "" {
			triager = agentPrincipal(responsible)
		} else if filed {
			// A report nobody is on the hook for is a report nobody reads. With no responsible
			// agent on the queue, the customer is the only remaining triager.
			triager = customer
		}
		if triager != "" {
			if err := enqueueNotificationTx(ctx, tx, sequence, triager, actor.Principal,
				"task.triage", task, "New unassigned task: "+task.Key+" "+task.Title, now); err != nil {
				return Task{}, err
			}
		}
	}
	return task, nil
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
	if err == sql.ErrNoRows {
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

func (s *Service) GetTask(ctx context.Context, actor Actor, key string) (TaskDetail, error) {
	if err := validateActor(actor); err != nil {
		return TaskDetail{}, err
	}
	task, err := taskByKey(s.db, strings.TrimSpace(key))
	if err != nil {
		return TaskDetail{}, err
	}
	access, holderOnly, err := holderReadAccess(ctx, s.db, actor, task.ID)
	if err != nil {
		return TaskDetail{}, err
	}
	if access == "" {
		return TaskDetail{}, notFound(key)
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

func (s *Service) ListTasks(ctx context.Context, actor Actor, filter ListFilter) (TaskPage, error) {
	if err := validateActor(actor); err != nil {
		return TaskPage{}, err
	}
	statuses := filter.Statuses
	if filter.Status != "" {
		statuses = append(slices.Clone(statuses), filter.Status)
	}
	statusView := filter.StatusView
	if statusView == "" && len(statuses) == 0 {
		statusView = "active"
	}
	if statusView != "" && statusView != "active" && statusView != "closed" && statusView != "all" {
		return TaskPage{}, domainError(http.StatusBadRequest, "invalid_status_view", "status_view must be active, closed, or all")
	}
	for _, category := range filter.Categories {
		switch category {
		case StatusOpen, StatusInProgress, StatusWaitCustomer, StatusDone, StatusCancelled:
		default:
			return TaskPage{}, domainError(http.StatusBadRequest, "invalid_category", "category must be open, in_progress, wait_customer, done, or cancelled")
		}
	}
	for _, priority := range filter.Priorities {
		if _, err := NormalizePriority(priority); err != nil || priority == "" {
			return TaskPage{}, domainError(http.StatusBadRequest, "invalid_priority", "priority must be P0, P1, P2, or P3")
		}
	}
	if filter.ParentKey != "" && filter.SubtreeKey != "" {
		return TaskPage{}, domainError(http.StatusBadRequest, "invalid_filter", "parent and subtree cannot be combined")
	}
	sortName := filter.Sort
	if sortName == "" {
		sortName = "tree"
	}
	order, ok := listOrders[sortName]
	if !ok {
		return TaskPage{}, domainError(http.StatusBadRequest, "invalid_sort", "sort must be tree, updated, created, or priority")
	}
	visibilityActor := actor
	if filter.ScopeAgent != "" {
		if !actor.IsCustomer &&
			actor.Principal != agentPrincipal(filter.ScopeAgent) {
			return TaskPage{}, domainError(http.StatusForbidden, "forbidden", "agent scope cannot be expanded")
		}
		visibilityActor = AgentActor(filter.ScopeAgent)
	}
	cte, args := visibleCTE(visibilityActor)
	query := cte + visibleTaskSelect + ` WHERE 1=1`
	if filter.Queue != "" {
		query += ` AND t.queue_prefix = ?`
		args = append(args, strings.ToUpper(filter.Queue))
	}
	if len(statuses) > 0 {
		marks := placeholders(len(statuses))
		query += ` AND (t.status IN (` + marks + `) OR t.workflow_status IN (` + marks + `))`
		for range 2 {
			for _, status := range statuses {
				args = append(args, status)
			}
		}
	}
	if len(filter.Categories) > 0 {
		query += ` AND t.status IN (` + placeholders(len(filter.Categories)) + `)`
		for _, category := range filter.Categories {
			args = append(args, category)
		}
	}
	if len(filter.Priorities) > 0 {
		query += ` AND t.priority IN (` + placeholders(len(filter.Priorities)) + `)`
		for _, priority := range filter.Priorities {
			args = append(args, string(priority))
		}
	}
	switch statusView {
	case "active":
		query += ` AND t.status IN ('open', 'in_progress', 'wait_customer')`
	case "closed":
		query += ` AND t.status IN ('done', 'cancelled')`
	}
	if filter.Customer != "" {
		customer := strings.TrimSpace(filter.Customer)
		if !strings.Contains(customer, ":") {
			customer = agentPrincipal(customer)
		}
		query += ` AND t.customer = ?`
		args = append(args, customer)
	}
	if filter.ParentKey != "" {
		query += ` AND t.parent_id = (SELECT id FROM tasks WHERE task_key = ?)`
		args = append(args, strings.TrimSpace(filter.ParentKey))
	}
	if filter.SubtreeKey != "" {
		query += ` AND t.id IN (
			WITH RECURSIVE subtree(id) AS (
				SELECT child.id FROM tasks child
				JOIN tasks root ON root.id = child.parent_id WHERE root.task_key = ?
				UNION ALL
				SELECT child.id FROM tasks child JOIN subtree ON child.parent_id = subtree.id
			)
			SELECT id FROM subtree
		)`
		args = append(args, strings.TrimSpace(filter.SubtreeKey))
	}
	if filter.Assignee != "" {
		query += ` AND t.assignee = ?`
		args = append(args, normalizeAssignee(filter.Assignee))
	}
	if filter.Author != "" {
		author := strings.TrimSpace(filter.Author)
		if !strings.Contains(author, ":") {
			author = agentPrincipal(author)
		}
		query += ` AND t.author = ?`
		args = append(args, author)
	}
	if filter.Group != "" {
		query += ` AND t.group_name = ?`
		args = append(args, filter.Group)
	}
	if filter.Text != "" {
		query += ` AND (unicode_lower(t.title) LIKE ? OR unicode_lower(t.description) LIKE ? OR unicode_lower(t.task_key) LIKE ?)`
		pattern := "%" + strings.ToLower(strings.TrimSpace(filter.Text)) + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if filter.WaitingFor != "" {
		query += ` AND EXISTS (
			SELECT 1 FROM task_waiting_for wf
			WHERE wf.task_id = t.id AND wf.expected_principal = ? AND wf.resolved_at = ''
		)`
		args = append(args, filter.WaitingFor)
	}
	if filter.Blocked != nil {
		if *filter.Blocked {
			query += ` AND (t.manual_block_reason <> '' OR EXISTS (
				SELECT 1 FROM task_relations r
				JOIN tasks blocker ON blocker.id = r.source_id
				WHERE r.target_id = t.id AND r.type = 'blocks'
				  AND blocker.status NOT IN ('done', 'cancelled')
			))`
		} else {
			query += ` AND t.manual_block_reason = '' AND NOT EXISTS (
				SELECT 1 FROM task_relations r
				JOIN tasks blocker ON blocker.id = r.source_id
				WHERE r.target_id = t.id AND r.type = 'blocks'
				  AND blocker.status NOT IN ('done', 'cancelled')
			)`
		}
	}
	if filter.AfterKey != "" {
		query += ` AND EXISTS (SELECT 1 FROM tasks c WHERE c.task_key = ? AND ` + order.after + `)`
		args = append(args, strings.TrimSpace(filter.AfterKey))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query += ` ORDER BY ` + order.by
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
	_ = s.reader().QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) FROM task_events`).Scan(&sequence)
	return TaskPage{Tasks: out, NextCursor: nextCursor, Sequence: sequence}, nil
}
