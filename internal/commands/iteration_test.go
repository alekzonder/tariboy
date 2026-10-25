package commands

import (
	"os"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestIterationLsInspectLogs(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	id := "smoke-20260706100000-1"
	as.CreateIteration(agent.Iteration{ID: id, Agent: "smoke", Trigger: "interval",
		Status: "done", StartedAt: time.Now().Format(time.RFC3339), ImageVersion: "1.2.3"})

	ls, err := h(t, "iteration.ls")(c, registry.Params{"name": "smoke"})
	if err != nil || ls.(IterationList).Count != 1 {
		t.Fatalf("ls: %v err=%v", ls, err)
	}
	insp, err := h(t, "iteration.inspect")(c, registry.Params{"name": "smoke", "id": id})
	if err != nil || insp.(IterationView).Status != "done" {
		t.Fatalf("inspect: %v err=%v", insp, err)
	}
	if insp.(IterationView).ImageVersion != "1.2.3" {
		t.Fatalf("inspect image_version = %v", insp.(IterationView).ImageVersion)
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
	m := logs.(IterationLogs)
	if m.Stdout != "out-line" || m.Stderr != "err-line" {
		t.Fatalf("logs: %v", m)
	}
}

func TestIterationProductiveInViews(t *testing.T) {
	t.Parallel()
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
	for _, r := range ls.(IterationList).Iterations {
		got[r.ID] = r.Productive
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
		if p := insp.(IterationView).Productive; p != want {
			t.Fatalf("inspect %s productive=%v, want %v", id, p, want)
		}
	}
}

func TestIterationLogsNotFound(t *testing.T) {
	t.Parallel()
	c, _, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	if _, err := h(t, "iteration.logs")(c, registry.Params{"name": "ghost", "id": "nope"}); err == nil {
		t.Fatal("iteration logs of a missing agent/iteration must be a not_found error")
	}
}
