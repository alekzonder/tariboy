package judge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
)

type retryingSnapshotter struct {
	calls atomic.Int32
	first chan struct{}
}

func (s *retryingSnapshotter) BuildRun(ctx context.Context, _ string) error {
	if s.calls.Add(1) == 1 {
		close(s.first)
		return errors.New("retry")
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestRunnerPublishesOnlyEligibleWorkerNotifications(t *testing.T) {
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedJudgeAgent(t, db.DB, "outsider")
	seedTarget(t, db.DB, "target", "worker", "done", "2026-07-01T10:00:00Z")
	r, ts, err := js.CreateRun(context.Background(), request("target"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`UPDATE judge_targets SET snapshot_status='ready' WHERE id=?`, ts[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = js.CreateAssignments(r.ID); err != nil {
		t.Fatal(err)
	}
	b := bus.New(db, time.Now)
	for _, a := range []string{"judge", "outsider"} {
		if _, err := b.Subscribe(a, bus.InboxChannel(a), nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	(&Runner{store: js, bus: b}).work(r)
	judge, err := b.Inbox("judge", "pending", 10, "")
	if err != nil || len(judge) != 1 || judge[0].Type != "judge.work.available" {
		t.Fatalf("judge inbox=%+v err=%v", judge, err)
	}
	outsider, err := b.Inbox("outsider", "pending", 10, "")
	if err != nil || len(outsider) != 0 {
		t.Fatalf("outsider inbox=%+v err=%v", outsider, err)
	}
}

func TestRunnerCancellationDoesNotStartRecoveryWork(t *testing.T) {
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "target", "worker", "done", "2026-07-01T10:00:00Z")
	if _, _, err := js.CreateRun(context.Background(), request("target")); err != nil {
		t.Fatal(err)
	}
	snapshotter := &retryingSnapshotter{first: make(chan struct{})}
	runner := NewRunner(RunnerConfig{Store: js, Snapshotter: snapshotter, Tick: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()
	<-snapshotter.first
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runner started uncancellable recovery during shutdown")
	}
}
