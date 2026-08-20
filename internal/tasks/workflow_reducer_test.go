package tasks

import (
	"context"
	"encoding/json"
	"testing"
)

func completeNextAssignment(
	t *testing.T,
	svc *Service,
	agent string,
	taskRevision int64,
	outcome string,
	key string,
) Assignment {
	t.Helper()
	work, err := svc.NextWork(context.Background(), AgentActor(agent), "DEV", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(work) == 0 {
		t.Fatalf("no next work for %s", agent)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor(agent), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: taskRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: key + "-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := svc.CompleteAssignment(context.Background(), AgentActor(agent), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: taskRevision, AssignmentRevision: claimed.Revision,
		Outcome: outcome, IdempotencyKey: key + "-complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	return completed
}

func TestCompletionRequiresOutputsFromCurrentAssignment(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "required-output", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{ID: "work", Requirements: []WorkflowRequirement{{
				ID: "work", Pool: "workers", Dispatch: DispatchClaimOne,
				Produces: []string{"result"}, Outcomes: []string{"completed"},
			}}, Transitions: []WorkflowTransition{{When: "work.completed", To: "done"}}},
			{ID: "done", Terminal: true},
		},
	}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{"workers": {"worker-a"}})
	work, err := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("work = %#v, err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("worker-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "required-output-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: "completed", IdempotencyKey: "required-output-missing",
	}); ErrorCode(err) != "missing_artifact" {
		t.Fatalf("completion without output error = %v; want missing_artifact", err)
	}
	var state string
	if err := svc.db.QueryRow(`SELECT state FROM task_assignments WHERE id = ?`, claimed.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != AssignmentLeased {
		t.Fatalf("assignment state after rejected completion = %q; want leased", state)
	}
	if _, err := svc.AddArtifact(context.Background(), AgentActor("worker-a"), assignmentID(claimed), AddArtifactInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Name: "result", Type: ArtifactMarkdown, Content: "current result", IdempotencyKey: "required-output-add",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: "completed", IdempotencyKey: "required-output-complete",
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil || detail.Task.WorkflowStatus != "done" {
		t.Fatalf("task after output completion = %#v, err=%v", detail.Task, err)
	}
}

func TestReentryDoesNotReuseArtifactFromEarlierExecution(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "output-reentry", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{ID: "work", Requirements: []WorkflowRequirement{{
				ID: "work", Pool: "workers", Dispatch: DispatchClaimOne,
				Produces: []string{"result"}, Outcomes: []string{"again", "done"},
			}}, Transitions: []WorkflowTransition{{When: "work.again", To: "work"}, {When: "work.done", To: "done"}}},
			{ID: "done", Terminal: true},
		},
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{"workers": {"worker-a"}})
	work, _ := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 1)
	first, err := svc.ClaimAssignment(context.Background(), AgentActor("worker-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "reentry-first-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddArtifact(context.Background(), AgentActor("worker-a"), assignmentID(first), AddArtifactInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: first.Revision,
		Name: "result", Type: ArtifactMarkdown, Content: "old", IdempotencyKey: "reentry-old-artifact",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(first), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: first.Revision,
		Outcome: "again", IdempotencyKey: "reentry-first-complete",
	}); err != nil {
		t.Fatal(err)
	}
	secondWork, _ := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 1)
	second, err := svc.ClaimAssignment(context.Background(), AgentActor("worker-a"), assignmentID(secondWork[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: secondWork[0].Revision, IdempotencyKey: "reentry-second-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(second), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: second.Revision,
		Outcome: "done", IdempotencyKey: "reentry-second-missing",
	}); ErrorCode(err) != "missing_artifact" {
		t.Fatalf("reentry completion using old artifact error = %v; want missing_artifact", err)
	}
}

func TestRequireAllUsesFrozenPoolSnapshotAndOptionalRequirementsDoNotBlock(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "join", Version: 1, InitialStatus: "verify",
		Statuses: []WorkflowStatus{
			{
				ID: "verify", Join: "require_all",
				Requirements: []WorkflowRequirement{
					{ID: "qa", Pool: "qa", Dispatch: DispatchRequireAll, Outcomes: []string{"passed", "failed"}},
					{ID: "observer", Pool: "observers", Dispatch: DispatchClaimOne, Outcomes: []string{"noted"}, Optional: true},
				},
				Transitions: []WorkflowTransition{
					{When: "qa.all(passed)", To: "done"},
					{When: "qa.any(failed)", To: "failed"},
				},
			},
			{ID: "done", Terminal: true},
			{ID: "failed", Terminal: true},
		},
	}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"qa": {"qa-a", "qa-b"}, "observers": {"observer-a"},
	})
	observerWork, err := svc.NextWork(context.Background(), AgentActor("observer-a"), "DEV", 10)
	if err != nil || len(observerWork) != 1 {
		t.Fatalf("observer work = %#v, err=%v; want one optional assignment", observerWork, err)
	}
	observerLease, err := svc.ClaimAssignment(context.Background(), AgentActor("observer-a"), assignmentID(observerWork[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: observerWork[0].Revision,
		IdempotencyKey: "claim-observer",
	})
	if err != nil {
		t.Fatal(err)
	}
	completeNextAssignment(t, svc, "qa-a", task.WorkflowRevision, "passed", "qa-a")
	if _, err := svc.RebindAgentPool(context.Background(), operator, "DEV", "qa", []string{"qa-c"}, 1, "rebind-live-qa"); err == nil || ErrorCode(err) != "agent_not_found" {
		// Add the future member and retry. The failed rebind proves no fixture
		// accidentally created qa-c before the status snapshot.
		if ErrorCode(err) != "agent_not_found" {
			t.Fatalf("pre-create qa-c rebind error = %v; want agent_not_found", err)
		}
	}
	if _, err := svc.db.Exec(`INSERT INTO agents(name, image_ref, image_digest) VALUES ('qa-c', 'basic:latest', 'digest')`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebindAgentPool(context.Background(), operator, "DEV", "qa", []string{"qa-c"}, 1, "rebind-live-qa-2"); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.WorkflowStatus != "verify" {
		t.Fatalf("status after first frozen member = %q; want verify", detail.Task.WorkflowStatus)
	}
	completeNextAssignment(t, svc, "qa-b", task.WorkflowRevision, "passed", "qa-b")
	detail, err = svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.WorkflowStatus != "done" {
		t.Fatalf("status after frozen members = %q; want done", detail.Task.WorkflowStatus)
	}
	if work, err := svc.NextWork(context.Background(), AgentActor("observer-a"), "DEV", 10); err != nil || len(work) != 0 {
		t.Fatalf("optional work after transition = %#v, err=%v; want none", work, err)
	}
	var observerState string
	if err := svc.db.QueryRow(`SELECT state FROM task_assignments WHERE id = ?`, observerLease.ID).Scan(&observerState); err != nil {
		t.Fatal(err)
	}
	if observerState != AssignmentReleased {
		t.Fatalf("optional assignment state = %q; want released on transition", observerState)
	}
}

func workflowErrorDefinition(multiple bool) WorkflowDefinition {
	secondGuard := "work.no"
	if multiple {
		secondGuard = "work.yes || task.priority == P2"
	}
	return WorkflowDefinition{
		Name: "workflow-error", Version: 1, InitialStatus: "decide",
		Statuses: []WorkflowStatus{
			{
				ID: "decide", Join: "require_all",
				Requirements: []WorkflowRequirement{
					{ID: "work", Pool: "workers", Dispatch: DispatchClaimOne, Outcomes: []string{"yes", "no", "maybe"}},
					{ID: "manager", Pool: "managers", Dispatch: DispatchClaimOne, Outcomes: []string{"acknowledged"}, Optional: true},
				},
				Transitions: []WorkflowTransition{
					{When: "work.yes", To: "done"},
					{When: secondGuard, To: "failed"},
				},
			},
			{ID: "done", Terminal: true},
			{ID: "failed", Terminal: true},
		},
		Questions: WorkflowQuestionPolicy{RouteTo: "managers"},
	}
}

func assertFrozenEscalation(t *testing.T, svc *Service, task Task, wantCode string, outcome string) {
	t.Helper()
	managerWork, err := svc.NextWork(context.Background(), AgentActor("manager-a"), "DEV", 10)
	if err != nil || len(managerWork) != 1 {
		t.Fatalf("manager optional work = %#v, err=%v; want one", managerWork, err)
	}
	managerLease, err := svc.ClaimAssignment(context.Background(), AgentActor("manager-a"), assignmentID(managerWork[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: managerWork[0].Revision,
		IdempotencyKey: wantCode + "-manager-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	work, err := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 10)
	if err != nil || len(work) == 0 {
		t.Fatalf("worker work = %#v, err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("worker-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: wantCode + "-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: outcome, IdempotencyKey: wantCode + "-complete",
	})
	if ErrorCode(err) != wantCode {
		t.Fatalf("completion error = %v; want %s", err, wantCode)
	}
	var state string
	if err := svc.db.QueryRow(`
		SELECT state FROM task_status_executions
		WHERE task_id = ? ORDER BY sequence DESC LIMIT 1`, task.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "frozen" {
		t.Fatalf("execution state = %q; want frozen", state)
	}
	var kind, payloadJSON string
	if err := svc.db.QueryRow(`
		SELECT kind, payload FROM task_workflow_outbox
		WHERE task_id = ? AND kind = 'workflow.escalated'
		ORDER BY rowid DESC LIMIT 1`, task.ID).Scan(&kind, &payloadJSON); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if kind != "workflow.escalated" || payload["agent"] != "manager-a" || payload["error_code"] != wantCode {
		t.Fatalf("escalation = %q %#v; want manager/error routing", kind, payload)
	}
	if managerWork, err := svc.NextWork(context.Background(), AgentActor("manager-a"), "DEV", 10); err != nil || len(managerWork) != 0 {
		t.Fatalf("manager work in frozen execution = %#v, err=%v; want none", managerWork, err)
	}
	var frozenState, owner, deadline string
	if err := svc.db.QueryRow(`
		SELECT state, lease_owner, lease_expires_at FROM task_assignments WHERE id = ?`, managerLease.ID).Scan(
		&frozenState, &owner, &deadline,
	); err != nil {
		t.Fatal(err)
	}
	if frozenState != AssignmentFailed || owner != "" || deadline != "" {
		t.Fatalf("frozen lease state/owner/deadline = %q/%q/%q; want failed/empty/empty", frozenState, owner, deadline)
	}
}

func TestReducerFreezesAndEscalatesWhenNoTransitionMatches(t *testing.T) {
	svc, _, task := runtimeWorkflowTask(t, workflowErrorDefinition(false), map[string][]string{
		"workers": {"worker-a"}, "managers": {"manager-a"},
	})
	assertFrozenEscalation(t, svc, task, "workflow_transition_missing", "maybe")
}

func TestReducerFreezesAndEscalatesWhenMultipleTransitionsMatch(t *testing.T) {
	svc, _, task := runtimeWorkflowTask(t, workflowErrorDefinition(true), map[string][]string{
		"workers": {"worker-a"}, "managers": {"manager-a"},
	})
	assertFrozenEscalation(t, svc, task, "workflow_transition_ambiguous", "yes")
}

func TestStatusReentryCreatesNewExecution(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "reentry", Version: 1, InitialStatus: "implement",
		Statuses: []WorkflowStatus{
			{
				ID: "implement", Requirements: []WorkflowRequirement{{
					ID: "implementation", Pool: "developers", Dispatch: DispatchClaimOne, Outcomes: []string{"completed"},
				}},
				Transitions: []WorkflowTransition{{When: "implementation.completed", To: "review"}},
			},
			{
				ID: "review", Requirements: []WorkflowRequirement{{
					ID: "review", Pool: "reviewers", Dispatch: DispatchClaimOne, Outcomes: []string{"approved", "changes"},
				}},
				Transitions: []WorkflowTransition{
					{When: "review.approved", To: "done"},
					{When: "review.changes", To: "implement"},
				},
			},
			{ID: "done", Terminal: true},
		},
	}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"developers": {"dev-a"}, "reviewers": {"reviewer-a"},
	})
	completeNextAssignment(t, svc, "dev-a", task.WorkflowRevision, "completed", "implementation-1")
	afterImplement, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	var wakePayload string
	if err := svc.db.QueryRow(`
		SELECT payload FROM task_workflow_outbox
		WHERE task_id = ? AND kind = 'workflow.assignment_ready'
		ORDER BY rowid DESC LIMIT 1`, task.ID).Scan(&wakePayload); err != nil {
		t.Fatal(err)
	}
	var wake map[string]any
	if err := json.Unmarshal([]byte(wakePayload), &wake); err != nil {
		t.Fatal(err)
	}
	if wake["pool"] != "reviewers" || wake["agent"] != "reviewer-a" {
		t.Fatalf("transition wake = %#v; want reviewer pool routing", wake)
	}
	completeNextAssignment(t, svc, "reviewer-a", afterImplement.Task.WorkflowRevision, "changes", "review-1")
	afterReview, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if afterReview.Task.WorkflowStatus != "implement" || afterReview.Task.WorkflowRevision != 3 {
		t.Fatalf("reentered task = %#v; want implement revision 3", afterReview.Task)
	}
	rows, err := svc.db.Query(`
		SELECT status_id, sequence, state FROM task_status_executions
		WHERE task_id = ? ORDER BY sequence`, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type execution struct {
		status   string
		sequence int
		state    string
	}
	var executions []execution
	for rows.Next() {
		var got execution
		if err := rows.Scan(&got.status, &got.sequence, &got.state); err != nil {
			t.Fatal(err)
		}
		executions = append(executions, got)
	}
	if len(executions) != 3 || executions[0] != (execution{"implement", 1, "transitioned"}) ||
		executions[1] != (execution{"review", 2, "transitioned"}) ||
		executions[2] != (execution{"implement", 3, "active"}) {
		t.Fatalf("status executions = %#v; want immutable implement/review/implement history", executions)
	}
}

func TestReducerEvaluatesDerivedTaskBlockedGuard(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "blocked-guard", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{
				ID: "work", Requirements: []WorkflowRequirement{{
					ID: "work", Pool: "workers", Dispatch: DispatchClaimOne, Outcomes: []string{"completed"},
				}},
				Transitions: []WorkflowTransition{
					{When: "work.completed && task.blocked == true", To: "failed"},
					{When: "work.completed && task.blocked == false", To: "done"},
				},
			},
			{ID: "done", Terminal: true},
			{ID: "failed", Terminal: true},
		},
	}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"workers": {"worker-a"},
	})
	blocker, err := svc.CreateTask(context.Background(), operator, CreateTaskInput{
		Queue: "DEV", Title: "active blocker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddRelation(context.Background(), operator, blocker.Key, RelationInput{
		TargetKey: task.Key, Type: "blocks", Revision: blocker.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	completeNextAssignment(t, svc, "worker-a", task.WorkflowRevision, "completed", "blocked-work")
	detail, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.WorkflowStatus != "failed" {
		t.Fatalf("blocked guard status = %q; want failed", detail.Task.WorkflowStatus)
	}
}

func TestCycleBudgetUsesDeclaredExhaustedStatus(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "cycles", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{
				ID: "work", Requirements: []WorkflowRequirement{{
					ID: "work", Pool: "workers", Dispatch: DispatchClaimOne, Outcomes: []string{"again", "finished"},
				}},
				Transitions: []WorkflowTransition{
					{When: "work.again", To: "work"},
					{When: "work.finished", To: "done"},
				},
			},
			{ID: "done", Terminal: true},
		},
		Budgets: WorkflowBudgetPolicy{MaxCycles: 1, OnExhausted: "done"},
	}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"workers": {"worker-a"},
	})
	completeNextAssignment(t, svc, "worker-a", task.WorkflowRevision, "again", "cycle-1")
	detail, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.WorkflowStatus != "done" || detail.Task.WorkflowRevision != 2 {
		t.Fatalf("cycle-exhausted task = %#v; want done revision 2", detail.Task)
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_status_executions WHERE task_id = ?`, task.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("cycle execution count = %d; want source plus exhausted terminal", count)
	}
}

func TestAssignmentBudgetFreezesBeforeCreatingWorkInAnotherExecution(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "assignment-budget", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{
				ID: "work", Requirements: []WorkflowRequirement{{
					ID: "work", Pool: "workers", Dispatch: DispatchClaimOne, Outcomes: []string{"again", "finished"},
				}},
				Transitions: []WorkflowTransition{
					{When: "work.again", To: "work"},
					{When: "work.finished", To: "done"},
				},
			},
			{ID: "done", Terminal: true},
		},
		Budgets: WorkflowBudgetPolicy{MaxAssignments: 1},
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"workers": {"worker-a"},
	})
	work, err := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("budget work = %#v, err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("worker-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision,
		IdempotencyKey: "budget-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(claimed), CompleteAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: claimed.Revision,
		Outcome: "again", IdempotencyKey: "budget-complete",
	})
	if ErrorCode(err) != "workflow_assignment_budget_exhausted" {
		t.Fatalf("assignment budget completion = %#v, err=%v; want persisted exhaustion", completed, err)
	}
	var assignments, executions int
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		WHERE se.task_id = ?`, task.ID).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_status_executions WHERE task_id = ?`, task.ID).Scan(&executions); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := svc.db.QueryRow(`SELECT state FROM task_status_executions WHERE task_id = ?`, task.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if assignments != 1 || executions != 1 || state != "frozen" {
		t.Fatalf("assignment budget rows/executions/state = %d/%d/%q; want 1/1/frozen", assignments, executions, state)
	}
}

func TestAssignmentBudgetUsesDeclaredExhaustedStatus(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "assignment-budget-route", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{
				ID: "work", Requirements: []WorkflowRequirement{{
					ID: "work", Pool: "workers", Dispatch: DispatchClaimOne, Outcomes: []string{"again", "stop"},
				}},
				Transitions: []WorkflowTransition{
					{When: "work.again", To: "work"},
					{When: "work.stop", To: "failed"},
				},
			},
			{ID: "failed", Terminal: true},
		},
		Budgets: WorkflowBudgetPolicy{MaxAssignments: 1, OnExhausted: "failed"},
	}
	svc, operator, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"workers": {"worker-a"},
	})
	completeNextAssignment(t, svc, "worker-a", task.WorkflowRevision, "again", "budget-route")
	detail, err := svc.GetTask(context.Background(), operator, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.WorkflowStatus != "failed" || detail.Task.Status != StatusDone || detail.Task.WorkflowRevision != 2 {
		t.Fatalf("assignment-exhausted task = %#v; want failed/done workflow revision 2", detail.Task)
	}
	var assignments, executions int
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		WHERE se.task_id = ?`, task.ID).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_status_executions WHERE task_id = ?`, task.ID).Scan(&executions); err != nil {
		t.Fatal(err)
	}
	if assignments != 1 || executions != 2 {
		t.Fatalf("assignment-exhausted rows/executions = %d/%d; want 1/2", assignments, executions)
	}
}

func TestInitialAssignmentBudgetPreflightCreatesNoReadyWakes(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "initial-assignment-budget", Version: 1, InitialStatus: "verify",
		Statuses: []WorkflowStatus{
			{
				ID: "verify", Requirements: []WorkflowRequirement{{
					ID: "verification", Pool: "workers", Dispatch: DispatchRequireAll,
					Outcomes: []string{"passed"},
				}},
				Transitions: []WorkflowTransition{{When: "verification.all(passed)", To: "done"}},
			},
			{ID: "done", Terminal: true},
		},
		Budgets: WorkflowBudgetPolicy{MaxAssignments: 1},
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"workers": {"worker-a", "worker-b"},
	})
	var assignments, readyWakes, escalationWakes int
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FROM task_assignments a
		JOIN task_requirement_executions re ON re.id = a.requirement_execution_id
		JOIN task_status_executions se ON se.id = re.status_execution_id
		WHERE se.task_id = ?`, task.ID).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`
		SELECT COUNT(*) FILTER (WHERE kind = 'workflow.assignment_ready'),
		       COUNT(*) FILTER (WHERE kind = 'workflow.escalated')
		FROM task_workflow_outbox WHERE task_id = ?`, task.ID).Scan(&readyWakes, &escalationWakes); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := svc.db.QueryRow(`SELECT state FROM task_status_executions WHERE task_id = ?`, task.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if assignments != 0 || readyWakes != 0 || escalationWakes != 1 || state != "frozen" {
		t.Fatalf("initial budget assignments/ready/escalated/state = %d/%d/%d/%q; want 0/0/1/frozen",
			assignments, readyWakes, escalationWakes, state)
	}
}

func TestInitialAssignmentBudgetTransitionRefreshesCreateResultReplayAndEvent(t *testing.T) {
	definition := WorkflowDefinition{
		Name: "initial-budget-transition", Version: 1, InitialStatus: "verify",
		Statuses: []WorkflowStatus{
			{
				ID: "verify", Requirements: []WorkflowRequirement{{
					ID: "verification", Pool: "developers", Dispatch: DispatchRequireAll,
					Outcomes: []string{"passed", "failed"},
				}},
				Transitions: []WorkflowTransition{
					{When: "verification.all(passed)", To: "done"},
					{When: "verification.any(failed)", To: "failed"},
				},
			},
			{ID: "done", Terminal: true},
			{ID: "failed", Terminal: true},
		},
		Budgets: WorkflowBudgetPolicy{MaxAssignments: 1, OnExhausted: "failed"},
	}
	svc, operator := workflowFixture(t)
	mustRebindPool(t, svc, operator, "developers", []string{"dev-1", "dev-2"}, 0)
	draft, err := svc.CreateWorkflowDraft(context.Background(), operator, definition)
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.PublishWorkflowVersion(context.Background(), operator, draft.Name, draft.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateQueueWorkflow(
		context.Background(), operator, "DEV", published.ID, 0, "activate-initial-budget-transition",
	); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTask(context.Background(), operator, CreateTaskInput{
		Queue: "DEV", Title: "initial budget transition", IdempotencyKey: "create-initial-budget-transition",
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.CreateTask(context.Background(), operator, CreateTaskInput{
		Queue: "DEV", Title: "ignored replay payload", IdempotencyKey: "create-initial-budget-transition",
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetTask(context.Background(), operator, created.Key)
	if err != nil {
		t.Fatal(err)
	}
	for name, task := range map[string]Task{
		"create result": created,
		"replay":        replayed,
		"persisted":     detail.Task,
	} {
		if task.Status != StatusDone || task.WorkflowStatus != "failed" ||
			task.Revision != 2 || task.WorkflowRevision != 2 || task.CompletedAt == "" {
			t.Fatalf("%s = %#v; want terminal failed at task/workflow revision 2", name, task)
		}
	}
	events, err := svc.ListEvents(context.Background(), operator, created.Key, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Kind != "task.created" || events[1].Kind != "workflow.transitioned" {
		t.Fatalf("creation events = %#v; want task.created then workflow.transitioned", events)
	}
	if events[0].TaskRevision != 1 || events[0].Payload["workflow_status"] != "verify" ||
		events[1].TaskRevision != 2 || events[1].Payload["to"] != "failed" {
		t.Fatalf("creation event revisions/payload = %#v; want initial revision 1 then failed revision 2", events)
	}
}
