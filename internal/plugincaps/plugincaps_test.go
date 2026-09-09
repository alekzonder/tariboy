package plugincaps

import (
	"reflect"
	"strings"
	"testing"
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
}

func TestTasksCapabilityIsOptionalAndContributesItsOwnPrompt(t *testing.T) {
	if !IsOptional("tasks") {
		t.Fatal("tasks must be an OPTIONAL built-in capability")
	}
	resolved, err := Resolve([]string{"tasks"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range resolved {
		if candidate == "tasks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tasks missing from resolved set %v", resolved)
	}
	without, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range without {
		if candidate == "tasks" {
			t.Fatal("tasks prompt appears when capability is disabled")
		}
	}
}

func TestGoalCapabilityReplacesCurrentTask(t *testing.T) {
	if _, err := ValidateExplicit([]string{"goal"}, nil); err != nil {
		t.Fatalf("goal plugin validation: %v", err)
	}
	if _, err := ValidateExplicit([]string{"current-task"}, nil); err == nil {
		t.Fatal("current-task plugin still validates")
	}
	if !IsOptional("goal") {
		t.Fatal("goal is not a capability")
	}
}
