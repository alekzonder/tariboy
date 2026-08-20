package commands

import (
	"regexp"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/retention"
)

func requireRetention(c *registry.Ctx) (*retention.RetentionAPI, error) {
	if c.Retention == nil {
		return nil, api.UserError{Code: "no_retention", Msg: "retention is not available"}
	}
	return c.Retention, nil
}

// agentNameRE mirrors plugins.ValidName's pattern: no separators, no leading
// dot/dash, so a value can never lexically escape a directory it is joined
// into (filepath.Join(agentsDir, name, ...)).
var agentNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// checkAgentOrKeyword rejects a traversing/invalid agent name at the CLI
// layer with a clean 400 (bad_agent), before it reaches retention.Store or
// retention.Pruner. The pruner builds filesystem paths directly from the
// agent name (agentdir.New, and the archive dir "<agentsDir>/<name>/archive"),
// so an unsanitized value such as "../victim" must never reach it — this is
// the M6 path-traversal lesson (plugin names hit the same class of bug)
// applied to the retention command surface. extra lists additional bare
// keywords this command accepts besides a real agent name (e.g. "default",
// "all").
func checkAgentOrKeyword(name string, extra ...string) error {
	for _, k := range extra {
		if name == k {
			return nil
		}
	}
	if !agentNameRE.MatchString(name) {
		return api.UserError{Code: "bad_agent", Msg: "invalid agent name " + name}
	}
	return nil
}

func policyToMap(p retention.Policy) map[string]any {
	return map[string]any{
		"keep_iterations": p.KeepIterations, "keep_days": p.KeepDays,
		"max_bytes": p.MaxBytes, "archive": p.Archive,
	}
}

func retentionGet() registry.Command {
	return registry.Command{
		Path:    "retention.get",
		Summary: "Show the effective retention policy for an agent (or 'default')",
		Args:    []registry.Arg{{Name: "agent", Type: registry.String, Required: true, Help: "agent name, or 'default'"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{agent}/retention"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			r, err := requireRetention(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "agent")
			if err := checkAgentOrKeyword(name, "default"); err != nil {
				return nil, err
			}
			if name == "default" {
				pol, err := r.Policies.Default()
				if err != nil {
					return nil, err
				}
				return policyToMap(pol), nil
			}
			pol, err := r.Policies.Effective(name)
			if err != nil {
				return nil, err
			}
			return policyToMap(pol), nil
		},
	}
}

func retentionSet() registry.Command {
	return registry.Command{
		Path:    "retention.set",
		Summary: "Set the retention policy for an agent (or 'default')",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "agent name, or 'default'"},
			{Name: "keep-iterations", Flag: "keep-iterations", Short: "i", Type: registry.Int, Help: "keep newest N iterations (0=unlimited)"},
			{Name: "keep-days", Flag: "keep-days", Short: "d", Type: registry.Int, Help: "keep iterations newer than N days (0=unlimited)"},
			{Name: "max-bytes", Flag: "max-bytes", Short: "b", Type: registry.Int, Help: "cap total iteration bytes (0=unlimited)"},
			{Name: "archive", Flag: "archive", Short: "a", Type: registry.Bool, Help: "archive pruned iterations to tar.gz (default true)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{agent}/retention"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			r, err := requireRetention(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "agent")
			if err := checkAgentOrKeyword(name, "default"); err != nil {
				return nil, err
			}
			// Read-modify-write the current policy so unset flags are preserved.
			var cur retention.Policy
			if name == "default" {
				cur, err = r.Policies.Default()
			} else if got, ok, gerr := r.Policies.Get(name); gerr == nil && ok {
				cur = got
			} else if gerr != nil {
				err = gerr
			} else {
				cur = retention.Policy{Archive: true}
			}
			if err != nil {
				return nil, err
			}
			if _, ok := p["keep-iterations"]; ok {
				cur.KeepIterations = intOf(p, "keep-iterations", cur.KeepIterations)
			}
			if _, ok := p["keep-days"]; ok {
				cur.KeepDays = intOf(p, "keep-days", cur.KeepDays)
			}
			if _, ok := p["max-bytes"]; ok {
				cur.MaxBytes = int64(intOf(p, "max-bytes", int(cur.MaxBytes)))
			}
			// A negative limit is nonsensical: the pruner treats <=0 as unlimited,
			// so storing -5 silently means "unlimited" and misleads the operator.
			// Reject it with a clean bad_value error instead.
			if cur.KeepIterations < 0 || cur.KeepDays < 0 || cur.MaxBytes < 0 {
				return nil, api.UserError{Code: "bad_value", Msg: "keep_iterations, keep_days and max_bytes must be >= 0"}
			}
			if v, ok := p["archive"].(bool); ok {
				cur.Archive = v
			}
			if name == "default" {
				if err := r.Policies.SetDefault(cur); err != nil {
					return nil, err
				}
			} else if err := r.Policies.Set(name, cur); err != nil {
				return nil, err
			}
			out := policyToMap(cur)
			out["agent"] = name
			return out, nil
		},
	}
}

func pruneCommand() registry.Command {
	return registry.Command{
		Path:    "prune",
		Summary: "Prune old iterations for an agent now (or 'all'); --dry-run lists victims",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "agent name, or 'all'"},
			{Name: "dry-run", Flag: "dry-run", Short: "n", Type: registry.Bool, Help: "list what would be removed without deleting"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{agent}/prune"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			r, err := requireRetention(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "agent")
			if err := checkAgentOrKeyword(name, "all"); err != nil {
				return nil, err
			}
			dry, _ := p["dry-run"].(bool)
			if name == "all" {
				reps, err := r.Pruner.PruneAll(dry)
				if err != nil {
					return nil, err
				}
				return map[string]any{"reports": reps, "dry_run": dry}, nil
			}
			rep, err := r.Pruner.PruneAgent(name, dry)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"agent": rep.Agent, "pruned": rep.Pruned, "archived": rep.Archived,
				"freed_bytes": rep.FreedBytes, "dry_run": rep.DryRun,
			}, nil
		},
	}
}
