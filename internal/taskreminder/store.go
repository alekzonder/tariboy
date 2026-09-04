package taskreminder

import (
	"database/sql"
	"fmt"
	"time"

	basestore "github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// Goal is the task currently selected for an agent.
type Goal struct {
	Agent    string
	TaskKey  string
	Revision int64
	Reason   string
	Waiting  bool
}

const goalTaskSelect = `
SELECT t.id, t.task_key, t.queue_prefix, COALESCE(p.task_key, ''),
       t.position, t.priority, t.title, t.description, t.status, t.pull_request, t.author, t.customer,
       t.group_name, t.assignee, t.manual_block_reason,
       EXISTS (
         SELECT 1
         FROM task_relations r
         JOIN tasks blocker ON blocker.id = r.source_id
         WHERE r.target_id = t.id AND r.type = 'blocks'
           AND blocker.status NOT IN ('done', 'cancelled')
       ) OR t.manual_block_reason <> '',
	   COALESCE(t.workflow_version_id, 0),
	   COALESCE(w.name || '@' || w.version, ''),
	   COALESCE(t.workflow_status, ''), COALESCE(t.workflow_revision, 0),
       t.revision, t.created_at, t.updated_at, t.completed_at,
       COALESCE((
         SELECT waiting.requested_at
         FROM task_waiting_for waiting
         WHERE waiting.task_id = t.id
           AND waiting.expected_principal = t.customer
           AND waiting.resolved_at = ''
         ORDER BY waiting.requested_at, waiting.id
         LIMIT 1
       ), '')
FROM tasks t
LEFT JOIN tasks p ON p.id = t.parent_id
LEFT JOIN task_workflow_versions w ON w.id = t.workflow_version_id`

type Store struct{ db *sql.DB }

func NewStore(s *basestore.Store) *Store { return &Store{db: s.DB} }

// ReconcileAgent validates the agent's sticky goal and selects a replacement
// when necessary. Selection and persistence share one transaction.
func (s *Store) ReconcileAgent(agent string, now time.Time) (Goal, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Goal{}, err
	}
	defer tx.Rollback()

	goal, _, err := reconcileAgent(tx, agent, now)
	if err != nil {
		return Goal{}, err
	}
	if err := tx.Commit(); err != nil {
		return Goal{}, err
	}
	return goal, nil
}

// Current reconciles the sticky selection and returns its authoritative task.
func (s *Store) Current(agent string, now time.Time) (tasks.Task, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return tasks.Task{}, false, err
	}
	defer tx.Rollback()

	goal, task, err := reconcileAgent(tx, agent, now)
	if err != nil {
		return tasks.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return tasks.Task{}, false, err
	}
	return task, goal.TaskKey != "", nil
}

func reconcileAgent(tx *sql.Tx, agent string, now time.Time) (Goal, tasks.Task, error) {
	var enabled, loopEnabled, goalEnabled bool
	var timeoutS int
	var current string
	if err := tx.QueryRow(`SELECT enabled, loop_enabled, goal_enabled,
		goal_wait_customer_timeout_s, current_goal_task_key
		FROM agents WHERE name=?`, agent).Scan(&enabled, &loopEnabled, &goalEnabled, &timeoutS, &current); err != nil {
		return Goal{}, tasks.Task{}, err
	}
	if !goalEnabled {
		if _, err := tx.Exec(`UPDATE agents SET current_goal_task_key='' WHERE name=?`, agent); err != nil {
			return Goal{}, tasks.Task{}, err
		}
		return Goal{}, tasks.Task{}, nil
	}
	if !enabled || !loopEnabled {
		return Goal{}, tasks.Task{}, nil
	}

	if current != "" {
		task, waitAt, err := readGoalTask(tx, current, agent)
		if err != nil && err != sql.ErrNoRows {
			return Goal{}, tasks.Task{}, err
		}
		if err == nil {
			valid, waiting, err := validGoal(task, waitAt, timeoutS, now)
			if err != nil {
				return Goal{}, tasks.Task{}, err
			}
			if valid {
				return Goal{Agent: agent, TaskKey: task.Key, Revision: task.Revision, Reason: "selected", Waiting: waiting}, task, nil
			}
		}
	}

	selected := ""
	var revision int64
	err := tx.QueryRow(`
			SELECT t.task_key, t.revision
			FROM tasks t
			WHERE t.assignee='agent:' || ?
			  AND t.pull_request=''
			  AND t.status IN ('in_progress','open')
			ORDER BY CASE t.priority WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 WHEN 'P2' THEN 2 ELSE 3 END,
			         CASE t.status WHEN 'in_progress' THEN 0 ELSE 1 END,
			         t.created_at, t.task_key
			LIMIT 1`, agent).Scan(&selected, &revision)
	if err != nil && err != sql.ErrNoRows {
		return Goal{}, tasks.Task{}, err
	}

	if _, err := tx.Exec(`UPDATE agents SET current_goal_task_key=? WHERE name=?`, selected, agent); err != nil {
		return Goal{}, tasks.Task{}, err
	}
	if selected == "" {
		return Goal{}, tasks.Task{}, nil
	}

	task, _, err := readGoalTask(tx, selected, agent)
	if err != nil {
		return Goal{}, tasks.Task{}, err
	}
	return Goal{Agent: agent, TaskKey: task.Key, Revision: task.Revision, Reason: "selected"}, task, nil
}

func readGoalTask(tx *sql.Tx, key, agent string) (tasks.Task, string, error) {
	var task tasks.Task
	var blocked bool
	var waitAt string
	err := tx.QueryRow(goalTaskSelect+` WHERE t.task_key=? AND t.assignee='agent:' || ?`, key, agent).Scan(
		&task.ID, &task.Key, &task.Queue, &task.ParentKey,
		&task.Position, &task.Priority, &task.Title, &task.Description, &task.Status, &task.PullRequest, &task.Author, &task.Customer,
		&task.Group, &task.Assignee, &task.ManualBlockReason, &blocked,
		&task.WorkflowVersionID, &task.WorkflowVersion, &task.WorkflowStatus, &task.WorkflowRevision,
		&task.Revision, &task.CreatedAt, &task.UpdatedAt, &task.CompletedAt, &waitAt,
	)
	task.Blocked = blocked
	return task, waitAt, err
}

func validGoal(task tasks.Task, waitAt string, timeoutS int, now time.Time) (bool, bool, error) {
	if task.PullRequest != "" {
		return false, false, nil
	}
	switch task.Status {
	case "open", "in_progress":
		return true, false, nil
	case "wait_customer":
		if waitAt == "" {
			return false, false, nil
		}
		requestedAt, err := parseTimestamp(waitAt)
		if err != nil {
			return false, false, fmt.Errorf("parse customer wait for %s: %w", task.Key, err)
		}
		waiting := now.Before(requestedAt.Add(time.Duration(timeoutS) * time.Second))
		return waiting, waiting, nil
	default:
		return false, false, nil
	}
}

func parseTimestamp(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}
