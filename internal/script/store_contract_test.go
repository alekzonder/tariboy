package script

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	basestore "github.com/alekzonder/tariboy/internal/store"
)

func newContractStore(t *testing.T, now time.Time) *Store {
	t.Helper()
	db, err := basestore.Open(filepath.Join(t.TempDir(), "scripts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db, func() time.Time { return now })
}

func intPtr(value int) *int { return &value }

func TestCreateOnceCreatesExactlyOnePendingRun(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, run, err := st.CreateOnce("alice", CreateOnce{Name: "check", Description: "checks repo", Command: "make check"})
	if err != nil {
		t.Fatal(err)
	}
	if definition.Mode != ModeOnce || definition.State != StateActive || run.ScriptID != definition.ID || run.Status != RunPending {
		t.Fatalf("definition/run = %#v / %#v", definition, run)
	}
	runs, err := st.ListRuns("alice", definition.ID)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs=%#v err=%v", runs, err)
	}
	second, secondRun, err := st.CreateOnce("alice", CreateOnce{Name: "check", Description: "checks again", Command: "make check"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == definition.ID || secondRun.ID == run.ID {
		t.Fatalf("frozen clock generated duplicate ids: %q / %q", second.ID, secondRun.ID)
	}
}

func TestCreateScheduleValidatesIntervalAndQuietExit(t *testing.T) {
	st := newContractStore(t, time.Now())
	for _, input := range []CreateSchedule{
		{Name: "watch", Description: "watch", Command: "true", IntervalSeconds: 0},
		{Name: "watch", Description: "watch", Command: "true", IntervalSeconds: 1, QuietExit: intPtr(-1)},
		{Name: "watch", Description: "watch", Command: "true", IntervalSeconds: 1, QuietExit: intPtr(256)},
	} {
		if _, _, err := st.CreateSchedule("alice", input); err == nil {
			t.Fatalf("accepted invalid schedule: %#v", input)
		}
	}
	definition, run, err := st.CreateSchedule("alice", CreateSchedule{Name: "watch", Description: "watch", Command: "true", IntervalSeconds: 20, QuietExit: intPtr(2)})
	if err != nil || definition.Mode != ModeEvery || definition.QuietExit == nil || *definition.QuietExit != 2 || run.Status != RunPending {
		t.Fatalf("definition/run=%#v/%#v err=%v", definition, run, err)
	}
}

func TestCompleteRunTreatsExitTwoAsFailureAndEnqueuesResult(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, run, err := st.CreateOnce("alice", CreateOnce{Name: "check", Description: "checks", Command: "make check"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", run.ID, now.Format(time.RFC3339), "/tmp/check.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	finished := now.Add(time.Minute).Format(time.RFC3339)
	completed, err := st.CompleteRun("alice", run.ID, Completion{Status: RunFailed, ExitCode: intPtr(2), FinishedAt: finished, LogPath: "/tmp/check.log"})
	if err != nil || completed.Status != RunFailed || completed.ExitCode == nil || *completed.ExitCode != 2 {
		t.Fatalf("completed=%#v err=%v", completed, err)
	}
	got, err := st.GetDefinition("alice", definition.ID)
	if err != nil || got.State != StateCompleted {
		t.Fatalf("definition=%#v err=%v", got, err)
	}
	var payload string
	if err := st.db.QueryRow(`SELECT payload FROM script_result_outbox WHERE run_id=?`, run.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload == "" {
		t.Fatal("empty result payload")
	}
}

func TestCompleteRunSuppressesOnlyExplicitQuietExit(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, run, err := st.CreateSchedule("alice", CreateSchedule{Name: "watch", Description: "watch", Command: "false", IntervalSeconds: 30, QuietExit: intPtr(2)})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", run.ID, now.Format(time.RFC3339), "/tmp/watch.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	finished := now.Add(10 * time.Second)
	if _, err := st.CompleteRun("alice", run.ID, Completion{Status: RunFailed, ExitCode: intPtr(2), FinishedAt: finished.Format(time.RFC3339), LogPath: "/tmp/watch.log"}); err != nil {
		t.Fatal(err)
	}
	var outboxCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM script_result_outbox WHERE run_id=?`, run.ID).Scan(&outboxCount); err != nil || outboxCount != 0 {
		t.Fatalf("quiet outbox count=%d err=%v", outboxCount, err)
	}
	got, err := st.GetDefinition("alice", definition.ID)
	if err != nil || got.State != StateActive || got.NextRunAt != finished.Add(30*time.Second).Format(time.RFC3339) {
		t.Fatalf("recurring definition=%#v err=%v", got, err)
	}
}

func TestRerunRejectsRecurringAndActiveDefinitions(t *testing.T) {
	st := newContractStore(t, time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC))
	recurring, _, err := st.CreateSchedule("alice", CreateSchedule{Name: "watch", Description: "watch", Command: "true", IntervalSeconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Rerun("alice", recurring.ID); !errors.Is(err, ErrMode) {
		t.Fatalf("recurring rerun error=%v", err)
	}
	once, _, err := st.CreateOnce("alice", CreateOnce{Name: "once", Description: "once", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Rerun("alice", once.ID); !errors.Is(err, ErrActive) {
		t.Fatalf("active rerun error=%v", err)
	}
}

func TestActiveRunConstraintPreventsOverlap(t *testing.T) {
	st := newContractStore(t, time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC))
	definition, _, err := st.CreateSchedule("alice", CreateSchedule{Name: "watch", Description: "watch", Command: "true", IntervalSeconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ScheduleNext("alice", definition.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlap error=%v", err)
	}
}

func TestCompletedOneShotCanBeRerunAndListsNewestFirst(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, first, err := st.CreateOnce("alice", CreateOnce{Name: "check", Description: "check", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", first.ID, now.Format(time.RFC3339), "/tmp/first.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	exit := 0
	if _, err := st.CompleteRun("alice", first.ID, Completion{Status: RunSucceeded, ExitCode: &exit, FinishedAt: now.Add(time.Second).Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	st.clock = func() time.Time { return now }
	second, err := st.Rerun("alice", definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns("alice", definition.ID)
	if err != nil || len(runs) != 2 || runs[0].ID != second.ID || runs[1].ID != first.ID {
		t.Fatalf("runs=%#v err=%v", runs, err)
	}
}

func TestCancelRecurringRunKeepsDefinitionActiveAndEnqueuesResult(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, run, err := st.CreateSchedule("alice", CreateSchedule{Name: "watch", Description: "watch", Command: "sleep 30", IntervalSeconds: 15})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CancelRun("alice", run.ID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetDefinition("alice", definition.ID)
	if err != nil || got.State != StateActive || got.NextRunAt != now.Add(15*time.Second).Format(time.RFC3339) {
		t.Fatalf("definition=%#v err=%v", got, err)
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM script_result_outbox WHERE run_id=?`, run.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("outbox count=%d err=%v", count, err)
	}
}

func TestCancellationIsIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC).Format(time.RFC3339)
	st := newContractStore(t, time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC))
	definition, run, err := st.CreateOnce("alice", CreateOnce{Name: "long", Description: "long", Command: "sleep 30"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CancelRun("alice", run.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := st.CancelRun("alice", run.ID, now); err != nil {
		t.Fatalf("repeated run cancellation: %v", err)
	}
	if err := st.CancelDefinition("alice", definition.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := st.CancelDefinition("alice", definition.ID, now); err != nil {
		t.Fatalf("repeated definition cancellation: %v", err)
	}
}

func TestCancelDefinitionLeavesRunningAttemptActiveUntilProcessExit(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, run, err := st.CreateSchedule("alice", CreateSchedule{Name: "long", Description: "long", Command: "sleep 30", IntervalSeconds: 15})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", run.ID, now.Format(time.RFC3339), "/tmp/long.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	if err := st.CancelDefinition("alice", definition.ID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun("alice", run.ID)
	if err != nil || got.Status != RunRunning || !got.CancelRequested {
		t.Fatalf("running attempt was made terminal before process exit: %#v err=%v", got, err)
	}
	if err := st.RemoveDefinition("alice", definition.ID); !errors.Is(err, ErrActive) {
		t.Fatalf("removed definition with running attempt: %v", err)
	}
}

func TestCancelledRunningOneShotDefinitionStaysCancelled(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, run, err := st.CreateOnce("alice", CreateOnce{Name: "once", Description: "once", Command: "sleep 30"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", run.ID, now.Format(time.RFC3339), "/tmp/once.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	if err := st.CancelDefinition("alice", definition.ID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := st.CancelRun("alice", run.ID, now.Add(time.Second).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetDefinition("alice", definition.ID); err != nil || got.State != StateCancelled {
		t.Fatalf("one-shot definition resurrected after cancellation: %#v err=%v", got, err)
	}
	if _, err := st.Rerun("alice", definition.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("cancelled one-shot became rerunnable: %v", err)
	}
}

func TestRecoveryHonorsDurableCancellationIntent(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	oneShot, oneRun, err := st.CreateOnce("alice", CreateOnce{Name: "once", Description: "once", Command: "sleep 30"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", oneRun.ID, now.Format(time.RFC3339), "/tmp/once.log"); err != nil || !claimed {
		t.Fatalf("claim one-shot=%v err=%v", claimed, err)
	}
	if err := st.CancelDefinition("alice", oneShot.ID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	recurring, recurringRun, err := st.CreateSchedule("alice", CreateSchedule{Name: "every", Description: "every", Command: "sleep 30", IntervalSeconds: 15})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", recurringRun.ID, now.Format(time.RFC3339), "/tmp/every.log"); err != nil || !claimed {
		t.Fatalf("claim recurring=%v err=%v", claimed, err)
	}
	if _, err := st.RequestRunCancellation("alice", recurringRun.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RecoverRunning(); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetRun("alice", oneRun.ID); err != nil || got.Status != RunCancelled || got.CancelRequested {
		t.Fatalf("recovered one-shot run=%#v err=%v", got, err)
	}
	if got, err := st.GetDefinition("alice", oneShot.ID); err != nil || got.State != StateCancelled {
		t.Fatalf("recovered one-shot definition=%#v err=%v", got, err)
	}
	if got, err := st.GetRun("alice", recurringRun.ID); err != nil || got.Status != RunCancelled || got.CancelRequested {
		t.Fatalf("recovered recurring run=%#v err=%v", got, err)
	}
	if got, err := st.GetDefinition("alice", recurring.ID); err != nil || got.State != StateActive || got.NextRunAt == "" {
		t.Fatalf("recovered recurring definition=%#v err=%v", got, err)
	}
	var cancelledResults int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM script_result_outbox WHERE run_id IN (?,?) AND payload LIKE '%"status":"cancelled"%'`, oneRun.ID, recurringRun.ID).Scan(&cancelledResults); err != nil || cancelledResults != 2 {
		t.Fatalf("cancelled recovery results=%d err=%v", cancelledResults, err)
	}
}

func TestRemoveDefinitionKeepsUnpublishedResult(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	definition, run, err := st.CreateOnce("alice", CreateOnce{Name: "done", Description: "done", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", run.ID, now.Format(time.RFC3339), "/tmp/done.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	exit := 0
	if _, err := st.CompleteRun("alice", run.ID, Completion{Status: RunSucceeded, ExitCode: &exit, FinishedAt: now.Format(time.RFC3339), LogPath: "/tmp/done.log"}); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveDefinition("alice", definition.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM script_result_outbox WHERE run_id=? AND published_at=''`, run.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unpublished result after remove = %d, err=%v", count, err)
	}
}
