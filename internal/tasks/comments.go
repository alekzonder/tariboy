package tasks

import (
	"context"
	"database/sql"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

var mentionRE = regexp.MustCompile(`@((?:agent|user):[A-Za-z0-9][A-Za-z0-9._-]*)`)

func (s *TaskService) AddComment(ctx context.Context, actor Actor, key string, in AddCommentInput) (CommentResult, error) {
	if err := validateActor(actor); err != nil {
		return CommentResult{}, err
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return CommentResult{}, newError(http.StatusBadRequest, "missing_comment", "comment body is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommentResult{}, err
	}
	defer tx.Rollback()
	if replayed, ok, err := readTaskIdempotency[CommentResult](
		ctx, tx, actor.Principal, "add_comment", in.IdempotencyKey,
	); err != nil {
		return CommentResult{}, err
	} else if ok {
		return replayed, nil
	}
	task, err := taskByKey(ctx, tx, strings.TrimSpace(key))
	if err != nil {
		return CommentResult{}, err
	}
	if err := requireRespond(ctx, tx, actor, task); err != nil {
		return CommentResult{}, err
	}
	now := s.now()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_comments(task_id, author, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, task.ID, actor.Principal, body, now, now)
	if err != nil {
		return CommentResult{}, err
	}
	commentID, _ := result.LastInsertId()
	comment := Comment{
		ID: commentID, TaskKey: task.Key, Author: actor.Principal, Body: body,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	resolved, err := s.resolveCommentWaitsTx(ctx, tx, task, actor, commentID, now)
	if err != nil {
		return CommentResult{}, err
	}

	mentions := s.parseMentions(body)
	created, err := createMentionWaitsTx(ctx, tx, task, actor, mentions, commentID, now)
	if err != nil {
		return CommentResult{}, err
	}
	previousStatus := task.Status
	if err := commentStatusTx(ctx, tx, &task, actor, mentions, created, resolved); err != nil {
		return CommentResult{}, err
	}
	if _, err := saveTaskStateTx(ctx, tx, &task, previousStatus, now, false); err != nil {
		return CommentResult{}, err
	}
	if task.Status != previousStatus {
		if _, err := appendEventTx(ctx, tx, task, "task.updated", actor,
			map[string]any{"status": task.Status, "pull_request": task.PullRequest, "assignee": task.Assignee, "priority": task.Priority}, now); err != nil {
			return CommentResult{}, err
		}
	}
	sequence, err := appendEventTx(ctx, tx, task, "task.comment_added", actor,
		map[string]any{
			"comment_id": commentID,
			"mentions":   mentions,
			"resolved":   waitPrincipals(resolved),
		}, now)
	if err != nil {
		return CommentResult{}, err
	}
	if err := notifyCommentTx(ctx, tx, sequence, task, actor, created, resolved, now); err != nil {
		return CommentResult{}, err
	}
	response := CommentResult{Comment: comment, CreatedWaits: created, ResolvedWaits: resolved}
	if err := writeTaskIdempotency(
		ctx, tx, actor.Principal, "add_comment", in.IdempotencyKey, response, now,
	); err != nil {
		return CommentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommentResult{}, err
	}
	s.signal()
	return response, nil
}

// resolveCommentWaitsTx resolves the open questions the commenting actor was
// asked on the task and returns them.
func (s *TaskService) resolveCommentWaitsTx(
	ctx context.Context, tx *sql.Tx, task Task, actor Actor, commentID int64, now string,
) ([]WaitingFor, error) {
	resolved, err := openWaitsForPrincipal(ctx, tx, task, actor.Principal)
	if err != nil {
		return nil, err
	}
	// A workflow's own question is answered by an outcome, never by a comment.
	resolved = withoutWorkflowWaits(resolved)
	if len(resolved) == 0 {
		return resolved, nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_waiting_for
		SET resolving_comment_id = ?, resolved_at = ?
		WHERE task_id = ? AND expected_principal = ? AND resolved_at = '' AND requesting_principal <> ?`,
		commentID, now, task.ID, actor.Principal, workflowActor); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_notification_state SET read_at = ?
		WHERE customer_principal = ? AND read_at = '' AND notification_id IN (
			SELECT o.notification_id
			FROM task_notification_outbox o
			JOIN task_events e ON e.sequence = o.event_sequence
			WHERE o.message_type = 'task.question' AND e.task_id = ?
		)`, now, userPrincipal(s.customer), task.ID); err != nil {
		return nil, err
	}
	for i := range resolved {
		resolved[i].ResolvingCommentID = commentID
		resolved[i].ResolvedAt = now
	}
	// The customer's answer lets the asker work again: iterations that
	// counted as idle before the question no longer count.
	if task.WorkflowDigest != "" && containsPrincipal(resolved, task.Customer) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_status_visits SET idle_iterations = 0 WHERE task_id = ? AND left_at = ''`, task.ID); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

// createMentionWaitsTx opens a question for each principal the comment
// mentions and returns the questions it opened.
func createMentionWaitsTx(
	ctx context.Context, tx *sql.Tx, task Task, actor Actor, mentions []string, commentID int64, now string,
) ([]WaitingFor, error) {
	created := make([]WaitingFor, 0, len(mentions))
	for _, principal := range mentions {
		if task.WorkflowDigest != "" {
			// The workflow already waits on this principal; keep its question.
			open, err := openWaitsForPrincipal(ctx, tx, task, principal)
			if err != nil {
				return nil, err
			}
			if len(withoutWorkflowWaits(open)) < len(open) {
				continue
			}
		}
		wait, err := upsertWait(ctx, tx, task, principal, actor.Principal, commentID, now)
		if err != nil {
			return nil, err
		}
		created = append(created, wait)
	}
	return created, nil
}

// commentStatusTx moves the task's status for the questions a comment opened
// and resolved.
func commentStatusTx(
	ctx context.Context, tx *sql.Tx, task *Task, actor Actor, mentions []string, created, resolved []WaitingFor,
) error {
	customerWaits, err := openWaitsForPrincipal(ctx, tx, *task, task.Customer)
	if err != nil {
		return err
	}
	// The assignee's question to the customer moves the task to wait_customer;
	// the customer's answer, or the customer mentioning the assignee, hands it
	// back.
	asks := task.Assignee == actor.Principal && !actor.IsCustomer && containsPrincipal(created, task.Customer)
	answered := task.Status == StatusWaitCustomer && len(customerWaits) == 0 &&
		(containsPrincipal(resolved, task.Customer) ||
			actor.IsCustomer && task.Assignee != "" && slices.Contains(mentions, task.Assignee))
	if task.Status != StatusDone && task.Status != StatusCancelled {
		if task.WorkflowDigest != "" {
			if err := commentWorkflowStateTx(ctx, tx, task, asks, answered); err != nil {
				return err
			}
		} else if asks {
			task.Status = StatusWaitCustomer
		} else if answered {
			task.Status = StatusInProgress
		}
	}
	return nil
}

// notifyCommentTx tells each mentioned principal about its new question and
// each asker about its answer.
func notifyCommentTx(
	ctx context.Context, tx *sql.Tx, sequence int64, task Task, actor Actor, created, resolved []WaitingFor, now string,
) error {
	for _, wait := range created {
		if wait.ExpectedPrincipal == actor.Principal {
			continue
		}
		if err := enqueueNotificationTx(ctx, tx, sequence, wait.ExpectedPrincipal, actor.Principal,
			"task.question", task, actor.Principal+" asked for an answer on "+task.Key, now); err != nil {
			return err
		}
	}
	notified := map[string]bool{}
	for _, wait := range resolved {
		if wait.RequestingPrincipal == actor.Principal || notified[wait.RequestingPrincipal] {
			continue
		}
		if err := enqueueNotificationTx(ctx, tx, sequence, wait.RequestingPrincipal, actor.Principal,
			"task.answered", task, actor.Principal+" answered on "+task.Key, now); err != nil {
			return err
		}
		notified[wait.RequestingPrincipal] = true
	}
	return nil
}

// commentWorkflowStateTx derives the state a comment leaves a workflow task
// in. Only the holder's question in a pool status, and its answer, move the
// category; a customer status, a script status, and a pause keep theirs.
func commentWorkflowStateTx(ctx context.Context, tx *sql.Tx, task *Task, asks, answered bool) error {
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT workflow_owner_kind FROM tasks WHERE id = ?`, task.ID).Scan(&owner); err != nil {
		return err
	}
	if owner == "" {
		// A terminal status, or one the pinned manifest does not declare.
		return nil
	}
	return applyState(task, taskState{
		Status: task.WorkflowStatus, Owner: owner, Paused: task.WorkflowPausedReason != "",
		Blocked: task.Blocked, Assignee: task.Assignee, Holder: task.Assignee,
		HolderAsks: asks || task.Status == StatusWaitCustomer && !answered,
	})
}

func (s *TaskService) parseMentions(body string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, match := range mentionRE.FindAllStringSubmatch(body, -1) {
		principal := match[1]
		if strings.HasPrefix(principal, "user:") && principal != userPrincipal(s.customer) {
			continue
		}
		if seen[principal] {
			continue
		}
		seen[principal] = true
		out = append(out, principal)
	}
	return out
}

func upsertWait(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	expected string,
	requester string,
	commentID int64,
	now string,
) (WaitingFor, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE task_waiting_for
		SET requesting_principal = ?, requesting_comment_id = ?
		WHERE task_id = ? AND expected_principal = ? AND resolved_at = ''`,
		requester, commentID, task.ID, expected)
	if err != nil {
		return WaitingFor{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_waiting_for(
				task_id, expected_principal, requesting_principal,
				requesting_comment_id, requested_at
			) VALUES (?, ?, ?, ?, ?)`,
			task.ID, expected, requester, commentID, now); err != nil {
			return WaitingFor{}, err
		}
	}
	var wait WaitingFor
	err = tx.QueryRowContext(ctx, `
		SELECT id, expected_principal, requesting_principal,
		       requesting_comment_id, requested_at
		FROM task_waiting_for
		WHERE task_id = ? AND expected_principal = ? AND resolved_at = ''`,
		task.ID, expected).Scan(
		&wait.ID, &wait.ExpectedPrincipal, &wait.RequestingPrincipal,
		&wait.RequestingCommentID, &wait.RequestedAt)
	wait.TaskKey = task.Key
	return wait, err
}

func openWaitsForPrincipal(ctx context.Context, q queryer, task Task, principal string) ([]WaitingFor, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, expected_principal, requesting_principal,
		       requesting_comment_id, requested_at
		FROM task_waiting_for
		WHERE task_id = ? AND expected_principal = ? AND resolved_at = ''
		ORDER BY id`, task.ID, principal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WaitingFor{}
	for rows.Next() {
		var wait WaitingFor
		if err := rows.Scan(&wait.ID, &wait.ExpectedPrincipal, &wait.RequestingPrincipal,
			&wait.RequestingCommentID, &wait.RequestedAt); err != nil {
			return nil, err
		}
		wait.TaskKey = task.Key
		out = append(out, wait)
	}
	return out, rows.Err()
}

func listOpenWaits(ctx context.Context, q queryer, task Task) ([]WaitingFor, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, expected_principal, requesting_principal,
		       requesting_comment_id, requested_at
		FROM task_waiting_for
		WHERE task_id = ? AND resolved_at = ''
		ORDER BY id`, task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WaitingFor{}
	for rows.Next() {
		var wait WaitingFor
		if err := rows.Scan(&wait.ID, &wait.ExpectedPrincipal, &wait.RequestingPrincipal,
			&wait.RequestingCommentID, &wait.RequestedAt); err != nil {
			return nil, err
		}
		wait.TaskKey = task.Key
		out = append(out, wait)
	}
	return out, rows.Err()
}

func listComments(ctx context.Context, q queryer, task Task) ([]Comment, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, author, body, revision, created_at, updated_at
		FROM task_comments WHERE task_id = ? ORDER BY id`, task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.Author, &comment.Body,
			&comment.Revision, &comment.CreatedAt, &comment.UpdatedAt); err != nil {
			return nil, err
		}
		comment.TaskKey = task.Key
		out = append(out, comment)
	}
	return out, rows.Err()
}

func withoutWorkflowWaits(waits []WaitingFor) []WaitingFor {
	out := waits[:0:0]
	for _, wait := range waits {
		if wait.RequestingPrincipal != workflowActor {
			out = append(out, wait)
		}
	}
	return out
}

func waitPrincipals(waits []WaitingFor) []string {
	out := make([]string, 0, len(waits))
	for _, wait := range waits {
		out = append(out, wait.ExpectedPrincipal)
	}
	return out
}

func containsPrincipal(waits []WaitingFor, principal string) bool {
	for _, wait := range waits {
		if wait.ExpectedPrincipal == principal {
			return true
		}
	}
	return false
}
