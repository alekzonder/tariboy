package aiproxy

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s, func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) })
}

func sampleReq(id, agent, image string, cost float64, ts time.Time) AIRequest {
	return AIRequest{
		ID: id, TS: ts.UTC().Format(time.RFC3339Nano), Agent: agent, Iteration: agent + "-1",
		ImageName: image, ImageTag: "latest", ImageDigest: "sha256:x", Provider: "anthropic",
		Model: "claude-opus-4-8", InputTokens: 100, OutputTokens: 50, CacheWriteTokens: 10,
		CacheReadTokens: 5, CostUSD: cost, LatencyMs: 42, Status: "ok", UpstreamStatus: 200,
	}
}

func TestInsertAndAggregate(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	if err := s.Insert(sampleReq("r1", "alice", "basic", 0.10, base)); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertBatch([]AIRequest{
		sampleReq("r2", "alice", "basic", 0.20, base.Add(time.Minute)),
		sampleReq("r3", "bob", "basic", 0.05, base.Add(2*time.Minute)),
	}); err != nil {
		t.Fatal(err)
	}
	// INSERT OR REPLACE: re-inserting r1 must not duplicate.
	if err := s.Insert(sampleReq("r1", "alice", "basic", 0.10, base)); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Aggregate(UsageFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byAgent := map[string]UsageRow{}
	for _, r := range rows {
		byAgent[r.Agent] = r
	}
	if byAgent["alice"].Requests != 2 || byAgent["alice"].CostUSD != 0.30 {
		t.Fatalf("alice agg = %+v", byAgent["alice"])
	}
	if byAgent["bob"].Requests != 1 || byAgent["bob"].InputTokens != 100 {
		t.Fatalf("bob agg = %+v", byAgent["bob"])
	}
}

func TestAggregateFilterAndCostSince(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	s.Insert(sampleReq("r1", "alice", "basic", 0.10, base))
	s.Insert(sampleReq("r2", "alice", "other", 0.20, base.Add(time.Hour)))
	// filter by agent + image
	rows, _ := s.Aggregate(UsageFilter{Agent: "alice", Image: "basic"})
	if len(rows) != 1 || rows[0].CostUSD != 0.10 {
		t.Fatalf("filtered agg = %+v", rows)
	}
	// cost since a cutoff after r1: only r2 counts.
	c, err := s.CostSince("alice", base.Add(30*time.Minute))
	if err != nil || c != 0.20 {
		t.Fatalf("CostSince = %v err=%v", c, err)
	}
	// global cost since epoch: both.
	if g, _ := s.CostSince("", base.Add(-time.Hour)); g != 0.30 {
		t.Fatalf("global CostSince = %v", g)
	}
}

func TestDeleteAll(t *testing.T) {
	s := newStore(t)
	s.Insert(sampleReq("r1", "alice", "basic", 0.10, time.Now()))
	if err := s.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.Aggregate(UsageFilter{}); len(rows) != 0 {
		t.Fatalf("DeleteAll left rows: %+v", rows)
	}
}

func TestIterationUsage(t *testing.T) {
	s := newStore(t)
	rows := []AIRequest{
		{ID: "a1", TS: "2026-07-06T10:00:00Z", Agent: "bot", Iteration: "bot-1-1", InputTokens: 100, OutputTokens: 50, CostUSD: 0.001},
		{ID: "a2", TS: "2026-07-06T10:00:01Z", Agent: "bot", Iteration: "bot-1-1", InputTokens: 10, OutputTokens: 5, CostUSD: 0.0002},
		{ID: "a3", TS: "2026-07-06T10:00:02Z", Agent: "bot", Iteration: "bot-1-2", InputTokens: 999, OutputTokens: 9, CostUSD: 9},
	}
	if err := s.InsertBatch(rows); err != nil {
		t.Fatal(err)
	}
	in, out, cost, err := s.IterationUsage("bot-1-1")
	if err != nil {
		t.Fatal(err)
	}
	if in != 110 || out != 55 {
		t.Fatalf("tokens = %d/%d, want 110/55", in, out)
	}
	if cost < 0.0011 || cost > 0.0013 {
		t.Fatalf("cost = %v, want ~0.0012", cost)
	}
	// Unknown iteration -> zeros, no error.
	if in, out, cost, err := s.IterationUsage("nope"); err != nil || in != 0 || out != 0 || cost != 0 {
		t.Fatalf("unknown = %d/%d/%v err=%v", in, out, cost, err)
	}
}

func TestNewRequestIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewRequestID(nil)
		if id == "" || seen[id] {
			t.Fatalf("bad/duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestStoreGroupSnapshotPersistsValuesAndNulls(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	tagged := sampleReq("r-grouped", "alice", "basic", 0.10, base)
	tagged.GroupID, tagged.GroupName = "alpha", "alpha"
	ungrouped := sampleReq("r-ungrouped", "bob", "basic", 0.10, base.Add(time.Minute))

	if err := s.InsertBatch([]AIRequest{tagged, ungrouped}); err != nil {
		t.Fatal(err)
	}

	read := func(id string) (groupID, groupName sql.NullString) {
		t.Helper()
		if err := s.db.QueryRow(
			`SELECT group_id, group_name FROM ai_requests WHERE id=?`, id,
		).Scan(&groupID, &groupName); err != nil {
			t.Fatal(err)
		}
		return
	}

	groupID, groupName := read("r-grouped")
	if !groupID.Valid || groupID.String != "alpha" || !groupName.Valid || groupName.String != "alpha" {
		t.Fatalf("grouped row = group_id=%v group_name=%v", groupID, groupName)
	}
	groupID, groupName = read("r-ungrouped")
	if groupID.Valid || groupName.Valid {
		t.Fatalf("ungrouped row should be NULL/NULL, got group_id=%v group_name=%v", groupID, groupName)
	}
}

func TestCurrentGroupSnapshotUsesAgentMembership(t *testing.T) {
	s := newStore(t)
	if _, err := s.db.Exec(`INSERT INTO agents(name, image_ref, "group") VALUES ('alice', 'basic:latest', 'alpha')`); err != nil {
		t.Fatal(err)
	}

	id, name, err := s.CurrentGroup("alice")
	if err != nil {
		t.Fatal(err)
	}
	if id != "alpha" || name != "alpha" {
		t.Fatalf("CurrentGroup(alice) = %q/%q, want alpha/alpha", id, name)
	}

	if _, err := s.db.Exec(`UPDATE agents SET "group"='' WHERE name='alice'`); err != nil {
		t.Fatal(err)
	}
	id, name, err = s.CurrentGroup("alice")
	if err != nil || id != "" || name != "" {
		t.Fatalf("CurrentGroup(ungrouped alice) = %q/%q err=%v, want empty/empty nil", id, name, err)
	}
}

func TestUsageGroupFilterRequestsCapsLimitAtStoreBoundary(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	rows := make([]AIRequest, 205)
	for i := range rows {
		rows[i] = sampleReq(fmt.Sprintf("r-%03d", i), "alice", "basic", 0.01, base.Add(time.Duration(i)*time.Second))
	}
	if err := s.InsertBatch(rows); err != nil {
		t.Fatal(err)
	}

	got, err := s.Requests(UsageFilter{}, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 200 || got[0].ID != "r-204" || got[199].ID != "r-005" {
		t.Fatalf("Requests oversized limit returned %d rows, endpoints %q..%q", len(got), got[0].ID, got[len(got)-1].ID)
	}
}
