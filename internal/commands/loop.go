package commands

import (
	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func loopToggle(path, verb string, enable bool) registry.Command {
	return registry.Command{
		Path:    path,
		Summary: "Turn the agent loop " + verb,
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/loop/" + verb},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			if control, ok := c.Control.(registry.LoopConfigControl); ok {
				if err := control.SetLoopEnabled(a.Name, enable); err != nil {
					return nil, err
				}
			} else {
				a.LoopEnabled = enable
				if err := agentStore(c).Update(a); err != nil {
					return nil, err
				}
			}
			return map[string]any{"name": a.Name, "loop_enabled": enable}, nil
		},
	}
}

func loopIntSetting(path, field string) registry.Command {
	return registry.Command{
		Path:    path,
		Summary: "Get or set loop " + field + " (seconds); omit value to read",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "value", Type: registry.Int, Help: "new value (omit to read)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/loop/" + field},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			key := fieldKey(field)
			if v, ok := asInt(p["value"]); ok {
				if v < 0 {
					return nil, api.UserError{Code: "bad_value", Msg: field + " must be >= 0"}
				}
				setIntField(&a, field, v)
				if err := agentStore(c).Update(a); err != nil {
					return nil, err
				}
				if control, ok := c.Control.(registry.LoopConfigControl); ok {
					control.RefreshLoopConfig(a.Name)
				}
			}
			return map[string]any{"name": a.Name, key: getIntField(a, field)}, nil
		},
	}
}

func loopStrSetting(path, field string, allowed []string) registry.Command {
	return registry.Command{
		Path:    path,
		Summary: "Get or set loop " + field + " policy; omit value to read",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "value", Type: registry.String, Help: "new value (omit to read)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/loop/" + field},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			key := fieldKey(field)
			if v := str(p, "value"); v != "" {
				if !contains(allowed, v) {
					return nil, api.UserError{Code: "bad_value", Msg: field + " must be one of restart|stop"}
				}
				setStrField(&a, field, v)
				if err := agentStore(c).Update(a); err != nil {
					return nil, err
				}
				if control, ok := c.Control.(registry.LoopConfigControl); ok {
					control.RefreshLoopConfig(a.Name)
				}
			}
			return map[string]any{"name": a.Name, key: getStrField(a, field)}, nil
		},
	}
}

func userPromptGet() registry.Command {
	return registry.Command{
		Path:    "user-prompt.get",
		Summary: "Read the agent's standing user-prompt",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/user-prompt"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			return map[string]any{"name": a.Name, "user_prompt": a.UserPrompt}, nil
		},
	}
}

func userPromptSet() registry.Command {
	return registry.Command{
		Path:    "user-prompt.set",
		Summary: "Set the agent's standing user-prompt",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "text", Type: registry.String, Required: true, Help: "prompt text"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/user-prompt"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			a.UserPrompt = str(p, "text")
			if err := agentStore(c).Update(a); err != nil {
				return nil, err
			}
			return map[string]any{"name": a.Name, "saved": true}, nil
		},
	}
}

func agentExec() registry.Command {
	return registry.Command{
		Path:    "agent.exec",
		Summary: "Run a manual iteration now, with an optional one-shot prompt",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "prompt", Type: registry.String, Help: "one-shot prompt (optional)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/exec"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			note, err := c.Control.Exec(str(p, "name"), str(p, "prompt"))
			if err != nil {
				return nil, api.UserError{Code: "exec_failed", Msg: err.Error()}
			}
			return map[string]any{"name": str(p, "name"), "status": note}, nil
		},
	}
}

// field helpers keep the get/set switch in one place.
func fieldKey(field string) string {
	switch field {
	case "interval":
		return "interval_s"
	case "timeout":
		return "timeout_s"
	case "hard-timeout":
		return "hard_timeout_s"
	case "on-timeout":
		return "on_timeout"
	case "on-error":
		return "on_error"
	case "max-idle":
		return "max_idle_iterations"
	}
	return field
}

func setIntField(a *agent.Agent, field string, v int) {
	switch field {
	case "interval":
		a.IntervalS = v
	case "timeout":
		a.TimeoutS = v
	case "hard-timeout":
		a.HardTimeoutS = v
	case "max-idle":
		a.MaxIdleIterations = v
	}
}

func getIntField(a agent.Agent, field string) int {
	switch field {
	case "interval":
		return a.IntervalS
	case "timeout":
		return a.TimeoutS
	case "hard-timeout":
		return a.HardTimeoutS
	case "max-idle":
		return a.MaxIdleIterations
	}
	return 0
}

func setStrField(a *agent.Agent, field, v string) {
	switch field {
	case "on-timeout":
		a.OnTimeout = v
	case "on-error":
		a.OnError = v
	}
}

func getStrField(a agent.Agent, field string) string {
	switch field {
	case "on-timeout":
		return a.OnTimeout
	case "on-error":
		return a.OnError
	}
	return ""
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case float64: // JSON numbers decode to float64 on the daemon side
		return int(t), true
	default:
		return 0, false
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
