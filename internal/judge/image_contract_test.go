package judge

import (
	"os"
	"strings"
	"testing"
)

func TestBuiltinJudgeImageDeclaresAutomaticCycleContract(t *testing.T) {
	manifest, err := os.ReadFile("../../store/images/llm-as-judge/Tariboyfile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	instructions, err := os.ReadFile("../../store/images/llm-as-judge/instructions.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"name: schedule", "name: tasks", "name: current-task"} {
		if !strings.Contains(string(manifest), required) {
			t.Fatalf("manifest missing %q", required)
		}
	}
	for _, required := range []string{"judge.review.requested", "judge automation begin", "exactly two configured workers", "@user:"} {
		if !strings.Contains(string(instructions), required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
}
