package plugincaps

import (
	"reflect"
	"strings"
	"testing"

	storeassets "github.com/alekzonder/tariboy/store"
)

func TestFragmentsContract(t *testing.T) {
	orders := map[int]bool{}
	for _, f := range fragments {
		if len(f.Teaches) == 0 {
			t.Errorf("%s teaches nothing", f.Name)
		}
		for _, cmd := range f.Teaches {
			if strings.TrimSpace(cmd) == "" {
				t.Errorf("%s has an empty Teaches entry", f.Name)
			}
		}
		if !known(f.Plugin) {
			t.Errorf("%s references unknown plugin %q", f.Name, f.Plugin)
		}
		if orders[f.Order] {
			t.Errorf("duplicate Order %d", f.Order)
		}
		orders[f.Order] = true
	}
}

func TestSchemaV1FragmentsResolveDirectSkillInstructions(t *testing.T) {
	for _, fragment := range fragments {
		if fragment.Tail {
			if fragment.Path != "prompts/iteration-finish.md" {
				t.Fatalf("finish fragment path = %q", fragment.Path)
			}
			continue
		}
		if want := "skills/" + fragment.Plugin + "/SKILL.md"; fragment.Path != want {
			t.Errorf("%s path = %q, want %q", fragment.Plugin, fragment.Path, want)
		}
		for _, command := range fragment.Teaches {
			if strings.HasPrefix(command, "tools ") {
				t.Errorf("%s retains dispatcher command %q", fragment.Plugin, command)
			}
		}
	}
}

func TestResolve(t *testing.T) {
	got, err := Resolve([]string{"context", "status"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"whoami", "loop", "messages", "context", "status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve = %v, want %v", got, want)
	}
	// dedupe: requesting a CORE plugin does not duplicate it
	got, _ = Resolve([]string{"whoami", "context"})
	if !reflect.DeepEqual(got, []string{"whoami", "loop", "messages", "context"}) {
		t.Fatalf("dedupe failed: %v", got)
	}
	if _, err := Resolve([]string{"nope"}); err == nil {
		t.Fatal("unknown plugin accepted")
	}
	if _, err := Resolve([]string{"be" + "ads"}); err == nil {
		t.Fatal("retired plugin accepted")
	}
}

func TestWorkdirIsV2InstructionPluginOnly(t *testing.T) {
	if _, err := Resolve([]string{"workdir"}); err == nil {
		t.Fatal("schema-v1 resolution accepted workdir")
	}
	got, err := ValidateExplicit([]string{"workdir"}, nil)
	if err != nil {
		t.Fatalf("ValidateExplicit(workdir): %v", err)
	}
	if !reflect.DeepEqual(got, []string{"workdir"}) {
		t.Fatalf("ValidateExplicit(workdir) = %v", got)
	}
	if IsOptional("workdir") {
		t.Fatal("instruction-only workdir reported as an optional capability")
	}
}

func TestBodyAndTailFragments(t *testing.T) {
	set, _ := Resolve([]string{"context"})
	body := BodyFragments(set)
	var names []string
	for _, f := range body {
		if f.Tail {
			t.Fatalf("BodyFragments returned a tail fragment: %s", f.Name)
		}
		names = append(names, f.Plugin)
	}
	if !reflect.DeepEqual(names, []string{"whoami", "messages", "context"}) {
		t.Fatalf("body order = %v", names)
	}
	tail := TailFragments(set)
	if len(tail) != 1 || tail[0].Name != "system:i-am-done" || !tail[0].Tail {
		t.Fatalf("tail = %+v", tail)
	}
}

func TestIAmDoneTailDocumentsIdle(t *testing.T) {
	// The loop's tail is what every agent actually receives; it must teach the
	// --idle self-report so idle iterations can drive the auto-stop policy.
	tail := TailFragments([]string{"loop"})
	if len(tail) != 1 || tail[0].Name != "system:i-am-done" {
		t.Fatalf("expected the i-am-done tail, got %+v", tail)
	}
	body := tail[0].Body
	for _, want := range []string{"i-am-done --idle", "productive", "idle"} {
		if !strings.Contains(body, want) {
			t.Fatalf("i-am-done tail missing %q; body:\n%s", want, body)
		}
	}
}

func TestIAmDoneTailReservesCompletionForRootOwner(t *testing.T) {
	// A child agent shares the same iteration prompt, but it must hand its
	// result to its parent rather than closing the root iteration itself.
	tail := TailFragments([]string{"loop"})
	if len(tail) != 1 || tail[0].Name != "system:i-am-done" {
		t.Fatalf("expected the i-am-done tail, got %+v", tail)
	}
	body := tail[0].Body
	normalizedBody := strings.Join(strings.Fields(body), " ")
	for _, want := range []string{
		"Only the root iteration owner may run `i-am-done`",
		"(with or without `--idle`)",
		"A subagent must never run `i-am-done`",
		"return its result to its parent",
		"This remains true even if the parent is unavailable, the subagent has no active work, or the task asks it to finish.",
	} {
		if !strings.Contains(normalizedBody, want) {
			t.Fatalf("i-am-done tail missing %q; body:\n%s", want, body)
		}
	}
}

func TestIsOptional(t *testing.T) {
	if IsOptional("whoami") {
		t.Fatal("core plugin reported optional")
	}
	if !IsOptional("context") {
		t.Fatal("context should be optional")
	}
}

func TestLLMAsJudgeCapability(t *testing.T) {
	resolved, err := Resolve([]string{"llm-as-judge"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !IsOptional("llm-as-judge") || !reflect.DeepEqual(resolved[len(resolved)-1:], []string{"llm-as-judge"}) {
		t.Fatalf("judge capability not resolved: %v", resolved)
	}
	var body string
	for _, f := range BodyFragments(resolved) {
		if f.Plugin == "llm-as-judge" {
			body = f.Body
		}
	}
	for _, command := range []string{"scripts/judge.sh", "evidence", "proposal"} {
		if !strings.Contains(body, command) {
			t.Fatalf("judge prompt missing %q: %s", command, body)
		}
	}
}

func TestImageCreatorCapability(t *testing.T) {
	if !IsOptional("image-creator") {
		t.Fatal("image-creator must be an OPTIONAL capability")
	}
	resolved, err := Resolve([]string{"image-creator"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	found := false
	for _, n := range resolved {
		if n == "image-creator" {
			found = true
		}
	}
	if !found {
		t.Fatalf("image-creator missing from resolved set %v", resolved)
	}
	// It must contribute a SYSTEM (body) fragment teaching the build tool.
	var body string
	for _, f := range BodyFragments(resolved) {
		if f.Plugin == "image-creator" {
			body = f.Body
		}
	}
	if body == "" {
		t.Fatal("image-creator has no system fragment")
	}
	if !strings.Contains(body, "scripts/image_creator.sh build") {
		t.Fatalf("image-creator fragment must teach the build tool, got:\n%s", body)
	}
}

func TestScriptsPromptTeachesExplicitRunAndSchedule(t *testing.T) {
	resolved, err := Resolve([]string{"scripts"})
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, fragment := range BodyFragments(resolved) {
		if fragment.Plugin == "scripts" {
			body = fragment.Body
		}
	}
	if !strings.Contains(body, "scripts/scripts.sh") {
		t.Fatalf("scripts compatibility instructions omit the direct launcher:\n%s", body)
	}
	skill, err := storeassets.ReadBundled("skills/scripts/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"scripts/scripts.sh run", "scripts/scripts.sh schedule", "never overlap", "--quiet-exit", "Queue it exactly once"} {
		if !strings.Contains(string(skill), want) {
			t.Fatalf("scripts skill missing %q:\n%s", want, skill)
		}
	}
	if strings.Contains(body, "tools script") {
		t.Fatalf("scripts prompt still teaches removed add command:\n%s", body)
	}
}

func TestTasksCapabilityIsOptionalAndContributesItsOwnPrompt(t *testing.T) {
	if !IsOptional("tasks") {
		t.Fatal("tasks must be an OPTIONAL built-in capability")
	}
	resolved, err := Resolve([]string{"tasks"})
	if err != nil {
		t.Fatal(err)
	}
	var fragment *Fragment
	for i := range BodyFragments(resolved) {
		candidate := BodyFragments(resolved)[i]
		if candidate.Plugin == "tasks" {
			fragment = &candidate
			break
		}
	}
	if fragment == nil || fragment.Name != "system:tasks" {
		t.Fatalf("tasks fragment = %#v", fragment)
	}
	skill, err := storeassets.ReadBundled("skills/tasks/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"scripts/tasks.sh mine", "scripts/tasks.sh ready", "scripts/tasks.sh create", "scripts/tasks.sh comment",
		"scripts/tasks.sh ask", "scripts/tasks.sh done", "scripts/tasks.sh work next", "scripts/tasks.sh work show",
		"scripts/tasks.sh observe",
	} {
		if !strings.Contains(fragment.Body+string(skill), command) {
			t.Fatalf("tasks instructions missing %q", command)
		}
	}
	without, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range BodyFragments(without) {
		if candidate.Plugin == "tasks" {
			t.Fatal("tasks prompt appears when capability is disabled")
		}
	}
}

func TestTasksPromptDistinguishesFlexibleAndWorkflowQuestions(t *testing.T) {
	body, err := storeassets.ReadBundled("skills/tasks/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	normalized := strings.Join(strings.Fields(string(body)), " ")
	for _, want := range []string{
		"For a flexible task",
		"scripts/tasks.sh ask <key> user:<login>|agent:<name> <text>",
		"A comment is not a blocking question",
		"For workflow-managed work",
		"Treat its packet as the complete authority",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("tasks skill missing %q:\n%s", want, body)
		}
	}
}

func TestCurrentTaskCapabilityUsesNativeTasksContract(t *testing.T) {
	resolved, err := Resolve([]string{"current-task"})
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, fragment := range BodyFragments(resolved) {
		if fragment.Plugin == "current-task" {
			body = fragment.Body
			break
		}
	}
	for _, want := range []string{"Current task", "scripts/current_task.sh KEY", "--clear"} {
		if !strings.Contains(body, want) {
			t.Fatalf("current-task prompt missing %q:\n%s", want, body)
		}
	}
	retired := "be" + "ads"
	if strings.Contains(strings.ToLower(body), retired) || strings.Contains(body, "bd ready") {
		t.Fatalf("current-task prompt still teaches a retired capability:\n%s", body)
	}
}
