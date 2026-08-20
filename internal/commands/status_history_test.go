package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestAgentStatusHistoryFiltersStatusEvents(t *testing.T) {
	c, _ := seedAgent(t)
	// Seed the agent's audit.jsonl with mixed events.
	logPath := agentdir.New(paths.New(c.BaseDir).AgentsDir(), "a1").AuditLog()
	log := audit.Open(logPath, nil)
	log.Record("iteration", "system", "", map[string]any{"foo": "bar"})
	log.Record("status", "agent", "iteration-1", map[string]any{"message": "first"})
	log.Record("audit", "harness", "", map[string]any{"noise": 1})
	log.Record("status", "agent", "iteration-2", map[string]any{"message": "second"})

	res, err := agentStatusHistory().Handler(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["count"].(int) != 2 {
		t.Fatalf("count=%v", m["count"])
	}
	evs := m["events"].([]map[string]any)
	// newest-first
	if evs[0]["message"] != "second" || evs[1]["message"] != "first" {
		t.Fatalf("order/content wrong: %v", evs)
	}
	if evs[0]["iteration_id"] != "iteration-2" || evs[1]["iteration_id"] != "iteration-1" {
		t.Fatalf("iteration ids wrong: %v", evs)
	}
}
