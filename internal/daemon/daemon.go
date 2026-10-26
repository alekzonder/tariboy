// Package daemon wires store + registry + api server into tariboyd.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/loop"
	"github.com/alekzonder/tariboy/internal/pricingcatalog"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/userpath"
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

	// Ready, if set, is called once every listener is bound and the workers
	// run: a client connecting after Ready reaches the API. In-process tests
	// wait on it instead of polling the socket.
	Ready func()

	// onShutdownStep and serveHook are test-only seams: onShutdownStep gets the
	// name of each shutdown step before it runs, and serveHook may wrap the
	// serve function of a server ("ai-proxy", "api", "web").
	onShutdownStep func(name string)
	serveHook      func(name string, serve func() error) func() error
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

type workflowDispatcher interface {
	DispatchPending(context.Context) (int, error)
	CheckHolders(context.Context, time.Time) (int, error)
}

const workflowDispatchInterval = time.Minute

type goalHooks interface {
	Signal()
	IterationCompleted(agentName, iterationID string)
}

// withWorkflowDispatch wraps the goal reconciler's hooks so each also wakes the
// workflow dispatcher, after the goal call: IterationCompleted reconciles
// synchronously, so the dispatch pass then sees the agent's Goal cleared.
func withWorkflowDispatch(goal goalHooks, dispatch func()) (signal func(), iterationCompleted func(string, string)) {
	return func() {
			goal.Signal()
			dispatch()
		}, func(agentName, iterationID string) {
			goal.IterationCompleted(agentName, iterationID)
			dispatch()
		}
}

// runWorkflowDispatcher assigns pool work to workflow tasks at startup, on each
// task change signal, and on the interval, until ctx ends. After each dispatch
// pass it checks that the holders of active pool statuses can work.
func runWorkflowDispatcher(ctx context.Context, dispatcher workflowDispatcher, signals <-chan struct{}, interval time.Duration, log *slog.Logger) {
	dispatch := func() {
		if _, err := dispatcher.DispatchPending(ctx); err != nil && ctx.Err() == nil {
			log.Warn("workflow dispatch", "err", err)
		}
		if _, err := dispatcher.CheckHolders(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Warn("workflow holders", "err", err)
		}
	}
	dispatch()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			dispatch()
		case <-ticker.C:
			dispatch()
		}
	}
}

// The access log gets one line per API request, so it rotates by size.
const (
	accessLogMaxBytes = 50 << 20
	accessLogKeep     = 3
)

// Run starts tariboyd and serves until ctx ends or a listener fails. Startup is
// a sequence of phases; each phase pushes the stop of what it creates onto one
// shutdown stack, which runs last-pushed first on every return path.
func Run(ctx context.Context, o Options) error {
	var stack shutdownStack
	var log *slog.Logger
	defer func() { stack.run(log, o.onShutdownStep) }()
	proc, err := setupProcess(ctx, o, &stack)
	if err != nil {
		return err
	}
	log = proc.log
	c, err := openStore(ctx, proc, &stack)
	if err != nil {
		return err
	}
	tk, err := buildTasks(ctx, proc, c)
	if err != nil {
		return err
	}
	px, err := buildProxy(ctx, o, proc, c)
	if err != nil {
		return err
	}
	if err := listenProxy(proc, c, px, &stack); err != nil {
		return err
	}
	pl := buildPlugins(proc, c)
	mg := buildManager(ctx, o, proc, c, tk, px, pl)
	startRuntime(ctx, proc, mg, pl, &stack)
	startWorkers(ctx, o, proc, c, tk, px, mg, &stack)
	return serve(ctx, o, proc, c, tk, px, pl, mg, &stack)
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
		"generation_time": store.FormatTime(d.At),
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

// reconcileAgentChats provisions every persisted agent's chats and their
// participants. It runs beside the inbox reconciliation at startup, so an agent
// created before the chats migration gets its conversations on the next start.
func reconcileAgentChats(agents *agent.Store, channelBus *bus.Bus, customerPrincipal string) error {
	rows, err := agents.List()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return channelBus.ReconcileChats(names, customerPrincipal)
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

// runtimeAliases are the agent commands linked into <runtime>/bin, each to
// its sibling payload.
var runtimeAliases = []struct{ payload, link string }{
	{"tariboy-tasks", "ttasks"},
	{"tariboy-mail", "tmail"},
}

// installTasksAlias exposes the sibling payloads ttasks and tmail to all
// daemon children, including fresh Desktop agents before optional CLI
// installation. The stable runtime path survives daemon handoff; explicit
// agent PATH overrides still win. It returns the runtime bin directory, or ""
// when no payload is present.
func installTasksAlias(exeDir, runtimeDir string) (string, error) {
	installed := ""
	for _, alias := range runtimeAliases {
		binDir, err := installRuntimeAlias(exeDir, runtimeDir, alias.payload, alias.link)
		if err != nil {
			return "", err
		}
		if binDir != "" {
			installed = binDir
		}
	}
	return installed, nil
}

func installRuntimeAlias(exeDir, runtimeDir, payload, name string) (string, error) {
	source := filepath.Join(exeDir, payload)
	info, err := os.Stat(source)
	if os.IsNotExist(err) {
		return "", nil // Direct development builds may contain only tariboyd.
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("payload of %s is not executable", payload)
	}
	binDir := filepath.Join(runtimeDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return "", err
	}
	if info, err := os.Lstat(binDir); err != nil || !info.IsDir() {
		return "", errors.New("runtime bin must be a directory")
	}
	link := filepath.Join(binDir, name)
	if info, err := os.Lstat(link); err == nil && info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("refusing to replace non-symlink %s command", name)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if target, err := os.Readlink(link); err == nil && (!filepath.IsAbs(target) || filepath.Base(target) != payload) {
		return "", fmt.Errorf("refusing to replace foreign %s command symlink", name)
	}
	stage, err := os.MkdirTemp(binDir, "."+name+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	temporary := filepath.Join(stage, name)
	if err := os.Symlink(source, temporary); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, link); err != nil {
		return "", err
	}
	return binDir, nil
}

// ensureLocalBin appends ~/.local/bin, where the managed tariboy, ttasks and tmail
// links live, so agents find them even when the account PATH lacks it.
func ensureLocalBin(home string) {
	dir := filepath.Join(home, ".local", "bin")
	path := os.Getenv("PATH")
	if slices.Contains(filepath.SplitList(path), dir) {
		return
	}
	if path != "" {
		dir = path + string(os.PathListSeparator) + dir
	}
	_ = os.Setenv("PATH", dir) // fails only on invalid names or ENOMEM; PATH is valid
}

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
		return err == nil && it.Status == agent.IterationRunning
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
		err = fmt.Errorf("persist AI proxy listen address: %w", err)
		if closeErr := proxy.CloseListener(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			err = errors.Join(err, fmt.Errorf("close AI proxy listener: %w", closeErr))
		}
		return "", err
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

// emitMessageEvent turns a bus publish into per-agent live events. An agent's own
// stream/inbox channel emits for that agent; other channels emit for each
// delivered-to agent. A message on the customer's own channel has no agent
// recipient at all, so it is attributed to its sending agent instead — that is
// the half of a chat the customer receives.
func emitMessageEvent(hub *events.Hub, msg bus.Message, agents []string) {
	typ := "message"
	if strings.HasSuffix(msg.Channel, ":stream") {
		typ = "stream"
	}
	from := bus.MessageFrom(msg)
	targets := agents
	// An agent's own inbox/stream always concerns that agent even with no sub.
	if a, ok := ownerOfChannel(msg.Channel); ok {
		targets = appendUnique(targets, a)
	}
	chatID, inChat := bus.ChatIDFromChannel(msg.Channel)
	// A message an agent wrote reaches no agent delivery, so without attributing
	// it to its author the half of the conversation the customer receives would
	// never reach a live client.
	if inChat || strings.HasPrefix(msg.Channel, "user:") {
		if sender, ok := strings.CutPrefix(from, "agent:"); ok {
			targets = appendUnique(targets, sender)
		}
	}
	for _, a := range targets {
		data := map[string]any{"id": msg.ID, "channel": msg.Channel, "type": msg.Type,
			"source": msg.Source, "from": from, "text": msg.Text}
		// The chat id is the channel's own suffix, so naming the chat costs no
		// lookup and a non-chat channel carries no chat key at all.
		if inChat {
			data["chat"] = chatID
		}
		hub.Emit(events.Event{Agent: a, Type: typ, Time: msg.TS, Data: data})
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
