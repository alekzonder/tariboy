package aiproxy

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// taskReq is sampleReq with task/epic attribution stamped on.
func taskReq(id, agent string, cost float64, ts time.Time, task, epic string) AIRequest {
	r := sampleReq(id, agent, "basic", cost, ts)
	r.TaskID, r.EpicID = task, epic
	return r
}

func TestAggregateByAllGroupings(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	// alice: two reqs on task T1/epic E1 (iterations it-1, it-2), one untagged on it-1.
	must(t, s.InsertBatch([]AIRequest{
		iterReq(taskReq("r1", "alice", 0.10, base, "T1", "E1"), "it-1"),
		iterReq(taskReq("r2", "alice", 0.20, base.Add(time.Minute), "T1", "E1"), "it-2"),
		iterReq(taskReq("r3", "alice", 0.05, base.Add(2*time.Minute), "", ""), "it-1"),
	}))

	// by task: T1 (2 reqs, 0.30) + untagged "" (1 req, 0.05).
	byKey := index(t, s, "task")
	if got := byKey["T1"]; got.Requests != 2 || got.CostUSD != 0.30 {
		t.Fatalf("task T1 = %+v", got)
	}
	if got := byKey[""]; got.Requests != 1 || got.CostUSD != 0.05 {
		t.Fatalf("task untagged = %+v", got)
	}

	// by epic: E1 (2) + untagged "" (1).
	byEpic := index(t, s, "epic")
	if byEpic["E1"].Requests != 2 || byEpic[""].Requests != 1 {
		t.Fatalf("epic groups = %+v", byEpic)
	}

	// by iteration: it-1 (r1+r3 = 2), it-2 (r2 = 1). Never NULL.
	byIt := index(t, s, "iteration")
	if byIt["it-1"].Requests != 2 || byIt["it-2"].Requests != 1 {
		t.Fatalf("iteration groups = %+v", byIt)
	}

	// by model: all three share claude-opus-4-8.
	byModel := index(t, s, "model")
	if byModel["claude-opus-4-8"].Requests != 3 {
		t.Fatalf("model groups = %+v", byModel)
	}

	// token sums land on the right group (each sampleReq is 100/50/10/5).
	if g := byKey["T1"]; g.InputTokens != 200 || g.OutputTokens != 100 || g.CacheWriteTokens != 20 || g.CacheReadTokens != 10 {
		t.Fatalf("task T1 tokens = %+v", g)
	}
}

func TestAggregateByUnknownGroupBy(t *testing.T) {
	s := newStore(t)
	if _, err := s.AggregateBy(UsageFilter{}, "nonsense"); !errors.Is(err, ErrBadGroupBy) {
		t.Fatalf("want ErrBadGroupBy, got %v", err)
	}
}

func TestSeriesBucketing(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // 09:00:00
	must(t, s.InsertBatch([]AIRequest{
		sampleReq("r1", "alice", "basic", 0.10, base),                     // 09:00 bucket (5m: 09:00, 1h: 09:00)
		sampleReq("r2", "alice", "basic", 0.20, base.Add(3*time.Minute)),  // 09:03 -> 5m bucket 09:00
		sampleReq("r3", "alice", "basic", 0.05, base.Add(7*time.Minute)),  // 09:07 -> 5m bucket 09:05
		sampleReq("r4", "alice", "basic", 0.01, base.Add(65*time.Minute)), // 10:05 -> new hour bucket
	}))

	// 5m buckets: 09:00 (r1+r2 = 2 reqs, 0.30), 09:05 (r3 = 1), 10:05 (r4 = 1).
	five := seriesByStart(t, s, "5m")
	if got := five["2026-07-06T09:00:00Z"]; got.Requests != 2 || got.CostUSD != 0.30 {
		t.Fatalf("5m 09:00 = %+v", got)
	}
	if five["2026-07-06T09:05:00Z"].Requests != 1 || five["2026-07-06T10:05:00Z"].Requests != 1 {
		t.Fatalf("5m buckets = %+v", five)
	}
	// tokens are the four-kind sum (100+50+10+5 = 165 per req).
	if got := five["2026-07-06T09:00:00Z"]; got.Tokens != 330 {
		t.Fatalf("5m 09:00 tokens = %d want 330", got.Tokens)
	}

	// 1h buckets: 09:00 (r1+r2+r3 = 3), 10:00 (r4 = 1).
	hour := seriesByStart(t, s, "1h")
	if hour["2026-07-06T09:00:00Z"].Requests != 3 || hour["2026-07-06T10:00:00Z"].Requests != 1 {
		t.Fatalf("1h buckets = %+v", hour)
	}
}

// TestWindowBoundsSubSecond guards UsageFilter.where against a lexical ts
// compare. ts is stored as RFC3339Nano text whose fractional part varies in
// width (trailing zeros stripped), while the UI pads its bounds to millis. A
// plain string compare then drops or admits the wrong sub-second rows:
//   - since=.123Z vs a row .123456Z compares '4' < 'Z' → row < since → dropped,
//     though it is chronologically inside the window.
//   - until=.124Z vs a row .1245Z compares '5' < 'Z' → row < until → admitted,
//     though it is chronologically at/after the upper bound.
//
// The bounds must compare on real time, so only the in-window row survives.
func TestWindowBoundsSubSecond(t *testing.T) {
	s := newStore(t)
	sec := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	// rIn:  .123456Z — inside [.123, .124), 6 frac digits vs 3-digit bound (0.11)
	// rLow: .122456Z — below since .123Z, also 6 frac digits                (0.22)
	// rHigh:.1245Z   — at/above until .124Z, lexically < '.124Z'            (0.33)
	rIn := sampleReq("rIn", "alice", "basic", 0.11, sec.Add(123456000))
	rLow := sampleReq("rLow", "alice", "basic", 0.22, sec.Add(122456000))
	rHigh := sampleReq("rHigh", "alice", "basic", 0.33, sec.Add(124500000))
	must(t, s.InsertBatch([]AIRequest{rIn, rLow, rHigh}))

	// Bounds mimic the UI's toISOString: padded to exactly 3 fractional digits.
	f := UsageFilter{Since: "2026-07-06T09:00:00.123Z", Until: "2026-07-06T09:00:00.124Z"}

	rows, err := s.Aggregate(f)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(rows) != 1 || rows[0].Requests != 1 || rows[0].CostUSD != 0.11 {
		t.Fatalf("window bounds admitted wrong rows: %+v (want 1 req, cost 0.11 = rIn)", rows)
	}

	// AggregateBy and Series re-expose the same where(); confirm the boundary
	// row survives there too rather than only through Aggregate.
	by, err := s.AggregateBy(f, "model")
	if err != nil {
		t.Fatalf("AggregateBy: %v", err)
	}
	if len(by) != 1 || by[0].Requests != 1 || by[0].CostUSD != 0.11 {
		t.Fatalf("AggregateBy window = %+v (want 1 req, cost 0.11)", by)
	}
	ser, err := s.Series(f, "1h")
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if len(ser) != 1 || ser[0].Requests != 1 || ser[0].CostUSD != 0.11 {
		t.Fatalf("Series window = %+v (want 1 req, cost 0.11)", ser)
	}
}

func TestSeriesUnknownBucket(t *testing.T) {
	s := newStore(t)
	if _, err := s.Series(UsageFilter{}, "3m"); !errors.Is(err, ErrBadBucket) {
		t.Fatalf("want ErrBadBucket, got %v", err)
	}
}

func TestUsageGroupFilterAppliesToEveryProjection(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	grouped := func(id, group string, cost float64, ts time.Time) AIRequest {
		r := sampleReq(id, "alice", "basic", cost, ts)
		r.GroupID, r.GroupName = group, group+" display"
		return r
	}
	must(t, s.InsertBatch([]AIRequest{
		grouped("alpha-old", "alpha", 0.10, base),
		grouped("beta", "beta", 0.30, base.Add(time.Hour)),
		grouped("alpha-new", "alpha", 0.20, base.Add(24*time.Hour)),
		sampleReq("ungrouped", "alice", "basic", 0.40, base.Add(25*time.Hour)),
	}))

	assertProjection := func(t *testing.T, filter UsageFilter, wantGroupID, wantGroupName string, wantRequests int, wantCost float64) {
		t.Helper()
		aggregate, err := s.Aggregate(filter)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if len(aggregate) != 1 || aggregate[0].GroupID != wantGroupID || aggregate[0].GroupName != wantGroupName ||
			aggregate[0].Requests != wantRequests || aggregate[0].CostUSD != wantCost {
			t.Fatalf("Aggregate(%q) = %+v", filter.Group, aggregate)
		}

		series, err := s.Series(filter, "1d")
		if err != nil {
			t.Fatalf("Series: %v", err)
		}
		seriesRequests := 0
		seriesCost := 0.0
		for _, row := range series {
			seriesRequests += row.Requests
			seriesCost += row.CostUSD
		}
		if seriesRequests != wantRequests || roundCost(seriesCost) != wantCost {
			t.Fatalf("Series(%q) = %+v", filter.Group, series)
		}

		requests, err := s.Requests(filter, 200)
		if err != nil {
			t.Fatalf("Requests: %v", err)
		}
		requestCost := 0.0
		for _, row := range requests {
			if row.GroupID != wantGroupID || row.GroupName != wantGroupName {
				t.Fatalf("Requests(%q) leaked group row: %+v", filter.Group, row)
			}
			requestCost += row.CostUSD
		}
		if len(requests) != wantRequests || roundCost(requestCost) != wantCost {
			t.Fatalf("Requests(%q) = %+v", filter.Group, requests)
		}
	}

	t.Run("concrete group", func(t *testing.T) {
		assertProjection(t, UsageFilter{Group: "alpha"}, "alpha", "alpha display", 2, 0.30)
	})
	t.Run("ungrouped", func(t *testing.T) {
		assertProjection(t, UsageFilter{Group: UngroupedFilter}, "", "", 1, 0.40)
	})

	t.Run("empty filter keeps distinct snapshot aggregates", func(t *testing.T) {
		aggregate, err := s.Aggregate(UsageFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(aggregate) != 3 {
			t.Fatalf("Aggregate(all) = %+v, want three group-aware rows", aggregate)
		}
		groups := map[string]int{}
		for _, row := range aggregate {
			groups[row.GroupID] = row.Requests
		}
		if groups["alpha"] != 2 || groups["beta"] != 1 || groups[""] != 1 {
			t.Fatalf("Aggregate(all) groups = %+v", groups)
		}

		series, err := s.Series(UsageFilter{}, "1d")
		if err != nil {
			t.Fatal(err)
		}
		if len(series) != 2 || series[0].Requests != 2 || series[1].Requests != 2 {
			t.Fatalf("Series(all) = %+v", series)
		}

		requests, err := s.Requests(UsageFilter{}, 200)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) != 4 || requests[0].ID != "ungrouped" || requests[1].ID != "alpha-new" ||
			requests[2].ID != "beta" || requests[3].ID != "alpha-old" {
			t.Fatalf("Requests(all) newest-first = %+v", requests)
		}
	})
}

func TestUsageReportUsesSingleReadSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage-report.db")
	readerDB, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { readerDB.Close() })
	writerDB, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writerDB.Close() })

	reader := NewStore(readerDB, nil)
	writer := NewStore(writerDB, nil)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	must(t, reader.Insert(sampleReq("before", "alice", "basic", 0.10, base)))

	hookCalls := 0
	reader.setReportProjectionHook(func() {
		hookCalls++
		must(t, writer.Insert(sampleReq("between", "alice", "basic", 0.20, base.Add(24*time.Hour))))
	})
	rows, series, requests, err := reader.Report(UsageFilter{}, "1d", 200)
	if err != nil {
		t.Fatal(err)
	}
	if hookCalls != 1 {
		t.Fatalf("projection hook calls = %d, want 1", hookCalls)
	}

	aggregateRequests := 0
	for _, row := range rows {
		aggregateRequests += row.Requests
	}
	seriesRequests := 0
	for _, row := range series {
		seriesRequests += row.Requests
	}
	if aggregateRequests != seriesRequests || aggregateRequests != len(requests) {
		t.Fatalf("usage report mixed snapshots after forced insert: aggregate=%d series=%d requests=%d; want all projections to include the inserted row or all to exclude it",
			aggregateRequests, seriesRequests, len(requests))
	}
	if aggregateRequests != 1 || requests[0].ID != "before" {
		t.Fatalf("usage report did not retain its pre-insert snapshot: aggregate=%d requests=%+v", aggregateRequests, requests)
	}
	committed, err := writer.Requests(UsageFilter{}, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed) != 2 || committed[0].ID != "between" || committed[1].ID != "before" {
		t.Fatalf("concurrent writer did not commit both rows: %+v", committed)
	}
}

func TestUsageGroupFilterRequestsOrdersRFC3339NanoExactlyAtCap(t *testing.T) {
	requestAt := func(id, ts string) AIRequest {
		r := sampleReq(id, "alice", "basic", 0.01, time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC))
		r.TS = ts
		return r
	}

	t.Run("sub-millisecond order", func(t *testing.T) {
		s := newStore(t)
		must(t, s.InsertBatch([]AIRequest{
			requestAt("older", "2026-07-06T09:00:00.123Z"),
			requestAt("newer", "2026-07-06T09:00:00.123456Z"),
		}))

		rows, err := s.Requests(UsageFilter{}, 200)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].ID != "newer" || rows[1].ID != "older" {
			t.Fatalf("sub-millisecond request order = %+v, want newer then older", rows)
		}
	})

	t.Run("limit keeps the genuinely newer boundary row", func(t *testing.T) {
		s := newStore(t)
		requests := []AIRequest{
			requestAt("boundary-older", "2026-07-06T09:00:00.123Z"),
			requestAt("boundary-newer", "2026-07-06T09:00:00.123456Z"),
		}
		later := time.Date(2026, 7, 6, 9, 0, 1, 0, time.UTC)
		for i := 0; i < 199; i++ {
			requests = append(requests, sampleReq(fmt.Sprintf("later-%03d", i), "alice", "basic", 0.01, later.Add(time.Duration(i)*time.Second)))
		}
		must(t, s.InsertBatch(requests))

		rows, err := s.Requests(UsageFilter{}, 200)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 200 {
			t.Fatalf("Requests returned %d rows, want 200", len(rows))
		}
		seen := map[string]bool{}
		for _, row := range rows {
			seen[row.ID] = true
		}
		if !seen["boundary-newer"] || seen["boundary-older"] {
			t.Fatalf("boundary rows present = newer:%v older:%v, want newer only", seen["boundary-newer"], seen["boundary-older"])
		}
	})
}

// --- helpers ---

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func iterReq(r AIRequest, iter string) AIRequest { r.Iteration = iter; return r }

func index(t *testing.T, s *Store, groupBy string) map[string]GroupRow {
	t.Helper()
	rows, err := s.AggregateBy(UsageFilter{}, groupBy)
	if err != nil {
		t.Fatalf("AggregateBy(%s): %v", groupBy, err)
	}
	m := map[string]GroupRow{}
	for _, r := range rows {
		m[r.Key] = r
	}
	return m
}

func seriesByStart(t *testing.T, s *Store, bucket string) map[string]SeriesRow {
	t.Helper()
	rows, err := s.Series(UsageFilter{}, bucket)
	if err != nil {
		t.Fatalf("Series(%s): %v", bucket, err)
	}
	m := map[string]SeriesRow{}
	for _, r := range rows {
		m[r.BucketStart] = r
	}
	return m
}
