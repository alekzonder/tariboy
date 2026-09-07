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
