package commands

import (
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/store"
)

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
				min := 0
				if field == "ai-stall-timeout" {
					min = 1
				}
				if v < min {
					return nil, api.UserError{Code: "bad_value", Msg: fmt.Sprintf("%s must be >= %d", field, min)}
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

func loopSchedule() registry.Command {
	return registry.Command{
		Path:    "loop.schedule",
		Summary: "Get, set or clear the loop schedule (RFC 5545 RRULE, whole minutes); omit value to read",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "value", Type: registry.String, Help: "RRULE, optionally after a DTSTART line (omit to read)"},
			{Name: "clear", Flag: "clear", Type: registry.Bool, Help: "remove the schedule"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/loop/schedule"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			rule, set := a.ScheduleRRule, false
			if toBool(p["clear"]) {
				rule, set = "", true
			} else if v := str(p, "value"); v != "" {
				rule, err = schedule.NormalizeRRule(v, time.Now(), time.Local)
				if err != nil {
					return nil, api.UserError{Code: "bad_schedule", Msg: err.Error()}
				}
				set = true
			}
			if set {
				if err := agentStore(c).SetSchedule(a.Name, rule); err != nil {
					return nil, err
				}
				a.ScheduleRRule = rule
				if control, ok := c.Control.(registry.LoopConfigControl); ok {
					control.RefreshLoopConfig(a.Name)
				}
			}
			return map[string]any{"name": a.Name, "schedule": a.ScheduleRRule, "schedule_next_at": scheduleNextAt(a)}, nil
		},
	}
}

// scheduleNextAt is the next scheduled start in the stored time format, or ""
// when the agent has no schedule or its rule has finished.
func scheduleNextAt(a agent.Agent) string {
	next, ok := schedule.NextRRule(a.ScheduleRRule, time.Now(), time.Local)
	if !ok {
		return ""
	}
	return store.FormatTime(next)
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
	case "ai-stall-timeout":
		return "ai_stall_timeout_s"
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
	case "ai-stall-timeout":
		a.AIStallTimeoutS = v
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
	case "ai-stall-timeout":
		return a.AIStallTimeoutS
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
