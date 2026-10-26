package commands

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
)

func agentStore(c *registry.Ctx) *agent.Store { return agent.NewStore(c.Store) }

func agentBudgetView(c *registry.Ctx, name string) (AgentBudget, error) {
	b, err := aiproxy.NewStore(c.Store, time.Now).AgentBudgetStatus(name, time.Now())
	if err != nil {
		return AgentBudget{}, err
	}
	return budgetView(b), nil
}

func budgetView(b aiproxy.AgentBudgetStatus) AgentBudget {
	return AgentBudget{
		HourUSD: b.HourUSD, DayUSD: b.DayUSD, WeekUSD: b.WeekUSD, MonthUSD: b.MonthUSD,
		HourSpentUSD: b.HourSpentUSD, DaySpentUSD: b.DaySpentUSD,
		WeekSpentUSD: b.WeekSpentUSD, MonthSpentUSD: b.MonthSpentUSD,
		Exhausted: b.Exhausted,
	}
}

// liveStateOf avoids a second read of the agent row when the control can
// compute the state from the row already in hand.
func liveStateOf(c *registry.Ctx, a agent.Agent) string {
	if sc, ok := c.Control.(registry.AgentStateControl); ok {
		return sc.LiveStateOf(a)
	}
	state, _ := c.Control.LiveState(a.Name)
	return state
}

func agentView(c *registry.Ctx, a agent.Agent, state string) (AgentView, error) {
	budget, err := agentBudgetView(c, a.Name)
	if err != nil {
		return AgentView{}, err
	}
	return AgentView{
		Name: a.Name, Image: a.ImageRef, Digest: a.ImageDigest, State: state,
		Cwd: a.Cwd, ConfiguredCwd: a.Cwd,
		Harness: a.HarnessType, Model: a.Model, Effort: a.Effort,
		Interactive: a.Interactive, Enabled: a.Enabled, IntervalS: a.IntervalS,
		TimeoutS: a.TimeoutS, HardTimeoutS: a.HardTimeoutS, OnTimeout: a.OnTimeout,
		OnError: a.OnError, UserPrompt: a.UserPrompt, Env: a.Env, Plugins: a.Plugins,
		MessagesBatch: a.MessagesBatch, MessagesMaxQueue: a.MessagesMaxQueue,
		GoalEnabled: a.GoalEnabled, GoalWaitCustomerTimeoutS: a.GoalWaitCustomerTimeoutS, GoalDeliveryCooldownS: a.GoalDeliveryCooldownS,
		CurrentGoalTaskKey: a.CurrentGoalTaskKey,
		Group:              a.Group, Alias: a.Alias, Notes: a.Notes, Color: a.Color,
		MaxIdleIterations: a.MaxIdleIterations,
		AIStallTimeoutS:   a.AIStallTimeoutS,
		Schedule:          a.ScheduleRRule, ScheduleNextAt: scheduleNextAt(a),
		Budget:    budget,
		AgentHalt: agentHalt(a, state),
	}, nil
}

// agentHalt derives the halt keys of every agent reader, so the idle-prefix
// test lives in exactly one place. error_reason is the persisted reason, or a
// derived AI-stall reason while the agent is in error.
func agentHalt(a agent.Agent, state string) AgentHalt {
	var h AgentHalt
	if a.ErrorReason != "" {
		h.ErrorReason = a.ErrorReason
	} else if state == "error" && a.AIStallTimeoutS > 0 {
		h.ErrorReason = fmt.Sprintf("agent stalled - no AI-proxy requests for %d seconds", a.AIStallTimeoutS)
	}
	h.HaltKind, h.HaltReason = a.HaltReason()
	return h
}

func agentImageSet() registry.Command {
	return registry.WithResult[AgentImageStatus](registry.Command{Path: "agent.image.set", Summary: "Select an image for the agent's next iteration", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "image", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/image"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		var result any
		// Agent image commands take the store lock: they read the agent's
		// current and pending images, which can have different names, and
		// change the assignment that removal and activation check.
		err := image.WithStoreLock(func() error {
			name := p.String("name")
			if _, err := getAgent(c, name); err != nil {
				return err
			}
			ref, err := image.ParseRef(p.String("image"))
			if err != nil {
				return api.UserError{Code: "bad_ref", Msg: err.Error(), Status: http.StatusBadRequest}
			}
			manifest, err := imageStore(c).Inspect(ref)
			if err != nil {
				return api.UserError{Code: "image_not_found", Msg: err.Error(), Status: http.StatusNotFound}
			}
			if err := agentStore(c).SetPendingImage(name, ref.String(), manifest.Digest); err != nil {
				return err
			}
			result, err = agentImageStatusValue(c, name)
			return err
		})
		return result, err
	}})
}

// The caller holds the image store lock, including when set returns its preview.
func agentImageStatusValue(c *registry.Ctx, name string) (AgentImageStatus, error) {
	a, err := getAgent(c, name)
	if err != nil {
		return AgentImageStatus{}, err
	}
	pending, err := agentStore(c).PendingImage(name)
	if err != nil {
		return AgentImageStatus{}, err
	}
	images := imageStore(c)
	inspect := func(refText, digest string, reason AgentImageReason) AgentImageProjection {
		projection := AgentImageProjection{ImageAssignment: agent.ImageAssignment{Ref: refText, Digest: digest}, Reason: reason}
		ref, err := image.ParseRef(refText)
		var manifest image.Manifest
		if err == nil {
			if reason == AgentImageReasonRefMoved {
				manifest, err = images.Inspect(ref)
			} else {
				manifest, err = images.InspectPinned(ref, digest)
			}
		}
		if err != nil {
			projection.Error = err.Error()
		} else {
			projection.Digest = manifest.Digest
			projection.ImageVersion = manifest.ImageVersion
		}
		return projection
	}
	current := inspect(a.ImageRef, a.ImageDigest, "")
	next := current
	next.Reason = AgentImageReasonCurrent
	if pending.Ref != "" {
		next = inspect(pending.Ref, pending.Digest, AgentImageReasonPending)
	} else if _, err := image.ParseRef(a.ImageRef); err == nil {
		// Any ref can be rebuilt, so the active ref's current content is what
		// the next iteration would adopt.
		next = inspect(a.ImageRef, "", AgentImageReasonRefMoved)
	}
	return AgentImageStatus{Name: name, Current: current, Pending: pending, Next: next}, nil
}

func agentImageStatus() registry.Command {
	return registry.WithResult[AgentImageStatus](registry.Command{Path: "agent.image.status", Summary: "Show current, pending, and next agent image", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/agents/{name}/image"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		var result any
		err := image.WithStoreLock(func() error {
			var err error
			result, err = agentImageStatusValue(c, p.String("name"))
			return err
		})
		return result, err
	}})
}

func agentImageCancel() registry.Command {
	return registry.WithResult[AgentImageStatus](registry.Command{Path: "agent.image.cancel", Summary: "Cancel a pending agent image", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodDelete, Path: "/api/agents/{name}/image"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		var result any
		err := image.WithStoreLock(func() error {
			name := p.String("name")
			if err := agentStore(c).ClearPendingImage(name); err != nil {
				return err
			}
			var err error
			result, err = agentImageStatusValue(c, name)
			return err
		})
		return result, err
	}})
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

func parseAgentEnv(value any) (map[string]string, error) {
	switch value := value.(type) {
	case nil:
		return map[string]string{}, nil
	case string:
		return parseKV(value), nil
	case map[string]string:
		return value, nil
	case map[string]any:
		out := make(map[string]string, len(value))
		for key, raw := range value {
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("env value for %q must be a string", key)
			}
			out[key] = text
		}
		return out, nil
	default:
		return nil, fmt.Errorf("env must be a K=V string or string-valued object")
	}
}

func parseAgentPlugins(value any) ([]string, error) {
	switch list := value.(type) {
	case string:
		return parseList(list), nil
	case []string:
		return list, nil
	}
	items, ok := value.([]any)
	if value != nil && !ok {
		return nil, fmt.Errorf("plugins must be a comma list or string array")
	}
	plugins := make([]string, 0, len(items))
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("plugins must be a comma list or string array")
		}
		plugins = append(plugins, name)
	}
	return plugins, nil
}

// intAtLeast reads a bound whole-number argument and enforces its minimum.
func intAtLeast(p registry.Params, key, code string, minimum int) (int, error) {
	value := p.Int(key)
	if p.Has(key) && value < minimum {
		return 0, api.UserError{Code: code, Msg: fmt.Sprintf("%s must be at least %d", key, minimum)}
	}
	return value, nil
}

var agentColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

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
	return registry.WithResult[AgentRunResult](registry.Command{
		Path:    "agent.run",
		Summary: "Create an agent from an image (image:tag); starts stopped — enable with 'agent start'",
		Args: []registry.Arg{
			{Name: "image", Type: registry.String, Required: true, Help: "image ref name:tag"},
			{Name: "name", Flag: "name", Type: registry.String, Help: "agent name (default: generated)"},
			{Name: "cwd", Flag: "cwd", Type: registry.String, Help: "working directory"},
			{Name: "create_cwd", Flag: "create-cwd", Type: registry.Bool, Help: "create the working directory if it does not exist"},
			{Name: "harness", Flag: "harness", Short: "a", Type: registry.String, Help: "harness: claude|codex|opencode|cursor|stub"},
			{Name: "model", Flag: "model", Short: "m", Type: registry.String, Help: "model"},
			{Name: "effort", Flag: "effort", Short: "e", Type: registry.String, Help: "effort"},
			{Name: "interactive", Flag: "interactive", Short: "i", Type: registry.Bool, Default: true, Help: "interactive (tmux) mode (default true)"},
			{Name: "env", Flag: "env", Type: registry.String, Help: "comma-separated K=V env pairs", Schema: map[string]any{"oneOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
			}}},
			{Name: "plugins", Flag: "plugins", Type: registry.String, Help: "comma-separated plugin override", Schema: map[string]any{"oneOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			}}},
			{Name: "group", Flag: "group", Short: "g", Type: registry.String, Help: "join this group"},
			{Name: "timeout", Flag: "timeout", Type: registry.String, Help: "soft iteration timeout duration (for example 60m or 2h; 0 = no timeout)"},
			{Name: "interval_s", Type: registry.Int, Help: "loop interval in whole seconds"},
			{Name: "timeout_s", Type: registry.Int, Help: "soft iteration timeout in whole seconds"},
			{Name: "hard_timeout_s", Type: registry.Int, Help: "hard iteration timeout in whole seconds (0 = no hard timeout)"},
			{Name: "on_timeout", Type: registry.String, Help: "timeout policy: restart or stop"},
			{Name: "on_error", Type: registry.String, Help: "error policy: restart or stop"},
			{Name: "max_idle_iterations", Type: registry.Int, Help: "maximum consecutive idle iterations; 0 disables"},
			{Name: "ai_stall_timeout_s", Type: registry.Int, Default: 1800, Help: "AI-proxy inactivity error threshold in whole seconds (default 1800)"},
			{Name: "user_prompt", Type: registry.String, Help: "standing user prompt"},
			{Name: "messages_batch", Type: registry.Int, Help: "maximum messages delivered per iteration"},
			{Name: "messages_max_queue", Type: registry.Int, Help: "maximum queued messages"},
			{Name: "goal_enabled", Flag: "goal-enabled", Type: registry.Bool, Default: true, Help: "enable Native Task goal selection (default true)"},
			{Name: "goal_wait_customer_timeout_s", Flag: "goal-wait-customer-timeout-s", Type: registry.Int, Default: 300, Help: "customer-wait grace period in whole seconds (default 300)"},
			{Name: "goal_delivery_cooldown_s", Flag: "goal-delivery-cooldown-s", Type: registry.Int, Default: 60, Help: "minimum seconds between Goal deliveries (default 60)"},
			{Name: "alias", Type: registry.String, Help: "display alias"},
			{Name: "notes", Type: registry.String, Help: "operator notes"},
			{Name: "color", Type: registry.String, Help: "agent accent color as #rrggbb"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			interactive := true
			if value, present := p.OptBool("interactive"); present {
				interactive = value
			}
			goalEnabled := true
			if value, present := p.OptBool("goal_enabled"); present {
				goalEnabled = value
			}
			goalWaitCustomerTimeoutS := 300
			if p.Has("goal_wait_customer_timeout_s") {
				var err error
				goalWaitCustomerTimeoutS, err = intAtLeast(p.Params, "goal_wait_customer_timeout_s", "bad_goal_wait_customer_timeout", 1)
				if err != nil {
					return nil, err
				}
			}
			goalDeliveryCooldownS := 60
			if p.Has("goal_delivery_cooldown_s") {
				var err error
				goalDeliveryCooldownS, err = intAtLeast(p.Params, "goal_delivery_cooldown_s", "bad_goal_delivery_cooldown", 1)
				if err != nil {
					return nil, err
				}
			}
			if p.Has("timeout") {
				if p.Has("timeout_s") {
					return nil, api.UserError{Code: "ambiguous_timeout", Msg: "timeout and timeout_s cannot be supplied together"}
				}
			}
			env, envErr := parseAgentEnv(p.Params["env"])
			if envErr != nil {
				return nil, api.UserError{Code: "bad_env", Msg: envErr.Error()}
			}
			pluginList, pluginsErr := parseAgentPlugins(p.Params["plugins"])
			if pluginsErr != nil {
				return nil, api.UserError{Code: "bad_plugins", Msg: pluginsErr.Error()}
			}
			timeoutS, timeoutErr := parseAgentTimeout(p.String("timeout"))
			if timeoutErr != nil {
				return nil, api.UserError{Code: "bad_timeout", Msg: timeoutErr.Error()}
			}
			intervalS, err := intAtLeast(p.Params, "interval_s", "bad_interval", 0)
			if err != nil {
				return nil, err
			}
			if p.Has("timeout_s") {
				timeoutS, err = intAtLeast(p.Params, "timeout_s", "bad_timeout", 0)
				if err != nil {
					return nil, err
				}
			}
			hardTimeoutS, err := intAtLeast(p.Params, "hard_timeout_s", "bad_hard_timeout", 0)
			if err != nil {
				return nil, err
			}
			maxIdleIterations, err := intAtLeast(p.Params, "max_idle_iterations", "bad_max_idle", 0)
			if err != nil {
				return nil, err
			}
			aiStallTimeoutS := 1800
			if p.Has("ai_stall_timeout_s") {
				aiStallTimeoutS, err = intAtLeast(p.Params, "ai_stall_timeout_s", "bad_ai_stall_timeout", 1)
				if err != nil {
					return nil, err
				}
			}
			messagesBatch, err := intAtLeast(p.Params, "messages_batch", "bad_messages_batch", 1)
			if err != nil {
				return nil, err
			}
			messagesMaxQueue, err := intAtLeast(p.Params, "messages_max_queue", "bad_messages_max_queue", 1)
			if err != nil {
				return nil, err
			}
			if !p.Has("messages_batch") {
				messagesBatch = 10
			}
			if !p.Has("messages_max_queue") {
				messagesMaxQueue = 100
			}
			onTimeout := p.String("on_timeout")
			if p.Has("on_timeout") && onTimeout != "restart" && onTimeout != "stop" {
				return nil, api.UserError{Code: "bad_on_timeout", Msg: "on_timeout must be restart or stop"}
			}
			onError := p.String("on_error")
			if p.Has("on_error") && onError != "restart" && onError != "stop" {
				return nil, api.UserError{Code: "bad_on_error", Msg: "on_error must be restart or stop"}
			}
			color := p.String("color")
			if color != "" && !agentColorPattern.MatchString(color) {
				return nil, api.UserError{Code: "bad_color", Msg: "color must be empty or a six-digit #rrggbb value"}
			}
			spec := registry.RunSpec{
				ImageRef: p.String("image"), Name: p.String("name"), Cwd: p.String("cwd"),
				Harness: p.String("harness"), Model: p.String("model"), Effort: p.String("effort"),
				Interactive: interactive, Env: env, Plugins: pluginList,
				IntervalS: intervalS, TimeoutS: timeoutS, HardTimeoutS: hardTimeoutS,
				OnTimeout: onTimeout, OnError: onError,
				MaxIdleIterations:        maxIdleIterations,
				AIStallTimeoutS:          aiStallTimeoutS,
				UserPrompt:               p.String("user_prompt"),
				MessagesBatch:            messagesBatch,
				MessagesMaxQueue:         messagesMaxQueue,
				GoalEnabled:              &goalEnabled,
				GoalWaitCustomerTimeoutS: goalWaitCustomerTimeoutS,
				GoalDeliveryCooldownS:    goalDeliveryCooldownS,
				Group:                    p.String("group"), Alias: p.String("alias"), Notes: p.String("notes"),
				Color: strings.ToLower(color),
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
			if err := ensureCwd(spec.Cwd, p.Bool("create_cwd")); err != nil {
				return nil, err
			}
			name, err := c.Control.Run(spec)
			if err != nil {
				return nil, api.UserError{Code: "run_failed", Msg: err.Error()}
			}
			state, _ := c.Control.LiveState(name)
			return AgentRunResult{Name: name, State: state,
				GoalEnabled: goalEnabled, GoalWaitCustomerTimeoutS: goalWaitCustomerTimeoutS, GoalDeliveryCooldownS: goalDeliveryCooldownS}, nil
		},
	})
}

func agentPs() registry.Command {
	return registry.WithResult[AgentListResult](registry.Command{
		Path:    "agent.ps",
		Summary: "List agents",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			list, err := agentStore(c).List()
			if err != nil {
				return nil, err
			}
			budgets, err := aiproxy.NewStore(c.Store, time.Now).AgentBudgetStatuses(time.Now())
			if err != nil {
				return nil, err
			}
			rows := make([]AgentRow, 0, len(list))
			for _, a := range list {
				state := liveStateOf(c, a)
				rows = append(rows, AgentRow{
					Name: a.Name, Image: a.ImageRef, State: state,
					Alias:   a.Alias,
					Harness: a.HarnessType, Enabled: a.Enabled, Group: a.Group,
					Color: a.Color, Cwd: a.Cwd, TimeoutS: a.TimeoutS,
					IntervalS: a.IntervalS, OnTimeout: a.OnTimeout, OnError: a.OnError,
					MaxIdleIterations: a.MaxIdleIterations,
					AIStallTimeoutS:   a.AIStallTimeoutS,
					Interactive:       a.Interactive,
					GoalEnabled:       a.GoalEnabled, GoalWaitCustomerTimeoutS: a.GoalWaitCustomerTimeoutS, GoalDeliveryCooldownS: a.GoalDeliveryCooldownS,
					CurrentGoalTaskKey: a.CurrentGoalTaskKey,
					Schedule:           a.ScheduleRRule, ScheduleNextAt: scheduleNextAt(a),
					Budget:    budgetView(budgets[a.Name]),
					AgentHalt: agentHalt(a, state),
				})
			}
			return AgentListResult{Agents: rows, Count: len(rows)}, nil
		},
	})
}

func agentGoalSettingsView(a agent.Agent) AgentGoalSettings {
	return AgentGoalSettings{Name: a.Name, GoalEnabled: a.GoalEnabled,
		GoalWaitCustomerTimeoutS: a.GoalWaitCustomerTimeoutS, GoalDeliveryCooldownS: a.GoalDeliveryCooldownS}
}

func refreshAgentGoal(c *registry.Ctx, name string) {
	if control, ok := c.Control.(registry.LoopConfigControl); ok {
		control.RefreshLoopConfig(name)
	}
}

func agentGoalEnabled() registry.Command {
	return registry.WithResult[AgentGoalSettings](registry.Command{
		Path: "agent.goal-enabled", Summary: "Enable or disable Native Task goal selection",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "enabled", Flag: "enabled", Type: registry.Bool, Required: true, Help: "true or false"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/goal-enabled"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			a.GoalEnabled = p.Bool("enabled")
			if err := agentStore(c).Update(a); err != nil {
				return nil, err
			}
			refreshAgentGoal(c, a.Name)
			return agentGoalSettingsView(a), nil
		},
	})
}

func agentGoalWaitCustomerTimeout() registry.Command {
	return registry.WithResult[AgentGoalSettings](registry.Command{
		Path: "agent.goal-wait-customer-timeout", Summary: "Set the Goal customer-wait grace period",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "seconds", Flag: "seconds", Type: registry.Int, Required: true, Help: "positive whole seconds"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/goal-wait-customer-timeout"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			seconds, err := intAtLeast(p.Params, "seconds", "bad_goal_wait_customer_timeout", 1)
			if err != nil {
				return nil, err
			}
			a.GoalWaitCustomerTimeoutS = seconds
			if err := agentStore(c).Update(a); err != nil {
				return nil, err
			}
			refreshAgentGoal(c, a.Name)
			return agentGoalSettingsView(a), nil
		},
	})
}

func agentGoalDeliveryCooldown() registry.Command {
	return registry.WithResult[AgentGoalSettings](registry.Command{Path: "agent.goal-delivery-cooldown", Summary: "Set the Goal delivery cooldown", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "seconds", Type: registry.Int, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/goal-delivery-cooldown"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		a, err := getAgent(c, p.String("name"))
		if err != nil {
			return nil, err
		}
		seconds, err := intAtLeast(p.Params, "seconds", "bad_goal_delivery_cooldown", 1)
		if err != nil {
			return nil, err
		}
		a.GoalDeliveryCooldownS = seconds
		if err := agentStore(c).Update(a); err != nil {
			return nil, err
		}
		refreshAgentGoal(c, a.Name)
		return agentGoalSettingsView(a), nil
	}})
}

func agentStatus() registry.Command {
	return registry.WithResult[AgentStatus](registry.Command{
		Path:    "agent.status.show",
		Summary: "Show one agent's runtime status",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/status"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			m, err := requireMail(c)
			if err != nil {
				return nil, err
			}
			messagesPending, err := m.OpenCount(a.Name)
			if err != nil {
				return nil, err
			}
			state, _ := c.Control.LiveState(a.Name)
			its, _ := agentStore(c).ListIterations(a.Name)
			last, lastID := "", ""
			if n := len(its); n > 0 {
				last = string(its[n-1].Status)
				lastID = its[n-1].ID
			}
			budget, err := agentBudgetView(c, a.Name)
			if err != nil {
				return nil, err
			}
			out := AgentStatus{Name: a.Name, State: state, Enabled: a.Enabled,
				Iterations: len(its), LastIteration: last, LastIterationID: lastID,
				StatusMessage: a.StatusMessage, StatusUpdated: a.StatusUpdated,
				MessagesPending: messagesPending, MessagesMaxQueue: a.MessagesMaxQueue,
				MessagesQueueFull: messagesPending >= a.MessagesMaxQueue,
				ServerNow:         store.FormatTime(time.Now()),
				Budget:            budget,
				AgentHalt:         agentHalt(a, state)}
			for i := len(its) - 1; i >= 0; i-- {
				it := its[i]
				if it.Status != agent.IterationRunning {
					continue
				}
				active := &AgentActiveIteration{ID: it.ID, StartedAt: it.StartedAt,
					TimeoutExtensions: it.TimeoutExtensions, TimeoutPeriodS: it.TimeoutPeriodS,
					TimeoutDeadline: it.TimeoutDeadline, HardTimeoutDeadline: it.HardTimeoutDeadline}
				if it.TimeoutDeadline != nil || it.HardTimeoutDeadline != nil {
					active.EffectiveDeadline = effectiveDeadline(it.TimeoutDeadline, it.HardTimeoutDeadline)
				}
				out.ActiveIteration = active
				break
			}
			return out, nil
		},
	})
}

func effectiveDeadline(soft, hard *string) string {
	if soft == nil {
		return *hard
	}
	if hard == nil {
		return *soft
	}
	st, serr := store.ParseTime(*soft)
	ht, herr := store.ParseTime(*hard)
	if serr == nil && herr == nil && ht.Before(st) {
		return *hard
	}
	return *soft
}

func agentInspect() registry.Command {
	return registry.WithResult[AgentView](registry.Command{
		Path:    "agent.inspect",
		Summary: "Show one agent's full config",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err
			}
			state, _ := c.Control.LiveState(a.Name)
			v, err := agentView(c, a, state)
			if err != nil {
				return nil, err
			}
			// Report the EFFECTIVE cwd (the workdir fallback when Cwd is unset),
			// matching cwdOf/agentCwdFor, so the UI never shows an empty value.
			if cwd, err := agentCwdFor(c, a.Name); err == nil {
				v.Cwd = cwd
			}
			return v, nil
		},
	})
}

func agentBudgetGet() registry.Command {
	return registry.WithResult[AgentBudget](registry.Command{Path: "agent.budget.get", Summary: "Show an agent's calendar USD budget", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/agents/{name}/budget"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		name := p.String("name")
		if _, err := getAgent(c, name); err != nil {
			return nil, err
		}
		return agentBudgetView(c, name)
	}})
}

func agentBudgetSet() registry.Command {
	return registry.WithResult[AgentBudget](registry.Command{Path: "agent.budget.set", Summary: "Set an agent's calendar USD budget", Args: []registry.Arg{
		{Name: "name", Type: registry.String, Required: true},
		{Name: "hour_usd", Type: registry.String, Required: true}, {Name: "day_usd", Type: registry.String, Required: true},
		{Name: "week_usd", Type: registry.String, Required: true}, {Name: "month_usd", Type: registry.String, Required: true},
	}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/budget"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		name := p.String("name")
		if _, err := getAgent(c, name); err != nil {
			return nil, err
		}
		values := []string{p.String("hour_usd"), p.String("day_usd"), p.String("week_usd"), p.String("month_usd")}
		parsed := [4]float64{}
		for i, value := range values {
			var err error
			parsed[i], err = parseFloat(value)
			if err != nil {
				return nil, api.UserError{Code: "bad_budget", Msg: "budget limits must be finite non-negative USD values"}
			}
		}
		if err := aiproxy.NewStore(c.Store, nil).SetAgentBudget(name, aiproxy.AgentBudget{HourUSD: parsed[0], DayUSD: parsed[1], WeekUSD: parsed[2], MonthUSD: parsed[3]}); err != nil {
			return nil, api.UserError{Code: "bad_budget", Msg: err.Error()}
		}
		return agentBudgetView(c, name)
	}})
}

func agentLifecycle(path, summary string, verb AgentAction) registry.Command {
	return registry.WithResult[AgentActionResult](registry.Command{
		Path:    path,
		Summary: summary,
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/" + string(verb)},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			var err error
			switch verb {
			case AgentActionStop:
				err = c.Control.Stop(name)
			case AgentActionStart:
				err = c.Control.Start(name)
			case AgentActionRestart:
				err = c.Control.Restart(name)
			case AgentActionKill:
				err = c.Control.Kill(name)
			}
			if err != nil {
				return nil, api.UserError{Code: string(verb) + "_failed", Msg: err.Error()}
			}
			return AgentActionResult{Name: name, Action: verb}, nil
		},
	})
}

func agentRm() registry.Command {
	return registry.WithResult[AgentRemoveResult](registry.Command{
		Path:    "agent.rm",
		Summary: "Remove an agent (stop it first, or --force)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "force", Flag: "force", Short: "f", Type: registry.Bool, Help: "kill and remove even if running"},
			{Name: "purge", Flag: "purge", Type: registry.Bool, Help: "also delete durable data (iterations, context, audit) and leaked rows; default preserves data"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/agents/{name}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			force := p.Bool("force")
			purge := p.Bool("purge")
			if err := c.Control.Remove(p.String("name"), force, purge); err != nil {
				return nil, api.UserError{Code: "rm_failed", Msg: err.Error()}
			}
			return AgentRemoveResult{Removed: p.String("name"), Purged: purge}, nil
		},
	})
}

func agentReprovision() registry.Command {
	return registry.WithResult[AgentStateResult](registry.Command{
		Path:    "agent.reprovision",
		Summary: "Re-unpack an agent's image tree in place and restart its loop, keeping its data",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "image", Flag: "image", Type: registry.String, Help: "image ref to (re)unpack; empty keeps the current image"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/reprovision"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			if err := c.Control.Reprovision(name, p.String("image")); err != nil {
				return nil, api.UserError{Code: "reprovision_failed", Msg: err.Error()}
			}
			state, _ := c.Control.LiveState(name)
			return AgentStateResult{Name: name, State: state}, nil
		},
	})
}

func getAgent(c *registry.Ctx, name string) (agent.Agent, error) {
	a, err := agentStore(c).Get(name)
	if err != nil {
		return agent.Agent{}, api.UserError{Code: "not_found", Msg: "agent " + name + " not found"}
	}
	return a, nil
}
