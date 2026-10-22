package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentapi"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/groups"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/logrotate"
	"github.com/alekzonder/tariboy/internal/loop"
	"github.com/alekzonder/tariboy/internal/maintenance"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/pricingcatalog"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/retention"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/scriptnotify"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/stores"
	"github.com/alekzonder/tariboy/internal/supportbundle"
	"github.com/alekzonder/tariboy/internal/taskgoal"
	"github.com/alekzonder/tariboy/internal/tasknotify"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/telegramplugin"
	"github.com/alekzonder/tariboy/internal/telemetry"
	"github.com/alekzonder/tariboy/internal/userpath"
	"github.com/alekzonder/tariboy/internal/version"
	"github.com/alekzonder/tariboy/internal/workflowimage"
	"github.com/alekzonder/tariboy/internal/workflowrun"

	"go.opentelemetry.io/otel"
)

// process is what setupProcess establishes: the logger, the resolved layout,
// the parsed listen spec and the directory of the daemon executable ("" when
// it cannot be resolved).
type process struct {
	log    *slog.Logger
	paths  paths.Paths
	listen api.Listen
	exeDir string
}

// setupProcess prepares the process: logger, PATH, base layout, the
// single-instance guard and pidfile, and the listen spec. Every change to the
// process environment happens here, before any goroutine starts.
func setupProcess(ctx context.Context, o Options, stack *shutdownStack) (*process, error) {
	lvl := slog.LevelInfo
	switch o.LogLevel {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	resolveUserPath := o.UserPathResolver
	if resolveUserPath == nil {
		resolveUserPath = userpath.Resolve
	}
	applyUserPath(ctx, log, resolveUserPath)
	if home, err := os.UserHomeDir(); err == nil {
		ensureLocalBin(home)
	}

	p := paths.New(o.BaseDir)
	if o.BaseDir == "" {
		var err error
		p, err = paths.Resolve(os.Getenv)
		if err != nil {
			return nil, err
		}
	}
	if err := p.EnsureBase(); err != nil {
		return nil, fmt.Errorf("ensure base dir: %w", err)
	}
	// Fail loudly at startup if the layout would produce an unbindable socket,
	// instead of surfacing an opaque EINVAL later. Sockets live in the short
	// home-rooted runtime dir, so this only trips on a pathological HOME.
	if err := paths.BindableSocketPath(p.Socket()); err != nil {
		return nil, fmt.Errorf("daemon socket: %w", err)
	}

	// Single-instance guard (approach A): if a live daemon already answers on the
	// socket, refuse to start rather than stomping its socket file. The probe->bind
	// window is microseconds and both racers would be our own daemon. Only for the
	// unix listener; the pidfile (written after the guard passes, removed on clean
	// exit) is read by `daemon stop`/`status`, not used as the guard.
	if strings.HasPrefix(o.Listen, "unix") {
		if c, derr := net.DialTimeout("unix", p.Socket(), 200*time.Millisecond); derr == nil {
			c.Close()
			return nil, fmt.Errorf("tariboyd already running (socket %s is live)", p.Socket())
		}
		if werr := os.WriteFile(p.PidFile(), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); werr != nil {
			return nil, fmt.Errorf("write pidfile: %w", werr)
		}
		stack.push("pidfile", func() { os.Remove(p.PidFile()) })
	}

	listen, err := api.ParseListen(o.Listen, o.AuthTokenFile)
	if err != nil {
		return nil, err
	}
	if o.HTTPAddr != "" {
		host, _, err := net.SplitHostPort(o.HTTPAddr)
		if err != nil {
			return nil, fmt.Errorf("bad --http-addr %q: %w", o.HTTPAddr, err)
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("--http-addr %q must be a loopback address (localhost-only listener)", o.HTTPAddr)
		}
	}

	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	} else {
		log.Warn("resolve daemon executable", "err", err)
	}
	tasksBinDir, err := installTasksAlias(firstNonEmpty(exeDir, "."), p.RuntimeDir())
	if err != nil {
		return nil, fmt.Errorf("prepare bundled Tasks command: %w", err)
	}
	if tasksBinDir != "" {
		if err := os.Setenv("PATH", tasksBinDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
			return nil, err
		}
	}
	return &process{log: log, paths: p, listen: listen, exeDir: exeDir}, nil
}

// coreParts are the store and what every later phase shares through it.
type coreParts struct {
	st            *store.Store
	metrics       *telemetry.Metrics
	agents        *agent.Store
	bus           *bus.Bus
	hub           *events.Hub
	audit         *audit.Registry
	rec           Recorder
	customerLogin string
}

// openStore opens the store and telemetry, and reconciles the agents' inboxes
// and chats.
func openStore(ctx context.Context, proc *process, stack *shutdownStack) (*coreParts, error) {
	log, p := proc.log, proc.paths
	st, err := store.Open(p.DB())
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	stack.push("store", func() { st.Close() })

	// Observability (spec §14): OTLP export is off unless an endpoint is set via
	// OTEL_EXPORTER_OTLP_ENDPOINT or the otlp_endpoint daemon-config key. Off ->
	// no-op providers, zero overhead. Read once at start (next-start pickup); the
	// env var wins, the store config key is the fallback. log is guaranteed
	// non-nil above, which telemetry.Setup relies on (it may log.Warn on flush).
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint == "" {
		if v, ok, cerr := st.ConfigGet("otlp_endpoint"); cerr == nil && ok {
			otlpEndpoint = v
		}
	}
	tel, err := telemetry.Setup(ctx, telemetry.Config{Endpoint: otlpEndpoint, ServiceName: "tariboy"}, log)
	if err != nil {
		return nil, fmt.Errorf("telemetry setup: %w", err)
	}
	// Force-flush buffered spans/metrics after the span/metric producers
	// (manager, plugin host, proxy) have stopped. OTLP off -> a cheap no-op.
	stack.push("telemetry", func() {
		// ctx is already canceled when the stack runs.
		fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(fctx)
	})
	metrics, err := telemetry.NewMetrics(otel.Meter("tariboy"))
	if err != nil {
		return nil, fmt.Errorf("telemetry metrics: %w", err)
	}

	as := agent.NewStore(st)
	channelBus := bus.New(st, time.Now)
	// Wired before any publish: workflow ingress numbers every message in its
	// publish transaction and keeps messages it has not consumed yet.
	channelBus.SetPublishTxHook(tasks.RecordWorkflowMessageSequence)
	channelBus.SetMessageRetention(tasks.WorkflowMessageRetainedSQL)
	if err := reconcileAgentInboxes(as, channelBus); err != nil {
		return nil, fmt.Errorf("reconcile agent inboxes: %w", err)
	}
	customerLogin, err := resolveCustomerLogin(st)
	if err != nil {
		return nil, fmt.Errorf("resolve customer login: %w", err)
	}
	if err := reconcileAgentChats(as, channelBus, "user:"+customerLogin); err != nil {
		return nil, fmt.Errorf("reconcile agent chats: %w", err)
	}
	hub := events.NewHub()
	// Per-agent audit log (audit.jsonl). One shared *audit.Log per agent backs
	// the Recorder, the loop engine's lifecycle sink, and the runner's log tailer.
	auditReg := audit.NewRegistry(
		func(a string) string { return agentdir.New(p.AgentsDir(), a).AuditLog() },
		time.Now,
	)
	return &coreParts{
		st: st, metrics: metrics, agents: as, bus: channelBus, hub: hub, audit: auditReg,
		rec:           Recorder{log: log, store: st, audit: auditReg, hub: hub},
		customerLogin: customerLogin,
	}, nil
}

// taskParts are the Tasks service and the goal and workflow machinery around it.
type taskParts struct {
	service   *tasks.Service
	hub       *tasks.Hub
	publisher *tasknotify.Publisher
	goal      *taskgoal.Reconciler
	goalStore *taskgoal.Store
	// ingress wakes the workflow observation reconciler; dispatch wakes the
	// workflow dispatcher.
	ingress, dispatch      *workflowIngressSignal
	goalSignal             func()
	goalIterationCompleted func(string, string)
	images                 *image.Store
	workflowImages         *workflowimage.Registry
	globalEnv              func() ([]string, error)
}

// buildTasks builds the Tasks service, seeds the default queue and the bare
// image, and reconciles workflow images.
func buildTasks(ctx context.Context, proc *process, c *coreParts) (*taskParts, error) {
	log, p, st := proc.log, proc.paths, c.st
	if err := tasks.MigrateLegacyKeys(st.DB, time.Now); err != nil {
		return nil, fmt.Errorf("migrate task keys: %w", err)
	}
	taskService := tasks.NewService(st.DB, c.customerLogin, time.Now)
	taskService.SetReader(st.Reader())
	taskHub := tasks.NewHub(taskService)
	taskService.SetHub(taskHub)
	taskService.SetRunBaseDir(p.Base)
	globalEnv := func() ([]string, error) { return loop.GlobalShellEnv(p.Base) }
	taskService.SetGlobalEnv(globalEnv)
	taskService.SetLogger(log)
	goalReconciler := taskgoal.NewReconciler(taskgoal.ReconcilerConfig{
		Store: st, Bus: c.bus, Clock: time.Now, Log: log,
	})
	// A task change, an agent change, and a finished iteration wake both the
	// goal reconciler and the workflow dispatcher.
	workflowDispatch := newWorkflowIngressSignal()
	goalSignal, goalIterationCompleted := withWorkflowDispatch(goalReconciler, workflowDispatch.Signal)
	taskService.SetGoalSignal(goalSignal)
	if err := taskService.EnsureDefaultQueue(ctx); err != nil {
		log.Error("seed default task queue", "err", err)
	}
	imgStore := &image.Store{Dir: p.ImagesDir()}
	if err := image.WithStoreLock(imgStore.Migrate); err != nil {
		return nil, fmt.Errorf("migrate image store: %w", err)
	}
	if err := image.EnsureBare(imgStore, time.Now); err != nil {
		log.Error("seed bare image", "err", err)
	}
	workflowImages := &workflowimage.Registry{Store: &workflowimage.Store{Dir: p.WorkflowsDir()}, DB: st.DB}
	if err := workflowImages.Reconcile(); err != nil {
		log.Warn("reconcile workflow images", "err", err)
	}
	taskService.SetWorkflowResolver(func(ref string) (string, error) {
		name, tag := ref, ""
		if i := strings.LastIndex(ref, ":"); i >= 0 {
			name, tag = ref[:i], ref[i+1:]
		}
		return workflowImages.Store.Resolve(name, tag)
	})
	// A queue bound by a tag follows it. Builds and imports move bindings
	// through registry.Ctx; startup catches a move a crash interrupted.
	if err := taskService.FollowQueueWorkflows(ctx, ""); err != nil {
		log.Warn("follow queue workflows", "err", err)
	}
	return &taskParts{
		service: taskService, hub: taskHub,
		publisher: tasknotify.New(st.DB, c.bus, time.Now, log),
		goal:      goalReconciler, goalStore: taskgoal.NewStore(st),
		ingress: newWorkflowIngressSignal(), dispatch: workflowDispatch,
		goalSignal: goalSignal, goalIterationCompleted: goalIterationCompleted,
		images: imgStore, workflowImages: workflowImages, globalEnv: globalEnv,
	}, nil
}

// proxyParts are the AI proxy and its pricing, usage and budget machinery.
type proxyParts struct {
	proxy    *aiproxy.Proxy
	tokens   *aiproxy.TokenRegistry
	catalog  *pricingcatalog.Catalog
	aiStore  *aiproxy.Store
	ingester *aiproxy.Ingester
	budget   *aiproxy.BudgetCache
}

// buildProxy loads pricing and the router and builds the AI proxy (spec §9).
// It binds no listener; listenProxy does.
//
// Pricing and the router are read once here from the "proxy" daemon-config
// key and env. A runtime `daemon config set proxy '<json>'` is therefore
// picked up on the next daemon start, not hot-reloaded.
func buildProxy(ctx context.Context, o Options, proc *process, c *coreParts) (*proxyParts, error) {
	log, p, st := proc.log, proc.paths, c.st
	if err := aiproxy.SeedDefaults(st); err != nil {
		return nil, fmt.Errorf("seed pricing: %w", err)
	}
	pricing, err := aiproxy.LoadPricing(st, aiproxy.DefaultPricing())
	if err != nil {
		return nil, fmt.Errorf("load pricing: %w", err)
	}
	pricingCatalog := pricingcatalog.New(pricingcatalog.Config{
		HTTPClient: o.PricingHTTPClient,
		Clock:      o.PricingClock,
		After:      o.PricingAfter,
		SourceURL:  o.PricingSourceURL,
		CachePath:  p.PricingCatalogFile(),
		Store:      st,
		Pricing:    pricing,
		Diagnostic: func(d pricingcatalog.Diagnostic) {
			fields := safePricingFields(d)
			log.LogAttrs(ctx, levelForPricing(d.Kind), d.Kind, safePricingAttrs(fields)...)
			payload, _ := json.Marshal(fields)
			c.rec.Record("", d.Kind, string(payload))
		},
	})
	// Cache reads are local and strictly size-bounded. A rejected cache is
	// diagnostic-only: fallback/manual pricing and proxy startup remain live.
	_ = pricingCatalog.LoadCache(ctx)
	router, err := aiproxy.LoadRouter(st, os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("load router: %w", err)
	}
	aiStore := aiproxy.NewStore(st, time.Now)
	ingester := aiproxy.NewIngester(aiStore, log)
	budgetCache := aiproxy.NewBudgetCache(aiStore, time.Now)
	_ = budgetCache.Refresh()
	if o.wireHook != nil {
		o.wireHook(ingester, aiStore)
	}

	proxyTokens, err := aiproxy.OpenTokenRegistry(p.ProxyHandoffFile(), nil)
	if err != nil {
		return nil, err
	}
	proxy := aiproxy.New(aiproxy.Config{
		Tokens: proxyTokens, Pricing: pricing, Store: aiStore, Router: router,
		AgentsDir: p.AgentsDir(), Clock: time.Now, Log: log,
		Ingest:        ingester.Enqueue,
		Activity:      c.agents.RecordAIRequest,
		GroupSnapshot: aiStore.CurrentGroup,
		Budget:        budgetCache,
		Emit: func(agent string, data map[string]any) {
			c.hub.Emit(events.Event{Agent: agent, Type: "proxy",
				Time: store.FormatTime(time.Now()), Data: data})
			// Record per-request token/cost/latency metrics. The emit closure is
			// called in-process with native Go types (emitProxyEvent), so int/
			// float64/string assertions hold; a mismatch yields a zero reading,
			// never a panic.
			in, _ := data["input_tokens"].(int)
			out, _ := data["output_tokens"].(int)
			cost, _ := data["cost_usd"].(float64)
			lat, _ := data["latency_ms"].(int)
			status, _ := data["status"].(string)
			c.metrics.RecordProxyRequest(ctx, status, float64(lat), in, out, cost)
		},
		Warn: func(agent string, d aiproxy.Decision) {
			_, _ = c.bus.Publish(bus.Message{Channel: bus.InboxChannel(agent), Type: "budget.warn",
				Source: "proxy", ProducedByAgent: agent,
				Text: "AI budget warning: spend exceeded limit",
				Data: map[string]any{"scope": d.Scope, "limit_usd": d.LimitUSD, "spent_usd": d.SpentUSD}})
			c.rec.Record(agent, "budget_warn", fmt.Sprintf(`{"scope":%q,"spent_usd":%g}`, d.Scope, d.SpentUSD))
		},
		Audit: c.rec.Record,
	})
	return &proxyParts{
		proxy: proxy, tokens: proxyTokens, catalog: pricingCatalog,
		aiStore: aiStore, ingester: ingester, budget: budgetCache,
	}, nil
}

// listenProxy binds the AI proxy before the manager starts, so an iteration
// the manager resumes already has a proxy address. serve starts serving it.
func listenProxy(proc *process, c *coreParts, px *proxyParts, stack *shutdownStack) error {
	addr, err := listenProxyWithHandoff(c.agents, px.proxy, px.tokens)
	if err != nil {
		return fmt.Errorf("proxy listen: %w", err)
	}
	// Releases the listener if startup fails before serve takes it over.
	stack.push("ai-proxy-listener", func() { _ = px.proxy.CloseListener() })
	proc.log.Info("ai proxy listening", "addr", addr)
	return nil
}

// recordCursorUsage records the usage a Cursor iteration reported, which
// bypasses the proxy, as if the proxy had seen the request.
func recordCursorUsage(ctx context.Context, proc *process, c *coreParts, px *proxyParts) func(string, string, string, string, string, loop.CursorResultUsage) {
	return func(agentName, iteration, imageName, imageTag, imageDigest string, usage loop.CursorResultUsage) {
		groupID, groupName, _ := px.aiStore.CurrentGroup(agentName)
		row := aiproxy.AIRequest{
			ID: aiproxy.NewRequestID(nil), TS: store.FormatTime(time.Now()),
			Agent: agentName, Iteration: iteration,
			ImageName: imageName, ImageTag: imageTag, ImageDigest: imageDigest,
			Provider: "cursor", Model: usage.Model,
			InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
			CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
			LatencyMs: usage.DurationMs, Status: "ok",
			GroupID: groupID, GroupName: groupName,
		}
		if err := aiproxy.RecordUsage(proc.paths.AgentsDir(), row, px.ingester.Enqueue); err != nil {
			proc.log.Warn("append proxy transcript", "agent", agentName, "iteration", iteration, "err", err)
		}
		c.hub.Emit(events.Event{Agent: agentName, Type: "proxy",
			Time: store.FormatTime(time.Now()), Data: map[string]any{
				"request_id": row.ID, "iteration_id": row.Iteration,
				"model": row.Model, "provider": row.Provider,
				"input_tokens": row.InputTokens, "output_tokens": row.OutputTokens,
				"cache_write_tokens": row.CacheWriteTokens, "cache_read_tokens": row.CacheReadTokens,
				"cost_usd": row.CostUSD, "latency_ms": row.LatencyMs, "status": row.Status,
			}})
		c.metrics.RecordProxyRequest(ctx, row.Status, float64(row.LatencyMs), row.InputTokens, row.OutputTokens, row.CostUSD)
	}
}

// pluginParts are the external plugin host (spec §7) and its API.
type pluginParts struct {
	store *plugins.Store
	host  *plugins.Host
	api   http.Handler
}

// buildPlugins builds the plugin host and API and wires the provider contract
// into the bus. One TokenRegistry is shared between the host (which mints
// per-plugin tokens) and the plugin API (which resolves them).
func buildPlugins(proc *process, c *coreParts) *pluginParts {
	log, p := proc.log, proc.paths
	pluginTokens := plugins.NewTokenRegistry(nil)
	pluginStore := plugins.NewStore(c.st, time.Now)
	pluginHost := plugins.NewHost(plugins.HostConfig{
		Store: pluginStore, Bus: c.bus, Tokens: pluginTokens,
		PluginsDir: p.PluginsDir(), DaemonSocket: p.Socket(),
		Clock: time.Now, After: time.After, Log: log,
	})
	pluginAPI := plugins.NewAPI(pluginTokens, c.bus, log, func(plugin, action, detail string) {
		c.rec.Record("", "plugin_"+action, fmt.Sprintf(`{"plugin":%q,"detail":%q}`, plugin, detail))
	})

	// Provider contract (spec §6.1): gate parameterized subscribes on the target
	// channel's declared params_schema. Records are read fresh per subscribe (an
	// infrequent op) so newly installed providers take effect without a restart. A
	// transient store-read failure treats the channel as non-provider and allows
	// the subscribe (ParamsValidatorFor) rather than failing every subscribe —
	// core channels never needed the plugin store.
	c.bus.SetParamsValidator(plugins.ParamsValidatorFor(pluginStore,
		func(channel string, err error) {
			log.Warn("params validator: list plugins", "channel", channel, "err", err)
		}))

	// Provider watch lifecycle (spec §6.2): after any subscribe/unsubscribe, map
	// the affected channel to its provider plugin and push the channel's full
	// current watch list to that plugin's socket. Non-provider channels are a
	// no-op. The push is best-effort (async, capped backoff inside the host); the
	// plugin's GET /api/plugin/watches pull path guarantees eventual consistency.
	c.bus.SetSubscriptionHook(func(channel string) {
		recs, err := pluginStore.List()
		if err != nil {
			log.Warn("subscription hook: list plugins", "channel", channel, "err", err)
			return
		}
		owner, _, ok := plugins.ProviderFor(recs, channel)
		if !ok {
			return // not a provided channel — no provider to notify
		}
		watches, err := c.bus.WatchList(channel)
		if err != nil {
			log.Warn("subscription hook: watch list", "channel", channel, "err", err)
			return
		}
		pluginHost.PushWatches(owner, channel, watches)
	})
	return &pluginParts{store: pluginStore, host: pluginHost, api: pluginAPI}
}

// providedChannels feeds the Messages skill provider-declared channels from
// installed plugin manifests (spec §6.1), read fresh per call so a newly
// installed provider is annotated without a restart.
func providedChannels(pluginStore *plugins.Store) func() ([]agentapi.ProvidedChannel, error) {
	return func() ([]agentapi.ProvidedChannel, error) {
		recs, err := pluginStore.List()
		if err != nil {
			return nil, err
		}
		infos := plugins.ProvidedChannels(recs)
		out := make([]agentapi.ProvidedChannel, len(infos))
		for i, in := range infos {
			out[i] = agentapi.ProvidedChannel{
				Channel: in.Channel, Provider: in.Provider,
				Params: in.Params, Help: in.Help,
			}
		}
		return out, nil
	}
}

// parseDeadline is the single source of truth for the request deadline format,
// shared by the pre-publish validator and the arm hook so both accept exactly
// the same strings (Go durations, e.g. 5m/3s/1h).
func parseDeadline(deadline string) (time.Duration, error) {
	dur, err := time.ParseDuration(deadline)
	if err != nil {
		return 0, fmt.Errorf("request deadline %q: %w", deadline, err)
	}
	return dur, nil
}

// deadlineHooks wire request deadlines (spec §4.2) to the schedule subsystem:
// arm adds a one-shot schedule that publishes a type=timeout event into the
// requester's inbox at the deadline; cancel removes exactly that entry by its
// correlation id when a reply lands first.
func deadlineHooks(sched *schedule.Store) (arm func(*sql.Tx, string, string, string) error, cancel func(*sql.Tx, string) error) {
	arm = func(tx *sql.Tx, agent, correlationID, deadline string) error {
		dur, err := parseDeadline(deadline)
		if err != nil {
			return err
		}
		fireAt := store.FormatTime(time.Now().Add(dur))
		tmpl, err := json.Marshal(map[string]any{
			"type": "timeout",
			"text": "request timed out with no reply",
			"data": map[string]any{"correlation_id": correlationID},
		})
		if err != nil {
			return err
		}
		_, err = sched.AddTx(tx, schedule.Schedule{
			Agent:           agent,
			Kind:            "oneshot",
			Spec:            fireAt,
			Channel:         bus.InboxChannel(agent),
			MessageTemplate: string(tmpl),
			CorrelationID:   correlationID,
		})
		return err
	}
	cancel = func(tx *sql.Tx, correlationID string) error {
		return sched.CancelByCorrelationTx(tx, correlationID)
	}
	return arm, cancel
}

// managerParts are the loop manager, the stores it drives, and the registry
// context the API and the Store auto-builder dispatch commands through.
type managerParts struct {
	manager         *loop.Manager
	schedules       *schedule.Store
	scriptPublisher *scriptnotify.Publisher
	maint           *maintenance.Service
	reg             *registry.Registry
	cctx            *registry.Ctx
}

// buildManager builds the loop manager and wires the bus hooks that need it.
func buildManager(ctx context.Context, o Options, proc *process, c *coreParts, tk *taskParts, px *proxyParts, pl *pluginParts) *managerParts {
	log, p, st := proc.log, proc.paths, c.st
	schedStore := schedule.NewStore(st, time.Now)
	scriptStore := script.NewStore(st, time.Now)
	scriptPublisher := scriptnotify.New(st.DB, c.bus, time.Now, log)
	groupProv := groups.NewProvisioner(groups.ProvisionerConfig{
		Groups:    groups.NewStore(st, time.Now),
		Agents:    c.agents,
		Bus:       c.bus,
		GroupsDir: filepath.Join(p.Base, "groups"),
		Clock:     time.Now,
	})
	var _ loop.GroupProvisioner = groupProv

	c.bus.SetDeadlineValidator(func(deadline string) error {
		_, err := parseDeadline(deadline)
		return err
	})
	c.bus.SetDeadlineHooks(deadlineHooks(schedStore))

	// Retention (spec §12): the nightly maintenance run archives and prunes
	// every agent's iterations per its policy after the database backup.
	retPolicies := retention.NewStore(st)
	retPruner := retention.NewPruner(st, c.agents, retPolicies, p.AgentsDir(), time.Now, log)
	// Nightly database backup, then iteration pruning, cleanup and
	// compaction. A worker; drained before the store closes.
	maint := maintenance.New(st, filepath.Join(p.Base, "backups", "db"), time.Now, log)
	maint.TasksDir = filepath.Join(p.Base, "tasks")
	maint.PruneIterations = func() (int64, error) {
		reps, err := retPruner.PruneAll(false)
		var n int64
		for _, r := range reps {
			n += int64(len(r.Pruned))
		}
		return n, err
	}

	// Gzip the AI-proxy transcript at iteration close (spec §9/§12). loop stays
	// free of an aiproxy import (see loop.ProxyBinder), so this is wired here.
	// GzipTranscript is a clean no-op when the plain transcript is missing.
	onIterationClose := func(agent, iterationID string) {
		if err := aiproxy.GzipTranscript(p.AgentsDir(), agent, iterationID); err != nil {
			log.Warn("gzip proxy transcript", "agent", agent, "iteration", iterationID, "err", err)
		}
	}
	manager := loop.NewManager(loop.ManagerConfig{
		AgentsDir: p.AgentsDir(), RuntimeDir: p.RuntimeDir(),
		ShimBin:  firstNonEmpty(os.Getenv("TARIBOY_SHIM_BIN"), filepath.Join(firstNonEmpty(proc.exeDir, "."), "tariboy-shim")),
		ImgStore: tk.images, Store: c.agents, Log: log, Clock: time.Now, Bus: c.bus,
		Schedules: schedStore, Scripts: scriptStore, ScriptResults: scriptPublisher, Emit: c.hub.Emit, Proxy: px.proxy,
		Groups: groupProv,
		CodexApproval: func() string {
			v, _, err := st.ConfigGet(harness.CodexApprovalConfigKey)
			if err != nil {
				log.Warn("read codex approval mode", "err", err)
			}
			return v
		},
		RecordCursorUsage:  recordCursorUsage(ctx, proc, c, px),
		Tasks:              tk.service,
		ExternalPlugins:    plugins.ResolveEnabledInstalledMetadata(p.PluginsDir(), pl.store),
		Spawner:            o.Spawner,
		OnIterationClose:   onIterationClose,
		GoalSignal:         tk.goalSignal,
		IterationCompleted: tk.goalIterationCompleted,
		RecordIterationEnd: tk.service.RecordIterationEnd,
		CurrentGoal:        tk.goalStore.Current,
		WorkflowGoals:      loop.TaskWorkflowGoals{Tasks: tk.service, Images: tk.workflowImages.Store, Log: log},
		SetGoal: func(agent, key string, activate func() (func(), error)) error {
			_, err := tk.goalStore.Set(agent, key, time.Now().UTC(), activate)
			return err
		},
		ProvidedChannels: providedChannels(pl.store),
		AuditFor:         func(a string) loop.Recorder { return c.audit.For(a) },
		Metrics:          c.metrics,
		// UsageLookup adapts aiStore.IterationUsage (which returns an error) into
		// the error-less accessor the loop expects: swallow the error and return
		// zeros so a lookup failure never propagates into an iteration.
		UsageLookup: func(iter string) (int, int, float64) {
			in, out, cost, _ := px.aiStore.IterationUsage(iter)
			return in, out, cost
		},
		UsageFlush: func() {
			if err := px.ingester.Flush(); err != nil {
				log.Error("flush terminal iteration usage", "err", err)
			}
		},
	})
	// Register the observable gauges exactly once (spec §14): bus queue depth,
	// healthy plugins, active agent loops. No-op instruments when OTel is off.
	_ = c.metrics.RegisterGauges(telemetry.GaugeSource{
		QueueDepth: func(context.Context) int64 {
			n, _ := c.bus.PendingTotal()
			return int64(n)
		},
		PluginsHealthy: func(context.Context) int64 {
			list, _ := pl.host.List()
			var n int64
			for _, plugin := range list {
				if s, _ := plugin["state"].(string); s == "running" {
					n++
				}
			}
			return n
		},
		ActiveAgents:  func(context.Context) int64 { return int64(manager.ActiveAgents()) },
		EventsDropped: func(context.Context) int64 { return c.hub.Dropped() },
	})

	// A publish wakes exactly the agents that received a delivery and feeds the
	// SSE hub with per-agent message/stream events.
	c.bus.SetPublishHook(func(msg bus.Message, agents []string) {
		manager.WakeAgents(agents, loop.WakeMessage)
		emitMessageEvent(c.hub, msg, agents)
		if tk.service.WorkflowIngressEnabled() {
			tk.ingress.Signal()
		}
	})
	// Route bus lifecycle events (message_processed/replied/requeued) to the
	// per-agent audit.jsonl so the §8.2 operator-vs-agent attribution actually
	// lands in the audit timeline.
	c.bus.SetAuditHook(c.rec.Record)

	cctx := &registry.Ctx{
		Store: st, Log: log, BaseDir: p.Base, Socket: p.Socket(), HTTPAddr: o.HTTPAddr,
		Version: version.Version, StartedAt: time.Now(), Control: manager, Scripts: manager, Bus: c.bus, Plugins: pl.host,
		Groups:         groupProv,
		Operator:       tk.service.CustomerLogin(),
		Retention:      &retention.RetentionAPI{Policies: retPolicies, Pruner: retPruner},
		Maintenance:    maint,
		Tasks:          tk.service,
		WorkflowImages: tk.workflowImages.Store,
	}
	return &managerParts{
		manager: manager, schedules: schedStore, scriptPublisher: scriptPublisher,
		maint: maint, reg: commands.BuildRegistry(), cctx: cctx,
	}
}

// startRuntime reattaches the agent loops and starts the enabled plugins.
func startRuntime(ctx context.Context, proc *process, mg *managerParts, pl *pluginParts, stack *shutdownStack) {
	log := proc.log
	if err := mg.manager.StartAll(ctx); err != nil {
		log.Error("reattach failed", "err", err)
	}
	stack.push("manager", mg.manager.Shutdown)

	if err := pl.host.StartAll(ctx); err != nil {
		log.Error("plugin host start", "err", err)
	}
	if proc.exeDir != "" {
		telegramExecutable := filepath.Join(proc.exeDir, "tariboy-plugin-telegram")
		if _, err := pl.host.EnsureBundled(telegramExecutable, telegramplugin.Manifest(version.Version)); err != nil {
			log.Error("install bundled telegram plugin", "err", err)
		}
	}
	// StopAll cancels every supervisor + sink drainer and waits, so plugins
	// stop and their outbox drains before the manager and the store (spec §13).
	stack.push("plugins", pl.host.StopAll)
}

// startWorkers starts the background workers on a context derived from ctx.
// Every worker touches the store, so the "workers" step cancels them and waits
// for their final flush (the ingester's last batch, an in-flight maintenance
// run or build) before the store closes (spec §13).
func startWorkers(ctx context.Context, o Options, proc *process, c *coreParts, tk *taskParts, px *proxyParts, mg *managerParts, stack *shutdownStack) {
	log := proc.log
	gctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	stack.push("workers", func() {
		cancel()
		wg.Wait()
	})
	scheduler := schedule.NewScheduler(mg.schedules, c.bus, log, time.Now, time.After)
	wg.Go(func() {
		if o.schedulerRun != nil {
			o.schedulerRun(gctx, scheduler)
			return
		}
		scheduler.Run(gctx)
	})
	wg.Go(func() { px.catalog.Run(gctx) })
	wg.Go(func() { px.ingester.Run(gctx) })
	wg.Go(func() { tk.publisher.Run(gctx) })
	wg.Go(func() { tk.goal.Run(gctx) })
	wg.Go(func() { mg.scriptPublisher.Run(gctx) })
	wg.Go(func() {
		runWorkflowObservationReconciler(gctx, tk.service, tk.ingress.C(), workflowObservationReconcileInterval, log)
	})
	wg.Go(func() {
		runWorkflowDispatcher(gctx, tk.service, tk.dispatch.C(), workflowDispatchInterval, log)
	})
	wg.Go(func() {
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-gctx.Done():
				return
			case <-tick.C:
				if err := px.budget.Refresh(); err != nil {
					log.Warn("budget cache refresh", "err", err)
				}
			}
		}
	})
	wg.Go(func() { mg.maint.Loop(gctx, time.After) })
	// Store automatic builds refresh due Stores and rebuild their selected
	// outdated images through the same registry command the API and CLI use, so
	// an automatic build is identical to an operator build (image_version plus
	// latest).
	wg.Go(func() {
		stores.New(c.st, proc.paths.Base).RunAuto(gctx, func(ctx context.Context, selector string) error {
			command, ok := mg.reg.Get("image.build")
			if !ok {
				return errors.New("image.build command is unavailable")
			}
			_, err := registry.Call(mg.cctx, ctx, command, map[string]any{"source": selector})
			return err
		}, log)
	})
	// The workflow script worker executes check and watch runs. Before it
	// starts any, it terminates the scripts a previous daemon left running and
	// records their runs as interrupted. On cancel it kills its scripts; their
	// rows stay running for the next start.
	workflowRunWake, stopWorkflowRunWake := tk.hub.Subscribe()
	workflowRunner := &workflowrun.Supervisor{
		Jobs: tk.service, Sources: tk.service, Images: tk.workflowImages.Store, BaseDir: proc.paths.Base,
		BaseEnv: tk.globalEnv, AgentRuntime: mg.manager.ScriptRuntime,
		Clock: time.Now, Wake: workflowRunWake, Log: log,
	}
	wg.Go(func() {
		defer stopWorkflowRunWake()
		workflowRunner.Run(gctx)
	})
}

// serve binds the API listeners, then serves them and the AI proxy until ctx
// ends or a server fails. On both paths every server is shut down and its
// goroutine awaited before the workers, the manager and the store stop.
func serve(ctx context.Context, o Options, proc *process, c *coreParts, tk *taskParts, px *proxyParts, pl *pluginParts, mg *managerParts, stack *shutdownStack) error {
	log, p := proc.log, proc.paths
	var proxyServe, apiServe sync.WaitGroup
	stack.push("ai-proxy", func() {
		// ctx is already canceled when the stack runs.
		shctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = px.proxy.Shutdown(shctx)
		proxyServe.Wait()
	})

	accessLog, err := logrotate.Open(p.AccessLogFile(), accessLogMaxBytes, accessLogKeep)
	if err != nil {
		return fmt.Errorf("open access log: %w", err)
	}
	stack.push("access-log", func() { accessLog.Close() })
	srv := api.NewServer(mg.reg, mg.cctx)
	var stopOnce sync.Once
	var stopErr error
	stopAPI := func() error {
		stopOnce.Do(func() {
			shctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			stopErr = srv.Shutdown(shctx)
			apiServe.Wait()
		})
		return stopErr
	}
	stack.push("api-servers", func() {
		if err := stopAPI(); err != nil {
			log.Warn("api server shutdown", "err", err)
		}
	})
	srv.SetEventSource(c.hub)
	srv.SetTasks(tk.hub, func() tasks.Actor {
		return tasks.CustomerActor(tk.service.CustomerLogin())
	})
	srv.SetPluginAPI(pl.api)
	srv.SetAccessLog(slog.New(slog.NewJSONHandler(accessLog, nil)))
	srv.SetExternalPlugins(plugins.ResolveInstalled(p.PluginsDir()))
	srv.SetSupportBundleSource(supportbundle.Collector{
		Store: c.st, Control: mg.manager, BaseDir: p.Base, LogFile: p.LogFile(),
		Version: version.Version, Now: time.Now, Environ: os.Environ,
	})

	var serveAPI func() error
	if proc.listen.Network == "tcp" {
		serveAPI, err = srv.BindTCP(proc.listen.Addr, proc.listen.AuthToken)
	} else {
		sock := proc.listen.Addr
		if sock == "" {
			sock = p.Socket()
		}
		serveAPI, err = srv.BindUnix(sock)
	}
	if err != nil {
		return err
	}
	var serveWeb func() error
	if o.HTTPAddr != "" {
		if serveWeb, err = srv.BindWeb(o.HTTPAddr); err != nil {
			return fmt.Errorf("http listener bind: %w", err)
		}
	}

	c.rec.Record("", "daemon_start", fmt.Sprintf(`{"pid":%d,"version":%q}`, os.Getpid(), version.Version))
	log.Info("tariboyd starting", "version", version.Version, "base", p.Base, "listen", o.Listen)
	errc := make(chan error, 2)
	start := func(wg *sync.WaitGroup, name string, fn func() error, onErr func(error)) {
		if o.serveHook != nil {
			fn = o.serveHook(name, fn)
		}
		wg.Go(func() {
			if err := fn(); err != nil {
				onErr(err)
			}
		})
	}
	// Per-iteration proxy tokens are minted and revoked by the runner; the
	// daemon only owns the listener's lifecycle. A proxy failure is logged, as
	// agents keep the rest of the daemon.
	start(&proxyServe, "ai-proxy", px.proxy.Serve, func(err error) { log.Error("ai proxy serve", "err", err) })
	start(&apiServe, "api", serveAPI, func(err error) { errc <- err })
	if serveWeb != nil {
		start(&apiServe, "web", serveWeb, func(err error) { errc <- fmt.Errorf("http listener serve: %w", err) })
		log.Info("http api listening", "addr", o.HTTPAddr)
	}
	if o.Ready != nil {
		o.Ready()
	}

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		c.rec.Record("", "daemon_stop", "{}")
		log.Info("tariboyd stopping")
		return stopAPI()
	}
}
