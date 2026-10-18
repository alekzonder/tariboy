package tasks

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// ActivityFilter selects task events in [Since, Until). Both bounds are
// RFC3339 and optional. Agent keeps only that agent's own events; Keys adds
// tasks the caller already knows had work in the window, such as AI requests.
type ActivityFilter struct {
	Since string
	Until string
	Agent string
	Keys  []string
}

// TaskActivity is what happened to one task in an activity window. FirstAt and
// LastAt bound the selected events as fixed-width UTC milliseconds and stay
// empty for a task listed only through ActivityFilter.Keys.
type TaskActivity struct {
	Task            Task
	Comments        int
	Outcomes        []string
	EnteredStatuses []string
	Completed       bool
	Agents          []string
	FirstAt         string
	LastAt          string
}

const activityTimeNorm = "strftime('%Y-%m-%dT%H:%M:%f',%s)"

// activityWindow returns the SQL condition bounding col to the window.
// Columns and bounds compare in one fixed-width UTC form because stored times
// are RFC3339Nano text of variable width.
func activityWindow(col string, f ActivityFilter) (string, []any) {
	norm := func(expr string) string { return strings.Replace(activityTimeNorm, "%s", expr, 1) }
	conds, args := []string{"1=1"}, []any{}
	if f.Since != "" {
		conds = append(conds, norm(col)+">="+norm("?"))
		args = append(args, f.Since)
	}
	if f.Until != "" {
		conds = append(conds, norm(col)+"<"+norm("?"))
		args = append(args, f.Until)
	}
	return strings.Join(conds, " AND "), args
}

// TaskActivity summarizes the window's task events per task the actor can see.
func (s *Service) TaskActivity(ctx context.Context, actor Actor, f ActivityFilter) ([]TaskActivity, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	q := s.reader()
	visible, err := visibleTaskIDs(ctx, q, actor)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*TaskActivity{}
	row := func(id int64) *TaskActivity {
		if byID[id] == nil {
			byID[id] = &TaskActivity{Outcomes: []string{}, EnteredStatuses: []string{}, Agents: []string{}}
		}
		return byID[id]
	}

	where, args := activityWindow("created_at", f)
	if f.Agent != "" {
		where += " AND actor = ?"
		args = append(args, agentPrincipal(f.Agent))
	}
	events, err := q.QueryContext(ctx, `
		SELECT task_id, kind, actor, payload, strftime('%Y-%m-%dT%H:%M:%fZ', created_at)
		FROM task_events WHERE task_id IS NOT NULL AND `+where+` ORDER BY sequence`, args...)
	if err != nil {
		return nil, err
	}
	defer events.Close()
	for events.Next() {
		var id int64
		var kind, principal, payload, at string
		if err := events.Scan(&id, &kind, &principal, &payload, &at); err != nil {
			return nil, err
		}
		if visible[id] == "" {
			continue
		}
		r := row(id)
		if r.FirstAt == "" || at < r.FirstAt {
			r.FirstAt = at
		}
		if at > r.LastAt {
			r.LastAt = at
		}
		if agent, ok := strings.CutPrefix(principal, "agent:"); ok && !slices.Contains(r.Agents, agent) {
			r.Agents = append(r.Agents, agent)
		}
		switch kind {
		case "task.comment_added":
			r.Comments++
		case "workflow.transition_requested":
			var p struct {
				Outcome string `json:"outcome"`
			}
			if json.Unmarshal([]byte(payload), &p) == nil && p.Outcome != "" {
				r.Outcomes = append(r.Outcomes, p.Outcome)
			}
		}
	}
	if err := events.Err(); err != nil {
		return nil, err
	}
	events.Close()

	if len(f.Keys) > 0 {
		keyArgs := make([]any, 0, len(f.Keys))
		for _, key := range f.Keys {
			keyArgs = append(keyArgs, key)
		}
		ids, err := q.QueryContext(ctx, `SELECT id FROM tasks WHERE task_key IN (`+placeholders(len(keyArgs))+`)`, keyArgs...)
		if err != nil {
			return nil, err
		}
		for ids.Next() {
			var id int64
			if err := ids.Scan(&id); err != nil {
				ids.Close()
				return nil, err
			}
			if visible[id] != "" {
				row(id)
			}
		}
		if err := ids.Err(); err != nil {
			ids.Close()
			return nil, err
		}
		ids.Close()
	}
	if len(byID) == 0 {
		return []TaskActivity{}, nil
	}

	where, args = activityWindow("entered_at", f)
	visits, err := q.QueryContext(ctx, `SELECT task_id, status_id FROM task_status_visits WHERE `+where+` ORDER BY task_id, sequence`, args...)
	if err != nil {
		return nil, err
	}
	defer visits.Close()
	for visits.Next() {
		var id int64
		var status string
		if err := visits.Scan(&id, &status); err != nil {
			return nil, err
		}
		if r := byID[id]; r != nil {
			r.EnteredStatuses = append(r.EnteredStatuses, status)
		}
	}
	if err := visits.Err(); err != nil {
		return nil, err
	}
	visits.Close()

	ids := make(map[int64]string, len(byID))
	for id := range byID {
		ids[id] = "read"
	}
	query, taskArgs := addVisibleClause(taskSelect+` WHERE 1=1`, ids)
	rows, err := q.QueryContext(ctx, query+` ORDER BY t.task_key`, taskArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TaskActivity, 0, len(byID))
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		r := byID[task.ID]
		r.Task = task
		r.Completed = inActivityWindow(task.CompletedAt, f)
		out = append(out, *r)
	}
	return out, rows.Err()
}

func inActivityWindow(at string, f ActivityFilter) bool {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return false
	}
	if since, err := time.Parse(time.RFC3339Nano, f.Since); err == nil && t.Before(since) {
		return false
	}
	if until, err := time.Parse(time.RFC3339Nano, f.Until); err == nil && !t.Before(until) {
		return false
	}
	return true
}
