package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/judge"
	"github.com/alekzonder/tariboy/internal/registry"
)

type iterationJudgeControl struct {
	reviews map[string][]judge.IterationJudgeReview
}

func (*iterationJudgeControl) OperatorReview(context.Context, []string, int) (judge.Run, []judge.Target, error) {
	return judge.Run{}, nil, nil
}
func (*iterationJudgeControl) OperatorList(judge.ListFilter) ([]judge.Run, error) { return nil, nil }
func (*iterationJudgeControl) OperatorInspect(string) (map[string]any, error)     { return nil, nil }
func (*iterationJudgeControl) OperatorEvidence(string, string, judge.EvidenceLocator) (map[string]any, error) {
	return nil, nil
}
func (*iterationJudgeControl) OperatorCancel(string) error { return nil }
func (*iterationJudgeControl) OperatorRetry(string) error  { return nil }
func (c *iterationJudgeControl) OperatorIterationReviews(ids []string) (map[string][]judge.IterationJudgeReview, error) {
	return c.reviews, nil
}

func TestIterationLsInspectLogs(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	id := "smoke-20260706100000-1"
	as.CreateIteration(agent.Iteration{ID: id, Agent: "smoke", Trigger: "interval",
		Status: "done", StartedAt: time.Now().Format(time.RFC3339), ImageVersion: "1.2.3"})

	ls, err := h(t, "iteration.ls")(c, registry.Params{"name": "smoke"})
	if err != nil || ls.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("ls: %v err=%v", ls, err)
	}
	insp, err := h(t, "iteration.inspect")(c, registry.Params{"name": "smoke", "id": id})
	if err != nil || insp.(map[string]any)["status"] != "done" {
		t.Fatalf("inspect: %v err=%v", insp, err)
	}
	if insp.(map[string]any)["image_version"] != "1.2.3" {
		t.Fatalf("inspect image_version = %v", insp.(map[string]any)["image_version"])
	}

	// write logs on disk
	l := agentdir.New(agentsDir(c), "smoke")
	l.EnsureIteration(id)
	os.WriteFile(l.HarnessStdout(id), []byte("out-line"), 0o600)
	os.WriteFile(l.HarnessStderr(id), []byte("err-line"), 0o600)
	logs, err := h(t, "iteration.logs")(c, registry.Params{"name": "smoke", "id": id})
	if err != nil {
		t.Fatal(err)
	}
	m := logs.(map[string]any)
	if m["stdout"] != "out-line" || m["stderr"] != "err-line" {
		t.Fatalf("logs: %v", m)
	}
	_ = filepath.Join // keep import if unused elsewhere
}

func TestIterationJudgeProjectionAndOwnedHistory(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	as.Create(agent.Agent{Name: "other", OnTimeout: "restart", OnError: "restart"})
	for _, row := range []struct{ id, owner string }{{"iteration-1", "smoke"}, {"other-iteration", "other"}} {
		if err := as.CreateIteration(agent.Iteration{ID: row.id, Agent: row.owner, Trigger: "manual", Status: "done", StartedAt: "2026-07-01T10:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	zero, pointEight := 0.0, 0.8
	reviews := []judge.IterationJudgeReview{
		{RunID: "pending", TargetID: "target-pending", CreatedAt: "2026-07-01T13:00:00Z", Pending: 1},
		{RunID: "complete", TargetID: "target-complete", CreatedAt: "2026-07-01T12:00:00Z", Verdict: "pass", Score: &pointEight, Completed: 1},
		{RunID: "zero", TargetID: "target-zero", CreatedAt: "2026-07-01T11:00:00Z", Verdict: "fail", Score: &zero, Completed: 1},
	}
	c.Judges = &iterationJudgeControl{reviews: map[string][]judge.IterationJudgeReview{"iteration-1": reviews}}

	ls, err := h(t, "iteration.ls")(c, registry.Params{"name": "smoke"})
	if err != nil {
		t.Fatal(err)
	}
	projection := ls.(map[string]any)["iterations"].([]map[string]any)[0]["judge"].(judge.IterationJudgeProjection)
	if projection.LatestCompleted == nil || projection.LatestCompleted.Score == nil || *projection.LatestCompleted.Score != 0.8 || projection.Active == nil || projection.Active.Pending != 1 {
		t.Fatalf("list projection = %+v", projection)
	}
	inspect, err := h(t, "iteration.inspect")(c, registry.Params{"name": "smoke", "id": "iteration-1"})
	if err != nil || inspect.(map[string]any)["judge"] != projection {
		t.Fatalf("inspect = %+v err=%v", inspect, err)
	}
	history, err := h(t, "iteration.judges")(c, registry.Params{"name": "smoke", "id": "iteration-1"})
	if err != nil || len(history.(map[string]any)["reviews"].([]judge.IterationJudgeReview)) != 3 {
		t.Fatalf("history = %+v err=%v", history, err)
	}
	if _, err := h(t, "iteration.judges")(c, registry.Params{"name": "smoke", "id": "other-iteration"}); err == nil {
		t.Fatal("history allowed an iteration owned by another agent")
	}
}

func TestIterationProductiveInViews(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})

	// Plain done iteration → productive=true.
	prodID := "smoke-20260712100000-1"
	as.CreateIteration(agent.Iteration{ID: prodID, Agent: "smoke", Trigger: "interval",
		Status: "done", StartedAt: time.Now().Format(time.RFC3339)})
	if err := as.SetIterationDone(prodID, true); err != nil {
		t.Fatal(err)
	}
	// Idle (--idle) done iteration → productive=false.
	idleID := "smoke-20260712100000-2"
	as.CreateIteration(agent.Iteration{ID: idleID, Agent: "smoke", Trigger: "interval",
		Status: "done", StartedAt: time.Now().Format(time.RFC3339)})
	if err := as.SetIterationDone(idleID, false); err != nil {
		t.Fatal(err)
	}
	// Running/in-flight iteration: SetIterationDone never called, so productive
	// relies purely on the column default (NOT NULL DEFAULT 1) → true. A
	// running row must never render idle; if that default ever flipped to 0
	// this assertion is what catches it.
	runID := "smoke-20260712100000-3"
	as.CreateIteration(agent.Iteration{ID: runID, Agent: "smoke", Trigger: "interval",
		Status: "running", StartedAt: time.Now().Format(time.RFC3339)})

	// List view: every row carries productive reflecting the flag/default.
	ls, err := h(t, "iteration.ls")(c, registry.Params{"name": "smoke"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range ls.(map[string]any)["iterations"].([]map[string]any) {
		p, ok := r["productive"].(bool)
		if !ok {
			t.Fatalf("ls row %v missing productive bool", r["id"])
		}
		got[r["id"].(string)] = p
	}
	if !got[prodID] {
		t.Fatalf("ls: plain done productive=%v, want true", got[prodID])
	}
	if got[idleID] {
		t.Fatalf("ls: idle done productive=%v, want false", got[idleID])
	}
	if !got[runID] {
		t.Fatalf("ls: running in-flight productive=%v, want true (column default)", got[runID])
	}

	// Inspect view: productive present and correct for each, including the
	// running row that never had SetIterationDone called.
	for id, want := range map[string]bool{prodID: true, idleID: false, runID: true} {
		insp, err := h(t, "iteration.inspect")(c, registry.Params{"name": "smoke", "id": id})
		if err != nil {
			t.Fatal(err)
		}
		p, ok := insp.(map[string]any)["productive"].(bool)
		if !ok {
			t.Fatalf("inspect %s missing productive bool", id)
		}
		if p != want {
			t.Fatalf("inspect %s productive=%v, want %v", id, p, want)
		}
	}
}

func TestIterationLogsNotFound(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	if _, err := h(t, "iteration.logs")(c, registry.Params{"name": "ghost", "id": "nope"}); err == nil {
		t.Fatal("iteration logs of a missing agent/iteration must be a not_found error")
	}
}
