package taskreminder

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	basestore "github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// Policy is retained until the reminder publisher is replaced by the goal
// publisher in the next migration step.
type Policy struct {
	Enabled        bool `json:"enabled"`
	IdleThresholdS int  `json:"idle_threshold_s"`
}

var DefaultPolicy = Policy{Enabled: false, IdleThresholdS: 300}

func ParsePolicy(raw string) (Policy, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultPolicy, nil
	}

	var parsed struct {
		Enabled        *bool `json:"enabled"`
		IdleThresholdS *int  `json:"idle_threshold_s"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return Policy{}, fmt.Errorf("parse task reminder policy: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Policy{}, fmt.Errorf("parse task reminder policy: multiple JSON values")
		}
		return Policy{}, fmt.Errorf("parse task reminder policy: %w", err)
	}
	if parsed.Enabled == nil {
		return Policy{}, fmt.Errorf("task reminder policy requires enabled")
	}
	if parsed.IdleThresholdS == nil {
		return Policy{}, fmt.Errorf("task reminder policy requires idle_threshold_s")
	}
	if *parsed.IdleThresholdS < 1 {
		return Policy{}, fmt.Errorf("task reminder policy idle_threshold_s must be at least 1")
	}
	return Policy{Enabled: *parsed.Enabled, IdleThresholdS: *parsed.IdleThresholdS}, nil
}

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

// Candidate is one idle agent and the assigned legacy Native Tasks that make
// up its current reminder generation.
type Candidate struct {
	Agent       string
	TaskKeys    []string
	ActivityAt  time.Time
	Fingerprint string
}

// Store reads eligible reminder generations and records generations that have
// been delivered through the normal inbox path.
type Store struct{ db *sql.DB }

type candidateGroup struct {
	agent, createdAt, terminalAt, savedFingerprint string
	taskKeys                                       []string
	latestTaskAt                                   time.Time
}

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

// Eligible returns enabled agents with enabled loops whose assigned, open,
// legacy tasks have been idle for the configured threshold. Interval mode is
// deliberately absent from the query: message-triggered loops (interval 0)
// are eligible just like timer-driven loops.
func (s *Store) Eligible(policy Policy, now time.Time) ([]Candidate, error) {
	if !policy.Enabled {
		return nil, nil
	}
	if policy.IdleThresholdS < 1 {
		return nil, fmt.Errorf("task reminder idle threshold must be at least 1 second")
	}

	rows, err := s.db.Query(`
		SELECT a.name, a.created_at, t.task_key, t.updated_at,
		       COALESCE((
		         SELECT i.ended_at
		         FROM iterations i
		         WHERE i.agent = a.name AND i.ended_at <> ''
		         ORDER BY i.ended_at DESC, i.id DESC
		         LIMIT 1
		       ), ''),
		       COALESCE(r.fingerprint, '')
		FROM agents a
		JOIN tasks t ON t.assignee = 'agent:' || a.name
		LEFT JOIN task_reminders r ON r.agent = a.name
		WHERE a.enabled = 1
		  AND a.loop_enabled = 1
		  AND t.workflow_version_id IS NULL
		  AND t.status NOT IN ('done', 'cancelled')
		ORDER BY a.name, t.task_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []candidateGroup
	for rows.Next() {
		var rowAgent, createdAt, taskKey, taskUpdatedAt, terminalAt, savedFingerprint string
		if err := rows.Scan(&rowAgent, &createdAt, &taskKey, &taskUpdatedAt, &terminalAt, &savedFingerprint); err != nil {
			return nil, err
		}
		if len(groups) == 0 || groups[len(groups)-1].agent != rowAgent {
			groups = append(groups, candidateGroup{
				agent: rowAgent, createdAt: createdAt, terminalAt: terminalAt, savedFingerprint: savedFingerprint,
			})
		}
		updatedAt, err := parseTimestamp(taskUpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse task %s activity time: %w", taskKey, err)
		}
		current := &groups[len(groups)-1]
		current.taskKeys = append(current.taskKeys, taskKey)
		if updatedAt.After(current.latestTaskAt) {
			current.latestTaskAt = updatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	threshold := time.Duration(policy.IdleThresholdS) * time.Second
	candidates := make([]Candidate, 0, len(groups))
	for _, group := range groups {
		activityAt, err := group.activityBoundary()
		if err != nil {
			return nil, err
		}
		if now.Before(activityAt.Add(threshold)) {
			continue
		}
		sort.Strings(group.taskKeys)
		candidate := Candidate{
			Agent:      group.agent,
			TaskKeys:   group.taskKeys,
			ActivityAt: activityAt,
		}
		candidate.Fingerprint = fingerprint(candidate)
		if candidate.Fingerprint == group.savedFingerprint {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (group candidateGroup) activityBoundary() (time.Time, error) {
	if group.terminalAt != "" {
		activityAt, err := parseTimestamp(group.terminalAt)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse terminal iteration activity for %s: %w", group.agent, err)
		}
		return activityAt, nil
	}
	if !group.latestTaskAt.IsZero() {
		return group.latestTaskAt, nil
	}
	activityAt, err := parseTimestamp(group.createdAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse agent %s activity time: %w", group.agent, err)
	}
	return activityAt, nil
}

// MarkSent persists a candidate only after its inbox message has been
// published successfully. It is intentionally independent of the loop
// manager; delivery and wake behavior stay owned by the existing bus.
func (s *Store) MarkSent(candidate Candidate, sentAt time.Time) error {
	if candidate.Agent == "" || candidate.Fingerprint == "" || candidate.ActivityAt.IsZero() {
		return fmt.Errorf("task reminder candidate is incomplete")
	}
	_, err := s.db.Exec(`INSERT INTO task_reminders(agent, fingerprint, activity_at, sent_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(agent) DO UPDATE SET
			fingerprint = excluded.fingerprint,
			activity_at = excluded.activity_at,
			sent_at = excluded.sent_at`,
		candidate.Agent, candidate.Fingerprint,
		candidate.ActivityAt.UTC().Format(time.RFC3339Nano), sentAt.UTC().Format(time.RFC3339Nano))
	return err
}

func parseTimestamp(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}

func fingerprint(candidate Candidate) string {
	parts := []string{
		candidate.Agent,
		candidate.ActivityAt.UTC().Format(time.RFC3339Nano),
		strings.Join(candidate.TaskKeys, "\x00"),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}
