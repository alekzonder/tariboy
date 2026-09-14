package commands

import "testing"

func TestRegistryDoesNotExposeRemovedJudgeCommands(t *testing.T) {
	r := BuildRegistry()
	for _, name := range []string{"judge.ls", "judge.review", "judge.automation.get", "improvement.ls"} {
		if _, ok := r.Get(name); ok {
			t.Errorf("removed command %q is still registered", name)
		}
	}
}

func TestRegistryDoesNotExposeRemovedEvalCommandsOrRoutes(t *testing.T) {
	r := BuildRegistry()
	for _, name := range []string{"eval.ls", "eval.inspect"} {
		if _, ok := r.Get(name); ok {
			t.Errorf("removed command %q is still registered", name)
		}
	}
	for _, command := range r.Commands() {
		if command.HTTP != nil && (command.HTTP.Path == "/api/evals" || command.HTTP.Path == "/api/evals/{iteration}") {
			t.Errorf("removed route %q is still registered by %s", command.HTTP.Path, command.Path)
		}
	}
}
