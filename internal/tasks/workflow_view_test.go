package tasks

import (
	"context"
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

func TestGetWorkflowOnAFlexibleTask(t *testing.T) {
	svc, actor, _ := requestFixture(t)
	flexible, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "TASK", Title: "flex"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetWorkflow(context.Background(), actor, flexible.Key); ErrorCode(err) != "workflow_not_bound" {
		t.Fatalf("flexible: %v", err)
	}
	if _, err := svc.GetWorkflow(context.Background(), actor, "DEV-nope"); ErrorCode(err) != "not_found" {
		t.Fatalf("unknown: %v", err)
	}
}

func TestGetWorkflowInAPoolStatus(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	holder := AgentActor("dev-1")
	view, err := svc.GetWorkflow(ctx, holder, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Name != "development" || view.Version != "0.1.0" || view.Digest != task.WorkflowDigest ||
		view.Status != "develop" || view.Category != StatusInProgress || view.WaitingOn != "" ||
		view.Owner != "pool:developers" || view.Holder != "agent:dev-1" || view.InstructionsPath != "" ||
		view.LastRequest != nil || len(view.Artifacts) != 0 || len(view.Visits) != 1 || view.Visits[0].Status != "develop" {
		t.Fatalf("view = %#v", view)
	}
	want := []OutcomeView{
		{On: "ready", To: "review", Requires: []string{"plan", "summary"}, Missing: []string{"plan", "summary"}},
		{On: "ask", To: "approval"},
	}
	if !reflect.DeepEqual(view.Outcomes, want) {
		t.Fatalf("outcomes = %#v", view.Outcomes)
	}
	if _, err := svc.SetArtifact(ctx, holder, task.Key, "summary", "s"); err != nil {
		t.Fatal(err)
	}
	view, err = svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view.Outcomes[0].Missing, []string{"plan"}) || len(view.Artifacts) != 1 || view.Artifacts[0].Name != "summary" {
		t.Fatalf("after summary = %#v", view)
	}

	// The holder's question to the customer waits on the customer.
	if _, err := svc.AddComment(ctx, holder, task.Key, AddCommentInput{Body: "@user:customer which API?"}); err != nil {
		t.Fatal(err)
	}
	view, err = svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Category != StatusWaitCustomer || view.WaitingOn != WaitingOnCustomer || view.Holder != "agent:dev-1" {
		t.Fatalf("after question = %#v", view)
	}
}

// The checks of an outcome are also described: script, description, and the
// resolved run mode and timeout, so a reader need not know the defaults.
func TestGetWorkflowExposesCheckDetails(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	editManifest(t, svc, task.WorkflowDigest, func(m *workflowimage.Manifest) {
		m.Definition.Statuses[0].Transitions[0].Checks = []workflowfile.Check{
			{Script: "./checks/ci.sh", Description: "The CI suite passes on the branch."},
			{Script: "./checks/tree.sh", RunAs: workflowfile.RunAsAgent, Timeout: "5m"},
		}
	})
	view, err := svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view.Outcomes[0].Checks, []string{"./checks/ci.sh", "./checks/tree.sh"}) {
		t.Fatalf("checks = %#v", view.Outcomes[0].Checks)
	}
	want := []CheckView{
		{Script: "./checks/ci.sh", Description: "The CI suite passes on the branch.", RunAs: "queue", Timeout: "1m0s"},
		{Script: "./checks/tree.sh", RunAs: "agent", Timeout: "5m0s"},
	}
	if !reflect.DeepEqual(view.Outcomes[0].CheckDetails, want) {
		t.Fatalf("check_details = %#v", view.Outcomes[0].CheckDetails)
	}
	fields := jsonFields(t, view.Outcomes[0])
	details := fields["check_details"].([]any)
	first := details[0].(map[string]any)
	if first["description"] != "The CI suite passes on the branch." || first["run_as"] != "queue" || first["timeout"] != "1m0s" {
		t.Fatalf("check_details JSON = %v", details)
	}
	if _, ok := details[1].(map[string]any)["description"]; ok {
		t.Fatalf("an empty description is omitted: %v", details[1])
	}
	if _, ok := jsonFields(t, view.Outcomes[1])["check_details"]; ok {
		t.Fatalf("an outcome without checks has no check_details: %v", view.Outcomes[1])
	}
}

func TestGetWorkflowAfterTransitions(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	holder := AgentActor("dev-1")
	setPlanAndSummary(t, svc, holder, task.Key)
	request, err := svc.Advance(ctx, holder, task.Key, AdvanceInput{Outcome: "ready", Message: "go"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Owner != "pool:reviewers" || view.Holder != "agent:reviewer-1" || view.LastRequest == nil ||
		*view.LastRequest != request || len(view.Visits) != 2 || view.Visits[0].Outcome != "ready" ||
		view.Visits[1].Status != "review" || len(view.Artifacts) != 2 || view.Artifacts[0].Name != "plan" {
		t.Fatalf("view = %#v", view)
	}
	if !reflect.DeepEqual(view.Outcomes[0].Checks, []string{"checks/ci.sh"}) {
		t.Fatalf("outcomes = %#v", view.Outcomes)
	}

	enter(t, svc, task.Key, "approval", "approve")
	view, err = svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Owner != "customer" || view.Holder != "" || view.Category != StatusWaitCustomer ||
		view.WaitingOn != WaitingOnCustomer || view.InstructionsPath != "statuses/approval.md" {
		t.Fatalf("customer view = %#v", view)
	}

	enter(t, svc, task.Key, "merge", "approved")
	view, err = svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Owner != "script" || view.Holder != "" || view.WaitingOn != WaitingOnScript {
		t.Fatalf("script view = %#v", view)
	}

	enter(t, svc, task.Key, "done", "merged")
	view, err = svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Owner != "" || view.Holder != "" || view.Status != "done" || view.Category != StatusDone ||
		view.Outcomes == nil || len(view.Outcomes) != 0 || len(view.Visits) != 5 || len(view.Artifacts) != 2 ||
		view.LastRequest == nil || *view.LastRequest != request {
		t.Fatalf("terminal view = %#v", view)
	}
}

func TestGetWorkflowOfACancelledTaskOffersNoExit(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	if _, err := svc.SetArtifact(ctx, AgentActor("dev-1"), task.Key, "plan", "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelWorkflowTask(ctx, actor, task.Key); err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Owner != "" || view.Holder != "" || view.Status != "develop" || view.Category != StatusCancelled ||
		view.Outcomes == nil || len(view.Outcomes) != 0 || len(view.Artifacts) != 1 ||
		len(view.Visits) != 1 || view.Visits[0].Message != "cancelled" || view.LastRequest != nil {
		t.Fatalf("cancelled view = %#v", view)
	}
	if f := jsonFields(t, view); f["outcomes"] == nil {
		t.Fatalf("JSON outcomes = %v", f["outcomes"])
	}
}

func TestGetWorkflowListsDeclaredArtifactsAndStatuses(t *testing.T) {
	svc, actor, task := requestFixture(t)
	editManifest(t, svc, task.WorkflowDigest, func(m *workflowimage.Manifest) { m.Definition.Artifacts[0].Description = "The plan." })
	ctx := context.Background()
	wantArtifacts := []DeclaredArtifact{{Name: "plan", Description: "The plan."}, {Name: "summary"}}
	wantStatuses := []StatusView{
		{ID: "develop", Owner: "pool:developers"}, {ID: "review", Owner: "pool:reviewers"},
		{ID: "approval", Owner: "customer"}, {ID: "merge", Owner: "script"},
		{ID: "done", Terminal: true}, {ID: "dropped", Terminal: true},
	}
	// identity keeps the fields this test is about; the graph fields have
	// their own test.
	identity := func(statuses []StatusView) []StatusView {
		out := make([]StatusView, 0, len(statuses))
		for _, s := range statuses {
			out = append(out, StatusView{ID: s.ID, Owner: s.Owner, Terminal: s.Terminal})
		}
		return out
	}
	view, err := svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view.DeclaredArtifacts, wantArtifacts) || !reflect.DeepEqual(identity(view.Statuses), wantStatuses) {
		t.Fatalf("declared = %#v statuses = %#v", view.DeclaredArtifacts, view.Statuses)
	}
	f := jsonFields(t, view)
	statuses, _ := f["statuses"].([]any)
	if f["declared_artifacts"] == nil || len(statuses) != 6 ||
		!reflect.DeepEqual(statuses[4], map[string]any{"id": "done", "owner": "", "terminal": true}) {
		t.Fatalf("JSON = %v", f)
	}
	// A closed task still lists them: an operator may still move it.
	if _, err := svc.CancelWorkflowTask(ctx, actor, task.Key); err != nil {
		t.Fatal(err)
	}
	if view, err = svc.GetWorkflow(ctx, actor, task.Key); err != nil || !reflect.DeepEqual(identity(view.Statuses), wantStatuses) ||
		!reflect.DeepEqual(view.DeclaredArtifacts, wantArtifacts) {
		t.Fatalf("cancelled view = %#v, %v", view, err)
	}
}

// Every status is described in full, not only the current one: the stepper
// in Desktop shows what each step needs and which script owns it.
func TestGetWorkflowDescribesEveryStatus(t *testing.T) {
	svc, actor, task := requestFixture(t)
	ctx := context.Background()
	six := 6
	editManifest(t, svc, task.WorkflowDigest, func(m *workflowimage.Manifest) {
		m.Definition.Limits = &workflowfile.Limits{RejectedRequests: &six}
		m.Definition.Statuses[0].Instructions = "./statuses/develop.md"
		m.Definition.Statuses[0].Limits = &workflowfile.Limits{IdleIterations: &six, UnavailableGrace: "10m"}
		m.Definition.Statuses[1].Transitions[0].Checks[0].Description = "CI is green."
		m.Definition.Statuses[3].Watch = &workflowfile.Watch{Script: "./watch/merge.sh", Every: "2m", Description: "Waits for the merge."}
	})
	view, err := svc.GetWorkflow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	develop := view.Statuses[0]
	if !develop.Initial || develop.InstructionsPath != "statuses/develop.md" || develop.Watch != nil ||
		!reflect.DeepEqual(develop.Transitions, []TransitionView{
			{On: "ready", To: "review", Requires: []string{"plan", "summary"}}, {On: "ask", To: "approval"}}) ||
		!reflect.DeepEqual(develop.Limits, &LimitsView{IdleIterations: 6, RejectedRequests: 6, ScriptFailures: 3, UnavailableGrace: "10m0s"}) {
		t.Fatalf("develop = %#v", develop)
	}
	review := view.Statuses[1]
	if review.Initial || !reflect.DeepEqual(review.Transitions, []TransitionView{
		{On: "approve", To: "approval", Checks: []CheckView{{Script: "checks/ci.sh", Description: "CI is green.", RunAs: "queue", Timeout: "1m0s"}}},
		{On: "changes", To: "develop"}}) ||
		!reflect.DeepEqual(review.Limits, &LimitsView{IdleIterations: 3, RejectedRequests: 6, ScriptFailures: 3, UnavailableGrace: "5m0s"}) {
		t.Fatalf("review = %#v", review)
	}
	merge := view.Statuses[3]
	if !reflect.DeepEqual(merge.Watch, &WatchView{Script: "./watch/merge.sh", Description: "Waits for the merge.", Every: "2m", Timeout: "1m0s"}) ||
		len(merge.Transitions) != 1 || merge.Limits == nil {
		t.Fatalf("merge = %#v", merge)
	}
	if done, dropped := view.Statuses[4], view.Statuses[5]; !done.Terminal || done.Cancelled || done.Limits != nil ||
		done.Transitions != nil || !dropped.Cancelled {
		t.Fatalf("terminal = %#v %#v", done, dropped)
	}
	statuses := jsonFields(t, view)["statuses"].([]any)
	if !reflect.DeepEqual(statuses[4], map[string]any{"id": "done", "owner": "", "terminal": true}) {
		t.Fatalf("terminal JSON = %v", statuses[4])
	}
	if first := statuses[0].(map[string]any); first["initial"] != true || first["limits"] == nil || first["transitions"] == nil {
		t.Fatalf("initial JSON = %v", first)
	}
}

// A visit carries its counters, so a reader can tell how close the task is
// to a pause.
func TestGetWorkflowVisitsCarryCounters(t *testing.T) {
	svc, actor, task, _ := runFixture(t)
	request, first := advanceApprove(t, svc, actor, task)
	complete(t, svc, first.ID, RunCompletion{Verdict: "reject", Message: "CI is red", ExitCode: exitCode(112)})
	view, err := svc.GetWorkflow(context.Background(), actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	open := view.Visits[len(view.Visits)-1]
	if open.LeftAt != "" || open.RejectedRequests != 1 || open.ScriptFailures != 0 || open.IdleIterations != 0 {
		t.Fatalf("open visit = %#v", open)
	}
	if fields := jsonFields(t, open); fields["rejected_requests"] != float64(1) || fields["script_failures"] != nil {
		t.Fatalf("visit JSON = %v", fields)
	}
	// The run of a check names its request, so a failed request can be
	// paired with its log; a watch run names none.
	if run := view.Runs[0]; run.RequestID != request.ID || jsonFields(t, run)["request_id"] != float64(request.ID) {
		t.Fatalf("run = %#v", run)
	}
}

func TestGetWorkflowWithNoDeclaredArtifactsHasEmptyLists(t *testing.T) {
	svc, actor, task := requestFixture(t)
	editManifest(t, svc, task.WorkflowDigest, func(m *workflowimage.Manifest) { m.Definition.Artifacts = nil })
	view, err := svc.GetWorkflow(context.Background(), actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if f := jsonFields(t, view); f["declared_artifacts"] == nil {
		t.Fatalf("JSON declared_artifacts = %v", f["declared_artifacts"])
	}
}

func TestGetWorkflowInAStatusTheManifestDoesNotDeclare(t *testing.T) {
	svc, actor, task := requestFixture(t)
	forceTask(t, svc, task, `workflow_status = 'gone'`)
	view, err := svc.GetWorkflow(context.Background(), actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != "gone" || view.Owner != "" || view.Holder != "" || view.Outcomes == nil || len(view.Outcomes) != 0 {
		t.Fatalf("view = %#v", view)
	}
}
