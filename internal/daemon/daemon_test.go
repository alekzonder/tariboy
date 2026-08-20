package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/telemetry"
)

type fakeObservationReconciler struct{ calls chan struct{} }

func (f *fakeObservationReconciler) ReconcileWorkflowObservations(context.Context, int) (int, error) {
	select {
	case f.calls <- struct{}{}:
	default:
	}
	return 1, nil
}

func TestWorkflowObservationReconcilerRunsAtStartupPeriodicallyAndStops(t *testing.T) {
	fake := &fakeObservationReconciler{calls: make(chan struct{}, 4)}
	signal := newWorkflowIngressSignal()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runWorkflowObservationReconciler(ctx, fake, signal.C(), 5*time.Millisecond, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		close(done)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-fake.calls:
		case <-time.After(time.Second):
			t.Fatalf("reconcile call %d did not arrive", i+1)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observation reconciler did not stop")
	}
}

func TestWorkflowIngressSignalProcessesPromptlyAndCoalescesWithoutBlockingPublish(t *testing.T) {
	fake := &fakeObservationReconciler{calls: make(chan struct{}, 8)}
	signal := newWorkflowIngressSignal()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runWorkflowObservationReconciler(ctx, fake, signal.C(), time.Hour, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		close(done)
	}()
	select {
	case <-fake.calls:
	case <-time.After(time.Second):
		t.Fatal("startup reconcile missing")
	}
	for range 1000 {
		signal.Signal()
	}
	select {
	case <-fake.calls:
	case <-time.After(time.Second):
		t.Fatal("signaled reconcile was not prompt")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("signaled reconciler leaked on shutdown")
	}
}

func TestOrdinaryPublishDoesNotSynchronouslyWriteWorkflowStateOrChangeLegacyDelivery(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "publish.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	agents := agent.NewStore(st)
	if err := agents.Create(agent.Agent{Name: "legacy"}); err != nil {
		t.Fatal(err)
	}
	b := bus.New(st, time.Now)
	if _, err := b.Subscribe("legacy", "chat:legacy", nil, nil); err != nil {
		t.Fatal(err)
	}
	signal := newWorkflowIngressSignal()
	taskService := tasks.NewService(st.DB, "customer", time.Now)
	b.SetPublishHook(func(bus.Message, []string) {
		if taskService.WorkflowIngressEnabled() {
			signal.Signal()
		}
	})
	first, err := b.Publish(bus.Message{Channel: "chat:legacy", Source: "operator", Type: "note", Text: "unchanged legacy payload"})
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan error, 1)
	go func() {
		_, err := b.Publish(bus.Message{Channel: "chat:legacy", Source: "operator", Type: "note", Text: "second"})
		returned <- err
	}()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("publish blocked on full workflow signal")
	}
	if _, err := taskService.ReconcileWorkflowObservations(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	select {
	case <-signal.C():
		t.Fatal("ordinary publish signaled workflow ingress without targets")
	default:
	}
	var idempotency, observations int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM task_idempotency WHERE actor='system:workflow-observation'`).Scan(&idempotency); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM task_observations`).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if idempotency != 0 || observations != 0 {
		t.Fatalf("workflow writes = idempotency:%d observations:%d", idempotency, observations)
	}
	pending, err := b.Pending("legacy", 10)
	if err != nil || len(pending) != 2 || pending[0].ID != first.ID || pending[0].Text != "unchanged legacy payload" {
		t.Fatalf("legacy pending = %#v, %v", pending, err)
	}
}

// Removing daemon-start reconciliation must make this fail: the legacy agent
// would still have no durable delivery lane after restart.
func TestReconcileAgentInboxesRepairsExistingAgentsIdempotently(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	agents := agent.NewStore(st)
	for _, name := range []string{"legacy", "already"} {
		if err := agents.Create(agent.Agent{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	channelBus := bus.New(st, time.Now)
	if _, err := channelBus.Subscribe("already", bus.InboxChannel("already"), bus.Matcher{}, nil); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := reconcileAgentInboxes(agents, channelBus); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"legacy", "already"} {
		subs, err := channelBus.ListSubscriptions(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(subs) != 1 || subs[0].Channel != bus.InboxChannel(name) || len(subs[0].Matcher) != 0 || len(subs[0].TypeFilter) != 0 {
			t.Fatalf("subscriptions for %s = %+v, want one unfiltered own inbox", name, subs)
		}
	}
}

func TestRecordEventAgentScopedGoesToAuditFile(t *testing.T) {
	agentsDir := t.TempDir()
	reg := audit.NewRegistry(
		func(a string) string { return agentdir.New(agentsDir, a).AuditLog() },
		func() time.Time { return time.Unix(0, 0).UTC() },
	)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	recordEvent(log, nil, reg, nil, "manager", "shim", `{"line":"boom"}`)

	evs, err := audit.ReadEvents(agentdir.New(agentsDir, "manager").AuditLog(), 0, 0)
	if err != nil || len(evs) != 1 {
		t.Fatalf("read = %d evs err=%v", len(evs), err)
	}
	if evs[0].Type != "shim" || evs[0].Data["line"] != "boom" {
		t.Fatalf("event = %+v", evs[0])
	}
}

// TestTelemetryNoopSetupShutdown pins the daemon's default observability path:
// with no OTLP endpoint, telemetry.Setup installs no-op providers (Enabled ==
// false) and Shutdown is a clean no-op. This is the path exercised by every
// daemon boot in these tests (no OTEL_EXPORTER_OTLP_ENDPOINT is set), so a
// non-nil logger is passed as Run does — Setup may log.Warn with no nil guard.
func TestTelemetryNoopSetupShutdown(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	tel, err := telemetry.Setup(context.Background(),
		telemetry.Config{Endpoint: "", ServiceName: "tariboy"}, log)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if tel.Enabled() {
		t.Fatal("providers must be disabled with no endpoint")
	}
	if err := tel.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown no-op must be clean: %v", err)
	}
}

func TestRunServesAndShutsDown(t *testing.T) {
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error"})) }()

	sock := paths.New(base).Socket()
	c := client.New(sock)
	var raw json.RawMessage
	var err error
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		raw, err = c.Call("GET", "/api/daemon/status", nil)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("daemon never came up: %v", err)
	}
	var st map[string]any
	json.Unmarshal(raw, &st)
	if st["base_dir"] != base {
		t.Fatalf("status base_dir = %v", st["base_dir"])
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down")
	}
}

func TestFreshDaemonStartsWithNoInstalledPlugins(t *testing.T) {
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error"}))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not shut down")
		}
	})

	c := client.New(paths.New(base).Socket())
	deadline := time.Now().Add(15 * time.Second)
	var raw json.RawMessage
	var err error
	for time.Now().Before(deadline) {
		raw, err = c.Call("GET", "/api/plugins", nil)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("daemon never came up: %v", err)
	}
	var listing struct {
		Plugins []map[string]any `json:"plugins"`
		Count   int              `json:"count"`
	}
	if err := json.Unmarshal(raw, &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Count != 0 || len(listing.Plugins) != 0 {
		t.Fatalf("fresh plugin list = %+v", listing)
	}
}

func TestRunRejectsPublicTCPWithoutAuth(t *testing.T) {
	err := Run(context.Background(), daemonTestOptions(Options{BaseDir: t.TempDir(), Listen: "tcp:0.0.0.0:0", LogLevel: "error"}))
	if err == nil {
		t.Fatal("public tcp without auth must fail")
	}
}

type fakeQuestionReconciler struct{ calls chan struct{} }

func (f *fakeQuestionReconciler) ReconcileWorkflowQuestions(context.Context) (int, error) {
	select {
	case f.calls <- struct{}{}:
	default:
	}
	return 1, nil
}

func TestWorkflowQuestionReconcilerRunsAtStartupPeriodicallyAndStops(t *testing.T) {
	fake := &fakeQuestionReconciler{calls: make(chan struct{}, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runWorkflowQuestionReconciler(ctx, fake, 5*time.Millisecond, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		close(done)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-fake.calls:
		case <-time.After(time.Second):
			t.Fatalf("reconcile call %d did not arrive", i+1)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not stop")
	}
}

func TestRecordEventLogsOnError(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close() // subsequent AddEvent must fail (database is closed)
	var buf bytes.Buffer
	recordEvent(slog.New(slog.NewTextHandler(&buf, nil)), s, nil, nil, "", "daemon_start", "{}")
	if !strings.Contains(buf.String(), "failed to record daemon event") ||
		!strings.Contains(buf.String(), "kind=daemon_start") {
		t.Fatalf("warn not logged: %q", buf.String())
	}
}

func TestSecondDaemonRefusesWhenSocketLive(t *testing.T) {
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error"})) }()

	p := paths.New(base)
	ready := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("unix", p.Socket(), 200*time.Millisecond); err == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("first daemon never became ready")
	}
	if _, err := os.Stat(p.PidFile()); err != nil {
		t.Fatalf("pidfile missing while running: %v", err)
	}

	err := Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error"}))
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second Run err = %v, want 'already running'", err)
	}

	cancel()
	<-done
	if _, err := os.Stat(p.PidFile()); !os.IsNotExist(err) {
		t.Fatalf("pidfile should be removed after shutdown, stat err = %v", err)
	}
}
