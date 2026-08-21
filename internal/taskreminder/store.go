package taskreminder

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	basestore "github.com/alekzonder/tariboy/internal/store"
)

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
