package commands

import (
	"strings"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

// validRuleScope accepts global | agent:<name> | group:<g>, validating the name
// with agent.ValidName BEFORE it becomes a scope string (injection guard: this
// is where a raw agent/group name enters a proxy rule).
func validRuleScope(scope string) bool {
	switch {
	case scope == "global":
		return true
	case strings.HasPrefix(scope, "agent:"):
		return agent.ValidName(strings.TrimPrefix(scope, "agent:"))
	case strings.HasPrefix(scope, "group:"):
		return agent.ValidName(strings.TrimPrefix(scope, "group:"))
	default:
		return false
	}
}

func ruleView(r aiproxy.PolicyRule) map[string]any {
	return map[string]any{
		"id": r.ID, "priority": r.Priority, "scope": r.Scope, "model_glob": r.ModelGlob,
		"kind": r.Kind, "max_requests": r.MaxRequests, "max_tokens": r.MaxTokens,
		"window_s": r.WindowS, "allow": r.Allow, "deny": r.Deny, "route": r.Route,
		"enabled": r.Enabled,
	}
}

func ruleSet() registry.Command {
	return registry.Command{
		Path:    "rule.set",
		Summary: "Set a proxy policy rule (kind rate-limit|model-policy, scope global|agent:<n>|group:<g>)",
		Args: []registry.Arg{
			{Name: "id", Flag: "id", Type: registry.String, Help: "rule id (generated if omitted)"},
			{Name: "scope", Flag: "scope", Short: "s", Type: registry.String, Required: true, Help: "global|agent:<n>|group:<g>"},
			{Name: "kind", Flag: "kind", Short: "k", Type: registry.String, Required: true, Help: "rate-limit|model-policy"},
			{Name: "priority", Flag: "priority", Type: registry.Int, Help: "evaluation order (asc; default 0)"},
			{Name: "model", Flag: "model", Short: "m", Type: registry.String, Help: "model glob to match (default any)"},
			{Name: "max-requests", Flag: "max-requests", Type: registry.Int, Help: "rate-limit: requests per window"},
			{Name: "max-tokens", Flag: "max-tokens", Type: registry.Int, Help: "rate-limit: tokens per window"},
			{Name: "window-s", Flag: "window-s", Type: registry.Int, Help: "rate-limit: rolling window seconds"},
			{Name: "allow", Flag: "allow", Type: registry.String, Help: "model-policy: comma-separated allow globs"},
			{Name: "deny", Flag: "deny", Type: registry.String, Help: "model-policy: comma-separated deny globs"},
			{Name: "route", Flag: "route", Type: registry.String, Help: "model-policy: rewrite model to this"},
			{Name: "disabled", Flag: "disabled", Type: registry.Bool, Help: "create the rule disabled"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/proxy-rules"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			scope := str(p, "scope")
			if !validRuleScope(scope) {
				return nil, api.UserError{Code: "bad_scope", Msg: "invalid scope " + scope + " (want global|agent:<n>|group:<g>)"}
			}
			kind := str(p, "kind")
			if kind != "rate-limit" && kind != "model-policy" {
				return nil, api.UserError{Code: "bad_kind", Msg: "invalid kind " + kind + " (want rate-limit|model-policy)"}
			}
			r := aiproxy.PolicyRule{
				ID: str(p, "id"), Priority: intOf(p, "priority", 0), Scope: scope, ModelGlob: str(p, "model"),
				Kind: kind, MaxRequests: intOf(p, "max-requests", 0), MaxTokens: intOf(p, "max-tokens", 0),
				WindowS: intOf(p, "window-s", 0), Allow: parseList(str(p, "allow")), Deny: parseList(str(p, "deny")),
				Route: str(p, "route"), Enabled: !toBool(p["disabled"]),
			}
			if r.Kind == "rate-limit" {
				if r.WindowS <= 0 {
					return nil, api.UserError{Code: "bad_window", Msg: "rate-limit requires a positive --window-s"}
				}
				if r.MaxRequests <= 0 && r.MaxTokens <= 0 {
					return nil, api.UserError{Code: "bad_limit", Msg: "rate-limit requires a positive --max-requests or --max-tokens"}
				}
			}
			if r.Kind == "model-policy" && len(r.Allow) == 0 && len(r.Deny) == 0 && r.Route == "" {
				return nil, api.UserError{Code: "bad_policy", Msg: "model-policy requires --allow, --deny, or --route"}
			}
			store := aiproxy.NewStore(c.Store, nil)
			if err := store.SetRule(r); err != nil {
				return nil, err
			}
			if c.Policy != nil {
				_ = c.Policy.Refresh()
			}
			// Re-read to surface the generated id / stored form.
			list, err := store.ListRules()
			if err != nil {
				return nil, err
			}
			for _, got := range list {
				if r.ID != "" && got.ID == r.ID {
					return ruleView(got), nil
				}
			}
			// id was generated: return the most-recent matching rule.
			for i := len(list) - 1; i >= 0; i-- {
				if list[i].Scope == r.Scope && list[i].Kind == r.Kind {
					return ruleView(list[i]), nil
				}
			}
			return map[string]any{"scope": r.Scope, "kind": r.Kind}, nil
		},
	}
}

func ruleLs() registry.Command {
	return registry.Command{
		Path:    "rule.ls",
		Summary: "List proxy policy rules (evaluation order)",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/proxy-rules"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			list, err := aiproxy.NewStore(c.Store, nil).ListRules()
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(list))
			for _, r := range list {
				rows = append(rows, ruleView(r))
			}
			return map[string]any{"rules": rows, "count": len(rows)}, nil
		},
	}
}

func ruleRm() registry.Command {
	return registry.Command{
		Path:    "rule.rm",
		Summary: "Remove a proxy policy rule by id",
		Args:    []registry.Arg{{Name: "id", Type: registry.String, Required: true, Help: "rule id"}},
		HTTP:    &registry.HTTPRoute{Method: "DELETE", Path: "/api/proxy-rules/{id}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			id := str(p, "id")
			if id == "" {
				return nil, api.UserError{Code: "bad_id", Msg: "id is required"}
			}
			if err := aiproxy.NewStore(c.Store, nil).DeleteRule(id); err != nil {
				return nil, err
			}
			if c.Policy != nil {
				_ = c.Policy.Refresh()
			}
			return map[string]any{"removed": id}, nil
		},
	}
}

// compile-time: the cache satisfies the refresher seam.
var _ registry.PolicyRefresher = (*aiproxy.PolicyCache)(nil)
