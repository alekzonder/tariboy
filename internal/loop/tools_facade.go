package loop

import (
	"context"
	"fmt"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentapi"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// The tools facade reaches the manager only through these narrow interfaces.

// iterationResolver names the iteration a tools call belongs to.
type iterationResolver interface {
	currentIterationID(agent string) string
}

// agentStatusSource reports and announces an agent's derived state.
type agentStatusSource interface {
	LiveState(agent string) (string, error)
	emitState(agent string)
}

// goalRecorder hears a Goal the agent selected during its iteration.
type goalRecorder interface {
	noteSelectedGoal(agent, iterationID, key string)
}

// loopController is the agent-facing Start/Stop.
type loopController interface {
	toolsLoopControl(agent, action string) (agent.Agent, error)
}

// groupTools serves the group tools of a caller agent.
type groupTools interface {
	groupToolsInfo(caller string) (map[string]any, error)
	groupToolsStatus(caller, member string) (map[string]any, error)
	groupToolsSend(caller, iterationID, member, typ, text, deadline string) (map[string]any, error)
	groupToolsObserve(caller, member string, tail int) (map[string]any, error)
	groupToolsLoop(caller, member, action string) (map[string]any, error)
}

// scriptControl manages an agent's durable scripts.
type scriptControl interface {
	RunOnce(agent string, in script.CreateOnce) (script.Definition, script.Run, error)
	ScheduleScript(agent string, in script.CreateSchedule) (script.Definition, script.Run, error)
	RerunScript(agent, id string) (script.Run, error)
	ListScripts(agent string) ([]script.Definition, error)
	ListScriptRuns(agent, id string) ([]script.Run, error)
	GetScriptRun(agent, id string) (script.Run, error)
	LogScriptRun(agent, id string) (string, error)
	CancelScriptTarget(agent, id string) error
	RemoveScript(agent, id string) error
}

// toolsFacade builds one agent's tools API. It deliberately depends on the
// agent name, never on a loop engine: the same server is bound for an agent
// whose engine has not started (and may never start) while its iteration is
// still running, and the current iteration is resolved per request.
type toolsFacade struct {
	agent      string
	cfg        *ManagerConfig
	iterations iterationResolver
	status     agentStatusSource
	goals      goalRecorder
	loop       loopController
	groups     groupTools
	scripts    scriptControl
	// pluginResolver resolves image plugins for an agent-built image.
	pluginResolver func() plugincaps.ExternalResolver
}

func (f toolsFacade) deps(ag agent.Agent, l agentdir.Layout) agentapi.Deps {
	d := agentapi.Deps{
		Agent: ag.Name, Cwd: cwdOf(ag, l), ContextPath: l.ContextPath(), Plugins: ag.Plugins,
		ProvidedChannels: f.cfg.ProvidedChannels,
		BuildImage: func(name, tag, path string) (map[string]any, error) {
			return buildImageForAgent(f.cfg.ImgStore, l.Workdir(), name, tag, path, f.pluginResolver())
		},
	}
	f.lifecycleDeps(&d)
	f.messageDeps(&d)
	f.requestDeps(&d)
	f.scheduleDeps(&d)
	f.scriptDeps(&d)
	f.groupDeps(&d)
	return d
}

// lifecycleDeps wires identity, iteration, status, Goal, Tasks and loop
// control.
func (f toolsFacade) lifecycleDeps(d *agentapi.Deps) {
	d.CurrentPlugins = func() []string {
		current, err := f.cfg.Store.Get(f.agent)
		if err != nil {
			return nil
		}
		return current.Plugins
	}
	d.CurrentIteration = func() string { return f.iterations.currentIterationID(f.agent) }
	// SetDone reconfirms the caller's iteration is still the current one
	// before flipping done_flag. The harness reads the id via CurrentIteration
	// (or TARIBOY_ITERATION) and POSTs loop/done during that same iteration;
	// if the iteration has already rolled over, the stale id is rejected rather
	// than marking a finished iteration done. The residual window between the
	// reconfirm and the DB write is negligible: the engine only advances
	// current after the runner returns, long after the harness process exits.
	d.SetDone = func(id string, productive bool) error {
		if f.iterations.currentIterationID(f.agent) != id {
			return fmt.Errorf("iteration %q is no longer running", id)
		}
		return f.cfg.Store.SetIterationDone(id, productive)
	}
	d.Status = f.statusSnapshot
	d.SetStatus = f.setStatus
	d.SetGoal = f.setGoal
	d.TaskAction = func(action string, body map[string]any) (any, error) {
		if f.cfg.Tasks == nil {
			return nil, fmt.Errorf("native Tasks service is unavailable")
		}
		body["iteration_id"] = f.iterations.currentIterationID(f.agent)
		return f.cfg.Tasks.AgentAction(context.Background(), tasks.AgentActor(f.agent), action, body)
	}
	d.LoopControl = func(action string) (map[string]any, error) {
		updated, err := f.loop.toolsLoopControl(f.agent, action)
		if err != nil {
			return nil, err
		}
		return map[string]any{"agent": f.agent, "action": action,
			"enabled": updated.Enabled}, nil
	}
}

func (f toolsFacade) statusSnapshot() (map[string]any, error) {
	state, err := f.status.LiveState(f.agent)
	if err != nil {
		return nil, err
	}
	a, err := f.cfg.Store.Get(f.agent)
	if err != nil {
		return nil, err
	}
	return map[string]any{"state": state, "enabled": a.Enabled,
		"current_iteration": f.iterations.currentIterationID(f.agent),
		"message":           a.StatusMessage, "updated": a.StatusUpdated}, nil
}

func (f toolsFacade) setStatus(message string) (map[string]any, error) {
	updated := store.FormatTime(f.cfg.Clock())
	if err := f.cfg.Store.SetStatus(f.agent, message, updated); err != nil {
		return nil, err
	}
	f.status.emitState(f.agent)
	// Mirror the write into the audit log so `iteration logs`/audit shows
	// the full status timeline (v1 recorded type="status" per set).
	if f.cfg.AuditFor != nil {
		if rec := f.cfg.AuditFor(f.agent); rec != nil {
			rec.Record("status", "agent", f.iterations.currentIterationID(f.agent), map[string]any{"message": message})
		}
	}
	return map[string]any{"message": message, "updated": updated}, nil
}

func (f toolsFacade) setGoal(key string) (map[string]any, error) {
	iteration := f.iterations.currentIterationID(f.agent)
	if iteration == "" {
		return nil, fmt.Errorf("no iteration is currently running")
	}
	proxy, ok := f.cfg.Proxy.(taskAttributionProxy)
	if !ok {
		return nil, fmt.Errorf("goal selection is unavailable")
	}
	result, err := setGoal(context.Background(), f.cfg.Tasks, f.cfg.SetGoal, proxy, iteration, f.agent, key)
	if err == nil {
		if taskID, _ := result["task_id"].(string); taskID != "" {
			f.goals.noteSelectedGoal(f.agent, iteration, taskID)
		}
	}
	return result, err
}

// messageDeps wires publish, subscriptions and the inbox.
func (f toolsFacade) messageDeps(d *agentapi.Deps) {
	agName := f.agent
	d.Publish = func(msg bus.Message) (bus.Message, error) {
		if f.cfg.Bus == nil {
			return bus.Message{}, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.Publish(msg)
	}
	d.Subscribe = func(channel string, matcher bus.Matcher, tf []string) (bus.Subscription, error) {
		if f.cfg.Bus == nil {
			return bus.Subscription{}, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.Subscribe(agName, channel, matcher, tf)
	}
	d.Unsubscribe = func(id string) error {
		if f.cfg.Bus == nil {
			return fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.Unsubscribe(agName, id)
	}
	d.ListSubscriptions = func() ([]bus.Subscription, error) {
		if f.cfg.Bus == nil {
			return nil, nil
		}
		return f.cfg.Bus.ListSubscriptions(agName)
	}
	d.Channels = func() ([]bus.Channel, error) {
		if f.cfg.Bus == nil {
			return nil, nil
		}
		return f.cfg.Bus.Channels()
	}
	d.Inbox = func(status string, limit int, before string) ([]bus.InboxItem, error) {
		if f.cfg.Bus == nil {
			return nil, nil
		}
		return f.cfg.Bus.Inbox(agName, status, limit, before)
	}
	d.MarkProcessed = func(msgID, result string) (bus.InboxItem, error) {
		if f.cfg.Bus == nil {
			return bus.InboxItem{}, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.MarkProcessed(agName, msgID, result)
	}
	d.SubscribeParams = func(channel string, matcher bus.Matcher, tf []string, params map[string]any) (bus.Subscription, error) {
		if f.cfg.Bus == nil {
			return bus.Subscription{}, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.SubscribeParams(agName, channel, matcher, tf, params)
	}
	d.UnsubscribeChannel = func(channel string) (int, error) {
		if f.cfg.Bus == nil {
			return 0, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.UnsubscribeChannel(agName, channel)
	}
}

// requestDeps wires replies, requests and their bookkeeping.
func (f toolsFacade) requestDeps(d *agentapi.Deps) {
	agName := f.agent
	d.Reply = func(msgID, text string, data map[string]any, typeOverride string) (bus.Message, error) {
		if f.cfg.Bus == nil {
			return bus.Message{}, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.Reply(agName, msgID, text, data, typeOverride)
	}
	d.Request = func(channel, text, deadline string) (bus.Message, error) {
		if f.cfg.Bus == nil {
			return bus.Message{}, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.Request(agName, channel, text, deadline)
	}
	d.Requeue = func(msgID string) error {
		if f.cfg.Bus == nil {
			return fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.Requeue(agName, msgID)
	}
	d.Unanswered = func() (map[string][]bus.Message, error) {
		if f.cfg.Bus == nil {
			return nil, fmt.Errorf("bus not configured")
		}
		return f.cfg.Bus.UnansweredForPrincipal("agent:" + agName)
	}
}

// scheduleDeps wires the agent's own message schedules.
func (f toolsFacade) scheduleDeps(d *agentapi.Deps) {
	agName := f.agent
	d.AddSchedule = func(kind, spec, channel, tpl string) (map[string]any, error) {
		if f.cfg.Schedules == nil {
			return nil, fmt.Errorf("schedule store not configured")
		}
		if channel == "" {
			// A schedule the agent set for itself is service traffic; an
			// explicit channel still wins, so an existing schedule and a
			// deliberate chat target both keep working.
			channel = bus.ChatChannelFor(bus.ChatIDService(agName))
		}
		sch, err := f.cfg.Schedules.Add(schedule.Schedule{
			Agent: agName, Kind: kind, Spec: spec, Channel: channel, MessageTemplate: tpl})
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": sch.ID, "kind": sch.Kind, "channel": sch.Channel,
			"next_fire_at": sch.NextFireAt}, nil
	}
	d.ListSchedules = func() ([]map[string]any, error) {
		if f.cfg.Schedules == nil {
			return nil, nil
		}
		list, err := f.cfg.Schedules.List(agName)
		if err != nil {
			return nil, err
		}
		rows := make([]map[string]any, 0, len(list))
		for _, s := range list {
			rows = append(rows, map[string]any{"id": s.ID, "kind": s.Kind, "spec": s.Spec,
				"channel": s.Channel, "next_fire_at": s.NextFireAt, "enabled": s.Enabled})
		}
		return rows, nil
	}
	d.CancelSchedule = func(id string) error {
		if f.cfg.Schedules == nil {
			return fmt.Errorf("schedule store not configured")
		}
		return f.cfg.Schedules.Cancel(agName, id)
	}
}

func (f toolsFacade) scriptDeps(d *agentapi.Deps) {
	agName, s := f.agent, f.scripts
	d.RunScriptOnce = func(in script.CreateOnce) (script.Definition, script.Run, error) { return s.RunOnce(agName, in) }
	d.ScheduleScript = func(in script.CreateSchedule) (script.Definition, script.Run, error) {
		return s.ScheduleScript(agName, in)
	}
	d.RerunScript = func(id string) (script.Run, error) { return s.RerunScript(agName, id) }
	d.ListScripts = func() ([]script.Definition, error) { return s.ListScripts(agName) }
	d.ListScriptRuns = func(id string) ([]script.Run, error) { return s.ListScriptRuns(agName, id) }
	d.GetScriptRun = func(id string) (script.Run, error) { return s.GetScriptRun(agName, id) }
	d.LogScriptRun = func(id string) (string, error) { return s.LogScriptRun(agName, id) }
	d.CancelScriptTarget = func(id string) error { return s.CancelScriptTarget(agName, id) }
	d.RemoveScript = func(id string) error { return s.RemoveScript(agName, id) }
}

func (f toolsFacade) groupDeps(d *agentapi.Deps) {
	agName, g := f.agent, f.groups
	d.GroupInfo = func() (map[string]any, error) { return g.groupToolsInfo(agName) }
	d.GroupStatus = func(member string) (map[string]any, error) { return g.groupToolsStatus(agName, member) }
	d.GroupSend = func(member, typ, text, deadline string) (map[string]any, error) {
		return g.groupToolsSend(agName, f.iterations.currentIterationID(agName), member, typ, text, deadline)
	}
	d.GroupObserve = func(member string, tail int) (map[string]any, error) {
		return g.groupToolsObserve(agName, member, tail)
	}
	d.GroupLoop = func(member, action string) (map[string]any, error) {
		return g.groupToolsLoop(agName, member, action)
	}
}
