package commands

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestEventsSinceCursor(t *testing.T) {
	base := t.TempDir()
	c := &registry.Ctx{BaseDir: base}
	path := agentdir.New(paths.New(base).AgentsDir(), "m").AuditLog()
	l := audit.Open(path, func() time.Time { return time.Unix(0, 0).UTC() })
	l.Record("iteration_started", "system", "m-1", nil)                                            // seq 1
	l.Record("shim", "shim", "m-1", map[string]any{"line": "boom"})                                // seq 2
	l.Record("harness_output", "harness", "m-1", map[string]any{"stream": "stdout", "line": "hi"}) // seq 3

	rows, err := eventsSince(c, "m", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("since=1 rows = %d, want 2", len(rows))
	}
	// Chronological (oldest first) with seq carried through.
	if rows[0]["seq"].(int64) != 2 || rows[0]["kind"] != "shim" {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1]["seq"].(int64) != 3 || rows[1]["source"] != "harness" {
		t.Fatalf("row1 = %+v", rows[1])
	}

	// recentEvents is newest-first and includes everything.
	recent, err := recentEvents(c, "m", 50)
	if err != nil || len(recent) != 3 || recent[0]["kind"] != "harness_output" {
		t.Fatalf("recent = %+v err=%v", recent, err)
	}
}

func TestEventsIterationAndBefore(t *testing.T) {
	base := t.TempDir()
	c := &registry.Ctx{BaseDir: base}
	path := agentdir.New(paths.New(base).AgentsDir(), "m").AuditLog()
	l := audit.Open(path, func() time.Time { return time.Unix(0, 0).UTC() })
	l.Record("iteration_started", "system", "m-1", nil)                       // 1
	l.Record("harness_output", "harness", "m-1", map[string]any{"line": "a"}) // 2
	l.Record("iteration_started", "system", "m-2", nil)                       // 3

	its, err := eventsForIteration(c, "m", "m-1")
	if err != nil || len(its) != 2 {
		t.Fatalf("iteration m-1 = %d err=%v", len(its), err)
	}
	if its[0]["iteration_id"] != "m-1" || its[0]["seq"].(int64) != 1 {
		t.Fatalf("row0 = %+v", its[0])
	}

	before, err := eventsBefore(c, "m", 3, 50)
	if err != nil || len(before) != 2 {
		t.Fatalf("before=3 = %d err=%v", len(before), err)
	}
	if before[1]["seq"].(int64) != 2 {
		t.Fatalf("before last = %+v", before[1])
	}
}

func TestFilteredEventsAndDistinctTypes(t *testing.T) {
	base := t.TempDir()
	c := &registry.Ctx{BaseDir: base}
	path := agentdir.New(paths.New(base).AgentsDir(), "m").AuditLog()
	l := audit.Open(path, func() time.Time { return time.Unix(0, 0).UTC() })
	l.Record("iteration_started", "system", "m-1", nil)                          // 1
	l.Record("status", "system", "m-1", map[string]any{"message": "reviewing"})  // 2
	l.Record("harness_output", "harness", "m-1", map[string]any{"line": "boom"}) // 3
	l.Record("status", "system", "m-2", map[string]any{"message": "building"})   // 4

	// Type filter: only the two status events, newest-first.
	byType, capped, err := filteredEvents(c, "m", "status", "", 500)
	if err != nil || capped || len(byType) != 2 {
		t.Fatalf("type=status = %d capped=%v err=%v", len(byType), capped, err)
	}
	if byType[0]["seq"].(int64) != 4 || byType[1]["seq"].(int64) != 2 {
		t.Fatalf("type rows = %+v", byType)
	}

	// Full-text query matches the data body of any type (the harness line).
	byText, _, err := filteredEvents(c, "m", "", "boom", 500)
	if err != nil || len(byText) != 1 || byText[0]["seq"].(int64) != 3 {
		t.Fatalf("q=boom = %+v err=%v", byText, err)
	}

	// Case-insensitive, and type AND query compose: only the "building" status.
	both, _, err := filteredEvents(c, "m", "status", "BUILD", 500)
	if err != nil || len(both) != 1 || both[0]["seq"].(int64) != 4 {
		t.Fatalf("type=status q=BUILD = %+v err=%v", both, err)
	}

	// Comma type list is a union.
	union, _, err := filteredEvents(c, "m", "status,harness_output", "", 500)
	if err != nil || len(union) != 3 {
		t.Fatalf("type union = %d err=%v", len(union), err)
	}

	// A tiny cap truncates and flags capped, keeping the newest match.
	one, capped, err := filteredEvents(c, "m", "status", "", 1)
	if err != nil || !capped || len(one) != 1 || one[0]["seq"].(int64) != 4 {
		t.Fatalf("cap=1 = %+v capped=%v err=%v", one, capped, err)
	}

	// Distinct types are sorted and deduped.
	types, err := distinctTypes(c, "m")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"harness_output", "iteration_started", "status"}
	if len(types) != len(want) {
		t.Fatalf("distinct = %v want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("distinct = %v want %v", types, want)
		}
	}
}
