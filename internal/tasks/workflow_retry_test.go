package tasks

import (
	"context"
	"testing"
)

// failApprove requests "approve" as reviewer-1 and fails its first check.
func failApprove(t *testing.T, svc *Service, actor Actor, task Task) TransitionRequest {
	t.Helper()
	request, err := svc.Advance(context.Background(), AgentActor("reviewer-1"), task.Key,
		AdvanceInput{Outcome: "approve", Message: "looks good"})
	if err != nil {
		t.Fatal(err)
	}
	run := listRuns(t, svc, actor, task)[0]
	complete(t, svc, run.ID, RunCompletion{Verdict: "failure", Message: "GitHub answered HTTP 401", ExitCode: exitCode(1)})
	return request
}

func TestRetryTransitionRequestRunsTheChecksAgain(t *testing.T) {
	svc, actor, task, _ := runFixture(t)
	ctx := context.Background()
	failed := failApprove(t, svc, actor, task)

	retried, err := svc.RetryTransitionRequest(ctx, actor, task.Key, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID == failed.ID || retried.State != "pending" || retried.Outcome != "approve" ||
		retried.Message != "looks good" || retried.Actor != "agent:reviewer-1" || retried.WaitSeconds == 0 {
		t.Fatalf("retried = %#v", retried)
	}
	if state, _ := requestRow(t, svc, task, failed.ID); state != "failed" {
		t.Fatalf("the failed request is now %s", state)
	}
	runs := listRuns(t, svc, actor, task)
	if len(runs) != 2 || runs[0].Script != "checks/ci.sh" || runs[0].State != "pending" {
		t.Fatalf("runs = %#v", runs)
	}
	requested := eventPayload(t, svc, task, "workflow.transition_requested")
	if requested["request_id"] != float64(retried.ID) || requested["retry_of"] != float64(failed.ID) ||
		requested["actor"] != "agent:reviewer-1" || requested["retried_by"] != actor.Principal {
		t.Fatalf("requested = %v", requested)
	}
	// The retry goes on like any request: passing checks apply the transition.
	complete(t, svc, runs[0].ID, RunCompletion{Verdict: "pass", ExitCode: exitCode(0)})
	second := listRuns(t, svc, actor, task)[0]
	complete(t, svc, second.ID, RunCompletion{Verdict: "pass", ExitCode: exitCode(0)})
	if state, _ := requestRow(t, svc, task, retried.ID); state != "applied" {
		t.Fatalf("retried request = %s", state)
	}
	if got := storedTask(t, svc, task.Key); got.WorkflowStatus == "review" {
		t.Fatalf("status = %q", got.WorkflowStatus)
	}
}

func TestRetryTransitionRequestRefusals(t *testing.T) {
	ctx := context.Background()
	t.Run("agent", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		failed := failApprove(t, svc, actor, task)
		if _, err := svc.RetryTransitionRequest(ctx, AgentActor("reviewer-1"), task.Key, failed.ID); ErrorCode(err) != "forbidden" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		if _, err := svc.RetryTransitionRequest(ctx, actor, task.Key, 999); ErrorCode(err) != "not_found" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("pending", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		request, _ := advanceApprove(t, svc, actor, task)
		if _, err := svc.RetryTransitionRequest(ctx, actor, task.Key, request.ID); ErrorCode(err) != "request_not_failed" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("rejected", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		request, run := advanceApprove(t, svc, actor, task)
		complete(t, svc, run.ID, RunCompletion{Verdict: "reject", Message: "red", ExitCode: exitCode(112)})
		if _, err := svc.RetryTransitionRequest(ctx, actor, task.Key, request.ID); ErrorCode(err) != "request_not_failed" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("superseded", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		older := failApprove(t, svc, actor, task)
		failApprove(t, svc, actor, task)
		if _, err := svc.RetryTransitionRequest(ctx, actor, task.Key, older.ID); ErrorCode(err) != "request_superseded" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("status changed", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		failed := failApprove(t, svc, actor, task)
		if _, err := svc.MoveWorkflow(ctx, actor, task.Key, "develop", "rework"); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.RetryTransitionRequest(ctx, actor, task.Key, failed.ID); ErrorCode(err) != "request_superseded" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("paused", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		failed := failApprove(t, svc, actor, task)
		forceTask(t, svc, task, `workflow_paused_reason = 'script_failures'`)
		if _, err := svc.RetryTransitionRequest(ctx, actor, task.Key, failed.ID); ErrorCode(err) != "workflow_paused" {
			t.Fatalf("err = %v", err)
		}
	})
}
