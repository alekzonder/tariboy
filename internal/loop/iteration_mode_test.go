package loop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestIterationModeFor(t *testing.T) {
	t.Parallel()
	workflow := tasks.Task{Key: "DEV-1", WorkflowDigest: "d1"}
	flexible := tasks.Task{Key: "FLEX-1"}
	for _, tt := range []struct {
		name    string
		task    tasks.Task
		hasGoal bool
		want    IterationMode
	}{
		{"workflow", workflow, true, IterationModeWorkflow},
		{"flexible", flexible, true, IterationModeFlexible},
		{"request", tasks.Task{}, false, IterationModeRequest},
		{"workflow task without goal", workflow, false, IterationModeRequest},
	} {
		if got := IterationModeFor(tt.task, tt.hasGoal); got != tt.want {
			t.Fatalf("%s: mode = %q, want %q", tt.name, got, tt.want)
		}
	}
}

type failingWorkflowGoals struct{}

func (failingWorkflowGoals) WorkflowGoal(context.Context, string, string) (WorkflowGoal, error) {
	return WorkflowGoal{}, errors.New("workflow store unavailable")
}

func TestIterationModeWorkflowWithoutWorkflowData(t *testing.T) {
	t.Parallel()
	task := workflowTestTask()
	for name, src := range map[string]WorkflowGoalSource{"nil source": nil, "failing source": failingWorkflowGoals{}} {
		goal := RuntimeGoal(context.Background(), src, task, "dev-1", nil)
		if !strings.Contains(goal, "workflow details could not be loaded") {
			t.Fatalf("%s: goal is not the fallback: %s", name, goal)
		}
		if got := IterationModeFor(task, true); got != IterationModeWorkflow {
			t.Fatalf("%s: mode = %q, want workflow", name, got)
		}
	}
}
