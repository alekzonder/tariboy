package tasks

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"
)

func TestListWorkflowAssignmentsDoesNotDeadlockSingleConnection(t *testing.T) {
	svc, operator, task := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{
		"developers": {"dev-a"},
	})
	svc.db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	items, err := svc.ListWorkflowAssignments(ctx, operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("assignments = %d, want 1", len(items))
	}
}

func TestWorkflowExecutionPreservesIndependentEmptyRequirementArrays(t *testing.T) {
	definition := claimOneDefinition()
	definition.Statuses[0].Requirements[0].Inputs = []string{}
	definition.Statuses[0].Requirements[0].Produces = []string{}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{"developers": {"dev-a"}})
	view, err := svc.GetWorkflowExecution(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Requirements) != 1 {
		t.Fatalf("requirements = %d", len(view.Requirements))
	}
	r := view.Requirements[0]
	if r.Inputs == nil || r.Produces == nil {
		t.Fatalf("decoded arrays must be non-nil: %#v", r)
	}
}

func TestTaskWorkflowSubscriptionsValidatesAssignmentTaskWhenEmpty(t *testing.T) {
	svc, operator, first := runtimeWorkflowTask(t, claimOneDefinition(), map[string][]string{"developers": {"dev-a"}})
	second, err := svc.CreateTask(context.Background(), operator, CreateTaskInput{Queue: first.Queue, Title: "second"})
	if err != nil {
		t.Fatal(err)
	}
	assignments, err := svc.ListWorkflowAssignments(context.Background(), operator, second.Key)
	if err != nil || len(assignments) != 1 {
		t.Fatalf("second assignments=%v err=%v", assignments, err)
	}
	if _, err := svc.ListTaskWorkflowSubscriptions(context.Background(), operator, first.Key, strconv.FormatInt(assignments[0].ID, 10)); ErrorCode(err) != "workflow_assignment_not_found" {
		t.Fatalf("cross-task empty subscriptions err=%v", err)
	}
}

func packetWorkflowTask(t *testing.T) (*Service, Task, Assignment) {
	t.Helper()
	definition := WorkflowDefinition{
		Name: "packet-contract", Version: 1, InitialStatus: "implement",
		Permissions: WorkflowPermissions{
			Tools:    []string{"git.status", "tasks.artifacts.add"},
			Channels: WorkflowChannelPermissions{Subscribe: []string{"logs:${task.artifacts.service_id}"}},
		},
		Statuses: []WorkflowStatus{
			{
				ID: "implement", Instructions: "produce a bounded implementation",
				Requirements: []WorkflowRequirement{{
					ID: "implementation", Pool: "developers", Dispatch: DispatchClaimOne,
					Inputs: []string{"requirements", "service_id"}, Produces: []string{"implementation", "sibling_secret"},
					Outcomes: []string{"completed"},
				}},
				Transitions: []WorkflowTransition{{When: "implementation.completed", To: "review"}},
			},
			{
				ID: "review", Instructions: "verify the implementation",
				Requirements: []WorkflowRequirement{{
					ID: "review", Pool: "reviewers", Dispatch: DispatchClaimOne,
					Inputs: []string{"previous_outputs", "requirements"}, Produces: []string{"review"},
					Outcomes: []string{"approved", "changes_requested"},
				}},
				Transitions: []WorkflowTransition{{When: "review.approved", To: "done"}, {When: "review.changes_requested", To: "implement"}},
			},
			{ID: "done", Terminal: true},
		},
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"developers": {"dev-a"}, "reviewers": {"review-a"},
	})
	work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("next work = %#v, err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "claim-packet", IterationID: "iter-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, task, claimed
}

func insertPacketArtifact(t *testing.T, svc *Service, task Task, assignmentID sql.NullInt64, name, content string) {
	t.Helper()
	now := svc.now()
	if _, err := svc.db.Exec(`
		INSERT INTO task_artifacts(task_id, assignment_id, name, type, content, metadata,
		                           revision, created_by, created_at, updated_at)
		VALUES (?, ?, ?, 'markdown', ?, '{"delivery_token":"must-not-leak"}', 1, 'system', ?, ?)`,
		task.ID, assignmentID, name, content, now, now); err != nil {
		t.Fatal(err)
	}
}

func TestWorkPacketContainsOnlyExactInputsAndNoDeliveryMetadata(t *testing.T) {
	svc, task, assignment := packetWorkflowTask(t)
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "requirements", "public requirement")
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "unrelated", "hidden sibling")

	packet, err := svc.GetWorkPacket(context.Background(), AgentActor("dev-a"), assignmentID(assignment))
	if err != nil {
		t.Fatal(err)
	}
	if packet.TaskKey != task.Key || packet.TaskRevision != task.WorkflowRevision || packet.Assignment.ID != assignment.ID {
		t.Fatalf("packet identity/revisions = %#v", packet)
	}
	if packet.Goal != "runtime work" || packet.Status != "implement" || packet.StatusInstructions != "produce a bounded implementation" {
		t.Fatalf("packet goal/status = %#v", packet)
	}
	if packet.Requirement.ID != "implementation" || len(packet.Inputs) != 1 || packet.Inputs[0].Name != "requirements" {
		t.Fatalf("packet requirement/inputs = %#v/%#v", packet.Requirement, packet.Inputs)
	}
	if packet.Inputs[0].Metadata != nil {
		t.Fatalf("packet leaked raw artifact metadata: %#v", packet.Inputs[0].Metadata)
	}
	if len(packet.AllowedOutcomes) != 1 || packet.AllowedOutcomes[0] != "completed" || len(packet.AllowedActions) == 0 {
		t.Fatalf("packet completion contract = %#v/%#v", packet.AllowedOutcomes, packet.AllowedActions)
	}
	if len(packet.AllowedTools) != 2 || len(packet.AllowedChannelSubscriptions) != 1 {
		t.Fatalf("packet permissions = %#v/%#v", packet.AllowedTools, packet.AllowedChannelSubscriptions)
	}
	visible, err := svc.ListArtifacts(context.Background(), AgentActor("dev-a"), task.Key, assignmentID(assignment))
	if err != nil || len(visible) != 1 || visible[0].Name != "requirements" {
		t.Fatalf("agent-visible artifacts = %#v, err=%v; want only requirements", visible, err)
	}
	if visible[0].Metadata != nil {
		t.Fatalf("agent-visible artifact leaked metadata: %#v", visible[0].Metadata)
	}
	operatorVisible, err := svc.ListArtifacts(context.Background(), CustomerActor("customer"), task.Key, "")
	if err != nil || len(operatorVisible) != 2 || operatorVisible[0].Metadata["delivery_token"] != "must-not-leak" {
		t.Fatalf("operator artifacts = %#v, err=%v; want retained metadata", operatorVisible, err)
	}
}

func TestArtifactReadsAreAssignmentScopedAndDoNotDeadlock(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "packet-siblings", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{
				ID: "work", Join: "require_all",
				Requirements: []WorkflowRequirement{
					{ID: "a", Pool: "a", Dispatch: DispatchClaimOne, Inputs: []string{"input_a"}, Outcomes: []string{"done"}},
					{ID: "b", Pool: "b", Dispatch: DispatchClaimOne, Inputs: []string{"input_b"}, Outcomes: []string{"done"}},
				},
				Transitions: []WorkflowTransition{{When: "a.done && b.done", To: "done"}},
			},
			{ID: "done", Terminal: true},
		},
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{"a": {"same-agent"}, "b": {"same-agent"}})
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "input_a", "visible a")
	var oldInputAID int64
	if err := svc.db.QueryRow(`SELECT id FROM task_artifacts WHERE task_id = ? AND name = 'input_a'`, task.ID).Scan(&oldInputAID); err != nil {
		t.Fatal(err)
	}
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "input_a", "latest visible a")
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "input_b", "hidden sibling b")
	work, err := svc.NextWork(context.Background(), AgentActor("same-agent"), "DEV", 10)
	if err != nil || len(work) != 2 {
		t.Fatalf("sibling work = %#v, err=%v", work, err)
	}
	var assignmentA Assignment
	for _, candidate := range work {
		current, loadErr := assignmentContextByID(context.Background(), svc.db, candidate.ID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if current.RequirementID == "a" {
			assignmentA = candidate
		}
	}
	if assignmentA.ID == 0 {
		t.Fatal("assignment a not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	visible, err := svc.ListArtifacts(ctx, AgentActor("same-agent"), task.Key, assignmentID(assignmentA))
	if err != nil || len(visible) != 1 || visible[0].Name != "input_a" {
		t.Fatalf("assignment-scoped list = %#v, err=%v; want only input_a without timeout", visible, err)
	}
	if _, err := svc.GetArtifact(ctx, AgentActor("same-agent"), task.Key, assignmentID(assignmentA), visible[0].ID); err != nil {
		t.Fatalf("assignment-scoped get: %v", err)
	}
	if _, err := svc.GetArtifact(ctx, AgentActor("same-agent"), task.Key, assignmentID(assignmentA), oldInputAID); ErrorCode(err) != "artifact_not_found" {
		t.Fatalf("historical GetArtifact error = %v; want artifact_not_found", err)
	}
	var inputBID int64
	if err := svc.db.QueryRow(`SELECT id FROM task_artifacts WHERE task_id = ? AND name = 'input_b'`, task.ID).Scan(&inputBID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetArtifact(ctx, AgentActor("same-agent"), task.Key, assignmentID(assignmentA), inputBID); ErrorCode(err) != "artifact_not_found" {
		t.Fatalf("sibling GetArtifact error = %v; want artifact_not_found", err)
	}
	if _, err := svc.ListArtifacts(ctx, AgentActor("same-agent"), task.Key, ""); ErrorCode(err) != "invalid_assignment_id" {
		t.Fatalf("unscoped agent list error = %v; want invalid_assignment_id", err)
	}
}

func TestPreviousOutputsExpandsToExactArtifactAllowlist(t *testing.T) {
	svc, task, assignment := packetWorkflowTask(t)
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "requirements", "requirements")
	insertPacketArtifact(t, svc, task, sql.NullInt64{Int64: assignment.ID, Valid: true}, "implementation", "diff ref")
	insertPacketArtifact(t, svc, task, sql.NullInt64{Int64: assignment.ID, Valid: true}, "sibling_secret", "visible because explicitly previous output")
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "unrelated", "never visible")

	completed, err := svc.CompleteAssignment(context.Background(), AgentActor("dev-a"), assignmentID(assignment), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision,
		Outcome: "completed", IdempotencyKey: "complete-for-review",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = completed
	reviewWork, err := svc.NextWork(context.Background(), AgentActor("review-a"), "DEV", 1)
	if err != nil || len(reviewWork) != 1 {
		t.Fatalf("review work = %#v, err=%v", reviewWork, err)
	}
	review, err := svc.ClaimAssignment(context.Background(), AgentActor("review-a"), assignmentID(reviewWork[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: reviewWork[0].Revision, IdempotencyKey: "claim-review",
	})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := svc.GetWorkPacket(context.Background(), AgentActor("review-a"), assignmentID(review))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(packet.Inputs))
	for _, artifact := range packet.Inputs {
		got = append(got, artifact.Name)
	}
	want := []string{"implementation", "requirements", "sibling_secret"}
	if len(got) != len(want) {
		t.Fatalf("input names = %#v; want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("input names = %#v; want %#v", got, want)
		}
	}
}

func TestArtifactRevisionConflictReturnsFreshWorkPacket(t *testing.T) {
	svc, task, assignment := packetWorkflowTask(t)
	_, err := svc.AddArtifact(context.Background(), AgentActor("dev-a"), assignmentID(assignment), AddArtifactInput{
		TaskRevision: task.WorkflowRevision + 99, AssignmentRevision: assignment.Revision,
		Name: "implementation", Type: ArtifactMarkdown, Content: "diff",
		IdempotencyKey: "stale-add",
	})
	if ErrorCode(err) != "revision_conflict" {
		t.Fatalf("stale add error = %v; want revision_conflict", err)
	}
	typed, ok := err.(*Error)
	if !ok {
		t.Fatalf("stale add error type = %T", err)
	}
	packet, ok := typed.Data["work_packet"].(WorkPacket)
	if !ok || packet.TaskRevision != task.WorkflowRevision || packet.Assignment.Revision != assignment.Revision {
		t.Fatalf("fresh work packet = %#v", typed.Data["work_packet"])
	}
}

func TestExpiredLeaseCannotReadWorkPacketOrArtifactsBeforeReconciliation(t *testing.T) {
	svc, task, assignment := packetWorkflowTask(t)
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "requirements", "visible only during lease")
	var artifactID int64
	if err := svc.db.QueryRow(`
		SELECT id FROM task_artifacts WHERE task_id = ? AND name = 'requirements'`, task.ID).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	expired := svc.clock().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := svc.db.Exec(`UPDATE task_assignments SET lease_expires_at = ? WHERE id = ?`, expired, assignment.ID); err != nil {
		t.Fatal(err)
	}
	actor := AgentActor("dev-a")
	if _, err := svc.GetWorkPacket(context.Background(), actor, assignmentID(assignment)); ErrorCode(err) != "assignment_lease_expired" {
		t.Fatalf("expired work packet error = %v; want assignment_lease_expired", err)
	}
	if _, err := svc.ListArtifacts(context.Background(), actor, task.Key, assignmentID(assignment)); ErrorCode(err) != "assignment_lease_expired" {
		t.Fatalf("expired artifact list error = %v; want assignment_lease_expired", err)
	}
	if _, err := svc.GetArtifact(context.Background(), actor, task.Key, assignmentID(assignment), artifactID); ErrorCode(err) != "assignment_lease_expired" {
		t.Fatalf("expired artifact get error = %v; want assignment_lease_expired", err)
	}
	if _, err := svc.GetWorkPacket(context.Background(), CustomerActor("customer"), assignmentID(assignment)); err != nil {
		t.Fatalf("operator work packet after expiry: %v", err)
	}
	if got, err := svc.ListArtifacts(context.Background(), CustomerActor("customer"), task.Key, ""); err != nil || len(got) != 1 {
		t.Fatalf("operator artifacts after expiry = %#v, err=%v", got, err)
	}
}
