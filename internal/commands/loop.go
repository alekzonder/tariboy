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

// loopIntSetting builds the command of one integer loop setting; each field
// has its own result type because its key differs.
func loopIntSetting(path, field string) registry.Command {
	switch field {
	case "interval":
		return loopIntCommand(path, field, func(name string, v int) LoopIntervalResult { return LoopIntervalResult{Name: name, IntervalS: v} })
	case "timeout":
		return loopIntCommand(path, field, func(name string, v int) LoopTimeoutResult { return LoopTimeoutResult{Name: name, TimeoutS: v} })
	case "hard-timeout":
		return loopIntCommand(path, field, func(name string, v int) LoopHardTimeoutResult {
			return LoopHardTimeoutResult{Name: name, HardTimeoutS: v}
		})
	case "max-idle":
		return loopIntCommand(path, field, func(name string, v int) LoopMaxIdleResult {
			return LoopMaxIdleResult{Name: name, MaxIdleIterations: v}
		})
	case "ai-stall-timeout":
		return loopIntCommand(path, field, func(name string, v int) LoopAIStallTimeoutResult {
			return LoopAIStallTimeoutResult{Name: name, AIStallTimeoutS: v}
		})
	}
	panic("unknown loop setting " + field)
}

func loopIntCommand[T any](path, field string, result func(name string, value int) T) registry.Command {
	return registry.WithResult[T](registry.Command{
		Path:    path,
		Summary: "Get or set loop " + field + " (seconds); omit value to read",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "value", Type: registry.Int, Help: "new value (omit to read)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/loop/" + field},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			if p.Has("value") {
				v := p.Int("value")
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
			return result(a.Name, getIntField(a, field)), nil
		},
	})
}

// loopStrSetting builds the command of one policy loop setting; each field
// has its own result type because its key differs.
func loopStrSetting(path, field string, allowed []string) registry.Command {
	switch field {
	case "on-timeout":
		return loopStrCommand(path, field, allowed, func(name, v string) LoopOnTimeoutResult { return LoopOnTimeoutResult{Name: name, OnTimeout: v} })
	case "on-error":
		return loopStrCommand(path, field, allowed, func(name, v string) LoopOnErrorResult { return LoopOnErrorResult{Name: name, OnError: v} })
	}
	panic("unknown loop setting " + field)
}

func loopStrCommand[T any](path, field string, allowed []string, result func(name, value string) T) registry.Command {
	return registry.WithResult[T](registry.Command{
		Path:    path,
		Summary: "Get or set loop " + field + " policy; omit value to read",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "value", Type: registry.String, Help: "new value (omit to read)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/loop/" + field},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			if v := p.String("value"); v != "" {
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
			return result(a.Name, getStrField(a, field)), nil
		},
	})
}

func loopSchedule() registry.Command {
	return registry.WithResult[LoopScheduleResult](registry.Command{
		Path:    "loop.schedule",
		Summary: "Get, set or clear the loop schedule (RFC 5545 RRULE, whole minutes); omit value to read",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "value", Type: registry.String, Help: "RRULE, optionally after a DTSTART line (omit to read)"},
			{Name: "clear", Flag: "clear", Type: registry.Bool, Help: "remove the schedule"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/loop/schedule"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			rule, set := a.ScheduleRRule, false
			if p.Bool("clear") {
				rule, set = "", true
			} else if v := p.String("value"); v != "" {
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
			return LoopScheduleResult{Name: a.Name, Schedule: a.ScheduleRRule, ScheduleNextAt: scheduleNextAt(a)}, nil
		},
	})
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
	return registry.WithResult[AgentUserPromptResult](registry.Command{
		Path:    "user-prompt.get",
		Summary: "Read the agent's standing user-prompt",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/user-prompt"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			return AgentUserPromptResult{Name: a.Name, UserPrompt: a.UserPrompt}, nil
		},
	})
}

func userPromptSet() registry.Command {
	return registry.WithResult[AgentSavedResult](registry.Command{
		Path:    "user-prompt.set",
		Summary: "Set the agent's standing user-prompt",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "text", Type: registry.String, Required: true, Help: "prompt text"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/user-prompt"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			a.UserPrompt = p.String("text")
			if err := agentStore(c).Update(a); err != nil {
				return nil, err
			}
			return AgentSavedResult{Name: a.Name, Saved: true}, nil
		},
	})
}

func agentExec() registry.Command {
	return registry.WithResult[AgentExecResult](registry.Command{
		Path:    "agent.exec",
		Summary: "Run a manual iteration now, with an optional one-shot prompt",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "prompt", Type: registry.String, Help: "one-shot prompt (optional)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/exec"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			note, err := c.Control.Exec(p.String("name"), p.String("prompt"))
			if err != nil {
				return nil, api.UserError{Code: "exec_failed", Msg: err.Error()}
			}
			return AgentExecResult{Name: p.String("name"), Status: note}, nil
		},
	})
}

// field helpers keep the get/set switch in one place.
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

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
