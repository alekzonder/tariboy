package judge

import (
	"context"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
)

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
