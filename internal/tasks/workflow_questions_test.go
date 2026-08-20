package tasks

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"
)

func questionWorkflowTask(t *testing.T, maxOpen int, timeout string) (*Service, Task, Assignment) {
	return questionWorkflowTaskWithRetries(t, maxOpen, timeout, WorkflowRetryPolicy{})
}

func questionWorkflowTaskWithRetries(t *testing.T, maxOpen int, timeout string, retries WorkflowRetryPolicy) (*Service, Task, Assignment) {
	t.Helper()
	definition := WorkflowDefinition{
		Name: "questions", Version: 1, InitialStatus: "investigate",
		Statuses: []WorkflowStatus{
			{ID: "investigate", Requirements: []WorkflowRequirement{
				{ID: "work", Pool: "workers", Dispatch: DispatchClaimOne, Inputs: []string{"runbook"}, Outcomes: []string{"done"}},
				{ID: "manager", Pool: "managers", Dispatch: DispatchClaimOne, Outcomes: []string{"ack"}, Optional: true},
			}, Transitions: []WorkflowTransition{{When: "work.done", To: "done"}}},
			{ID: "done", Terminal: true},
		},
		Questions: WorkflowQuestionPolicy{RouteTo: "managers", AllowedHolds: []string{HoldAssignment, HoldRequirement}, MaxOpenPerAssignment: maxOpen, Timeout: timeout},
		Retries:   retries,
	}
	if problems := ValidateWorkflow(definition); len(problems) != 0 {
		t.Fatalf("question workflow invalid: %#v", problems)
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"workers": {"worker-a"}, "managers": {"manager-a", "manager-b"},
	})
	work, err := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("work=%#v err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("worker-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "question-worker-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, task, claimed
}

func askQuestion(t *testing.T, svc *Service, task Task, assignment Assignment, scope, key string) WorkflowQuestion {
	t.Helper()
	q, err := svc.AskWorkflowQuestion(context.Background(), AgentActor("worker-a"), assignmentID(assignment), AskWorkflowQuestionInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision,
		Question: "Which retry limit?", Context: "The runbook has no retry limit.", BlockingScope: scope,
		Anchor: "runbook#retry", Options: []string{"3", "5"}, SuggestedAnswer: "3",
		ArtifactAttachments: []int64{}, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestWorkflowQuestionRequiresUniversalEnvelopeAndAllowedHold(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "30m")
	base := AskWorkflowQuestionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Question: "q", Context: "c", BlockingScope: HoldNone, IdempotencyKey: "valid"}
	for name, edit := range map[string]func(*AskWorkflowQuestionInput){
		"question": func(in *AskWorkflowQuestionInput) { in.Question = "" },
		"context":  func(in *AskWorkflowQuestionInput) { in.Context = "" },
		"scope":    func(in *AskWorkflowQuestionInput) { in.BlockingScope = "task" },
	} {
		t.Run(name, func(t *testing.T) {
			in := base
			edit(&in)
			if _, err := svc.AskWorkflowQuestion(context.Background(), AgentActor("worker-a"), assignmentID(assignment), in); ErrorCode(err) == "" {
				t.Fatalf("expected typed error, got %v", err)
			}
		})
	}

	q := askQuestion(t, svc, task, assignment, HoldAssignment, "blocking")
	if q.BlockingScope != HoldAssignment || q.Anchor == "" || len(q.Options) != 2 {
		t.Fatalf("question=%#v", q)
	}
	var holds int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_holds WHERE question_id = ? AND released_at = ''`, q.ID).Scan(&holds); err != nil || holds != 1 {
		t.Fatalf("holds=%d err=%v", holds, err)
	}
	var status string
	if err := svc.db.QueryRow(`SELECT workflow_status FROM tasks WHERE id = ?`, task.ID).Scan(&status); err != nil || status != "investigate" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	packet, err := svc.GetWorkPacket(context.Background(), AgentActor("worker-a"), assignmentID(assignment))
	if err != nil || len(packet.Holds) != 1 || packet.Holds[0].QuestionID != q.ID {
		t.Fatalf("held packet=%#v err=%v", packet, err)
	}
	if work, err := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 10); err != nil || len(work) != 0 {
		t.Fatalf("held next work=%#v err=%v", work, err)
	}
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(assignment), CompleteAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, Outcome: "done", IdempotencyKey: "held-complete"}); ErrorCode(err) != "workflow_assignment_held" {
		t.Fatalf("complete held err=%v", err)
	}
}

func TestWorkflowQuestionRoutesToOneManagerAndAnswerResumesPacket(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "30m")
	q := askQuestion(t, svc, task, assignment, HoldRequirement, "ask-route")

	managerA, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: 1, IdempotencyKey: "manager-a-claim"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-b"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: 1, IdempotencyKey: "manager-b-claim"}); ErrorCode(err) != "assignment_already_claimed" {
		t.Fatalf("second manager claim=%v", err)
	}
	packet, err := svc.GetWorkPacket(context.Background(), AgentActor("manager-a"), assignmentID(managerA))
	if err != nil || len(packet.Questions) != 1 || packet.Questions[0].ID != q.ID || !containsString(packet.AllowedActions, "answer") {
		t.Fatalf("manager packet=%#v err=%v", packet, err)
	}

	// All question, hold, and lease state is durable; a fresh service can resume it.
	svc = NewService(svc.db, "customer", svc.clock)
	answered, err := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: managerA.Revision, Answer: "Use 3 retries.", IdempotencyKey: "answer-once"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: -1, AssignmentRevision: -1, Answer: "different", IdempotencyKey: "answer-once"})
	if err != nil || replayed.Answer != answered.Answer {
		t.Fatalf("answer replay=%#v err=%v", replayed, err)
	}
	var openHolds, resumes int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_holds WHERE question_id = ? AND released_at = ''`, q.ID).Scan(&openHolds)
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_outbox WHERE task_id = ? AND assignment_id = ? AND kind = 'workflow.assignment_resumed'`, task.ID, assignment.ID).Scan(&resumes)
	if openHolds != 0 || resumes != 1 {
		t.Fatalf("open holds=%d resumes=%d", openHolds, resumes)
	}
	packet, err = svc.GetWorkPacket(context.Background(), AgentActor("worker-a"), assignmentID(assignment))
	if err != nil || len(packet.Questions) != 1 || packet.Questions[0].Answer != "Use 3 retries." {
		t.Fatalf("worker packet=%#v err=%v", packet, err)
	}
}

func TestWorkflowQuestionNonBlockingBudgetAndListing(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 1, "30m")
	q := askQuestion(t, svc, task, assignment, HoldNone, "nonblocking")
	if _, err := svc.AskWorkflowQuestion(context.Background(), AgentActor("worker-a"), assignmentID(assignment), AskWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, Question: "another", Context: "still unclear", BlockingScope: HoldNone, IdempotencyKey: "over-budget"}); ErrorCode(err) != "workflow_question_budget_exhausted" {
		t.Fatalf("budget err=%v", err)
	}
	questions, err := svc.ListWorkflowQuestions(context.Background(), AgentActor("worker-a"), task.Key, assignmentID(assignment))
	if err != nil || len(questions) != 1 || questions[0].ID != q.ID {
		t.Fatalf("questions=%#v err=%v", questions, err)
	}
	var holds int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_holds WHERE question_id = ?`, q.ID).Scan(&holds)
	if holds != 0 {
		t.Fatalf("nonblocking holds=%d", holds)
	}
}

func TestWorkflowQuestionTimeoutPreventsClaim(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "1m")
	q := askQuestion(t, svc, task, assignment, HoldAssignment, "timeout")
	created, _ := time.Parse(time.RFC3339Nano, q.CreatedAt)
	svc.clock = func() time.Time { return created.Add(2 * time.Minute) }
	_, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: 1, IdempotencyKey: "late-claim"})
	if ErrorCode(err) != "workflow_question_timeout" {
		t.Fatalf("timeout err=%v", err)
	}
	if _, replayErr := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{IdempotencyKey: "late-claim"}); ErrorCode(replayErr) != "workflow_question_timeout" {
		t.Fatalf("timeout claim replay=%v", replayErr)
	}
	var executionState string
	if err := svc.db.QueryRow(`SELECT state FROM task_status_executions WHERE task_id = ? ORDER BY sequence DESC LIMIT 1`, task.ID).Scan(&executionState); err != nil || executionState != "frozen" {
		t.Fatalf("timed out execution=%q err=%v", executionState, err)
	}
}

func TestWorkflowQuestionTimeoutPreventsLateAnswer(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "1m")
	q := askQuestion(t, svc, task, assignment, HoldAssignment, "timeout-answer")
	manager, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: 1, IdempotencyKey: "early-claim"})
	if err != nil {
		t.Fatal(err)
	}
	created, _ := time.Parse(time.RFC3339Nano, q.CreatedAt)
	svc.clock = func() time.Time { return created.Add(2 * time.Minute) }
	_, err = svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: manager.Revision, Answer: "late", IdempotencyKey: "late-answer"})
	if ErrorCode(err) != "workflow_question_timeout" {
		t.Fatalf("late answer err=%v", err)
	}
	if _, replayErr := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), AnswerWorkflowQuestionInput{IdempotencyKey: "late-answer"}); ErrorCode(replayErr) != "workflow_question_timeout" {
		t.Fatalf("timeout answer replay=%v", replayErr)
	}
}

func TestWorkflowQuestionPersistsOnlyVisibleArtifactAttachments(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "30m")
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "runbook", "visible")
	insertPacketArtifact(t, svc, task, sql.NullInt64{}, "secret", "hidden")
	var visibleID, hiddenID int64
	if err := svc.db.QueryRow(`SELECT id FROM task_artifacts WHERE task_id = ? AND name = 'runbook'`, task.ID).Scan(&visibleID); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT id FROM task_artifacts WHERE task_id = ? AND name = 'secret'`, task.ID).Scan(&hiddenID); err != nil {
		t.Fatal(err)
	}
	in := AskWorkflowQuestionInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision, Question: "See runbook?", Context: "The attached line is ambiguous.", BlockingScope: HoldNone, ArtifactAttachments: []int64{visibleID}, IdempotencyKey: "visible-attachment"}
	q, err := svc.AskWorkflowQuestion(context.Background(), AgentActor("worker-a"), assignmentID(assignment), in)
	if err != nil || len(q.ArtifactAttachments) != 1 || q.ArtifactAttachments[0] != visibleID {
		t.Fatalf("question=%#v err=%v", q, err)
	}
	listed, err := svc.ListWorkflowQuestions(context.Background(), AgentActor("worker-a"), task.Key, assignmentID(assignment))
	if err != nil || len(listed) != 1 || len(listed[0].ArtifactAttachments) != 1 {
		t.Fatalf("listed=%#v err=%v", listed, err)
	}
	manager, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: 1, IdempotencyKey: "attachment-manager-claim"})
	if err != nil {
		t.Fatal(err)
	}
	managerPacket, err := svc.GetWorkPacket(context.Background(), AgentActor("manager-a"), assignmentID(manager))
	if err != nil || len(managerPacket.Inputs) != 1 || managerPacket.Inputs[0].ID != visibleID || managerPacket.Inputs[0].Metadata != nil {
		t.Fatalf("manager packet=%#v err=%v", managerPacket, err)
	}
	in.TaskRevision, in.AssignmentRevision, in.ArtifactAttachments, in.IdempotencyKey = task.WorkflowRevision+1, assignment.Revision+1, []int64{hiddenID}, "hidden-attachment"
	if _, err := svc.AskWorkflowQuestion(context.Background(), AgentActor("worker-a"), assignmentID(assignment), in); ErrorCode(err) != "invalid_artifact_attachment" {
		t.Fatalf("hidden attachment err=%v", err)
	}
}

func TestWorkflowHoldRequirementBlocksEveryAssignment(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "30m")
	_ = askQuestion(t, svc, task, assignment, HoldRequirement, "hold-requirement")
	var requirementID int64
	if err := svc.db.QueryRow(`SELECT requirement_execution_id FROM task_assignments WHERE id = ?`, assignment.ID).Scan(&requirementID); err != nil {
		t.Fatal(err)
	}
	var holds int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_holds WHERE requirement_execution_id = ? AND assignment_id IS NULL AND released_at = ''`, requirementID).Scan(&holds); err != nil || holds != 1 {
		t.Fatalf("requirement holds=%d err=%v", holds, err)
	}
}

func TestWorkflowHoldCannotBeBypassedByReleaseOrLeaseExpiry(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "2h")
	q := askQuestion(t, svc, task, assignment, HoldAssignment, "pause-lease")
	if _, err := svc.ReleaseAssignment(context.Background(), AgentActor("worker-a"), assignmentID(assignment), ReleaseAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, IdempotencyKey: "release-held"}); ErrorCode(err) != "workflow_assignment_held" {
		t.Fatalf("release held err=%v", err)
	}
	created, _ := time.Parse(time.RFC3339Nano, q.CreatedAt)
	svc.clock = func() time.Time { return created.Add(31 * time.Minute) }
	if expired, err := svc.ExpireLeases(context.Background(), svc.clock().UTC()); err != nil || expired != 0 {
		t.Fatalf("expired=%d err=%v", expired, err)
	}
	manager, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: 1, IdempotencyKey: "claim-after-worker-deadline"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: manager.Revision, Answer: "continue", IdempotencyKey: "resume-paused-lease"}); err != nil {
		t.Fatal(err)
	}
	work, err := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 10)
	if err != nil || len(work) != 1 || work[0].ID != assignment.ID || work[0].State != AssignmentLeased {
		t.Fatalf("resumed work=%#v err=%v", work, err)
	}
}

func TestWorkflowQuestionNonBlockingSurvivesStatusTransition(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "2h")
	q := askQuestion(t, svc, task, assignment, HoldNone, "survive-transition")
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(assignment), CompleteAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, Outcome: "done", IdempotencyKey: "transition-with-question"}); err != nil {
		t.Fatal(err)
	}
	manager, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 2, AssignmentRevision: 1, IdempotencyKey: "claim-after-transition"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 2, AssignmentRevision: manager.Revision, Answer: "late but useful", IdempotencyKey: "answer-after-transition"}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowQuestionReconcileTimesOutWithoutManagerInteractionAfterRestart(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "1m")
	q := askQuestion(t, svc, task, assignment, HoldAssignment, "reconcile-timeout")
	created, _ := time.Parse(time.RFC3339Nano, q.CreatedAt)
	restarted := NewService(svc.db, "customer", func() time.Time { return created.Add(2 * time.Minute) })
	count, err := restarted.ReconcileWorkflowQuestions(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("reconciled=%d err=%v", count, err)
	}
	var state, released string
	if err := svc.db.QueryRow(`SELECT state FROM task_workflow_questions WHERE id = ?`, q.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT released_at FROM task_workflow_holds WHERE question_id = ?`, q.ID).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if state != "timed_out" || released == "" {
		t.Fatalf("question state=%q released=%q", state, released)
	}
	if again, err := restarted.ReconcileWorkflowQuestions(context.Background()); err != nil || again != 0 {
		t.Fatalf("second reconcile=%d err=%v", again, err)
	}
}

func TestWorkflowQuestionLeastContextScopesAndRejectsHistoricalManagerWork(t *testing.T) {
	definition := WorkflowDefinition{Name: "question-scope", Version: 1, InitialStatus: "work", Statuses: []WorkflowStatus{
		{ID: "work", Join: "require_all", Requirements: []WorkflowRequirement{
			{ID: "workers", Pool: "workers", Dispatch: DispatchRequireAll, Inputs: []string{"shared"}, Outcomes: []string{"done"}},
			{ID: "manager", Pool: "managers", Dispatch: DispatchClaimOne, Outcomes: []string{"ack"}, Optional: true},
		}, Transitions: []WorkflowTransition{{When: "workers.done", To: "done"}}}, {ID: "done", Terminal: true},
	}, Questions: WorkflowQuestionPolicy{RouteTo: "managers", AllowedHolds: []string{HoldAssignment, HoldRequirement}, MaxOpenPerAssignment: 2, Timeout: "2h"}}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{"workers": {"worker-a", "worker-b"}, "managers": {"manager-a"}})
	workA, _ := svc.NextWork(context.Background(), AgentActor("worker-a"), "DEV", 10)
	workB, _ := svc.NextWork(context.Background(), AgentActor("worker-b"), "DEV", 10)
	a, _ := svc.ClaimAssignment(context.Background(), AgentActor("worker-a"), assignmentID(workA[0]), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: 1, IdempotencyKey: "scope-a-claim"})
	b, _ := svc.ClaimAssignment(context.Background(), AgentActor("worker-b"), assignmentID(workB[0]), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision, AssignmentRevision: 1, IdempotencyKey: "scope-b-claim"})
	q := askQuestion(t, svc, task, a, HoldAssignment, "scope-assignment")
	packetB, err := svc.GetWorkPacket(context.Background(), AgentActor("worker-b"), assignmentID(b))
	if err != nil || len(packetB.Questions) != 0 || len(packetB.Holds) != 0 {
		t.Fatalf("sibling packet leaked=%#v err=%v", packetB, err)
	}
	manager, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: 1, IdempotencyKey: "scope-manager-claim"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: manager.Revision, Answer: "answer", IdempotencyKey: "scope-manager-answer"}); err != nil {
		t.Fatal(err)
	}
	packetA, err := svc.GetWorkPacket(context.Background(), AgentActor("worker-a"), assignmentID(a))
	if err != nil {
		t.Fatal(err)
	}
	requirementQuestion, err := svc.AskWorkflowQuestion(context.Background(), AgentActor("worker-a"), assignmentID(a), AskWorkflowQuestionInput{TaskRevision: packetA.TaskRevision, AssignmentRevision: packetA.Assignment.Revision, Question: "Shared contract?", Context: "Both workers require the answer.", BlockingScope: HoldRequirement, IdempotencyKey: "scope-requirement"})
	if err != nil {
		t.Fatal(err)
	}
	packetB, err = svc.GetWorkPacket(context.Background(), AgentActor("worker-b"), assignmentID(b))
	if err != nil || len(packetB.Questions) != 1 || packetB.Questions[0].ID != requirementQuestion.ID || len(packetB.Holds) != 1 {
		t.Fatalf("requirement packet=%#v err=%v", packetB, err)
	}
	if _, err := svc.ListWorkflowQuestions(context.Background(), AgentActor("manager-a"), task.Key, assignmentID(manager)); ErrorCode(err) != "assignment_not_active" {
		t.Fatalf("historical manager list err=%v", err)
	}
	if _, err := svc.GetWorkPacket(context.Background(), AgentActor("manager-a"), assignmentID(manager)); ErrorCode(err) != "assignment_not_active" {
		t.Fatalf("historical manager packet err=%v", err)
	}
}

func TestWorkflowQuestionReconcileTimesOutNonBlockingQuestionAfterTransition(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 2, "1m")
	q := askQuestion(t, svc, task, assignment, HoldNone, "late-nonblocking-timeout")
	if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(assignment), CompleteAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, Outcome: "done", IdempotencyKey: "transition-before-timeout"}); err != nil {
		t.Fatal(err)
	}
	created, _ := time.Parse(time.RFC3339Nano, q.CreatedAt)
	restarted := NewService(svc.db, "customer", func() time.Time { return created.Add(2 * time.Minute) })
	if count, err := restarted.ReconcileWorkflowQuestions(context.Background()); err != nil || count != 1 {
		t.Fatalf("reconciled=%d err=%v", count, err)
	}
	var questionState, assignmentState string
	if err := svc.db.QueryRow(`SELECT state FROM task_workflow_questions WHERE id = ?`, q.ID).Scan(&questionState); err != nil {
		t.Fatal(err)
	}
	manager, _, err := questionAssignment(context.Background(), svc.db, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.db.QueryRow(`SELECT state FROM task_assignments WHERE id = ?`, manager.ID).Scan(&assignmentState); err != nil {
		t.Fatal(err)
	}
	if questionState != "timed_out" || assignmentState != AssignmentFailed {
		t.Fatalf("question/assignment=%q/%q", questionState, assignmentState)
	}
}

func TestWorkflowQuestionRetryExhaustionAfterTransitionDoesNotFreezeInactiveStatus(t *testing.T) {
	for _, mode := range []string{"release", "expiry"} {
		t.Run(mode, func(t *testing.T) {
			svc, task, assignment := questionWorkflowTaskWithRetries(t, 2, "2h", WorkflowRetryPolicy{MaxAttempts: 1})
			q := askQuestion(t, svc, task, assignment, HoldNone, "exhaust-"+mode)
			if _, err := svc.CompleteAssignment(context.Background(), AgentActor("worker-a"), assignmentID(assignment), CompleteAssignmentInput{TaskRevision: task.WorkflowRevision + 1, AssignmentRevision: assignment.Revision + 1, Outcome: "done", IdempotencyKey: "transition-" + mode}); err != nil {
				t.Fatal(err)
			}
			manager, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(q.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 2, AssignmentRevision: 1, IdempotencyKey: "claim-exhaust-" + mode})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "release" {
				if _, err := svc.ReleaseAssignment(context.Background(), AgentActor("manager-a"), assignmentID(manager), ReleaseAssignmentInput{TaskRevision: task.WorkflowRevision + 2, AssignmentRevision: manager.Revision, IdempotencyKey: "release-exhaust"}); err != nil {
					t.Fatal(err)
				}
			} else {
				deadline, _ := time.Parse(time.RFC3339Nano, manager.LeaseExpiresAt)
				if count, err := svc.ExpireLeases(context.Background(), deadline.Add(time.Second)); err != nil || count != 1 {
					t.Fatalf("expired=%d err=%v", count, err)
				}
			}
			stored, err := workflowQuestionByID(context.Background(), svc.db, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			var executionState string
			if err := svc.db.QueryRow(`SELECT state FROM task_status_executions WHERE id = (SELECT status_execution_id FROM task_requirement_executions WHERE id = ?)`, manager.RequirementExecutionID).Scan(&executionState); err != nil {
				t.Fatal(err)
			}
			if stored.State != "exhausted" || executionState != "transitioned" {
				t.Fatalf("question/execution=%q/%q", stored.State, executionState)
			}
		})
	}
}

func TestWorkflowQuestionOverlappingHoldsResumeOnlyAfterLastHold(t *testing.T) {
	svc, task, assignment := questionWorkflowTask(t, 3, "2h")
	first := askQuestion(t, svc, task, assignment, HoldAssignment, "overlap-assignment")
	packet, err := svc.GetWorkPacket(context.Background(), AgentActor("worker-a"), assignmentID(assignment))
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.AskWorkflowQuestion(context.Background(), AgentActor("worker-a"), assignmentID(assignment), AskWorkflowQuestionInput{TaskRevision: packet.TaskRevision, AssignmentRevision: packet.Assignment.Revision, Question: "Shared blocker?", Context: "This overlaps the assignment blocker.", BlockingScope: HoldRequirement, IdempotencyKey: "overlap-requirement"})
	if err != nil {
		t.Fatal(err)
	}
	manager1, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(first.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 2, AssignmentRevision: 1, IdempotencyKey: "overlap-claim-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(first.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 2, AssignmentRevision: manager1.Revision, Answer: "first", IdempotencyKey: "overlap-answer-1"}); err != nil {
		t.Fatal(err)
	}
	var resumes int
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_outbox WHERE assignment_id = ? AND kind = 'workflow.assignment_resumed'`, assignment.ID).Scan(&resumes)
	if resumes != 0 {
		t.Fatalf("premature resumes=%d", resumes)
	}
	manager2, err := svc.ClaimWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(second.ID, 10), ClaimAssignmentInput{TaskRevision: task.WorkflowRevision + 3, AssignmentRevision: 1, IdempotencyKey: "overlap-claim-2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AnswerWorkflowQuestion(context.Background(), AgentActor("manager-a"), strconv.FormatInt(second.ID, 10), AnswerWorkflowQuestionInput{TaskRevision: task.WorkflowRevision + 3, AssignmentRevision: manager2.Revision, Answer: "second", IdempotencyKey: "overlap-answer-2"}); err != nil {
		t.Fatal(err)
	}
	_ = svc.db.QueryRow(`SELECT COUNT(*) FROM task_workflow_outbox WHERE assignment_id = ? AND kind = 'workflow.assignment_resumed'`, assignment.ID).Scan(&resumes)
	if resumes != 1 {
		t.Fatalf("final resumes=%d", resumes)
	}
}
