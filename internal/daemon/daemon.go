// Package daemon wires store + registry + api server into tariboyd.
package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
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
	"github.com/alekzonder/tariboy/internal/builtinimages"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/evals"
	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/groups"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imageprovenance"
	"github.com/alekzonder/tariboy/internal/improvement"
	"github.com/alekzonder/tariboy/internal/judge"
	"github.com/alekzonder/tariboy/internal/loop"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/pricingcatalog"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/retention"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/scriptnotify"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/supportbundle"
	"github.com/alekzonder/tariboy/internal/tasknotify"
	"github.com/alekzonder/tariboy/internal/taskreminder"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/telegramplugin"
	"github.com/alekzonder/tariboy/internal/telemetry"
	"github.com/alekzonder/tariboy/internal/userpath"
	"github.com/alekzonder/tariboy/internal/version"
	storeassets "github.com/alekzonder/tariboy/store"

	"go.opentelemetry.io/otel"
)

type Options struct {
	BaseDir       string
	Listen        string // "unix" | "unix:/path" | "tcp:host:port"
	AuthTokenFile string
	LogLevel      string
	// HTTPAddr is a loopback host:port for the JSON API / WS listener (empty
	// disables it). A non-loopback address is refused at startup: this listener
	// is unauthenticated, and only localhost clients — the desktop app or an SSH
	// port-forward — are meant to reach it.
	HTTPAddr string
	// Spawner is an optional seam for launching iteration shims. nil uses the
	// real ExecSpawner; tests inject a capturing spawner to observe the env the
	// harness is started with (e.g. that it points at the AI proxy).
	Spawner loop.Spawner
	// PricingHTTPClient, PricingClock, PricingAfter, and PricingSourceURL are
	// catalog lifecycle seams. Production callers leave them unset to use the
	// fixed LiteLLM HTTPS source, real clock, and daily timer.
	PricingHTTPClient pricingcatalog.HTTPClient
	PricingClock      func() time.Time
	PricingAfter      func(time.Duration) <-chan time.Time
	PricingSourceURL  string

	// wireHook, if set, is invoked once the AI-proxy ingester and store are
	// wired, before their background goroutines start. It is a test-only seam
	// (unexported, so it cannot be set from outside package daemon) that lets
	// tests push AIRequest rows straight into the real *aiproxy.Ingester the
	// daemon manages, and read the store back, to exercise the shutdown-drain
	// path end to end.
	wireHook     func(ing *aiproxy.Ingester, aiStore *aiproxy.Store)
	schedulerRun func(context.Context, *schedule.Scheduler)

	// UserPathResolver optionally overrides login-shell PATH discovery. Embedders
	// and in-process tests may inject a deterministic resolver; nil preserves the
	// normal production behavior and uses userpath.Resolve.
	UserPathResolver UserPathResolver
}

type workflowQuestionReconciler interface {
	ReconcileWorkflowQuestions(context.Context) (int, error)
}

type workflowObservationReconciler interface {
	ReconcileWorkflowObservations(context.Context, int) (int, error)
}

// workflowIngressSignal is a level-triggered, coalescing wakeup. Publish never
// waits for Tasks/SQLite work: durable bus sequence is the actual queue.
type workflowIngressSignal struct{ ch chan struct{} }

func newWorkflowIngressSignal() *workflowIngressSignal {
	return &workflowIngressSignal{ch: make(chan struct{}, 1)}
}

func (s *workflowIngressSignal) Signal() {
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

func (s *workflowIngressSignal) C() <-chan struct{} { return s.ch }

const workflowQuestionReconcileInterval = time.Minute
const workflowObservationReconcileInterval = 5 * time.Second

func runWorkflowObservationReconciler(ctx context.Context, reconciler workflowObservationReconciler, signals <-chan struct{}, interval time.Duration, log *slog.Logger) {
	reconcile := func() {
		for ctx.Err() == nil {
			n, err := reconciler.ReconcileWorkflowObservations(ctx, 100)
			if err != nil {
				if ctx.Err() == nil {
					log.Warn("workflow observation reconciliation", "err", err)
				}
				return
			}
			if n < 100 {
				return
			}
		}
	}
	reconcile()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			reconcile()
		case <-ticker.C:
			reconcile()
		}
	}
}

func runWorkflowQuestionReconciler(ctx context.Context, reconciler workflowQuestionReconciler, interval time.Duration, log *slog.Logger) {
	reconcile := func() {
		if _, err := reconciler.ReconcileWorkflowQuestions(ctx); err != nil && ctx.Err() == nil {
			log.Warn("workflow question reconciliation", "err", err)
		}
	}
	reconcile()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func Run(ctx context.Context, o Options) error {
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

	p := paths.New(o.BaseDir)
	if o.BaseDir == "" {
		var err error
		p, err = paths.Resolve(os.Getenv)
		if err != nil {
			return err
		}
	}
	if err := p.EnsureBase(); err != nil {
		return fmt.Errorf("ensure base dir: %w", err)
	}
	if err := storeassets.Ensure(p, version.Version); err != nil {
		return fmt.Errorf("install built-in Store assets: %w", err)
	}
	// Fail loudly at startup if the layout would produce an unbindable socket,
	// instead of surfacing an opaque EINVAL later. Sockets live in the short
	// home-rooted runtime dir, so this only trips on a pathological HOME.
	if err := paths.BindableSocketPath(p.Socket()); err != nil {
		return fmt.Errorf("daemon socket: %w", err)
	}

	// Single-instance guard (approach A): if a live daemon already answers on the
	// socket, refuse to start rather than stomping its socket file. The probe->bind
	// window is microseconds and both racers would be our own daemon. Only for the
	// unix listener; the pidfile (written after the guard passes, removed on clean
	// exit) is read by `daemon stop`/`status`, not used as the guard.
	if strings.HasPrefix(o.Listen, "unix") {
		if c, derr := net.DialTimeout("unix", p.Socket(), 200*time.Millisecond); derr == nil {
			c.Close()
			return fmt.Errorf("tariboyd already running (socket %s is live)", p.Socket())
		}
		if werr := os.WriteFile(p.PidFile(), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); werr != nil {
			return fmt.Errorf("write pidfile: %w", werr)
		}
		defer os.Remove(p.PidFile())
	}

	listen, err := api.ParseListen(o.Listen, o.AuthTokenFile)
	if err != nil {
		return err
	}

	st, err := store.Open(p.DB())
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

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
		return fmt.Errorf("telemetry setup: %w", err)
	}
	// Force-flush buffered spans/metrics on the way out. Registered early so it
	// runs late in the LIFO defer chain, after the span/metric producers (manager,
	// plugin host, proxy) have stopped. OTLP off -> Shutdown is a cheap no-op.
	defer func() {
		fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(fctx)
	}()
	metrics, err := telemetry.NewMetrics(otel.Meter("tariboy"))
	if err != nil {
		return fmt.Errorf("telemetry metrics: %w", err)
	}

	as := agent.NewStore(st)
	channelBus := bus.New(st, time.Now)
	if err := reconcileAgentInboxes(as, channelBus); err != nil {
		return fmt.Errorf("reconcile agent inboxes: %w", err)
	}
	taskService := tasks.NewService(st.DB, daemonCustomerLogin(), time.Now)
	workflowIngress := newWorkflowIngressSignal()
	taskHub := tasks.NewHub(taskService)
	taskService.SetHub(taskHub)
	taskPublisher := tasknotify.New(st.DB, channelBus, time.Now, log)
	taskReminder := taskreminder.NewReconciler(taskreminder.ReconcilerConfig{
		Store: st, Bus: channelBus, Clock: time.Now, Log: log,
	})
	imgStore := &image.Store{Dir: p.ImagesDir()}
	if err := image.WithPublicationGate(func() error {
		return imgStore.RecoverMutablePublications((imageprovenance.Store{DB: st.DB}).IsCommitted)
	}); err != nil {
		return fmt.Errorf("recover image publications: %w", err)
	}
	if err := image.EnsureBare(imgStore, time.Now); err != nil {
		log.Error("seed bare image", "err", err)
	}
	if err := builtinimages.EnsureBasic(imgStore, log); err != nil {
		log.Error("install builtin basic image", "err", err)
	}
	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	shimBin := firstNonEmpty(os.Getenv("TARIBOY_SHIM_BIN"), filepath.Join(exeDir, "tariboy-shim"))
	skillsDir := filepath.Join(p.CurrentVersionStoreDir(version.Version), "skills")

	schedStore := schedule.NewStore(st, time.Now)
	scriptStore := script.NewStore(st, time.Now)
	scriptPublisher := scriptnotify.New(st.DB, channelBus, time.Now, log)

	groupProv := groups.NewProvisioner(groups.ProvisionerConfig{
		Groups:    groups.NewStore(st, time.Now),
		Agents:    as,
		Bus:       channelBus,
		GroupsDir: filepath.Join(p.Base, "groups"),
		Clock:     time.Now,
	})
	var _ loop.GroupProvisioner = groupProv

	hub := events.NewHub()

	// Per-agent audit log (audit.jsonl). One shared *audit.Log per agent backs
	// recordEvent, the loop engine's lifecycle sink, and the runner's log tailer.
	auditReg := audit.NewRegistry(
		func(a string) string { return agentdir.New(p.AgentsDir(), a).AuditLog() },
		time.Now,
	)

	// LLM-as-Judge survives daemon restarts: the runner recovers durable runs on
	// startup and is drained before SQLite closes during shutdown.
	judgeStore := judge.NewStore(st, time.Now)
	improvementStore := improvement.NewStore(st, time.Now)
	improvementService := improvement.NewService(improvementStore, channelBus)
	judgeRunner := judge.NewRunner(judge.RunnerConfig{
		Store:       judgeStore,
		Snapshotter: judge.NewSnapshotter(judge.SnapshotConfig{Store: judgeStore, BaseDir: p.Base, AgentsDir: p.AgentsDir()}),
		Bus:         channelBus,
	})
	judgeAutomation := judge.NewAutomationService(judgeStore, schedStore, judge.AutomationValidator{
		Customer: daemonCustomerLogin(),
		AgentExists: func(_ context.Context, name string) bool {
			_, err := as.Get(name)
			return err == nil
		},
		ImagePlugins: func(refText string) ([]string, error) {
			ref, err := image.ParseRef(refText)
			if err != nil {
				return nil, err
			}
			manifest, err := imgStore.Inspect(ref)
			plugins := make([]string, len(manifest.Plugins))
			for i, plugin := range manifest.Plugins {
				plugins[i] = plugin.Name
			}
			return plugins, err
		},
		ImageDigest: func(refText string) (string, error) {
			ref, err := image.ParseRef(refText)
			if err != nil {
				return "", err
			}
			manifest, err := imgStore.Inspect(ref)
			return manifest.Digest, err
		},
		TargetImageUsed: func(ctx context.Context, names []string, ref string) bool {
			if len(names) == 0 {
				return false
			}
			args := make([]any, 0, len(names)+1)
			args = append(args, ref)
			marks := make([]string, len(names))
			for i, name := range names {
				marks[i], args = "?", append(args, name)
			}
			var count int
			err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM iterations WHERE image_ref=? AND agent IN (`+strings.Join(marks, ",")+`)`, args...).Scan(&count)
			return err == nil && count > 0
		},
	}, time.Now)
	judgeAutomation.ConfigureExecution(taskService, judgeRunner.Enqueue)
	judgeRunner.SetFailureCallback(func(ctx context.Context, runID string, err error) {
		_ = judgeAutomation.Fail(ctx, runID, err)
	})
	judgeService := judge.NewService(judge.ServiceConfig{
		Store: judgeStore, Agents: as, Groups: groups.NewStore(st, time.Now), Bus: channelBus,
		Evidence: judge.NewEvidenceReader(p.Base), Enqueue: judgeRunner.Enqueue, Improvements: improvementStore, Automation: judgeAutomation,
		Audit: func(agent, kind, iteration string, data map[string]any) {
			auditReg.For(agent).Record(kind, "system", iteration, data)
		},
	})

	// AI proxy (spec §9): load pricing/router, build the token
	// registry, ingester and budget cache, then the proxy itself.
	//
	// Pricing and the router are read once here from the "proxy" daemon-config
	// key and env. A runtime `daemon config set proxy '<json>'` is therefore
	// picked up on the next daemon start, not hot-reloaded (hot-reload is a
	// follow-up; the read path is lock-guarded but reload plumbing is out of M5).
	if err := aiproxy.SeedDefaults(st); err != nil {
		return fmt.Errorf("seed pricing: %w", err)
	}
	pricing, err := aiproxy.LoadPricing(st, aiproxy.DefaultPricing())
	if err != nil {
		return fmt.Errorf("load pricing: %w", err)
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
			log.LogAttrs(context.Background(), levelForPricing(d.Kind), d.Kind, safePricingAttrs(fields)...)
			payload, _ := json.Marshal(fields)
			recordEvent(log, st, auditReg, hub, "", d.Kind, string(payload))
		},
	})
	// Cache reads are local and strictly size-bounded. A rejected cache is
	// diagnostic-only: fallback/manual pricing and proxy startup remain live.
	_ = pricingCatalog.LoadCache(ctx)
	router, err := aiproxy.LoadRouter(st, os.Getenv)
	if err != nil {
		return fmt.Errorf("load router: %w", err)
	}
	aiStore := aiproxy.NewStore(st, time.Now)
	ingester := aiproxy.NewIngester(aiStore, log)
	budgetCache := aiproxy.NewBudgetCache(aiStore, time.Now)
	_ = budgetCache.Refresh()
	policyCache := aiproxy.NewPolicyCache(aiStore, time.Now)
	_ = policyCache.Refresh()
	if o.wireHook != nil {
		o.wireHook(ingester, aiStore)
	}

	proxyTokens, err := aiproxy.OpenTokenRegistry(p.ProxyHandoffFile(), nil)
	if err != nil {
		return err
	}
	proxy := aiproxy.New(aiproxy.Config{
		Tokens: proxyTokens, Pricing: pricing, Store: aiStore, Router: router,
		AgentsDir: p.AgentsDir(), Clock: time.Now, Log: log,
		Ingest:        ingester.Enqueue,
		GroupSnapshot: aiStore.CurrentGroup,
		Budget:        budgetCache,
		Policy:        policyCache,
		Emit: func(agent string, data map[string]any) {
			hub.Emit(events.Event{Agent: agent, Type: "proxy",
				Time: time.Now().UTC().Format(time.RFC3339), Data: data})
			// Record per-request token/cost/latency metrics. The emit closure is
			// called in-process with native Go types (emitProxyEvent), so int/
			// float64/string assertions hold; a mismatch yields a zero reading,
			// never a panic.
			in, _ := data["input_tokens"].(int)
			out, _ := data["output_tokens"].(int)
			cost, _ := data["cost_usd"].(float64)
			lat, _ := data["latency_ms"].(int)
			status, _ := data["status"].(string)
			metrics.RecordProxyRequest(context.Background(), status, float64(lat), in, out, cost)
		},
		Warn: func(agent string, d aiproxy.Decision) {
			_, _ = channelBus.Publish(bus.Message{Channel: bus.InboxChannel(agent), Type: "budget.warn",
				Source: "proxy", ProducedByAgent: agent,
				Text: "AI budget warning: spend exceeded limit",
				Data: map[string]any{"scope": d.Scope, "limit_usd": d.LimitUSD, "spent_usd": d.SpentUSD}})
			recordEvent(log, st, auditReg, hub, agent, "budget_warn",
				fmt.Sprintf(`{"scope":%q,"spent_usd":%g}`, d.Scope, d.SpentUSD))
		},
		Audit: func(agent, kind, dataJSON string) { recordEvent(log, st, auditReg, hub, agent, kind, dataJSON) },
	})
	proxyAddr, err := listenProxyWithHandoff(as, proxy, proxyTokens)
	if err != nil {
		return fmt.Errorf("proxy listen: %w", err)
	}
	log.Info("ai proxy listening", "addr", proxyAddr)

	// Gzip the AI-proxy transcript at iteration close (spec §9/§12). loop stays
	// free of an aiproxy import (see loop.ProxyBinder), so this is wired here as
	// a closure over the daemon's AgentsDir. GzipTranscript is a clean no-op when
	// the plain transcript is missing, so this is harmless for iterations that
	// never made a proxied AI call; guarded on proxy != nil anyway since with no
	// proxy configured there is never a transcript to compress.
	var onIterationClose func(agent, iterationID string)
	if proxy != nil {
		onIterationClose = func(agent, iterationID string) {
			if err := aiproxy.GzipTranscript(p.AgentsDir(), agent, iterationID); err != nil {
				log.Warn("gzip proxy transcript", "agent", agent, "iteration", iterationID, "err", err)
			}
		}
	}

	// External plugin host (spec §7): the daemon owns plugin lifecycle. One
	// TokenRegistry is shared between the host (which mints per-plugin tokens)
	// and the plugin API (which resolves them). Enabled plugins are (re)started
	// after the manager is up; StopAll drains the sink outbox before the store
	// closes on the way out.
	pluginTokens := plugins.NewTokenRegistry(nil)
	pluginStore := plugins.NewStore(st, time.Now)
	pluginHost := plugins.NewHost(plugins.HostConfig{
		Store: pluginStore, Bus: channelBus, Tokens: pluginTokens,
		PluginsDir: p.PluginsDir(), DaemonSocket: p.Socket(),
		Clock: time.Now, After: time.After, Log: log,
	})
	pluginAPI := plugins.NewAPI(pluginTokens, channelBus, log, func(plugin, action, detail string) {
		recordEvent(log, st, auditReg, hub, "", "plugin_"+action,
			fmt.Sprintf(`{"plugin":%q,"detail":%q}`, plugin, detail))
	})

	// Provider contract (spec §6.1): gate parameterized subscribes on the target
	// channel's declared params_schema. Records are read fresh per subscribe (an
	// infrequent op) so newly installed providers take effect without a restart. A
	// transient store-read failure treats the channel as non-provider and allows
	// the subscribe (ParamsValidatorFor) rather than failing every subscribe —
	// core channels never needed the plugin store.
	channelBus.SetParamsValidator(plugins.ParamsValidatorFor(pluginStore,
		func(channel string, err error) {
			log.Warn("params validator: list plugins", "channel", channel, "err", err)
		}))

	// Provider watch lifecycle (spec §6.2): after any subscribe/unsubscribe, map
	// the affected channel to its provider plugin and push the channel's full
	// current watch list to that plugin's socket. Non-provider channels are a
	// no-op. The push is best-effort (async, capped backoff inside the host); the
	// plugin's GET /api/plugin/watches pull path guarantees eventual consistency.
	channelBus.SetSubscriptionHook(func(channel string) {
		recs, err := pluginStore.List()
		if err != nil {
			log.Warn("subscription hook: list plugins", "channel", channel, "err", err)
			return
		}
		owner, _, ok := plugins.ProviderFor(recs, channel)
		if !ok {
			return // not a provided channel — no provider to notify
		}
		watches, err := channelBus.WatchList(channel)
		if err != nil {
			log.Warn("subscription hook: watch list", "channel", channel, "err", err)
			return
		}
		pluginHost.PushWatches(owner, channel, watches)
	})

	// Request deadlines (spec §4.2): a Request with a --deadline arms a one-shot
	// schedule that publishes a type=timeout event into the requester's inbox at
	// the deadline; a reply landing first cancels exactly that entry by its
	// correlation id. This wires the bus's deadline seam to the schedule
	// subsystem so cross-agent requests from the Messages skill honour
	// their deadlines instead of failing ErrDeadlineUnsupported.
	// parseDeadline is the single source of truth for the deadline format,
	// shared by the pre-publish validator and the arm hook so both accept exactly
	// the same strings (Go durations, e.g. 5m/3s/1h).
	parseDeadline := func(deadline string) (time.Duration, error) {
		dur, err := time.ParseDuration(deadline)
		if err != nil {
			return 0, fmt.Errorf("request deadline %q: %w", deadline, err)
		}
		return dur, nil
	}
	channelBus.SetDeadlineValidator(func(deadline string) error {
		_, err := parseDeadline(deadline)
		return err
	})
	channelBus.SetDeadlineHooks(
		func(tx *sql.Tx, agent, correlationID, deadline string) error {
			dur, err := parseDeadline(deadline)
			if err != nil {
				return err
			}
			fireAt := time.Now().UTC().Add(dur).Format(time.RFC3339)
			tmpl, err := json.Marshal(map[string]any{
				"type": "timeout",
				"text": "request timed out with no reply",
				"data": map[string]any{"correlation_id": correlationID},
			})
			if err != nil {
				return err
			}
			_, err = schedStore.AddTx(tx, schedule.Schedule{
				Agent:           agent,
				Kind:            "oneshot",
				Spec:            fireAt,
				Channel:         bus.InboxChannel(agent),
				MessageTemplate: string(tmpl),
				CorrelationID:   correlationID,
			})
			return err
		},
		func(tx *sql.Tx, correlationID string) error {
			return schedStore.CancelByCorrelationTx(tx, correlationID)
		},
	)

	// Retention (spec §12): a background goroutine periodically prunes every
	// agent's iterations per its policy. Drained before st.Close (see the wg
	// block below) since the pruner touches the store.
	retPolicies := retention.NewStore(st)
	retPruner := retention.NewPruner(st, as, retPolicies, p.AgentsDir(), time.Now, log)
	retRunner := retention.NewRunner(retPruner, time.Hour, time.After, log)

	// Eval runner (spec §7.3/§8): an ingester-style worker that, after each
	// iteration, dispatches the image's declared evals to the eval plugin and
	// persists a verdict keyed by iteration+image_digest+eval_name. For llm-judge
	// it mints a per-eval proxy token (via proxy) so the AI call is accounted and
	// the real key stays server-side. RunEvals is non-blocking; Run drains the
	// queue on cancel (drained before st.Close via the wg block below).
	evalRunner := evals.NewRunner(evals.RunnerConfig{
		Store:     evals.NewStore(st, time.Now),
		ImgStore:  imgStore,
		Plugins:   pluginHost,
		Minter:    proxy,
		AgentsDir: p.AgentsDir(),
		Clock:     time.Now,
		Log:       log,
	})
	var _ loop.EvalRunner = evalRunner

	manager := loop.NewManager(loop.ManagerConfig{
		AgentsDir: p.AgentsDir(), RuntimeDir: p.RuntimeDir(), SkillsDir: skillsDir, ShimBin: shimBin,
		ImgStore: imgStore, Store: as, Log: log, Clock: time.Now, Bus: channelBus,
		Schedules: schedStore, Scripts: scriptStore, ScriptResults: scriptPublisher, Emit: hub.Emit, Proxy: proxy,
		Groups:          groupProv,
		Evals:           evalRunner,
		Tasks:           taskService,
		ExternalPlugins: plugins.ResolveEnabledInstalledMetadata(p.PluginsDir(), pluginStore),
		Spawner:         o.Spawner, OnIterationClose: onIterationClose,
		// ProvidedChannels feeds the Messages skill provider-declared channels
		// from installed plugin manifests (spec §6.1), read fresh per call so a
		// newly installed provider is annotated without a restart.
		ProvidedChannels: func() ([]agentapi.ProvidedChannel, error) {
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
		},
		JudgeAction: func(agent, iteration, action string, body map[string]any) (map[string]any, error) {
			return judgeService.AgentAction(context.Background(), agent, iteration, action, body)
		},
		AuditFor: func(a string) loop.Recorder { return auditReg.For(a) },
		Metrics:  metrics,
		// UsageLookup adapts aiStore.IterationUsage (which returns an error) into
		// the error-less accessor the loop expects: swallow the error and return
		// zeros so a lookup failure never propagates into an iteration.
		UsageLookup: func(iter string) (int, int, float64) {
			in, out, cost, _ := aiStore.IterationUsage(iter)
			return in, out, cost
		},
	})
	judgeAutomation.SetActivator(func(names []string) error {
		for _, name := range names {
			if err := manager.Start(name); err != nil {
				return err
			}
		}
		return nil
	})

	// Register the observable gauges exactly once (spec §14): bus queue depth,
	// healthy plugins, active agent loops. No-op instruments when OTel is off.
	_ = metrics.RegisterGauges(telemetry.GaugeSource{
		QueueDepth: func(context.Context) int64 {
			n, _ := channelBus.PendingTotal()
			return int64(n)
		},
		PluginsHealthy: func(context.Context) int64 {
			list, _ := pluginHost.List()
			var n int64
			for _, pl := range list {
				if s, _ := pl["state"].(string); s == "running" {
					n++
				}
			}
			return n
		},
		ActiveAgents: func(context.Context) int64 { return int64(manager.ActiveAgents()) },
	})

	// A publish wakes exactly the agents that received a delivery and feeds the
	// SSE hub with per-agent message/stream events.
	channelBus.SetPublishHook(func(msg bus.Message, agents []string) {
		manager.WakeAgents(agents, loop.WakeMessage)
		emitMessageEvent(hub, msg, agents)
		if taskService.WorkflowIngressEnabled() {
			workflowIngress.Signal()
		}
	})

	// Route bus lifecycle events (message_processed/replied/requeued) to the
	// per-agent audit.jsonl so the §8.2 operator-vs-agent attribution actually
	// lands in the audit timeline. recordEvent has the matching signature.
	channelBus.SetAuditHook(func(agent, kind, dataJSON string) {
		recordEvent(log, st, auditReg, hub, agent, kind, dataJSON)
	})

	cctx := &registry.Ctx{
		Store: st, Log: log, BaseDir: p.Base, Socket: p.Socket(), HTTPAddr: o.HTTPAddr,
		Version: version.Version, StartedAt: time.Now(), Control: manager, Scripts: manager, Bus: channelBus, Plugins: pluginHost,
		Groups:          groupProv,
		Judges:          judgeService,
		JudgeAutomation: judgeAutomation,
		Improvements:    improvementService,
		Operator:        taskService.CustomerLogin(),
		Retention:       &retention.RetentionAPI{Policies: retPolicies, Pruner: retPruner},
		Policy:          policyCache,
		Tasks:           taskService,
	}
	srv := api.NewServer(commands.BuildRegistry(), cctx)
	srv.SetEventSource(hub)
	srv.SetTasks(taskHub, func() tasks.Actor {
		return tasks.CustomerActor(taskService.CustomerLogin())
	})
	srv.SetPluginAPI(pluginAPI)
	srv.SetExternalPlugins(plugins.ResolveInstalled(p.PluginsDir()))
	srv.SetSupportBundleSource(supportbundle.Collector{
		Store: st, Control: manager, BaseDir: p.Base, LogFile: p.LogFile(),
		Version: version.Version, Now: time.Now, Environ: os.Environ,
	})

	if o.HTTPAddr != "" {
		host, _, err := net.SplitHostPort(o.HTTPAddr)
		if err != nil {
			return fmt.Errorf("bad --http-addr %q: %w", o.HTTPAddr, err)
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("--http-addr %q must be a loopback address (localhost-only listener)", o.HTTPAddr)
		}
	}

	if err := manager.StartAll(ctx); err != nil {
		log.Error("reattach failed", "err", err)
	}
	defer manager.Shutdown()

	if err := pluginHost.StartAll(ctx); err != nil {
		log.Error("plugin host start", "err", err)
	}
	if executable, err := os.Executable(); err == nil {
		telegramExecutable := filepath.Join(filepath.Dir(executable), "tariboy-plugin-telegram")
		if _, err := pluginHost.EnsureBundled(telegramExecutable, telegramplugin.Manifest(version.Version)); err != nil {
			log.Error("install bundled telegram plugin", "err", err)
		}
	} else {
		log.Warn("resolve daemon executable for bundled plugins", "err", err)
	}
	// StopAll cancels every supervisor + sink drainer and waits (drain-before-
	// close): registered after manager.Shutdown so, by LIFO, plugins stop and
	// their outbox drains before the store closes (spec §13).
	defer pluginHost.StopAll()

	// Serve the proxy, drain the ingest queue and periodically refresh budget
	// aggregates on the daemon context. Per-iteration proxy tokens are minted and
	// revoked by the runner; the daemon only owns the listener's lifecycle.
	go func() {
		if err := proxy.Serve(); err != nil {
			log.Error("ai proxy serve", "err", err)
		}
	}()
	// The ingester and budget-cache refresher both touch the store (InsertBatch /
	// Refresh reads). They must finish draining before st.Close() runs on the way
	// out, or the ingester's final flush can race db.Close() and lose the last
	// buffered batch of ai_requests (spec §13). gctx/cancel is a daemon-owned
	// derivation of ctx so both goroutines are guaranteed to observe cancellation
	// and exit on every return path of Run (not just the ctx.Done() shutdown
	// branch below), and wg lets Run block until they have actually finished
	// their final flush/refresh before the store closes.
	gctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(12)
	scheduler := schedule.NewScheduler(schedStore, channelBus, log, time.Now, time.After)
	go func() {
		defer wg.Done()
		if o.schedulerRun != nil {
			o.schedulerRun(gctx, scheduler)
			return
		}
		scheduler.Run(gctx)
	}()
	go func() {
		defer wg.Done()
		pricingCatalog.Run(gctx)
	}()
	go func() {
		defer wg.Done()
		ingester.Run(gctx)
	}()
	// Eval worker: drains queued post-iteration evals. cancel()+wg.Wait() (below,
	// LIFO before pluginHost.StopAll and st.Close) lets it best-effort drain its
	// queue while the plugin host and store are still live; an llm-judge queued at
	// the instant of shutdown records an "error" verdict if the proxy is gone.
	go func() {
		defer wg.Done()
		evalRunner.Run(gctx)
	}()
	go func() {
		defer wg.Done()
		judgeRunner.Run(gctx)
	}()
	go func() {
		defer wg.Done()
		taskPublisher.Run(gctx)
	}()
	go func() {
		defer wg.Done()
		taskReminder.Run(gctx)
	}()
	go func() {
		defer wg.Done()
		scriptPublisher.Run(gctx)
	}()
	go func() {
		defer wg.Done()
		runWorkflowQuestionReconciler(gctx, taskService, workflowQuestionReconcileInterval, log)
	}()
	go func() {
		defer wg.Done()
		runWorkflowObservationReconciler(gctx, taskService, workflowIngress.C(), workflowObservationReconcileInterval, log)
	}()
	go func() {
		defer wg.Done()
		tk := time.NewTicker(15 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-gctx.Done():
				return
			case <-tk.C:
				if err := budgetCache.Refresh(); err != nil {
					log.Warn("budget cache refresh", "err", err)
				}
				if err := policyCache.Refresh(); err != nil {
					log.Warn("policy cache refresh", "err", err)
				}
			}
		}
	}()
	// The retention runner also touches the store (PruneAll deletes iteration/
	// ai_requests rows). It uses gctx like the goroutines above so an in-flight
	// prune finishes before st.Close() on the way out.
	go func() {
		defer wg.Done()
		retRunner.Run(gctx)
	}()
	// Defers run LIFO. Registration order top-to-bottom is:
	//   st.Close (top of Run) -> manager.Shutdown -> [cancel+wg.Wait] -> proxy.Shutdown
	// so execution order (bottom-to-top) is:
	//   proxy.Shutdown -> cancel+wg.Wait -> manager.Shutdown -> st.Close
	// i.e. the proxy HTTP listener drains first, then the ingester/refresher
	// goroutines are cancelled and awaited (their final flush/refresh lands),
	// then the loop manager, and only then does the store close.
	defer func() {
		cancel()
		wg.Wait()
	}()
	defer func() {
		shctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Shutdown(shctx)
	}()

	recordEvent(log, st, auditReg, hub, "", "daemon_start", fmt.Sprintf(`{"pid":%d,"version":%q}`, os.Getpid(), version.Version))
	log.Info("tariboyd starting", "version", version.Version, "base", p.Base, "listen", o.Listen)

	errc := make(chan error, 2)
	go func() {
		if listen.Network == "tcp" {
			errc <- srv.ServeTCP(listen.Addr, listen.AuthToken)
			return
		}
		sock := listen.Addr
		if sock == "" {
			sock = p.Socket()
		}
		errc <- srv.ServeUnix(sock)
	}()

	if o.HTTPAddr != "" {
		go func() {
			if err := srv.ServeWeb(o.HTTPAddr); err != nil {
				errc <- fmt.Errorf("http listener serve: %w", err)
			}
		}()
		log.Info("http api listening", "addr", o.HTTPAddr)
	}

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		recordEvent(log, st, auditReg, hub, "", "daemon_stop", "{}")
		log.Info("tariboyd stopping")
		if err := srv.Shutdown(shctx); err != nil {
			return err
		}
		return <-errc
	}
}

const maxPricingDiagnosticSourceBytes = 256

func levelForPricing(kind string) slog.Level {
	if kind == pricingcatalog.DiagnosticError {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func safePricingFields(d pricingcatalog.Diagnostic) map[string]any {
	fields := map[string]any{
		"source":          boundedPricingSource(d.Source),
		"generation_time": d.At.UTC().Format(time.RFC3339Nano),
	}
	if d.AcceptedModels > 0 {
		fields["model_count"] = d.AcceptedModels
	}
	if d.Err != nil {
		fields["error_class"] = pricingErrorClass(d)
	}
	return fields
}

func safePricingAttrs(fields map[string]any) []slog.Attr {
	attrs := make([]slog.Attr, 0, 4)
	for _, key := range []string{"source", "model_count", "generation_time", "error_class"} {
		if value, ok := fields[key]; ok {
			attrs = append(attrs, slog.Any(key, value))
		}
	}
	return attrs
}

func boundedPricingSource(source string) string {
	if len(source) <= maxPricingDiagnosticSourceBytes {
		return source
	}
	return source[:maxPricingDiagnosticSourceBytes]
}

func pricingErrorClass(d pricingcatalog.Diagnostic) string {
	if errors.Is(d.Err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(d.Err, context.DeadlineExceeded) {
		return "timeout"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(d.Err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	switch d.Source {
	case "cache":
		return "cache"
	case "database":
		return "database"
	case "download":
		return "download"
	default:
		return "other"
	}
}

func reconcileAgentInboxes(agents *agent.Store, channelBus *bus.Bus) error {
	rows, err := agents.List()
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := channelBus.Subscribe(row.Name, bus.InboxChannel(row.Name), bus.Matcher{}, nil); err != nil {
			return fmt.Errorf("reconcile inbox for agent %q: %w", row.Name, err)
		}
	}
	return nil
}

// UserPathResolver resolves the effective PATH for a login shell at startup.
// A nil Options.UserPathResolver selects the platform resolver.
type UserPathResolver func(context.Context, string) (string, error)

const userPathFallbackWarning = "resolve user PATH: keeping inherited PATH"

// shellEnvMarker is set by a launcher that already started tariboyd inside
// the account's login shell (Desktop does; a terminal or SSH launch does not).
const shellEnvMarker = "TARIBOY_SHELL_ENV"

func applyUserPath(ctx context.Context, log *slog.Logger, resolve UserPathResolver) {
	// Desktop launches tariboyd through the account's login shell, so the
	// whole environment — PATH included — is already the one a terminal gets.
	// Running the probe on top would re-execute the account's startup files for
	// nothing, and a probe that failed could only make the result worse.
	if os.Getenv(shellEnvMarker) != "" {
		log.Info("user PATH inherited from the login-shell launcher")
		return
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		log.Warn(userPathFallbackWarning, "reason", "no_shell")
		return
	}

	resolved, err := resolve(ctx, shell)
	if err != nil {
		log.Warn(userPathFallbackWarning, "reason", userpath.Reason(err))
		return
	}
	if err := os.Setenv("PATH", resolved); err != nil {
		log.Warn(userPathFallbackWarning, "reason", "not_applied")
	}
}

func listenProxyWithHandoff(
	agents *agent.Store,
	proxy *aiproxy.Proxy,
	tokens *aiproxy.TokenRegistry,
) (string, error) {
	if err := tokens.Prune(func(attr aiproxy.Attribution) bool {
		it, err := agents.GetIteration(attr.Agent, attr.Iteration)
		return err == nil && it.Status == "running"
	}); err != nil {
		return "", fmt.Errorf("prune AI proxy handoff leases: %w", err)
	}

	saved := tokens.ListenAddr()
	addr, err := proxy.ListenAt(saved)
	if err != nil && saved != "" && tokens.Count() == 0 {
		addr, err = proxy.ListenAt("")
	}
	if err != nil {
		if saved != "" && tokens.Count() > 0 {
			return "", fmt.Errorf("bind carried AI proxy address %s with active leases: %w", saved, err)
		}
		return "", err
	}
	if err := tokens.SetListenAddr(addr); err != nil {
		_ = proxy.CloseListener()
		return "", fmt.Errorf("persist AI proxy listen address: %w", err)
	}
	return addr, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func daemonCustomerLogin() string {
	if current, err := user.Current(); err == nil {
		if login := strings.TrimSpace(current.Username); login != "" {
			if i := strings.LastIndexAny(login, `\/`); i >= 0 {
				login = login[i+1:]
			}
			if login != "" {
				return login
			}
		}
	}
	for _, key := range []string{"LOGNAME", "USER"} {
		if login := strings.TrimSpace(os.Getenv(key)); login != "" {
			return login
		}
	}
	return "customer"
}

func recordEvent(log *slog.Logger, st *store.Store, reg *audit.Registry, hub *events.Hub, agent, kind, data string) {
	if agent != "" && reg != nil {
		// Agent-scoped events go to the durable per-agent audit.jsonl.
		var m map[string]any
		if data != "" && data != "{}" {
			if err := json.Unmarshal([]byte(data), &m); err != nil {
				m = map[string]any{"data": data}
			}
		}
		reg.For(agent).Record(kind, "system", "", m)
	} else if err := st.AddEvent(agent, kind, data); err != nil {
		// Agentless (global) events — daemon_start/stop, plugin_* — stay in the DB.
		log.Warn("failed to record daemon event", "kind", kind, "err", err)
	}
	// Agent-scoped audit events also stream to any SSE subscribers.
	if hub != nil && agent != "" {
		hub.Emit(events.Event{Agent: agent, Type: "audit", Time: time.Now().UTC().Format(time.RFC3339),
			Data: map[string]any{"kind": kind, "data": data}})
	}
}

// emitMessageEvent turns a bus publish into per-agent SSE events. An agent's own
// stream/inbox channel emits for that agent; other channels emit for each
// delivered-to agent.
func emitMessageEvent(hub *events.Hub, msg bus.Message, agents []string) {
	typ := "message"
	if strings.HasSuffix(msg.Channel, ":stream") {
		typ = "stream"
	}
	targets := agents
	// An agent's own inbox/stream always concerns that agent even with no sub.
	if a, ok := ownerOfChannel(msg.Channel); ok {
		targets = appendUnique(targets, a)
	}
	for _, a := range targets {
		hub.Emit(events.Event{Agent: a, Type: typ, Time: msg.TS,
			Data: map[string]any{"id": msg.ID, "channel": msg.Channel, "type": msg.Type,
				"source": msg.Source, "text": msg.Text}})
	}
}

func ownerOfChannel(channel string) (string, bool) {
	if !strings.HasPrefix(channel, "agent:") {
		return "", false
	}
	rest := strings.TrimPrefix(channel, "agent:")
	if i := strings.LastIndex(rest, ":"); i > 0 {
		return rest[:i], true
	}
	return "", false
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
