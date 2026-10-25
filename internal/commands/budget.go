package commands

import (
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

// validBudgetScope reports whether scope is one of the accepted forms:
// "global", "agent:<name>" or "group:<name>" with a non-empty name.
func validBudgetScope(scope string) bool {
	if scope == "global" {
		return true
	}
	for _, prefix := range []string{"agent:", "group:"} {
		if strings.HasPrefix(scope, prefix) && len(scope) > len(prefix) {
			return true
		}
	}
	return false
}

func budgetSet() registry.Command {
	return registry.WithResult[BudgetLimit](registry.Command{
		Path:    "budget.set",
		Summary: "Set a cost budget (scope agent:<name>|group:<g>|global)",
		Args: []registry.Arg{
			{Name: "scope", Flag: "scope", Short: "s", Type: registry.String, Required: true, Help: "agent:<name>|group:<g>|global"},
			{Name: "limit-usd", Flag: "limit-usd", Short: "l", Type: registry.String, Required: true, Help: "USD limit"},
			{Name: "period", Flag: "period", Short: "p", Type: registry.String, Help: "rolling period, e.g. 24h (default 24h)"},
			{Name: "mode", Flag: "mode", Short: "m", Type: registry.String, Help: "warn|block (default warn)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/budgets"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			scope := p.String("scope")
			if !validBudgetScope(scope) {
				return nil, api.UserError{Code: "bad_scope", Msg: "scope must be global, agent:<name>, or group:<name>"}
			}
			limit, err := parseFloat(p.String("limit-usd"))
			if err != nil {
				return nil, api.UserError{Code: "bad_limit", Msg: "limit-usd must be a number"}
			}
			if limit < 0 {
				return nil, api.UserError{Code: "bad_limit", Msg: "limit-usd must be non-negative"}
			}
			periodS := 86400
			if pr := p.String("period"); pr != "" {
				d, derr := time.ParseDuration(pr)
				if derr != nil || d <= 0 {
					return nil, api.UserError{Code: "bad_period", Msg: "period must be a duration like 24h"}
				}
				periodS = int(d.Seconds())
			}
			mode := p.String("mode")
			if mode == "" {
				mode = "warn"
			}
			if mode != "warn" && mode != "block" {
				return nil, api.UserError{Code: "bad_mode", Msg: "mode must be warn|block"}
			}
			b := aiproxy.Budget{Scope: scope, LimitUSD: limit, PeriodS: periodS, Mode: mode}
			if err := aiproxy.NewStore(c.Store, nil).SetBudget(b); err != nil {
				return nil, err
			}
			return newBudgetLimit(b), nil
		},
	})
}

func newBudgetLimit(b aiproxy.Budget) BudgetLimit {
	return BudgetLimit{Scope: b.Scope, LimitUSD: b.LimitUSD, PeriodS: b.PeriodS, Mode: b.Mode}
}

func budgetLs() registry.Command {
	return registry.WithResult[BudgetList](registry.Command{
		Path:    "budget.ls",
		Summary: "List configured budgets",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/budgets"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			list, err := aiproxy.NewStore(c.Store, nil).ListBudgets()
			if err != nil {
				return nil, err
			}
			rows := make([]BudgetLimit, 0, len(list))
			for _, b := range list {
				rows = append(rows, newBudgetLimit(b))
			}
			return BudgetList{Budgets: rows, Count: len(rows)}, nil
		},
	})
}

func budgetStatus() registry.Command {
	return registry.WithResult[BudgetStatusList](registry.Command{
		Path:    "budget.status",
		Summary: "Show current spend vs limit for each budget",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/budgets/status"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			s := aiproxy.NewStore(c.Store, nil)
			list, err := s.ListBudgets()
			if err != nil {
				return nil, err
			}
			// Reverse the agent->group map into group->members so a group budget's
			// spend aggregates its current members exactly like proxy enforcement
			// (BudgetCache.Refresh via GroupCostSince), not a global CostSince("").
			groupOf, err := s.AgentGroups()
			if err != nil {
				return nil, err
			}
			membersByGroup := map[string][]string{}
			for ag, g := range groupOf {
				membersByGroup[g] = append(membersByGroup[g], ag)
			}
			now := time.Now()
			rows := make([]BudgetSpend, 0, len(list))
			for _, b := range list {
				since := now.Add(-time.Duration(b.PeriodS) * time.Second)
				var spent float64
				switch {
				case b.Scope == "global":
					spent, err = s.CostSince("", since)
				case len(b.Scope) > 6 && b.Scope[:6] == "agent:":
					spent, err = s.CostSince(b.Scope[6:], since)
				case len(b.Scope) > 6 && b.Scope[:6] == "group:":
					spent, err = s.GroupCostSince(membersByGroup[b.Scope[6:]], since)
				default:
					continue
				}
				if err != nil {
					return nil, err
				}
				rows = append(rows, BudgetSpend{Scope: b.Scope, LimitUSD: b.LimitUSD,
					SpentUSD: spent, Mode: b.Mode, Over: spent >= b.LimitUSD})
			}
			return BudgetStatusList{Budgets: rows, Count: len(rows)}, nil
		},
	})
}
