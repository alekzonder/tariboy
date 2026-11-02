package loop

import (
	"errors"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// IterationMode names how an agent works this iteration. The prompt states it
// so an image reads the process instead of guessing it from the Goal block.
type IterationMode string

const (
	// IterationModeWorkflow: the goal task follows a workflow.
	IterationModeWorkflow IterationMode = "workflow"
	// IterationModeFlexible: the goal task has no workflow.
	IterationModeFlexible IterationMode = "flexible"
	// IterationModeRequest: no goal task is selected.
	IterationModeRequest IterationMode = "request"
)

// ErrIterationModeNotSet rejects a prompt built without a known mode, so a
// caller that forgets the mode fails instead of telling the agent a wrong one.
var ErrIterationModeNotSet = errors.New("iteration mode is not set")

// IterationModeFor derives the mode from the selected goal task row alone;
// whether its workflow data loads does not change it.
func IterationModeFor(task tasks.Task, hasGoal bool) IterationMode {
	switch {
	case !hasGoal:
		return IterationModeRequest
	case task.WorkflowDigest != "":
		return IterationModeWorkflow
	default:
		return IterationModeFlexible
	}
}

var iterationModeText = map[IterationMode]string{
	IterationModeWorkflow: "The goal task follows a workflow: the status instructions in the Goal section set the process.",
	IterationModeFlexible: "The goal task has no workflow: your image instructions set the process.",
	IterationModeRequest:  "No goal task is selected: work from the one-shot instruction or the messages.",
}

func formatIterationMode(mode IterationMode) (string, error) {
	text, ok := iterationModeText[mode]
	if !ok {
		return "", ErrIterationModeNotSet
	}
	return "Iteration mode: `" + string(mode) + "`. " + text, nil
}
