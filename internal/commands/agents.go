package commands

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
)

func agentStore(c *registry.Ctx) *agent.Store { return agent.NewStore(c.Store) }

func agentView(a agent.Agent, state string) map[string]any {
	v := map[string]any{
		"name": a.Name, "image": a.ImageRef, "digest": a.ImageDigest, "state": state,
		"cwd": a.Cwd, "harness": a.HarnessType, "model": a.Model, "effort": a.Effort,
		"interactive": a.Interactive, "loop_enabled": a.LoopEnabled, "enabled": a.Enabled, "interval_s": a.IntervalS,
		"timeout_s": a.TimeoutS, "hard_timeout_s": a.HardTimeoutS, "on_timeout": a.OnTimeout,
		"on_error": a.OnError, "user_prompt": a.UserPrompt, "env": a.Env, "plugins": a.Plugins,
		"group": a.Group, "alias": a.Alias, "notes": a.Notes, "color": a.Color,
		"max_idle_iterations": a.MaxIdleIterations,
	}
	if a.ErrorReason != "" {
		v["error_reason"] = a.ErrorReason
	}
	addHaltReason(v, a)
	return v
}

func agentImageSet() registry.Command {
	return registry.Command{Path: "agent.image.set", Summary: "Select an image for the agent's next iteration", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "image", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/image"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		name := str(p, "name")
		if _, err := getAgent(c, name); err != nil {
			return nil, err
		}
		ref, err := image.ParseRef(str(p, "image"))
		if err != nil {
			return nil, api.UserError{Code: "bad_ref", Msg: err.Error(), Status: http.StatusBadRequest}
		}
		manifest, err := imageStore(c).Inspect(ref)
		if err != nil {
			return nil, api.UserError{Code: "image_not_found", Msg: err.Error(), Status: http.StatusNotFound}
		}
		if err := agentStore(c).SetPendingImage(name, ref.String(), manifest.Digest); err != nil {
			return nil, err
		}
		return agentImageStatusValue(c, name)
	}}
}

func agentImageStatusValue(c *registry.Ctx, name string) (map[string]any, error) {
	a, err := getAgent(c, name)
	if err != nil {
		return nil, err
	}
	pending, err := agentStore(c).PendingImage(name)
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": name, "current": agent.ImageAssignment{Ref: a.ImageRef, Digest: a.ImageDigest}, "pending": pending}, nil
}

func agentImageStatus() registry.Command {
	return registry.Command{Path: "agent.image.status", Summary: "Show current and pending agent image", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/agents/{name}/image"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) { return agentImageStatusValue(c, str(p, "name")) }}
}

func agentImageCancel() registry.Command {
	return registry.Command{Path: "agent.image.cancel", Summary: "Cancel a pending agent image", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodDelete, Path: "/api/agents/{name}/image"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		name := str(p, "name")
		if err := agentStore(c).ClearPendingImage(name); err != nil {
			return nil, err
		}
		return agentImageStatusValue(c, name)
	}}
}

// addHaltReason emits the derived halt keys on a reader's output, and only when
// there is a halt: with no reason NEITHER key appears, matching the
// emit-only-when-non-empty style of error_reason above. Every reader goes
// through here so the idle-prefix test lives in exactly one place.
func addHaltReason(v map[string]any, a agent.Agent) {
	if kind, reason := a.HaltReason(); kind != "" {
		v["halt_kind"] = kind
		v["halt_reason"] = reason
	}
}

func parseKV(list string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(list, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		if i := strings.Index(pair, "="); i >= 0 {
			out[pair[:i]] = pair[i+1:]
		}
	}
	return out
}

func parseList(list string) []string {
	var out []string
	for _, s := range strings.Split(list, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseAgentTimeout(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, fmt.Errorf("invalid timeout %q (want 60m, 2h, or 90s)", value)
	}
	return int(duration.Seconds()), nil
}

func agentRun() registry.Command {
	return registry.Command{
		Path:    "agent.run",
		Summary: "Create an agent from an image (image:tag); starts stopped — enable with 'agent start'",
		Args: []registry.Arg{
			{Name: "image", Type: registry.String, Required: true, Help: "image ref name:tag"},
			{Name: "name", Flag: "name", Type: registry.String, Help: "agent name (default: generated)"},
			{Name: "cwd", Flag: "cwd", Type: registry.String, Help: "working directory"},
			{Name: "harness", Flag: "harness", Short: "a", Type: registry.String, Help: "harness: claude|codex|opencode|stub"},
			{Name: "model", Flag: "model", Short: "m", Type: registry.String, Help: "model"},
			{Name: "effort", Flag: "effort", Short: "e", Type: registry.String, Help: "effort"},
			{Name: "interactive", Flag: "interactive", Short: "i", Type: registry.Bool, Help: "interactive (tmux) mode"},
			{Name: "env", Flag: "env", Type: registry.String, Help: "comma-separated K=V env pairs"},
			{Name: "plugins", Flag: "plugins", Type: registry.String, Help: "comma-separated plugin override"},
			{Name: "group", Flag: "group", Short: "g", Type: registry.String, Help: "join this group"},
			{Name: "loop", Flag: "loop", Type: registry.Bool, Default: true, Help: "set loop intent (nested; agent still starts stopped — default true)"},
			{Name: "timeout", Flag: "timeout", Type: registry.String, Help: "soft iteration timeout duration (for example 60m or 2h)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			loop := true
			if v, ok := p["loop"].(bool); ok {
				loop = v
			}
			interactive, _ := p["interactive"].(bool)
			timeoutS, timeoutErr := parseAgentTimeout(str(p, "timeout"))
			if timeoutErr != nil {
				return nil, api.UserError{Code: "bad_timeout", Msg: timeoutErr.Error()}
			}
			spec := registry.RunSpec{
				ImageRef: str(p, "image"), Name: str(p, "name"), Cwd: str(p, "cwd"),
				Harness: str(p, "harness"), Model: str(p, "model"), Effort: str(p, "effort"),
				Interactive: interactive, Env: parseKV(str(p, "env")),
				Plugins: parseList(str(p, "plugins")), Loop: loop, TimeoutS: timeoutS, Group: str(p, "group"),
			}
			if spec.ImageRef == "" {
				return nil, api.UserError{Code: "missing_image", Msg: "image ref is required"}
			}
			// Expand a relative/~ cwd against the daemon's fs root ($HOME) before
			// validating — the UI cwd picker offers paths relative to that root, so
			// "tmp/" must become $HOME/tmp rather than be rejected as non-absolute.
			resolvedCwd, err := resolveCwd(spec.Cwd)
			if err != nil {
				return nil, api.UserError{Code: "bad_cwd", Msg: err.Error()}
			}
			spec.Cwd = resolvedCwd
			if err := agent.ValidateCwd(spec.Cwd); err != nil {
				return nil, api.UserError{Code: "bad_cwd", Msg: err.Error()}
			}
			name, err := c.Control.Run(spec)
			if err != nil {
				return nil, api.UserError{Code: "run_failed", Msg: err.Error()}
			}
			state, _ := c.Control.LiveState(name)
			return map[string]any{"name": name, "state": state}, nil
		},
	}
}

func agentPs() registry.Command {
	return registry.Command{
		Path:    "agent.ps",
		Summary: "List agents",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			list, err := agentStore(c).List()
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(list))
			for _, a := range list {
				state, _ := c.Control.LiveState(a.Name)
				row := map[string]any{
					"name": a.Name, "image": a.ImageRef, "state": state,
					"alias":   a.Alias,
					"harness": a.HarnessType, "loop_enabled": a.LoopEnabled, "enabled": a.Enabled, "group": a.Group,
					"color": a.Color, "cwd": a.Cwd, "timeout_s": a.TimeoutS,
					"interval_s": a.IntervalS, "on_timeout": a.OnTimeout, "on_error": a.OnError,
					"max_idle_iterations": a.MaxIdleIterations,
					"interactive":         a.Interactive,
				}
				addHaltReason(row, a)
				rows = append(rows, row)
			}
			return map[string]any{"agents": rows, "count": len(rows)}, nil
		},
	}
}

func agentStatus() registry.Command {
	return registry.Command{
		Path:    "agent.status.show",
		Summary: "Show one agent's runtime status",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			state, _ := c.Control.LiveState(a.Name)
			its, _ := agentStore(c).ListIterations(a.Name)
			last, lastID := "", ""
			if n := len(its); n > 0 {
				last = its[n-1].Status
				lastID = its[n-1].ID
			}
			out := map[string]any{"name": a.Name, "state": state, "loop_enabled": a.LoopEnabled, "enabled": a.Enabled,
				"iterations": len(its), "last_iteration": last, "last_iteration_id": lastID,
				"status_message": a.StatusMessage, "status_updated": a.StatusUpdated,
				"server_now": time.Now().UTC().Format(time.RFC3339Nano)}
			addHaltReason(out, a)
			for i := len(its) - 1; i >= 0; i-- {
				it := its[i]
				if it.Status != "running" {
					continue
				}
				active := map[string]any{"id": it.ID, "started_at": it.StartedAt,
					"timeout_extensions": it.TimeoutExtensions}
				if it.TimeoutPeriodS != nil {
					active["timeout_period_s"] = *it.TimeoutPeriodS
				}
				if it.TimeoutDeadline != nil {
					active["timeout_deadline"] = *it.TimeoutDeadline
				}
				if it.HardTimeoutDeadline != nil {
					active["hard_timeout_deadline"] = *it.HardTimeoutDeadline
				}
				if it.TimeoutDeadline != nil || it.HardTimeoutDeadline != nil {
					active["effective_deadline"] = effectiveDeadline(it.TimeoutDeadline, it.HardTimeoutDeadline)
				}
				out["active_iteration"] = active
				break
			}
			return out, nil
		},
	}
}

func effectiveDeadline(soft, hard *string) string {
	if soft == nil {
		return *hard
	}
	if hard == nil {
		return *soft
	}
	st, serr := time.Parse(time.RFC3339Nano, *soft)
	ht, herr := time.Parse(time.RFC3339Nano, *hard)
	if serr == nil && herr == nil && ht.Before(st) {
		return *hard
	}
	return *soft
}

func agentInspect() registry.Command {
	return registry.Command{
		Path:    "agent.inspect",
		Summary: "Show one agent's full config",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			state, _ := c.Control.LiveState(a.Name)
			v := agentView(a, state)
			// Report the EFFECTIVE cwd (the workdir fallback when Cwd is unset),
			// matching cwdOf/agentCwdFor, so the UI never shows an empty value.
			if cwd, err := agentCwdFor(c, a.Name); err == nil {
				v["cwd"] = cwd
			}
			return v, nil
		},
	}
}

func agentLifecycle(path, summary, verb string) registry.Command {
	return registry.Command{
		Path:    path,
		Summary: summary,
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/" + verb},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			name := str(p, "name")
			var err error
			switch verb {
			case "stop":
				err = c.Control.Stop(name)
			case "start":
				err = c.Control.Start(name)
			case "restart":
				err = c.Control.Restart(name)
			case "kill":
				err = c.Control.Kill(name)
			}
			if err != nil {
				return nil, api.UserError{Code: verb + "_failed", Msg: err.Error()}
			}
			return map[string]any{"name": name, "action": verb}, nil
		},
	}
}

func agentRm() registry.Command {
	return registry.Command{
		Path:    "agent.rm",
		Summary: "Remove an agent (stop it first, or --force)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "force", Flag: "force", Short: "f", Type: registry.Bool, Help: "kill and remove even if running"},
			{Name: "purge", Flag: "purge", Type: registry.Bool, Help: "also delete durable data (iterations, context, audit) and leaked rows; default preserves data"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/agents/{name}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			force := toBool(p["force"])
			purge := toBool(p["purge"])
			if err := c.Control.Remove(str(p, "name"), force, purge); err != nil {
				return nil, api.UserError{Code: "rm_failed", Msg: err.Error()}
			}
			return map[string]any{"removed": str(p, "name"), "purged": purge}, nil
		},
	}
}

func agentReprovision() registry.Command {
	return registry.Command{
		Path:    "agent.reprovision",
		Summary: "Re-unpack an agent's image tree in place and restart its loop, keeping its data",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "image", Flag: "image", Type: registry.String, Help: "image ref to (re)unpack; empty keeps the current image"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/reprovision"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			name := str(p, "name")
			if err := c.Control.Reprovision(name, str(p, "image")); err != nil {
				return nil, api.UserError{Code: "reprovision_failed", Msg: err.Error()}
			}
			state, _ := c.Control.LiveState(name)
			return map[string]any{"name": name, "state": state}, nil
		},
	}
}

func getAgent(c *registry.Ctx, name string) (agent.Agent, error) {
	a, err := agentStore(c).Get(name)
	if err != nil {
		return agent.Agent{}, api.UserError{Code: "not_found", Msg: "agent " + name + " not found"}
	}
	return a, nil
}

func str(p registry.Params, key string) string {
	s, _ := p[key].(string)
	return s
}

// toBool accepts the CLI's typed bool and the daemon's query-string "true".
func toBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	default:
		return false
	}
}
