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

func retentionGet() registry.Command {
	return registry.WithResult[RetentionView](registry.Command{
		Path:    "retention.get",
		Summary: "Show the effective retention policy for an agent, with its own override and the default (or 'default')",
		Args:    []registry.Arg{{Name: "agent", Type: registry.String, Required: true, Help: "agent name, or 'default'"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{agent}/retention"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			r, err := requireRetention(c)
			if err != nil {
				return nil, err
			}
			name := p.String("agent")
			if err := checkAgentOrKeyword(name, "default"); err != nil {
				return nil, err
			}
			if name == "default" {
				pol, err := r.Policies.Default()
				if err != nil {
					return nil, err
				}
				return RetentionView{RetentionPolicy: newRetentionPolicy(pol)}, nil
			}
			pol, err := r.Policies.Effective(name)
			if err != nil {
				return nil, err
			}
			own, _, err := r.Policies.Get(name)
			if err != nil {
				return nil, err
			}
			def, err := r.Policies.Default()
			if err != nil {
				return nil, err
			}
			override, defaults := newRetentionPolicy(own), newRetentionPolicy(def)
			return RetentionView{RetentionPolicy: newRetentionPolicy(pol), Override: &override, Default: &defaults}, nil
		},
	})
}

func retentionSet() registry.Command {
	return registry.WithResult[RetentionSetResult](registry.Command{
		Path:    "retention.set",
		Summary: "Set the retention policy for an agent (0 = use the default) or 'default' (0 = unlimited)",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "agent name, or 'default'"},
			{Name: "keep-iterations", Flag: "keep-iterations", Short: "i", Type: registry.Int, Help: "keep newest N iterations (0: agent inherits the default; default is unlimited)"},
			{Name: "keep-days", Flag: "keep-days", Short: "d", Type: registry.Int, Help: "keep iterations newer than N days (0: agent inherits the default; default is unlimited)"},
			{Name: "max-bytes", Flag: "max-bytes", Short: "b", Type: registry.Int, Help: "cap total iteration bytes (0: agent inherits the default; default is unlimited)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{agent}/retention"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			r, err := requireRetention(c)
			if err != nil {
				return nil, err
			}
			name := p.String("agent")
			if err := checkAgentOrKeyword(name, "default"); err != nil {
				return nil, err
			}
			// Read-modify-write the current policy so unset flags are preserved.
			var cur retention.Policy
			if name == "default" {
				cur, err = r.Policies.Default()
			} else {
				cur, _, err = r.Policies.Get(name)
			}
			if err != nil {
				return nil, err
			}
			if p.Has("keep-iterations") {
				cur.KeepIterations = p.IntOr("keep-iterations", cur.KeepIterations)
			}
			if p.Has("keep-days") {
				cur.KeepDays = p.IntOr("keep-days", cur.KeepDays)
			}
			if p.Has("max-bytes") {
				cur.MaxBytes = int64(p.IntOr("max-bytes", int(cur.MaxBytes)))
			}
			// A negative limit is nonsensical: the pruner treats <=0 as unlimited,
			// so storing -5 silently means "unlimited" and misleads the operator.
			// Reject it with a clean bad_value error instead.
			if cur.KeepIterations < 0 || cur.KeepDays < 0 || cur.MaxBytes < 0 {
				return nil, api.UserError{Code: "bad_value", Msg: "keep_iterations, keep_days and max_bytes must be >= 0"}
			}
			if name == "default" {
				if err := r.Policies.SetDefault(cur); err != nil {
					return nil, err
				}
			} else if err := r.Policies.Set(name, cur); err != nil {
				return nil, err
			}
			return RetentionSetResult{RetentionPolicy: newRetentionPolicy(cur), Agent: name}, nil
		},
	})
}

func pruneCommand() registry.Command {
	return registry.WithResult[PruneResult](registry.Command{
		Path:    "prune",
		Summary: "Prune old iterations for an agent now (or 'all'); --dry-run lists victims",
		Args: []registry.Arg{
			{Name: "agent", Type: registry.String, Required: true, Help: "agent name, or 'all'"},
			{Name: "dry-run", Flag: "dry-run", Short: "n", Type: registry.Bool, Help: "list what would be removed without deleting"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{agent}/prune"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			r, err := requireRetention(c)
			if err != nil {
				return nil, err
			}
			name := p.String("agent")
			if err := checkAgentOrKeyword(name, "all"); err != nil {
				return nil, err
			}
			dry := p.Bool("dry-run")
			if name == "all" {
				reps, err := r.Pruner.PruneAll(dry)
				if err != nil {
					return nil, err
				}
				var reports []PruneReport
				for _, rep := range reps {
					reports = append(reports, PruneReport(rep))
				}
				return PruneResult{Reports: &reports, DryRun: dry}, nil
			}
			rep, err := r.Pruner.PruneAgent(name, dry)
			if err != nil {
				return nil, err
			}
			return PruneResult{
				Agent: rep.Agent, Pruned: rep.Pruned, Archived: rep.Archived,
				FreedBytes: &rep.FreedBytes, DryRun: rep.DryRun,
			}, nil
		},
	})
}
