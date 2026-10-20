package tasks

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/internal/store"
)

// ActivityFilter selects task events in [Since, Until). Both bounds are
// optional store.TimeLayout text; another RFC 3339 form is normalized. Agent
// keeps only that agent's own events; Keys adds tasks the caller already knows
// had work in the window, such as AI requests.
type ActivityFilter struct {
	Since string
	Until string
	Agent string
	Keys  []string
}

// TaskActivity is what happened to one task in an activity window. FirstAt and
// LastAt bound the selected events as store.TimeLayout text and stay
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

// activityWindow returns the SQL condition bounding col to the window. The
// column and the bounds are store.TimeLayout text, so they compare in time
// order as strings.
func activityWindow(col string, f ActivityFilter) (string, []any) {
	conds, args := []string{"1=1"}, []any{}
	if f.Since != "" {
		conds = append(conds, col+" >= ?")
		args = append(args, f.Since)
	}
	if f.Until != "" {
		conds = append(conds, col+" < ?")
		args = append(args, f.Until)
	}
	return strings.Join(conds, " AND "), args
}

// TaskActivity summarizes the window's task events per task the actor can see.
func (s *TaskService) TaskActivity(ctx context.Context, actor Actor, f ActivityFilter) ([]TaskActivity, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	// The command layer passes canonical bounds; older RFC 3339 forms are
	// normalized so they still compare in time order.
	f.Since, _ = store.NormalizeTimeValue(f.Since)
	f.Until, _ = store.NormalizeTimeValue(f.Until)
	q := s.reader()
	cte, cteArgs := visibleCTE(actor)
	byID := activityRows{}
	if err := activityEventRows(ctx, q, cte, cteArgs, f, byID); err != nil {
		return nil, err
	}
	if len(f.Keys) > 0 {
		if err := activityKeyRows(ctx, q, cte, cteArgs, f.Keys, byID); err != nil {
			return nil, err
		}
	}
	if len(byID) == 0 {
		return []TaskActivity{}, nil
	}
	if err := activityVisitRows(ctx, q, f, byID); err != nil {
		return nil, err
	}
	return activityTaskRows(ctx, q, f, byID)
}

// activityRows holds the activity being summarized for each task id.
type activityRows map[int64]*TaskActivity

// row returns the activity of task id, starting an empty one when needed.
func (byID activityRows) row(id int64) *TaskActivity {
	if byID[id] == nil {
		byID[id] = &TaskActivity{Outcomes: []string{}, EnteredStatuses: []string{}, Agents: []string{}}
	}
	return byID[id]
}

// activityEventRows folds the window's visible task events into byID.
func activityEventRows(
	ctx context.Context,
	q queryer,
	cte string,
	cteArgs []any,
	f ActivityFilter,
	byID activityRows,
) error {
	where, args := activityWindow("e.created_at", f)
	if f.Agent != "" {
		where += " AND e.actor = ?"
		args = append(args, agentPrincipal(f.Agent))
	}
	events, err := q.QueryContext(ctx, cte+`
		SELECT e.task_id, e.kind, e.actor, e.payload, e.created_at
		FROM task_events e JOIN visible_access v ON v.id = e.task_id
		WHERE `+where+` ORDER BY e.sequence`, append(slices.Clone(cteArgs), args...)...)
	if err != nil {
		return err
	}
	defer events.Close()
	for events.Next() {
		var id int64
		var kind, principal, payload, at string
		if err := events.Scan(&id, &kind, &principal, &payload, &at); err != nil {
			return err
		}
		r := byID.row(id)
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
	return events.Err()
}

// activityKeyRows adds to byID each visible task named in keys.
func activityKeyRows(
	ctx context.Context,
	q queryer,
	cte string,
	cteArgs []any,
	keys []string,
	byID activityRows,
) error {
	keysJSON, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	ids, err := q.QueryContext(ctx, cte+`
		SELECT t.id FROM tasks t JOIN visible_access v ON v.id = t.id
		WHERE t.task_key IN (SELECT value FROM json_each(?))`, append(slices.Clone(cteArgs), string(keysJSON))...)
	if err != nil {
		return err
	}
	defer ids.Close()
	for ids.Next() {
		var id int64
		if err := ids.Scan(&id); err != nil {
			return err
		}
		byID.row(id)
	}
	return ids.Err()
}

// activityVisitRows records the statuses each task in byID entered in the
// window.
func activityVisitRows(ctx context.Context, q queryer, f ActivityFilter, byID activityRows) error {
	where, args := activityWindow("entered_at", f)
	visits, err := q.QueryContext(ctx, `SELECT task_id, status_id FROM task_status_visits WHERE `+where+` ORDER BY task_id, sequence`, args...)
	if err != nil {
		return err
	}
	defer visits.Close()
	for visits.Next() {
		var id int64
		var status string
		if err := visits.Scan(&id, &status); err != nil {
			return err
		}
		if r := byID[id]; r != nil {
			r.EnteredStatuses = append(r.EnteredStatuses, status)
		}
	}
	return visits.Err()
}

// activityTaskRows loads the task of each activity in byID and returns the
// activities in task key order.
func activityTaskRows(ctx context.Context, q queryer, f ActivityFilter, byID activityRows) ([]TaskActivity, error) {
	idList := make([]int64, 0, len(byID))
	for id := range byID {
		idList = append(idList, id)
	}
	idsJSON, err := json.Marshal(idList)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, taskSelect+`
		WHERE t.id IN (SELECT value FROM json_each(?)) ORDER BY t.task_key`, string(idsJSON))
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

// inActivityWindow reports whether the stored time at is in the window.
func inActivityWindow(at string, f ActivityFilter) bool {
	if at == "" {
		return false
	}
	return (f.Since == "" || at >= f.Since) && (f.Until == "" || at < f.Until)
}
