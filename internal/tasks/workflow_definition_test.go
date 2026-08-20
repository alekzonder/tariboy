package tasks

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowVersionDraftValidatePublishLifecycle(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	actor := CustomerActor("customer")
	definition := validWorkflowDefinition()

	draft, err := svc.CreateWorkflowDraft(ctx, actor, definition)
	if err != nil {
		t.Fatal(err)
	}
	if draft.State != "draft" || draft.PublishedAt != "" {
		t.Fatalf("created workflow = %#v; want unpublished draft", draft)
	}
	wantJSON, err := json.Marshal(normalizeWorkflowDefinition(definition))
	if err != nil {
		t.Fatal(err)
	}
	var storedJSON string
	if err := svc.db.QueryRow(`SELECT definition FROM task_workflow_versions WHERE id = ?`, draft.ID).Scan(&storedJSON); err != nil {
		t.Fatal(err)
	}
	if storedJSON != string(wantJSON) {
		t.Fatalf("stored definition = %s; want canonical %s", storedJSON, wantJSON)
	}

	validationErrors, err := svc.ValidateWorkflowVersion(ctx, actor, definition.Name, definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(validationErrors) != 0 {
		t.Fatalf("validation errors = %#v; want none", validationErrors)
	}

	got, err := svc.GetWorkflowVersion(ctx, actor, definition.Name, definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, draft) {
		t.Fatalf("get = %#v; want %#v", got, draft)
	}
	versions, err := svc.ListWorkflowVersions(ctx, actor, definition.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || !reflect.DeepEqual(versions[0], draft) {
		t.Fatalf("list = %#v; want %#v", versions, []WorkflowVersion{draft})
	}

	published, err := svc.PublishWorkflowVersion(ctx, actor, definition.Name, definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "published" || published.PublishedAt == "" {
		t.Fatalf("published workflow = %#v", published)
	}
	if _, err := svc.PublishWorkflowVersion(ctx, actor, definition.Name, definition.Version); ErrorCode(err) != "workflow_immutable" {
		t.Fatalf("second publish error = %v; want workflow_immutable", err)
	}

	definition.Statuses[0].Instructions = "changed after publish"
	if _, err := svc.CreateWorkflowDraft(ctx, actor, definition); ErrorCode(err) != "workflow_immutable" {
		t.Fatalf("published update error = %v; want workflow_immutable", err)
	}
	after, err := svc.GetWorkflowVersion(ctx, actor, definition.Name, definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(after.Definition, definition) {
		t.Fatal("published definition was updated")
	}
}

func TestCreateWorkflowDraftUpdatesDraftWithoutCreatingAnotherVersion(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	actor := CustomerActor("customer")
	definition := validWorkflowDefinition()
	first, err := svc.CreateWorkflowDraft(ctx, actor, definition)
	if err != nil {
		t.Fatal(err)
	}
	definition.Statuses[0].Instructions = "updated instructions"
	updated, err := svc.CreateWorkflowDraft(ctx, actor, definition)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID || updated.Definition.Statuses[0].Instructions != "updated instructions" {
		t.Fatalf("updated draft = %#v; want same row with new definition", updated)
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_versions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("workflow version rows = %d; want 1", count)
	}
}

func TestCreateWorkflowDraftStoresNormalizedIdentifiers(t *testing.T) {
	svc := newTestService(t)
	definition := validWorkflowDefinition()
	definition.Name = " development "
	definition.InitialStatus = " implement "
	definition.Statuses[0].ID = " implement "
	definition.Statuses[0].Requirements[0].ID = " implementation "
	definition.Statuses[0].Requirements[0].Pool = " developers "
	definition.Statuses[0].Requirements[0].Outcomes[0] = " completed "
	definition.Statuses[0].Transitions[0] = WorkflowTransition{When: " implementation.completed ", To: " verify "}

	got, err := svc.CreateWorkflowDraft(context.Background(), CustomerActor("customer"), definition)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "development" || got.Definition.InitialStatus != "implement" ||
		got.Definition.Statuses[0].ID != "implement" ||
		got.Definition.Statuses[0].Requirements[0].ID != "implementation" ||
		got.Definition.Statuses[0].Requirements[0].Pool != "developers" ||
		got.Definition.Statuses[0].Requirements[0].Outcomes[0] != "completed" ||
		got.Definition.Statuses[0].Transitions[0].To != "verify" ||
		got.Definition.Statuses[0].Transitions[0].When != "implementation.completed" {
		t.Fatalf("stored workflow was not normalized: %#v", got.Definition)
	}
}

func TestWorkflowDefinitionCanonicalizesRequiredCollectionsAsEmptyArrays(t *testing.T) {
	svc := newTestService(t)
	definition := validWorkflowDefinition()
	definition.Statuses[0].Requirements[0].Inputs = nil
	definition.Statuses[0].Requirements[0].Produces = nil
	definition.Statuses[len(definition.Statuses)-1].Requirements = nil
	definition.Statuses[len(definition.Statuses)-1].Transitions = nil
	got, err := svc.CreateWorkflowDraft(context.Background(), CustomerActor("customer"), definition)
	if err != nil {
		t.Fatal(err)
	}
	requirement := got.Definition.Statuses[0].Requirements[0]
	terminal := got.Definition.Statuses[len(got.Definition.Statuses)-1]
	if requirement.Inputs == nil || requirement.Produces == nil || terminal.Requirements == nil || terminal.Transitions == nil {
		t.Fatalf("required collections were not canonicalized: %#v / %#v", requirement, terminal)
	}
	raw, err := json.Marshal(got.Definition)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"inputs":null`, `"produces":null`, `"requirements":null`, `"transitions":null`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("canonical JSON contains %s: %s", forbidden, raw)
		}
	}
}

func TestLegacyWorkflowDefinitionCanonicalizesOnReadWithoutMutatingStoredJSON(t *testing.T) {
	svc := newTestService(t)
	legacy := `{"name":"legacy","version":1,"initial_status":"work","statuses":[{"id":"work","requirements":[{"id":"do","pool":"workers","dispatch":"claim_one","inputs":null,"produces":null,"outcomes":["done"]}],"transitions":[{"when":"do.done","to":"done"}]},{"id":"done","requirements":null,"transitions":null,"terminal":true}]}`
	if _, err := svc.db.Exec(`INSERT INTO task_workflow_versions(name,version,definition,state,created_at,updated_at,published_at) VALUES('legacy',1,?,'published','t','t','t')`, legacy); err != nil {
		t.Fatal(err)
	}
	actor := CustomerActor("customer")
	got, err := svc.GetWorkflowVersion(context.Background(), actor, "legacy", 1)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.ListWorkflowVersions(context.Background(), actor, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range []WorkflowDefinition{got.Definition, listed[0].Definition} {
		if definition.Statuses[0].Requirements[0].Inputs == nil || definition.Statuses[0].Requirements[0].Produces == nil || definition.Statuses[1].Requirements == nil || definition.Statuses[1].Transitions == nil {
			t.Fatalf("legacy read not canonical: %#v", definition)
		}
	}
	var stored string
	if err := svc.db.QueryRow(`SELECT definition FROM task_workflow_versions WHERE name='legacy' AND version=1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != legacy {
		t.Fatalf("read mutated immutable JSON:\n got %s\nwant %s", stored, legacy)
	}
}

func TestCreateWorkflowDraftExpandsPreviousOutputsAcrossBranchesAndCycles(t *testing.T) {
	svc := newTestService(t)
	definition := WorkflowDefinition{
		Name: "branch-cycle", Version: 1, InitialStatus: "start",
		Statuses: []WorkflowStatus{
			{ID: "start", Requirements: []WorkflowRequirement{{
				ID: "plan", Pool: "managers", Dispatch: DispatchClaimOne,
				Produces: []string{"requirements"}, Outcomes: []string{"path_a", "path_b"},
			}}, Transitions: []WorkflowTransition{
				{When: "plan.path_a", To: "implement"}, {When: "plan.path_b", To: "verify"},
			}},
			{ID: "implement", Requirements: []WorkflowRequirement{{
				ID: "implementation", Pool: "developers", Dispatch: DispatchClaimOne,
				Inputs: []string{"previous_outputs"}, Produces: []string{"implementation"}, Outcomes: []string{"completed"},
			}}, Transitions: []WorkflowTransition{{When: "implementation.completed", To: "verify"}}},
			{ID: "verify", Requirements: []WorkflowRequirement{{
				ID: "qa", Pool: "qa", Dispatch: DispatchClaimOne,
				Inputs: []string{"previous_outputs"}, Produces: []string{"test_report"}, Outcomes: []string{"passed", "failed"},
			}}, Transitions: []WorkflowTransition{
				{When: "qa.passed", To: "done"}, {When: "qa.failed", To: "implement"},
			}},
			{ID: "done", Terminal: true},
		},
	}

	got, err := svc.CreateWorkflowDraft(context.Background(), CustomerActor("customer"), definition)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"requirements", "implementation", "test_report"}
	for _, statusIndex := range []int{1, 2} {
		inputs := got.Definition.Statuses[statusIndex].Requirements[0].Inputs
		if !reflect.DeepEqual(inputs, want) {
			t.Fatalf("status %s previous outputs = %v; want %v", got.Definition.Statuses[statusIndex].ID, inputs, want)
		}
	}
}

func TestCreateWorkflowDraftRejectsInvalidDefinitionBeforeWrite(t *testing.T) {
	svc := newTestService(t)
	definition := validWorkflowDefinition()
	definition.Statuses[0].Transitions[0].To = "missing"

	if _, err := svc.CreateWorkflowDraft(context.Background(), CustomerActor("customer"), definition); ErrorCode(err) != "workflow_invalid" {
		t.Fatalf("create invalid workflow error = %v; want workflow_invalid", err)
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_versions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("workflow version rows = %d; want 0", count)
	}
}

func TestWorkflowVersionMethodsRequireCustomerAndReturnTypedNotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	definition := validWorkflowDefinition()
	if _, err := svc.CreateWorkflowDraft(ctx, AgentActor("alice"), definition); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent create error = %v; want forbidden", err)
	}
	if _, err := svc.GetWorkflowVersion(ctx, CustomerActor("customer"), "missing", 1); ErrorCode(err) != "workflow_version_not_found" {
		t.Fatalf("missing workflow error = %v; want workflow_version_not_found", err)
	}
}

func TestPublishedWorkflowUpdateReturnsImmutableBeforeCandidateValidation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	actor := CustomerActor("customer")
	definition := validWorkflowDefinition()
	if _, err := svc.CreateWorkflowDraft(ctx, actor, definition); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishWorkflowVersion(ctx, actor, definition.Name, definition.Version); err != nil {
		t.Fatal(err)
	}
	definition.InitialStatus = "missing"
	if _, err := svc.CreateWorkflowDraft(ctx, actor, definition); ErrorCode(err) != "workflow_immutable" {
		t.Fatalf("published invalid update error = %v; want workflow_immutable", err)
	}
}
