// Package commands declares every daemon command in the shared registry.
package commands

import (
	"encoding/json"
	"os"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/taskreminder"
)

func BuildRegistry() *registry.Registry {
	r := registry.New()
	mustRegister(r, versionCommand())
	mustRegister(r, daemonStatus())
	mustRegister(r, daemonConfigGet())
	mustRegister(r, daemonConfigSet())
	mustRegister(r, imageBuild())
	mustRegister(r, imageValidate())
	mustRegister(r, imageLs())
	mustRegister(r, imageInspect())
	mustRegister(r, imagePrompt())
	mustRegister(r, imageTemplate())
	mustRegister(r, imageProvenance())
	mustRegister(r, imageFiles())
	mustRegister(r, imageFileRead())
	mustRegister(r, imageRm())
	mustRegister(r, pushCmd())
	mustRegister(r, pullCmd())
	mustRegister(r, loginCmd())
	mustRegister(r, agentRun())
	mustRegister(r, agentImageSet())
	mustRegister(r, agentImageStatus())
	mustRegister(r, agentImageCancel())
	mustRegister(r, agentPs())
	mustRegister(r, agentStatus())
	mustRegister(r, agentInspect())
	mustRegister(r, agentLifecycle("agent.stop", "Stop an agent's loop (current iteration lives)", "stop"))
	mustRegister(r, agentLifecycle("agent.start", "Start (enable) an agent's loop", "start"))
	mustRegister(r, agentLifecycle("agent.restart", "Restart an agent and run one iteration now", "restart"))
	mustRegister(r, agentLifecycle("agent.kill", "Kill the current iteration via its shim", "kill"))
	mustRegister(r, agentRm())
	mustRegister(r, agentReprovision())
	mustRegister(r, agentExec())
	mustRegister(r, loopToggle("loop.enable", "enable", true))
	mustRegister(r, loopToggle("loop.disable", "disable", false))
	mustRegister(r, loopIntSetting("loop.interval", "interval"))
	mustRegister(r, loopIntSetting("loop.timeout", "timeout"))
	mustRegister(r, loopIntSetting("loop.hard-timeout", "hard-timeout"))
	mustRegister(r, loopStrSetting("loop.on-timeout", "on-timeout", []string{"restart", "stop"}))
	mustRegister(r, loopStrSetting("loop.on-error", "on-error", []string{"restart", "stop"}))
	mustRegister(r, loopIntSetting("loop.max-idle", "max-idle"))
	mustRegister(r, userPromptGet())
	mustRegister(r, userPromptSet())
	mustRegister(r, contextGet())
	mustRegister(r, contextSet())
	mustRegister(r, promptGet())
	mustRegister(r, iterationLs())
	mustRegister(r, iterationInspect())
	mustRegister(r, iterationExtendTimeout())
	mustRegister(r, iterationLogs())
	mustRegister(r, secretStore())
	mustRegister(r, secretSet())
	mustRegister(r, secretLs())
	mustRegister(r, secretRm())
	mustRegister(r, agentPush())
	mustRegister(r, agentPull())
	mustRegister(r, cpCommand())
	mustRegister(r, fileList())
	mustRegister(r, fileRead())
	mustRegister(r, fileWrite())
	mustRegister(r, fileCreate())
	mustRegister(r, fileRename())
	mustRegister(r, fileDelete())
	mustRegister(r, fsList())
	mustRegister(r, agentScreen())
	mustRegister(r, agentSendKeys())
	mustRegister(r, agentModel())
	mustRegister(r, agentEffort())
	mustRegister(r, agentCwd())
	mustRegister(r, agentInteractive())
	mustRegister(r, agentHarness())
	mustRegister(r, agentAlias())
	mustRegister(r, agentAliasGet())
	mustRegister(r, agentNotes())
	mustRegister(r, agentNotesGet())
	mustRegister(r, agentColor())
	mustRegister(r, agentColorGet())
	mustRegister(r, agentStatusHistory())
	mustRegister(r, channelLs())
	mustRegister(r, agentSubscriptions())
	mustRegister(r, agentSubscribe())
	mustRegister(r, agentUnsubscribe())
	mustRegister(r, channelInspect())
	mustRegister(r, channelTail())
	mustRegister(r, channelWatches())
	mustRegister(r, messageSend())
	mustRegister(r, agentInbox())
	mustRegister(r, agentInboxProcessed())
	mustRegister(r, agentInboxReply())
	mustRegister(r, agentInboxRequeue())
	mustRegister(r, scheduleLs())
	mustRegister(r, scriptLs())
	mustRegister(r, scriptRunOnce())
	mustRegister(r, scriptSchedule())
	mustRegister(r, scriptRerun())
	mustRegister(r, scriptRuns())
	mustRegister(r, scriptRunDetail())
	mustRegister(r, scriptLogs())
	mustRegister(r, scriptCancel())
	mustRegister(r, scriptRemove())
	mustRegister(r, logsCommand())
	mustRegister(r, transcriptCommand())
	mustRegister(r, usageCommand())
	mustRegister(r, agentUsage())
	mustRegister(r, budgetSet())
	mustRegister(r, budgetLs())
	mustRegister(r, budgetStatus())
	mustRegister(r, agentBudgetGet())
	mustRegister(r, agentBudgetSet())
	mustRegister(r, daemonReindex())
	mustRegister(r, pluginInstall())
	mustRegister(r, pluginLs())
	mustRegister(r, pluginInspect())
	mustRegister(r, pluginRm())
	mustRegister(r, pluginRestart())
	mustRegister(r, pluginLogs())
	mustRegister(r, pluginRoutes())
	mustRegister(r, pluginAction())
	mustRegister(r, groupCreate())
	mustRegister(r, groupLs())
	mustRegister(r, groupInspect())
	mustRegister(r, groupRm())
	mustRegister(r, groupAssign())
	mustRegister(r, groupRename())
	mustRegister(r, groupLeadSet())
	mustRegister(r, groupMemberRm())
	mustRegister(r, teamCompose())
	mustRegister(r, teamImportYAML())
	mustRegister(r, teamImportArchiveApply())
	mustRegister(r, teamImportArchiveStatus())
	mustRegister(r, teamImportArchivePlan())
	mustRegister(r, evalLs())
	mustRegister(r, evalInspect())
	mustRegister(r, judgeLs())
	mustRegister(r, judgeInspect())
	mustRegister(r, judgeEvidence())
	mustRegister(r, judgeCancel())
	mustRegister(r, judgeRetry())
	mustRegister(r, ruleSet())
	mustRegister(r, ruleLs())
	mustRegister(r, ruleRm())
	mustRegister(r, retentionGet())
	mustRegister(r, retentionSet())
	mustRegister(r, pruneCommand())
	mustRegister(r, backupCommand())
	mustRegister(r, restoreCommand())
	for _, command := range taskCommands() {
		mustRegister(r, command)
	}
	mustGroup(r, "agent", "Manage agents (run, stop, inspect, exec, files)")
	mustGroup(r, "agent.budget", "Manage an agent's calendar USD budget")
	mustGroup(r, "agent.status", "Agent runtime status and history")
	mustGroup(r, "agent.alias", "Agent display alias")
	mustGroup(r, "agent.notes", "Freeform agent notes")
	mustGroup(r, "agent.color", "Agent accent color")
	mustGroup(r, "agent.file", "Browse and edit files in an agent's cwd")
	mustGroup(r, "agent.inbox", "Inspect and act on an agent's inbox")
	mustGroup(r, "loop", "Configure an agent's iteration loop")
	mustGroup(r, "image", "Build and manage agent images")
	mustGroup(r, "judge", "Inspect and recover LLM-as-Judge runs")
	mustGroup(r, "daemon", "Control the tariboy daemon")
	mustGroup(r, "daemon.config", "Read and set daemon config")
	mustGroup(r, "user-prompt", "Per-agent user prompt")
	mustGroup(r, "context", "Per-agent context document")
	mustGroup(r, "iteration", "Inspect agent iterations")
	mustGroup(r, "secret", "Manage agent secrets")
	mustGroup(r, "channel", "Inspect channels and messages")
	mustGroup(r, "message", "Send channel messages")
	mustGroup(r, "schedule", "Manage agent schedules")
	mustGroup(r, "script", "Manage agent background scripts")
	mustGroup(r, "budget", "Manage token budgets")
	mustGroup(r, "plugin", "Install and manage external plugins")
	mustGroup(r, "agent.image", "Select an agent image")
	mustGroup(r, "group", "Manage agent groups")
	mustGroup(r, "group.lead", "Manage a group's lead")
	mustGroup(r, "group.member", "Manage group members")
	mustGroup(r, "team", "Manage agent teams")
	mustGroup(r, "team.import", "Import agent teams")
	mustGroup(r, "team.import.archive", "Import portable team archives")
	mustGroup(r, "eval", "Inspect evaluations")
	mustGroup(r, "rule", "Manage AI-proxy rules")
	mustGroup(r, "retention", "Manage data retention")
	mustGroup(r, "prompt", "Read composed agent prompt")
	mustGroup(r, "fs", "Browse the daemon filesystem root ($HOME-jailed)")
	mustGroup(r, "tasks", "Manage native Tariboy tasks")
	mustGroup(r, "tasks.queue", "Manage native task queues")
	mustGroup(r, "tasks.queue.workflow", "Manage task queue workflow bindings")
	mustGroup(r, "tasks.queue.pool", "Manage task queue agent pools")
	mustGroup(r, "tasks.queue.trigger", "Manage task queue workflow triggers")
	mustGroup(r, "tasks.workflows", "Manage task workflow definitions")
	mustGroup(r, "tasks.workflow", "Inspect and control managed task workflows")
	mustGroup(r, "tasks.workflow.artifact", "Inspect managed task artifacts")
	mustGroup(r, "tasks.workflow.question", "Inspect managed task questions")
	mustGroup(r, "tasks.comments", "Manage native task comments")
	mustGroup(r, "tasks.relations", "Manage native task relations")
	mustGroup(r, "tasks.notifications", "Manage customer task notifications")
	mustValidate(r)
	return r
}

func mustRegister(r *registry.Registry, c registry.Command) {
	if err := r.Register(c); err != nil {
		panic(err) // programming error: caught by tests at build time
	}
}

func mustGroup(r *registry.Registry, path, summary string) {
	if err := r.RegisterGroup(path, summary); err != nil {
		panic(err)
	}
}

func mustValidate(r *registry.Registry) {
	if err := r.Validate(); err != nil {
		panic(err)
	}
}

func daemonStatus() registry.Command {
	return registry.Command{
		Path:      "daemon.status",
		Summary:   "Show daemon version, uptime and base directory",
		CLIHidden: true,
		HTTP:      &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			schema := 0
			if c.Store != nil {
				schema, _ = c.Store.SchemaVersion()
			}
			return map[string]any{
				"version":        c.Version,
				"pid":            os.Getpid(),
				"started_at":     c.StartedAt.UTC().Format(time.RFC3339),
				"uptime_seconds": int64(time.Since(c.StartedAt).Seconds()),
				"base_dir":       c.BaseDir,
				"http_addr":      c.HTTPAddr,
				"schema_version": schema,
			}, nil
		},
	}
}

func daemonConfigGet() registry.Command {
	return registry.Command{
		Path:    "daemon.config.get",
		Summary: "Read daemon config (all keys, or one with --key)",
		Args: []registry.Arg{
			{Name: "key", Flag: "key", Type: registry.String, Help: "config key to read"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/config"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			out := map[string]any{}
			if key, _ := p["key"].(string); key != "" {
				v, ok, err := c.Store.ConfigGet(key)
				if err != nil {
					return nil, err
				}
				if ok {
					out[key] = v
				}
				return out, nil
			}
			rows, err := c.Store.DB.Query(`SELECT key, value FROM daemon_config ORDER BY key`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var k, v string
				if err := rows.Scan(&k, &v); err != nil {
					return nil, err
				}
				out[k] = v
			}
			return out, rows.Err()
		},
	}
}

func daemonConfigSet() registry.Command {
	return registry.Command{
		Path:    "daemon.config.set",
		Summary: "Set a daemon config key (runtime-mutable)",
		Args: []registry.Arg{
			{Name: "key", Type: registry.String, Required: true, Help: "config key"},
			{Name: "value", Type: registry.String, Required: true, Help: "new value"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/daemon/config"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			key, _ := p["key"].(string)
			value, _ := p["value"].(string)
			if key == "" {
				return nil, api.UserError{Code: "missing_key", Msg: "key is required"}
			}
			if key == "task_reminder" {
				policy, err := taskreminder.ParsePolicy(value)
				if err != nil {
					return nil, api.UserError{Code: "bad_task_reminder", Msg: err.Error()}
				}
				normalized, err := json.Marshal(policy)
				if err != nil {
					return nil, err
				}
				value = string(normalized)
			}
			if err := c.Store.ConfigSet(key, value); err != nil {
				return nil, err
			}
			data, _ := json.Marshal(map[string]string{"key": key, "value": value})
			if err := c.Store.AddEvent("", "config_set", string(data)); err != nil {
				return nil, err
			}
			return map[string]any{"key": key, "value": value}, nil
		},
	}
}
