package aiproxy

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// planQueryer runs each query and records its EXPLAIN QUERY PLAN first.
type planQueryer struct {
	db    *sql.DB
	plans *[]string
}

func (p planQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	*p.plans = append(*p.plans, explain(p.db, query, args...))
	return p.db.Query(query, args...)
}

func explain(db *sql.DB, query string, args ...any) string {
	rows, err := db.Query(`EXPLAIN QUERY PLAN `+query, args...)
	if err != nil {
		return "error: " + err.Error()
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return "error: " + err.Error()
		}
		details = append(details, detail)
	}
	return strings.Join(details, "; ")
}

// tsIndexSeek matches a search that bounds ts inside one of the ts indexes,
// not only a seek on agent with every ts filtered row by row.
var tsIndexSeek = regexp.MustCompile(`INDEX idx_ai_requests_(ts|agent_ts_cost) \([^)]*ts[<>]`)

func TestUsageQueriesUseTsIndex(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for _, agent := range []string{"", "alice"} {
		filters := map[string]UsageFilter{
			"since":       {Agent: agent, Since: store.FormatTime(since)},
			"until":       {Agent: agent, Until: store.FormatTime(since)},
			"since+until": {Agent: agent, Since: store.FormatTime(since), Until: store.FormatTime(since.Add(time.Hour))},
		}
		for name, f := range filters {
			var plans []string
			q := planQueryer{db: s.db, plans: &plans}
			if _, err := aggregate(q, f); err != nil {
				t.Fatal(err)
			}
			if _, err := requests(q, f, 10); err != nil {
				t.Fatal(err)
			}
			if _, err := series(q, f, "1h"); err != nil {
				t.Fatal(err)
			}
			if _, err := taskUsage(q, f); err != nil {
				t.Fatal(err)
			}
			for i, plan := range plans {
				if !tsIndexSeek.MatchString(plan) {
					t.Errorf("agent=%q %s query %d plan = %q; want a ts index range", agent, name, i, plan)
				}
			}
		}
		query, args := costSinceQuery(agent, since)
		if plan := explain(s.db, query, args...); !tsIndexSeek.MatchString(plan) {
			t.Errorf("CostSince(agent=%q) plan = %q; want a ts index range", agent, plan)
		}
	}
	query, args := agentWindowSpendQuery("alice", since)
	if plan := explain(s.db, query, args...); !tsIndexSeek.MatchString(plan) {
		t.Errorf("agent budget plan = %q; want a ts index range", plan)
	}
	query, args = allAgentsWindowSpendQuery(since)
	if plan := explain(s.db, query, args...); !tsIndexSeek.MatchString(plan) {
		t.Errorf("all-agents budget plan = %q; want a ts index range", plan)
	}
}

// Rows one nanosecond apart around a bound land on the right side of it,
// including two values in one second whose RFC3339Nano text has a different
// width (10ns and 11ns).
func TestUsageBoundsAreExactToTheNanosecond(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	bound := time.Date(2026, 7, 6, 9, 0, 0, 11, time.UTC)
	for id, ts := range map[string]time.Time{
		"before": bound.Add(-time.Nanosecond),
		"at":     bound,
		"after":  bound.Add(time.Nanosecond),
	} {
		r := sampleReq(id, "alice", "basic", 1, ts)
		r.TS = ts.Format(time.RFC3339Nano)
		if err := s.Insert(r); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.Requests(UsageFilter{Since: store.FormatTime(bound)}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "after" || rows[1].ID != "at" {
		t.Fatalf("Since rows = %+v; want after, at", rows)
	}
	rows, err = s.Requests(UsageFilter{Until: store.FormatTime(bound)}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "before" {
		t.Fatalf("Until rows = %+v; want before", rows)
	}
	cost, err := s.CostSince("alice", bound)
	if err != nil || cost != 2 {
		t.Fatalf("CostSince = %v, %v; want 2", cost, err)
	}
}
