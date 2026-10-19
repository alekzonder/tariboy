package commands

import (
	"math"
	"net/http"
	"slices"
	"sort"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// maxActivityTasks bounds one response; totals still cover every task.
const maxActivityTasks = 500

type taskActivityRow struct {
	Key             string   `json:"key"`
	Title           string   `json:"title"`
	Queue           string   `json:"queue"`
	Status          string   `json:"status"`
	Category        string   `json:"category"`
	Assignee        string   `json:"assignee"`
	Completed       bool     `json:"completed"`
	Iterations      int      `json:"iterations"`
	Requests        int      `json:"requests"`
	Tokens          int      `json:"tokens"`
	CostUSD         float64  `json:"cost_usd"`
	Comments        int      `json:"comments"`
	Outcomes        []string `json:"outcomes"`
	EnteredStatuses []string `json:"entered_statuses"`
	Agents          []string `json:"agents"`
	FirstAt         string   `json:"first_at"`
	LastAt          string   `json:"last_at"`
}

type taskActivityTotals struct {
	Tasks       int     `json:"tasks"`
	Completed   int     `json:"completed"`
	Transitions int     `json:"transitions"`
	Comments    int     `json:"comments"`
	Iterations  int     `json:"iterations"`
	Requests    int     `json:"requests"`
	Tokens      int     `json:"tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

type taskActivityAgent struct {
	Agent      string  `json:"agent"`
	Tasks      int     `json:"tasks"`
	Completed  int     `json:"completed"`
	Iterations int     `json:"iterations"`
	Requests   int     `json:"requests"`
	CostUSD    float64 `json:"cost_usd"`
}

type taskActivityReport struct {
	Since     string              `json:"since"`
	Until     string              `json:"until"`
	Agent     string              `json:"agent,omitempty"`
	Truncated bool                `json:"truncated"`
	Totals    taskActivityTotals  `json:"totals"`
	Tasks     []taskActivityRow   `json:"tasks"`
	Agents    []taskActivityAgent `json:"agents,omitempty"`
}

func roundUSD(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// taskActivity summarizes the work done on tasks in a time window: AI requests
// tagged with a task plus the task events, per task and, server-wide, per agent.
func taskActivity() registry.Command {
	return registry.Command{
		Path:    "tasks.activity",
		Summary: "Summarize work on tasks in a time window",
		Args: []registry.Arg{
			{Name: "since", Flag: "since", Short: "s", Type: registry.String, Help: "RFC 3339 time (inclusive lower bound)"},
			{Name: "until", Flag: "until", Short: "u", Type: registry.String, Help: "RFC 3339 time (exclusive upper bound)"},
			{Name: "agent", Flag: "agent", Short: "a", Type: registry.String, Help: "only this agent's work"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/task-activity"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			if c.Tasks == nil {
				return nil, api.UserError{Status: http.StatusServiceUnavailable,
					Code: "tasks_unavailable", Msg: "native Tasks service is unavailable"}
			}
			since, err := activityBound(str(p, "since"))
			if err != nil {
				return nil, err
			}
			until, err := activityBound(str(p, "until"))
			if err != nil {
				return nil, err
			}
			agentName := str(p, "agent")

			usage, err := aiproxy.NewStore(c.Store, nil).TaskUsage(aiproxy.UsageFilter{Agent: agentName, Since: since, Until: until})
			if err != nil {
				return nil, err
			}
			keys := []string{}
			for _, u := range usage {
				if !slices.Contains(keys, u.TaskKey) {
					keys = append(keys, u.TaskKey)
				}
			}
			activity, err := c.Tasks.TaskActivity(registry.RequestContext(p), tasks.CustomerActor(c.Tasks.CustomerLogin()),
				tasks.ActivityFilter{Since: since, Until: until, Agent: agentName, Keys: keys})
			if err != nil {
				return nil, err
			}

			byKey := map[string]*taskActivityRow{}
			rows := make([]*taskActivityRow, 0, len(activity))
			for _, a := range activity {
				status := a.Task.Status
				if a.Task.WorkflowStatus != "" {
					status = a.Task.WorkflowStatus
				}
				row := &taskActivityRow{
					Key: a.Task.Key, Title: a.Task.Title, Queue: a.Task.Queue, Status: status,
					Category: a.Task.Category, Assignee: a.Task.Assignee, Completed: a.Completed,
					Comments: a.Comments, Outcomes: a.Outcomes, EnteredStatuses: a.EnteredStatuses,
					Agents: a.Agents, FirstAt: a.FirstAt, LastAt: a.LastAt,
				}
				byKey[row.Key] = row
				rows = append(rows, row)
			}
			perAgent := map[string]*taskActivityAgent{}
			for _, u := range usage {
				row := byKey[u.TaskKey]
				if row == nil {
					continue // deleted or not visible
				}
				// shortcut: an iteration that switched tasks counts once per task.
				row.Iterations += u.Iterations
				row.Requests += u.Requests
				row.Tokens += u.Tokens
				row.CostUSD = roundUSD(row.CostUSD + u.CostUSD)
				if !slices.Contains(row.Agents, u.Agent) {
					row.Agents = append(row.Agents, u.Agent)
				}
				if row.FirstAt == "" || u.FirstTS < row.FirstAt {
					row.FirstAt = u.FirstTS
				}
				if u.LastTS > row.LastAt {
					row.LastAt = u.LastTS
				}
				if perAgent[u.Agent] == nil {
					perAgent[u.Agent] = &taskActivityAgent{Agent: u.Agent}
				}
				a := perAgent[u.Agent]
				a.Iterations += u.Iterations
				a.Requests += u.Requests
				a.CostUSD = roundUSD(a.CostUSD + u.CostUSD)
			}
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].LastAt != rows[j].LastAt {
					return rows[i].LastAt > rows[j].LastAt
				}
				return rows[i].Key < rows[j].Key
			})

			report := taskActivityReport{Since: since, Until: until, Agent: agentName, Tasks: []taskActivityRow{}}
			for _, row := range rows {
				report.Totals.Tasks++
				if row.Completed {
					report.Totals.Completed++
				}
				report.Totals.Transitions += len(row.EnteredStatuses)
				report.Totals.Comments += row.Comments
				report.Totals.Iterations += row.Iterations
				report.Totals.Requests += row.Requests
				report.Totals.Tokens += row.Tokens
				report.Totals.CostUSD = roundUSD(report.Totals.CostUSD + row.CostUSD)
				for _, name := range row.Agents {
					if perAgent[name] == nil {
						perAgent[name] = &taskActivityAgent{Agent: name}
					}
					perAgent[name].Tasks++
					if row.Completed {
						perAgent[name].Completed++
					}
				}
				if len(report.Tasks) < maxActivityTasks {
					report.Tasks = append(report.Tasks, *row)
				} else {
					report.Truncated = true
				}
			}
			if agentName == "" {
				report.Agents = []taskActivityAgent{}
				for _, a := range perAgent {
					report.Agents = append(report.Agents, *a)
				}
				sort.Slice(report.Agents, func(i, j int) bool { return report.Agents[i].Agent < report.Agents[j].Agent })
			}
			return report, nil
		},
	}
}

// activityBound validates an optional RFC 3339 bound with any offset and
// returns it in the stored time format.
func activityBound(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	t, err := store.ParseInputTime(value)
	if err != nil {
		return "", api.UserError{Code: "bad_time", Msg: "since and until must be RFC 3339 times"}
	}
	return store.FormatTime(t), nil
}
