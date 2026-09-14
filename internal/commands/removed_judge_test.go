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
