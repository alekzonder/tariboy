package aiproxy

import (
	"testing"
	"time"
)

func TestTaskUsageGroupsTaggedRequestsByTaskAndAgentInWindow(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	must(t, s.InsertBatch([]AIRequest{
		iterReq(taskReq("r1", "alice", 0.10, base, "T1", ""), "it-1"),
		iterReq(taskReq("r2", "alice", 0.20, base.Add(time.Minute), "T1", ""), "it-1"),
		iterReq(taskReq("r3", "alice", 0.30, base.Add(2*time.Minute), "T1", ""), "it-2"),
		iterReq(taskReq("r4", "bob", 0.40, base.Add(3*time.Minute), "T1", ""), "it-3"),
		iterReq(taskReq("r5", "alice", 0.05, base.Add(4*time.Minute), "", ""), "it-2"),
		// Outside the window on both sides.
		iterReq(taskReq("r6", "alice", 9, base.Add(-time.Minute), "T1", ""), "it-0"),
		iterReq(taskReq("r7", "alice", 9, base.Add(time.Hour), "T2", ""), "it-9"),
	}))
	f := UsageFilter{Since: base.Format(time.RFC3339Nano), Until: base.Add(time.Hour).Format(time.RFC3339Nano)}

	rows, err := s.TaskUsage(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	alice, bob := rows[0], rows[1]
	if alice.TaskKey != "T1" || alice.Agent != "alice" || alice.Iterations != 2 || alice.Requests != 3 ||
		alice.CostUSD != 0.60 || alice.Tokens != 3*165 {
		t.Fatalf("alice = %+v", alice)
	}
	if alice.FirstTS != "2026-07-06T09:00:00.000000000Z" || alice.LastTS != "2026-07-06T09:02:00.000000000Z" {
		t.Fatalf("alice bounds = %+v", alice)
	}
	if bob.Agent != "bob" || bob.Iterations != 1 || bob.Requests != 1 {
		t.Fatalf("bob = %+v", bob)
	}

	f.Agent = "bob"
	rows, err = s.TaskUsage(f)
	if err != nil || len(rows) != 1 || rows[0].Agent != "bob" {
		t.Fatalf("agent filter = %+v, %v", rows, err)
	}
}
