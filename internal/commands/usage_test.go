package commands

import (
	"math"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestUsageAndBudget(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	now := time.Now().UTC()
	ap := aiproxy.NewStore(c.Store, func() time.Time { return now })
	base := now.Add(-1 * time.Hour)
	ap.Insert(aiproxy.AIRequest{ID: "r1", TS: base.Format(time.RFC3339Nano), Agent: "alice",
		ImageName: "basic", Provider: "anthropic", Model: "claude-opus-4-8",
		InputTokens: 100, OutputTokens: 50, CostUSD: 0.10, Status: "ok"})

	res, err := h(t, "usage")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	rows := res.(map[string]any)["rows"].([]map[string]any)
	if len(rows) != 1 || rows[0]["agent"] != "alice" || rows[0]["cost_usd"].(float64) != 0.10 {
		t.Fatalf("usage rows = %+v", rows)
	}

	// budget set + ls + status.
	if _, err := h(t, "budget.set")(c, registry.Params{"scope": "agent:alice", "limit-usd": "0.05",
		"period": "24h", "mode": "block"}); err != nil {
		t.Fatal(err)
	}
	ls, _ := h(t, "budget.ls")(c, registry.Params{})
	if ls.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("budget ls = %v", ls)
	}
	st, err := h(t, "budget.status")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	srows := st.(map[string]any)["budgets"].([]map[string]any)
	if len(srows) != 1 || srows[0]["over"].(bool) != true {
		t.Fatalf("budget status = %+v", srows)
	}
}

func TestUsageGroupFilterKeepsResponseProjectionsConsistent(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	ap := aiproxy.NewStore(c.Store, nil)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	request := func(id, group string, cost float64, ts time.Time) aiproxy.AIRequest {
		r := aiproxy.AIRequest{
			ID: id, TS: ts.Format(time.RFC3339Nano), Agent: "alice", Iteration: "alice-1",
			ImageName: "basic", Provider: "anthropic", Model: "claude-opus-4-8",
			InputTokens: 100, OutputTokens: 50, CacheWriteTokens: 10, CacheReadTokens: 5,
			CostUSD: cost, Status: "ok", GroupID: group, GroupName: group + " display",
		}
		if group == "" {
			r.GroupName = ""
		}
		return r
	}
	if err := ap.InsertBatch([]aiproxy.AIRequest{
		request("alpha-old", "alpha", 0.10, base),
		request("beta", "beta", 0.30, base.Add(time.Hour)),
		request("alpha-new", "alpha", 0.20, base.Add(24*time.Hour)),
		request("ungrouped", "", 0.40, base.Add(25*time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, group  string
		wantRequests int
		wantCost     float64
		wantRows     int
	}{
		{name: "concrete group", group: "alpha", wantRequests: 2, wantCost: 0.30, wantRows: 1},
		{name: "ungrouped", group: aiproxy.UngroupedFilter, wantRequests: 1, wantCost: 0.40, wantRows: 1},
		{name: "all groups", wantRequests: 4, wantCost: 1.00, wantRows: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := registry.Params{}
			if tt.group != "" {
				params["group"] = tt.group
			}
			res, err := h(t, "usage")(c, params)
			if err != nil {
				t.Fatal(err)
			}
			report := res.(map[string]any)
			rows := report["rows"].([]map[string]any)
			series := report["series"].([]map[string]any)
			requests := report["requests"].([]map[string]any)
			if len(rows) != tt.wantRows || report["count"].(int) != tt.wantRows {
				t.Fatalf("rows = %+v count=%v", rows, report["count"])
			}
			if report["total_requests"].(int) != tt.wantRequests || report["total_cost_usd"].(float64) != tt.wantCost ||
				report["total_input_tokens"].(int) != 100*tt.wantRequests || report["total_output_tokens"].(int) != 50*tt.wantRequests ||
				report["total_cache_write_tokens"].(int) != 10*tt.wantRequests || report["total_cache_read_tokens"].(int) != 5*tt.wantRequests {
				t.Fatalf("totals = %+v", report)
			}

			seriesRequests, seriesCost := 0, 0.0
			for _, row := range series {
				seriesRequests += row["requests"].(int)
				seriesCost += row["cost_usd"].(float64)
			}
			requestCost := 0.0
			for _, row := range requests {
				requestCost += row["cost_usd"].(float64)
				if _, ok := row["group_id"]; !ok {
					t.Fatalf("request missing group_id: %+v", row)
				}
				if _, ok := row["group_name"]; !ok {
					t.Fatalf("request missing group_name: %+v", row)
				}
				if tt.group == "alpha" && row["group_id"] != "alpha" {
					t.Fatalf("alpha response leaked request: %+v", row)
				}
				if tt.group == aiproxy.UngroupedFilter && row["group_id"] != "" {
					t.Fatalf("ungrouped response leaked request: %+v", row)
				}
			}
			if seriesRequests != tt.wantRequests || len(requests) != tt.wantRequests ||
				math.Round(seriesCost*1e6)/1e6 != tt.wantCost || math.Round(requestCost*1e6)/1e6 != tt.wantCost {
				t.Fatalf("projection mismatch: series=%+v requests=%+v", series, requests)
			}
		})
	}
}

// TestBudgetStatusShowsGroupScope guards that an enforced group:<name> budget
// (M8 Task 6) is visible in `budget status` with a member-aggregate spend that
// matches enforcement (GroupCostSince), and does NOT fold in non-member/global
// spend the way a naive CostSince("") would.
func TestBudgetStatusShowsGroupScope(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	ap := aiproxy.NewStore(c.Store, nil)

	// Two members of group "research-team" plus a non-member agent.
	as.Create(agent.Agent{Name: "scout", ImageRef: "i:latest", Group: "research-team"})
	as.Create(agent.Agent{Name: "writer", ImageRef: "i:latest", Group: "research-team"})
	as.Create(agent.Agent{Name: "outsider", ImageRef: "i:latest"})

	now := time.Now().UTC().Format(time.RFC3339Nano)
	rec := func(id, ag string, cost float64) {
		if err := ap.Insert(aiproxy.AIRequest{ID: id, TS: now, Agent: ag, Iteration: "i1",
			CostUSD: cost, Status: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	// Members spend 3 + 4 = 7. The non-member's 100 is deliberately large so a
	// naive global CostSince("") sum (107) would visibly overshoot.
	rec("r1", "scout", 3)
	rec("r2", "writer", 4)
	rec("r3", "outsider", 100)

	if _, err := h(t, "budget.set")(c, registry.Params{"scope": "group:research-team",
		"limit-usd": "10", "period": "24h", "mode": "block"}); err != nil {
		t.Fatal(err)
	}

	st, err := h(t, "budget.status")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	rows := st.(map[string]any)["budgets"].([]map[string]any)
	var got map[string]any
	for _, r := range rows {
		if r["scope"] == "group:research-team" {
			got = r
		}
	}
	// Before the fix the group scope is skipped entirely -> absent (RED).
	if got == nil {
		t.Fatalf("group:research-team scope absent from budget status: %+v", rows)
	}
	// Member-aggregate spend, NOT a global sum that would include outsider's 100.
	if spent := got["spent_usd"].(float64); spent != 7 {
		t.Fatalf("group spend = %v, want 7 (members scout+writer only, excludes outsider's 100)", spent)
	}
	if got["over"].(bool) {
		t.Fatalf("group spend 7 under 10 limit must not be over: %+v", got)
	}
}

func TestBudgetSetValidation(t *testing.T) {
	c, _, _ := ctxWithStore(t)

	countBudgets := func() int {
		ls, err := h(t, "budget.ls")(c, registry.Params{})
		if err != nil {
			t.Fatal(err)
		}
		return ls.(map[string]any)["count"].(int)
	}

	// Bogus scope is rejected and nothing is stored.
	_, err := h(t, "budget.set")(c, registry.Params{"scope": "bogus", "limit-usd": "1"})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for bogus scope, got %v", err)
	}
	if got := countBudgets(); got != 0 {
		t.Fatalf("bogus scope: expected 0 budgets stored, got %d", got)
	}

	// Empty agent name after "agent:" is rejected.
	_, err = h(t, "budget.set")(c, registry.Params{"scope": "agent:", "limit-usd": "1"})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for empty agent name, got %v", err)
	}
	if got := countBudgets(); got != 0 {
		t.Fatalf("empty agent name: expected 0 budgets stored, got %d", got)
	}

	// Negative limit is rejected and nothing is stored.
	_, err = h(t, "budget.set")(c, registry.Params{"scope": "agent:alice", "limit-usd": "-5"})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("expected api.UserError for negative limit, got %v", err)
	}
	if got := countBudgets(); got != 0 {
		t.Fatalf("negative limit: expected 0 budgets stored, got %d", got)
	}

	// Valid global scope still succeeds (regression).
	if _, err := h(t, "budget.set")(c, registry.Params{"scope": "global", "limit-usd": "10"}); err != nil {
		t.Fatalf("global scope should succeed: %v", err)
	}
	if got := countBudgets(); got != 1 {
		t.Fatalf("global scope: expected 1 budget stored, got %d", got)
	}

	// group:<g> is accepted for forward-compat even though enforcement is deferred.
	if _, err := h(t, "budget.set")(c, registry.Params{"scope": "group:team", "limit-usd": "10"}); err != nil {
		t.Fatalf("group scope should succeed: %v", err)
	}
	if got := countBudgets(); got != 2 {
		t.Fatalf("group scope: expected 2 budgets stored, got %d", got)
	}
}
