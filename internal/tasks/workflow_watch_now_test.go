package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func TestRunWatchNowCreatesAPendingWatchRun(t *testing.T) {
	svc, actor, task := watchFixture(t)
	ctx := context.Background()
	// A quiet watch waits for every before its next run.
	forceNextWatchAt(t, svc, task, store.FormatTime(svc.clock().Add(10*time.Minute)))
	run, err := svc.RunWatchNow(ctx, actor, task.Key)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == 0 || run.TaskKey != task.Key || run.Kind != RunKindWatch || run.Script != "watch/merge.sh" ||
		run.RunAs != "queue" || run.State != "pending" {
		t.Fatalf("run = %#v", run)
	}
	if runs := listRuns(t, svc, actor, task); len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %#v", runs)
	}
	if status, _, _, next := openVisit(t, svc, task); status != "merge" || next != "" {
		t.Fatalf("visit = %s next %q", status, next)
	}
	// The run follows the usual path: a quiet exit schedules the next watch.
	complete(t, svc, run.ID, RunCompletion{Verdict: "quiet", ExitCode: exitCode(111)})
	if _, _, _, next := openVisit(t, svc, task); next == "" {
		t.Fatal("no next watch after the manual run")
	}
}

func TestRunWatchNowRefusals(t *testing.T) {
	ctx := context.Background()
	t.Run("agent", func(t *testing.T) {
		svc, _, task := watchFixture(t)
		if _, err := svc.RunWatchNow(ctx, AgentActor("dev-1"), task.Key); ErrorCode(err) != "forbidden" || ErrorStatus(err) != 403 {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("closed", func(t *testing.T) {
		svc, actor, task := watchFixture(t)
		forceTask(t, svc, task, `status = 'cancelled'`)
		if _, err := svc.RunWatchNow(ctx, actor, task.Key); ErrorCode(err) != "workflow_closed" || ErrorStatus(err) != 409 {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("paused", func(t *testing.T) {
		svc, actor, task := watchFixture(t)
		forceTask(t, svc, task, `workflow_paused_reason = 'hold'`)
		if _, err := svc.RunWatchNow(ctx, actor, task.Key); ErrorCode(err) != "workflow_paused" || ErrorStatus(err) != 409 {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("pool status", func(t *testing.T) {
		svc, actor, task, _ := runFixture(t)
		if _, err := svc.RunWatchNow(ctx, actor, task.Key); ErrorCode(err) != "workflow_no_watch" || ErrorStatus(err) != 409 {
			t.Fatalf("err = %v", err)
		}
		if runs := listRuns(t, svc, actor, task); len(runs) != 0 {
			t.Fatalf("runs = %#v", runs)
		}
	})
	t.Run("active run", func(t *testing.T) {
		svc, actor, task := watchFixture(t)
		scheduleWatch(t, svc, actor, task)
		if _, err := svc.RunWatchNow(ctx, actor, task.Key); ErrorCode(err) != "script_running" || ErrorStatus(err) != 409 {
			t.Fatalf("err = %v", err)
		}
		if runs := listRuns(t, svc, actor, task); len(runs) != 1 {
			t.Fatalf("runs = %d", len(runs))
		}
	})
	t.Run("unknown task", func(t *testing.T) {
		svc, actor, _ := watchFixture(t)
		if _, err := svc.RunWatchNow(ctx, actor, "DEV-999"); ErrorCode(err) != "not_found" {
			t.Fatalf("err = %v", err)
		}
	})
}
