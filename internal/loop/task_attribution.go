package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/alekzonder/tariboy/internal/tasks"
)

type nativeTaskReader interface {
	GetTask(context.Context, tasks.Actor, string) (tasks.TaskDetail, error)
}

type taskAttributionProxy interface {
	UpdateTask(key, taskID, epicID string) int
}

func resolveNativeTaskAttribution(ctx context.Context, reader nativeTaskReader, agentName, key string) (string, string, error) {
	if reader == nil {
		return "", "", fmt.Errorf("native tasks unavailable")
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

func setCurrentTaskAttribution(ctx context.Context, reader nativeTaskReader, proxy taskAttributionProxy, iteration, agentName, key string, clear bool) (map[string]any, error) {
	if proxy == nil {
		return nil, fmt.Errorf("AI proxy unavailable")
	}
	if clear {
		updated := proxy.UpdateTask(iteration, "", "")
		return map[string]any{"task_id": "", "epic_id": "", "cleared": true, "updated": updated}, nil
	}

	taskID, epicID, err := resolveNativeTaskAttribution(ctx, reader, agentName, key)
	if err != nil {
		return nil, err
	}
	updated := proxy.UpdateTask(iteration, taskID, epicID)
	return map[string]any{"task_id": taskID, "epic_id": epicID, "updated": updated}, nil
}
