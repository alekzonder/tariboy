package loop

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// chooseIterationTask picks the one task an iteration works from the agent's
// Goal and the tasks of its open letters, in render order ("" is a letter
// about no task): the Goal while it has letters or no letter is open,
// otherwise the task of the first letter.
func chooseIterationTask(goal string, hasGoal bool, keys []string) string {
	if len(keys) == 0 || (hasGoal && slices.Contains(keys, goal)) {
		return goal
	}
	return keys[0]
}

// chooseTask sets the iteration task of in from the agent's open letters, and
// the Goal block for it: a task the agent may work as a Goal is shown as one;
// another task makes a request iteration about that task. When the letters
// cannot be read, the iteration keeps the Goal and shows every letter.
func (r *ShimRunner) chooseTask(ag agent.Agent, in *prepareInputs, at time.Time) {
	if in.hasCurrentGoal {
		in.taskKey = in.currentGoal.Key
	}
	if r.cfg.Mail == nil {
		return
	}
	if r.cfg.CloseStaleTaskLetters != nil {
		if _, err := r.cfg.CloseStaleTaskLetters(context.Background(), ag.Name); err != nil {
			r.cfg.Logger.Warn("close stale task letters", "agent", ag.Name, "err", err)
		}
	}
	keys, err := r.cfg.Mail.OpenTaskKeys(ag.Name)
	if err != nil {
		r.cfg.Logger.Warn("read letter tasks", "agent", ag.Name, "err", err)
		return
	}
	in.lettersByTask = true
	key := chooseIterationTask(in.taskKey, in.hasCurrentGoal, keys)
	if key == in.taskKey {
		return
	}
	in.taskKey, in.currentGoal, in.hasCurrentGoal = key, tasks.Task{}, false
	if key == "" {
		return
	}
	if r.cfg.GoalCandidate != nil {
		task, ok, err := r.cfg.GoalCandidate(ag.Name, key, at)
		if err != nil {
			r.cfg.Logger.Warn("read letter task as a goal", "agent", ag.Name, "task", key, "err", err)
		}
		in.currentGoal, in.hasCurrentGoal = task, ok && err == nil
	}
	in.letterTaskOnly = !in.hasCurrentGoal
}

// letterTaskGoal is the Goal block of a request iteration whose letters are
// about a task the agent does not work as a Goal.
func letterTaskGoal(key string) string {
	return "No goal selected for this iteration. Letters of this iteration are about task `" + key + "`.\n\n" +
		strings.TrimPrefix(FormatRuntimeGoalGuidance(), "# Agent Goal\n\n")
}

// wakeForOtherTasks asks for the next message iteration when a done iteration
// left open letters only about other tasks. While a letter of its own task is
// open (an unanswered request), that task would come first again, so no wake.
func (r *ShimRunner) wakeForOtherTasks(agentName, taskKey string) {
	if r.cfg.WakeMessages == nil || r.cfg.Mail == nil {
		return
	}
	keys, err := r.cfg.Mail.OpenTaskKeys(agentName)
	if err != nil {
		r.cfg.Logger.Warn("read letter tasks", "agent", agentName, "err", err)
		return
	}
	if len(keys) > 0 && !slices.Contains(keys, taskKey) {
		r.cfg.WakeMessages(agentName)
	}
}
