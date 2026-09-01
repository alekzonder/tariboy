package judge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestBuiltinJudgeImageDeclaresAutomaticCycleContract(t *testing.T) {
	source, err := imagefile.ParseV2("../../store/images/llm-as-judge")
	if err != nil {
		t.Fatal(err)
	}
	validated, err := image.ValidateV2Detailed(source, imagefile.ResolveRoots{
		Store: filepath.Clean("../../store"), CurrentVersionStore: filepath.Clean("../../store"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(validated.Skills) != len(source.Plugins) {
		t.Fatalf("packaged skills = %d, enabled capabilities = %d", len(validated.Skills), len(source.Plugins))
	}
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
	for _, required := range []string{
		"skills/whoami", "skills/loop", "skills/messages", "skills/context",
		"skills/status", "skills/schedule", "skills/tasks", "skills/current-task",
		"skills/llm-as-judge",
	} {
		if !strings.Contains(string(manifest), required) {
			t.Errorf("manifest does not package %q", required)
		}
	}
	if strings.Contains(string(manifest), "/prompt.md") {
		t.Fatalf("manifest retains migrated prompt fragments:\n%s", manifest)
	}
	for _, required := range []string{
		"judge.review.requested", "scripts/judge.sh automation begin", "exactly two configured workers", "@user:",
		`"schema_version": 1`, `"recommendations": [{"description": "..."}]`, `"evidence_gaps": ["..."]`,
		`"locator": "exact string returned by evidence search"`,
		"scripts/judge.sh summary claim RUN", "scripts/judge.sh summary inputs RUN", "scripts/judge.sh summary submit RUN --file summary.json",
		"scripts/judge.sh improvement submit RUN --file proposal.json", `"rollback_image": "name:immutable-tag"`,
	} {
		if !strings.Contains(string(instructions), required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
}
