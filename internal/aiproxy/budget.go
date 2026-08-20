package aiproxy

import (
	"database/sql"
	"sync"
	"time"
)

type Budget struct {
	Scope     string // agent:<name> | group:<g> | global
	LimitUSD  float64
	PeriodS   int
	Mode      string // warn | block
	CreatedAt string
}

func (s *Store) SetBudget(b Budget) error {
	if b.Mode == "" {
		b.Mode = "warn"
	}
	if b.PeriodS == 0 {
		b.PeriodS = 86400
	}
	_, err := s.db.Exec(`INSERT INTO budgets(scope, limit_usd, period_s, mode)
		VALUES (?,?,?,?)
		ON CONFLICT(scope) DO UPDATE SET limit_usd=excluded.limit_usd,
			period_s=excluded.period_s, mode=excluded.mode`,
		b.Scope, b.LimitUSD, b.PeriodS, b.Mode)
	return err
}

func (s *Store) ListBudgets() ([]Budget, error) {
	rows, err := s.db.Query(`SELECT scope, limit_usd, period_s, mode, created_at FROM budgets ORDER BY scope`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Budget
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.Scope, &b.LimitUSD, &b.PeriodS, &b.Mode, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetBudget(scope string) (Budget, bool, error) {
	var b Budget
	err := s.db.QueryRow(`SELECT scope, limit_usd, period_s, mode, created_at FROM budgets WHERE scope=?`,
		scope).Scan(&b.Scope, &b.LimitUSD, &b.PeriodS, &b.Mode, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return Budget{}, false, nil
	}
	return b, err == nil, err
}

func (s *Store) DeleteBudget(scope string) error {
	_, err := s.db.Exec(`DELETE FROM budgets WHERE scope=?`, scope)
	return err
}

// Decision is the outcome of a budget check for one scope.
type Decision struct {
	Over     bool
	Mode     string
	Scope    string
	LimitUSD float64
	SpentUSD float64
}

// BudgetCache holds pre-computed spend per scope, refreshed periodically so the
// request path never scans ai_requests (spec §9).
type BudgetCache struct {
	store   *Store
	clock   func() time.Time
	mu      sync.RWMutex
	decided map[string]Decision // scope -> decision
	groupOf map[string]string   // agent -> group (for Check)
}

func NewBudgetCache(s *Store, clock func() time.Time) *BudgetCache {
	if clock == nil {
		clock = time.Now
	}
	return &BudgetCache{store: s, clock: clock, decided: map[string]Decision{}, groupOf: map[string]string{}}
}

// Refresh recomputes each budget's spend over its rolling period. Group scopes
// aggregate the spend of their members (agents.group), and the agent->group map
// is cached so Check can fold a member's group decision in without a DB hit.
func (c *BudgetCache) Refresh() error {
	budgets, err := c.store.ListBudgets()
	if err != nil {
		return err
	}
	groupOf, err := c.store.AgentGroups()
	if err != nil {
		return err
	}
	membersByGroup := map[string][]string{}
	for ag, g := range groupOf {
		membersByGroup[g] = append(membersByGroup[g], ag)
	}
	now := c.clock()
	next := map[string]Decision{}
	for _, b := range budgets {
		since := now.Add(-time.Duration(b.PeriodS) * time.Second)
		var spent float64
		switch {
		case b.Scope == "global":
			spent, err = c.store.CostSince("", since)
		case len(b.Scope) > 6 && b.Scope[:6] == "agent:":
			spent, err = c.store.CostSince(b.Scope[6:], since)
		case len(b.Scope) > 6 && b.Scope[:6] == "group:":
			spent, err = c.store.GroupCostSince(membersByGroup[b.Scope[6:]], since)
		default:
			continue
		}
		if err != nil {
			return err
		}
		next[b.Scope] = Decision{Over: spent >= b.LimitUSD, Mode: b.Mode, Scope: b.Scope,
			LimitUSD: b.LimitUSD, SpentUSD: spent}
	}
	c.mu.Lock()
	c.decided = next
	c.groupOf = groupOf
	c.mu.Unlock()
	return nil
}

// Check returns the worst applicable decision for an agent across its agent,
// group and global scopes. block beats warn. A non-over agent scope still
// yields a group/global block if either is over.
func (c *BudgetCache) Check(agent string) Decision {
	c.mu.RLock()
	defer c.mu.RUnlock()
	scopes := []string{"agent:" + agent, "global"}
	if g := c.groupOf[agent]; g != "" {
		scopes = append(scopes, "group:"+g)
	}
	worst := Decision{}
	for _, scope := range scopes {
		if d, ok := c.decided[scope]; ok && d.Over {
			// block beats warn.
			if !worst.Over || (worst.Mode != "block" && d.Mode == "block") {
				worst = d
			}
		}
	}
	return worst
}
