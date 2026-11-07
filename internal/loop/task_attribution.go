package loop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/tasks"
)

var errGoalUnavailable = api.UserError{Status: http.StatusServiceUnavailable, Code: "unavailable",
	Msg: "goal selection is unavailable"}

type nativeTaskReader interface {
	GetTask(context.Context, tasks.Actor, string) (tasks.TaskDetail, error)
	CustomerLogin() string
}

type taskAttributionProxy interface {
	UpdateTask(key, taskID, epicID string) int
	SetTaskIfEmpty(key, taskID, epicID string) (int, bool)
}

// taskAttribution is the cost attribution of a Goal: the goal itself and the
// real root of its task tree.
type taskAttribution struct {
	TaskID string
	EpicID string
	// EpicVisible reports whether the agent may read EpicID; a hidden root is
	// never shown to the agent.
	EpicVisible bool
	// Partial holds why the walk stopped below the real root: an ancestor the
	// daemon could not read. EpicID is then the last ancestor it read.
	Partial error
}

// warnPartial logs an attribution that stopped below the real root.
func (a taskAttribution) warnPartial(log *slog.Logger, agentName string) {
	if a.Partial != nil && log != nil {
		log.Warn("goal attribution stopped below the task tree root",
			"agent", agentName, "goal", a.TaskID, "root", a.EpicID, "err", a.Partial)
	}
}

// resolveNativeTaskAttribution reads the goal as the agent, so a goal the agent
// cannot see or holds read-only is refused, and its ancestors as the customer,
// so a parent in a queue hidden from the agent does not stop the walk.
func resolveNativeTaskAttribution(ctx context.Context, reader nativeTaskReader, agentName, key string) (taskAttribution, error) {
	if reader == nil {
		return taskAttribution{}, api.UserError{Status: http.StatusServiceUnavailable, Code: "unavailable",
			Msg: "native tasks unavailable"}
	}

	agentActor := tasks.AgentActor(agentName)
	detail, err := reader.GetTask(ctx, agentActor, strings.TrimSpace(key))
	if err != nil {
		return taskAttribution{}, err
	}
	// A former workflow holder may read its task, not take it as a Goal.
	if detail.HolderReadOnly {
		return taskAttribution{}, &tasks.Error{Status: http.StatusNotFound, Code: "not_found",
			Msg: fmt.Sprintf("task %q not found", key)}
	}
	return walkTaskRoot(ctx, reader, agentName, detail.Task)
}

// resolveLetterTaskAttribution is the attribution of a task the agent's
// letters are about but that is no Goal of the agent: the task is read as the
// customer, so a task in a customer status or one the agent holds read-only
// still gets its spend.
func resolveLetterTaskAttribution(ctx context.Context, reader nativeTaskReader, agentName, key string) (taskAttribution, error) {
	if reader == nil {
		return taskAttribution{}, api.UserError{Status: http.StatusServiceUnavailable, Code: "unavailable",
			Msg: "native tasks unavailable"}
	}
	detail, err := reader.GetTask(ctx, tasks.CustomerActor(reader.CustomerLogin()), strings.TrimSpace(key))
	if err != nil {
		return taskAttribution{}, fmt.Errorf("%w: %w", errLetterTaskUnreadable, err)
	}
	return walkTaskRoot(ctx, reader, agentName, detail.Task)
}

// errLetterTaskUnreadable marks a letter task the daemon could not read: the
// iteration then runs unattributed instead of failing.
var errLetterTaskUnreadable = errors.New("read letter task")

// walkTaskRoot walks from goal to the root of its tree as the customer and
// reports whether the agent may see that root.
func walkTaskRoot(ctx context.Context, reader nativeTaskReader, agentName string, goal tasks.Task) (taskAttribution, error) {
	agentActor := tasks.AgentActor(agentName)
	out := taskAttribution{TaskID: goal.Key, EpicID: goal.Key, EpicVisible: true}
	customer := tasks.CustomerActor(reader.CustomerLogin())
	seen := map[string]struct{}{goal.Key: {}}
	for parent := goal.ParentKey; parent != ""; {
		detail, err := reader.GetTask(ctx, customer, parent)
		if err != nil {
			out.Partial = err
			break
		}
		task := detail.Task
		if _, ok := seen[task.Key]; ok {
			return taskAttribution{}, fmt.Errorf("native task parent cycle at %q", task.Key)
		}
		seen[task.Key] = struct{}{}
		out.EpicID = task.Key
		parent = task.ParentKey
	}
	if out.EpicID != goal.Key {
		_, err := reader.GetTask(ctx, agentActor, out.EpicID)
		out.EpicVisible = err == nil
	}
	return out, nil
}

// setGoal selects the goal and stamps its real root on the live proxy token.
// The answer names the root only when the agent may see it.
func setGoal(ctx context.Context, log *slog.Logger, reader nativeTaskReader, selectGoal func(agent, key string, activate func() (func(), error)) error, proxy taskAttributionProxy, iteration, agent, key string) (map[string]any, error) {
	if selectGoal == nil || proxy == nil {
		return nil, errGoalUnavailable
	}
	attribution, err := resolveNativeTaskAttribution(ctx, reader, agent, key)
	if err != nil {
		return nil, err
	}
	attribution.warnPartial(log, agent)
	taskID, epicID := attribution.TaskID, attribution.EpicID
	updated := 0
	if err := selectGoal(agent, taskID, func() (func(), error) {
		var changed bool
		updated, changed = proxy.SetTaskIfEmpty(iteration, taskID, epicID)
		if updated != 1 {
			return nil, api.UserError{Code: "goal_failed",
				Msg: "current iteration already has a goal or no live proxy lease"}
		}
		if !changed {
			return nil, nil
		}
		return func() { proxy.UpdateTask(iteration, "", "") }, nil
	}); err != nil {
		return nil, err
	}
	if !attribution.EpicVisible {
		epicID = ""
	}
	return map[string]any{"task_id": taskID, "epic_id": epicID, "updated": updated}, nil
}
