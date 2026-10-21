package loop

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/tasks"
)

var errGoalUnavailable = api.UserError{Status: http.StatusServiceUnavailable, Code: "unavailable",
	Msg: "goal selection is unavailable"}

type nativeTaskReader interface {
	GetTask(context.Context, tasks.Actor, string) (tasks.TaskDetail, error)
}

type taskAttributionProxy interface {
	UpdateTask(key, taskID, epicID string) int
	SetTaskIfEmpty(key, taskID, epicID string) (int, bool)
}

func resolveNativeTaskAttribution(ctx context.Context, reader nativeTaskReader, agentName, key string) (string, string, error) {
	if reader == nil {
		return "", "", api.UserError{Status: http.StatusServiceUnavailable, Code: "unavailable",
			Msg: "native tasks unavailable"}
	}

	actor := tasks.AgentActor(agentName)
	current := strings.TrimSpace(key)
	seen := make(map[string]struct{})
	var taskID string
	for {
		detail, err := reader.GetTask(ctx, actor, current)
		if err != nil {
			return "", "", err
		}
		task := detail.Task
		// A former workflow holder may read its task, not take it as a Goal;
		// ancestors are read as context and stay attributable.
		if taskID == "" && detail.HolderReadOnly {
			return "", "", &tasks.Error{Status: http.StatusNotFound, Code: "not_found",
				Msg: fmt.Sprintf("task %q not found", key)}
		}
		if _, ok := seen[task.Key]; ok {
			return "", "", fmt.Errorf("native task parent cycle at %q", task.Key)
		}
		seen[task.Key] = struct{}{}
		if taskID == "" {
			taskID = task.Key
		}
		if task.ParentKey == "" {
			return taskID, task.Key, nil
		}
		current = task.ParentKey
	}
}

func setGoal(ctx context.Context, reader nativeTaskReader, selectGoal func(agent, key string, activate func() (func(), error)) error, proxy taskAttributionProxy, iteration, agent, key string) (map[string]any, error) {
	if selectGoal == nil || proxy == nil {
		return nil, errGoalUnavailable
	}
	taskID, epicID, err := resolveNativeTaskAttribution(ctx, reader, agent, key)
	if err != nil {
		return nil, err
	}
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
	return map[string]any{"task_id": taskID, "epic_id": epicID, "updated": updated}, nil
}
