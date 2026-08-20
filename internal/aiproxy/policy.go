package aiproxy

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
)

// PolicyDecision is the outcome of the policy engine for one request. Zero value
// = allow. The Task 7 middleware turns Deny into 403, RateLimited into 429, and
// RewriteModel into an in-place model rewrite before forward.
type PolicyDecision struct {
	Deny         bool
	DenyReason   string
	RateLimited  bool
	RateReason   string
	RewriteModel string
}

// compiledRule is a rule snapshot; for rate-limit, over/reason are precomputed at
// refresh so Decide is a pure, lock-guarded, DB-free read.
type compiledRule struct {
	rule   PolicyRule
	over   bool
	reason string
}

// PolicyCache is the race-safe policy engine (spec §9). Refreshed off proxy_rules
// on the daemon's 15s ticker; Decide is read on the proxy request path under RLock.
type PolicyCache struct {
	store   *Store
	clock   func() time.Time
	mu      sync.RWMutex
	rules   []compiledRule
	groupOf map[string]string // agent -> group
}

func NewPolicyCache(s *Store, clock func() time.Time) *PolicyCache {
	if clock == nil {
		clock = time.Now
	}
	return &PolicyCache{store: s, clock: clock, groupOf: map[string]string{}}
}

// Refresh recompiles enabled rules and precomputes each rate-limit rule's window
// state. Malformed rules (unknown kind, unparseable/invalid scope, non-positive
// window) are skipped — never fail-open into blocking all traffic.
func (c *PolicyCache) Refresh() error {
	rules, err := c.store.ListRules()
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
	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		// Defense in depth: every scope is parsed and its extracted agent/group
		// name validated before use. An unparseable or invalid-name scope is
		// malformed and skipped (never enforced).
		kind, name, scopeOK := parseScope(r.Scope)
		if !scopeOK {
			continue
		}
		switch r.Kind {
		case "model-policy":
			compiled = append(compiled, compiledRule{rule: r})
		case "rate-limit":
			if r.WindowS <= 0 {
				continue // malformed ⇒ skip (allow)
			}
			members, global := resolveMembers(kind, name, membersByGroup)
			since := now.Add(-time.Duration(r.WindowS) * time.Second)
			over, reason, err := c.overWindow(r, members, global, since)
			if err != nil {
				return err
			}
			compiled = append(compiled, compiledRule{rule: r, over: over, reason: reason})
		default:
			continue // unknown kind ⇒ skip
		}
	}
	c.mu.Lock()
	c.rules = compiled
	c.groupOf = groupOf
	c.mu.Unlock()
	return nil
}

// overWindow reports whether a rate-limit rule's scope is over its request or
// token limit within the window. A global scope uses nil members (all rows); a
// non-global scope with no members is trivially under (counts ZERO, never global).
func (c *PolicyCache) overWindow(r PolicyRule, members []string, global bool, since time.Time) (bool, string, error) {
	var qMembers []string
	if global {
		qMembers = nil // nil is reserved STRICTLY for the global scope (count all).
	} else if len(members) == 0 {
		// Non-global scope resolving to no members counts ZERO, not global. Short
		// circuit — equivalent to passing a non-nil empty slice to the store.
		return false, "", nil
	} else {
		qMembers = members
	}
	if r.MaxRequests > 0 {
		n, err := c.store.RequestCountSince(qMembers, since)
		if err != nil {
			return false, "", err
		}
		if n >= r.MaxRequests {
			return true, "request rate limit exceeded", nil
		}
	}
	if r.MaxTokens > 0 {
		tok, err := c.store.TokenSumSince(qMembers, since)
		if err != nil {
			return false, "", err
		}
		if tok >= r.MaxTokens {
			return true, "token rate limit exceeded", nil
		}
	}
	return false, "", nil
}

// Decide applies the locked precedence: model-policy first-match (deny | route |
// allow, terminal), then rate-limit aggregated (any over ⇒ limited). Zero value ⇒
// allow.
func (c *PolicyCache) Decide(agent, model string) PolicyDecision {
	c.mu.RLock()
	defer c.mu.RUnlock()
	group := c.groupOf[agent]
	var dec PolicyDecision
	policyDecided := false
	for _, cr := range c.rules {
		if !ruleScopeMatches(cr.rule.Scope, agent, group) {
			continue
		}
		if cr.rule.ModelGlob != "" && !globMatch(cr.rule.ModelGlob, model) {
			continue
		}
		switch cr.rule.Kind {
		case "model-policy":
			if policyDecided {
				continue
			}
			policyDecided = true
			if matchesAny(cr.rule.Deny, model) {
				dec.Deny = true
				dec.DenyReason = "model " + model + " denied by rule " + cr.rule.ID
				continue
			}
			if len(cr.rule.Allow) > 0 && !matchesAny(cr.rule.Allow, model) {
				dec.Deny = true
				dec.DenyReason = "model " + model + " not in allowlist of rule " + cr.rule.ID
				continue
			}
			if cr.rule.Route != "" {
				dec.RewriteModel = cr.rule.Route
			}
		case "rate-limit":
			if cr.over && !dec.RateLimited {
				dec.RateLimited = true
				dec.RateReason = cr.reason + " (rule " + cr.rule.ID + ")"
			}
		}
	}
	return dec
}

// parseScope validates a rule scope and extracts its kind and name. The name of
// an agent:/group: scope is validated with agent.ValidName (defense in depth —
// refuses path-traversal names such as "agent:../evil"). ok=false ⇒ malformed.
func parseScope(scope string) (kind, name string, ok bool) {
	switch {
	case scope == "global":
		return "global", "", true
	case strings.HasPrefix(scope, "agent:"):
		name = strings.TrimPrefix(scope, "agent:")
		return "agent", name, agent.ValidName(name)
	case strings.HasPrefix(scope, "group:"):
		name = strings.TrimPrefix(scope, "group:")
		return "group", name, agent.ValidName(name)
	default:
		return "", "", false
	}
}

// resolveMembers maps a parsed scope to its member agents for rate-limit counting.
// A global scope ⇒ (nil, true) so the store counts ALL rows. An agent/group scope
// ⇒ (members, false); an empty/unknown group yields a nil slice that overWindow
// treats as ZERO, never global.
func resolveMembers(kind, name string, membersByGroup map[string][]string) (members []string, global bool) {
	switch kind {
	case "global":
		return nil, true
	case "agent":
		return []string{name}, false
	case "group":
		return membersByGroup[name], false
	default:
		return nil, false
	}
}

func ruleScopeMatches(scope, agent, group string) bool {
	switch {
	case scope == "global":
		return true
	case scope == "agent:"+agent:
		return true
	case group != "" && scope == "group:"+group:
		return true
	default:
		return false
	}
}

func matchesAny(globs []string, model string) bool {
	for _, g := range globs {
		if globMatch(g, model) {
			return true
		}
	}
	return false
}

// rewriteModel replaces the top-level "model" field, preserving everything else.
// Best-effort: an unparseable body is returned unchanged.
func rewriteModel(body []byte, model string) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	mb, err := json.Marshal(model)
	if err != nil {
		return body
	}
	m["model"] = mb
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}
