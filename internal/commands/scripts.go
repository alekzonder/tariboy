package commands

import (
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/script"
)

func requireScripts(c *registry.Ctx) (registry.ScriptControl, error) {
	if c.Scripts == nil {
		return nil, api.UserError{Code: "scripts_unavailable", Msg: "script controls are not available"}
	}
	return c.Scripts, nil
}

func scriptLs() registry.Command {
	return registry.WithResult[AgentScriptList](registry.Command{Path: "script.ls", Summary: "List an agent's local scripts",
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/scripts"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			sc, err := scriptControlForAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			definitions, err := sc.ListScripts(p.String("name"))
			if err != nil {
				return nil, err
			}
			rows := make([]AgentScript, 0, len(definitions))
			for _, definition := range definitions {
				rows = append(rows, scriptDefinitionFrom(definition))
			}
			return AgentScriptList{Scripts: rows, Count: len(rows)}, nil
		},
	})
}

func scriptRunOnce() registry.Command {
	return registry.WithResult[AgentScriptStarted](registry.Command{Path: "script.run", Summary: "Run a local command once",
		Args: scriptCreateArgs(false), HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/scripts/run"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			sc, err := scriptControlForAgent(c, name)
			if err != nil {
				return nil, err
			}
			definition, run, err := sc.RunOnce(name, script.CreateOnce{Name: p.String("script_name"), Description: p.String("description"), Command: p.String("command")})
			if err != nil {
				return nil, err
			}
			return AgentScriptStarted{Script: scriptDefinitionFrom(definition), Run: scriptRunFrom(run)}, nil
		},
	})
}

func scriptSchedule() registry.Command {
	args := scriptCreateArgs(true)
	return registry.WithResult[AgentScriptStarted](registry.Command{Path: "script.schedule", Summary: "Schedule a fixed-interval local command",
		Args: args, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/scripts/schedule"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			sc, err := scriptControlForAgent(c, name)
			if err != nil {
				return nil, err
			}
			var quietExit *int
			if p.Has("quiet_exit") {
				value := p.IntOr("quiet_exit", 0)
				quietExit = &value
			}
			definition, run, err := sc.ScheduleScript(name, script.CreateSchedule{Name: p.String("script_name"), Description: p.String("description"), Command: p.String("command"), IntervalSeconds: p.IntOr("interval_seconds", 0), QuietExit: quietExit})
			if err != nil {
				return nil, err
			}
			return AgentScriptStarted{Script: scriptDefinitionFrom(definition), Run: scriptRunFrom(run)}, nil
		},
	})
}

func scriptCreateArgs(schedule bool) []registry.Arg {
	args := []registry.Arg{
		{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
		{Name: "script_name", Flag: "script-name", Type: registry.String, Required: true, Help: "script name"},
		{Name: "description", Type: registry.String, Required: true, Help: "description"},
		{Name: "command", Type: registry.String, Required: true, Help: "shell command"},
	}
	if schedule {
		args = append(args,
			registry.Arg{Name: "interval_seconds", Flag: "interval-seconds", Type: registry.Int, Required: true, Help: "delay after each completion"},
			registry.Arg{Name: "quiet_exit", Flag: "quiet-exit", Type: registry.Int, Help: "deprecated: the quiet exit code is always 111; another value wraps the command so that code is reported as 111"})
	}
	return args
}

func scriptRerun() registry.Command {
	return registry.WithResult[AgentScriptRun](scriptRunAction("script.rerun", "Run a stored script now, resuming a stopped schedule", "POST", "/api/agents/{name}/scripts/{id}/rerun", func(sc registry.ScriptControl, name, id string) (any, error) {
		run, err := sc.RerunScript(name, id)
		return scriptRunFrom(run), err
	}))
}

func scriptRuns() registry.Command {
	return registry.WithResult[AgentScriptRunList](scriptRunAction("script.runs", "List a script's runs", "GET", "/api/agents/{name}/scripts/{id}/runs", func(sc registry.ScriptControl, name, id string) (any, error) {
		runs, err := sc.ListScriptRuns(name, id)
		if err != nil {
			return nil, err
		}
		rows := make([]AgentScriptRun, 0, len(runs))
		for _, run := range runs {
			rows = append(rows, scriptRunFrom(run))
		}
		return AgentScriptRunList{Runs: rows, Count: len(rows)}, nil
	}))
}

func scriptRunDetail() registry.Command {
	return registry.WithResult[AgentScriptRun](scriptRunAction("script.run-get", "Get one script run", "GET", "/api/agents/{name}/script-runs/{id}", func(sc registry.ScriptControl, name, id string) (any, error) {
		run, err := sc.GetScriptRun(name, id)
		return scriptRunFrom(run), err
	}))
}

func scriptLogs() registry.Command {
	return registry.WithResult[AgentScriptRunLog](scriptRunAction("script.logs", "Read a bounded script run log tail", "GET", "/api/agents/{name}/script-runs/{id}/logs", func(sc registry.ScriptControl, name, id string) (any, error) {
		run, err := sc.GetScriptRun(name, id)
		if err != nil {
			return nil, err
		}
		logText, err := sc.LogScriptRun(name, id)
		if err != nil {
			return nil, err
		}
		return AgentScriptRunLog{Run: scriptRunFrom(run), Log: logText}, nil
	}))
}

func scriptCancel() registry.Command {
	return registry.WithResult[AgentScriptCancelResult](scriptRunAction("script.cancel", "Cancel a script or one run", "POST", "/api/agents/{name}/script-targets/{id}/cancel", func(sc registry.ScriptControl, name, id string) (any, error) {
		if err := sc.CancelScriptTarget(name, id); err != nil {
			return nil, err
		}
		return AgentScriptCancelResult{ID: id, Cancelled: true}, nil
	}))
}

func scriptRemove() registry.Command {
	return registry.WithResult[AgentScriptRemoveResult](scriptRunAction("script.rm", "Remove an inactive script", "DELETE", "/api/agents/{name}/scripts/{id}", func(sc registry.ScriptControl, name, id string) (any, error) {
		if err := sc.RemoveScript(name, id); err != nil {
			return nil, err
		}
		return AgentScriptRemoveResult{ID: id, Removed: true}, nil
	}))
}

func scriptControlForAgent(c *registry.Ctx, name string) (registry.ScriptControl, error) {
	if _, err := getAgent(c, name); err != nil {
		return nil, err
	}
	return requireScripts(c)
}

func scriptRunAction(path, summary, method, route string, action func(registry.ScriptControl, string, string) (any, error)) registry.Command {
	return registry.Command{Path: path, Summary: summary,
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}, {Name: "id", Type: registry.String, Required: true, Help: "script or run id"}},
		HTTP: &registry.HTTPRoute{Method: method, Path: route},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			sc, err := scriptControlForAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			result, err := action(sc, p.String("name"), p.String("id"))
			if err != nil {
				return nil, err
			}
			return result, nil
		},
	}
}
