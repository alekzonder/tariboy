package daemon

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// phaseOptions are daemon options for a phase test rooted in a seeded base.
func phaseOptions(t *testing.T) Options {
	t.Helper()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	return daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error"})
}

// phaseCore runs setupProcess and openStore, and unwinds them when the test ends.
func phaseCore(t *testing.T, o Options) (*process, *coreParts, *shutdownStack) {
	t.Helper()
	stack := &shutdownStack{}
	t.Cleanup(func() { stack.run(nil, nil) })
	proc, err := setupProcess(context.Background(), o, stack)
	if err != nil {
		t.Fatal(err)
	}
	c, err := openStore(context.Background(), proc, stack)
	if err != nil {
		t.Fatal(err)
	}
	return proc, c, stack
}

func TestSetupProcessWritesAndRemovesPidfile(t *testing.T) {
	t.Parallel()
	o := phaseOptions(t)
	var stack shutdownStack
	proc, err := setupProcess(context.Background(), o, &stack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(proc.paths.PidFile()); err != nil {
		t.Fatalf("pidfile missing after setup: %v", err)
	}
	stack.run(proc.log, nil)
	if _, err := os.Stat(proc.paths.PidFile()); !os.IsNotExist(err) {
		t.Fatalf("pidfile left after shutdown: %v", err)
	}
}

// The guard is the one phase test with a socket: a live socket is what it
// detects.
func TestSetupProcessRefusesLiveSocket(t *testing.T) {
	t.Parallel()
	o := phaseOptions(t)
	sock := paths.New(o.BaseDir).Socket()
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var stack shutdownStack
	if _, err := setupProcess(context.Background(), o, &stack); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("setupProcess = %v, want already running", err)
	}
}

func TestSetupProcessRejectsNonLoopbackHTTPAddr(t *testing.T) {
	t.Parallel()
	o := phaseOptions(t)
	o.HTTPAddr = "0.0.0.0:0"
	var stack shutdownStack
	defer stack.run(nil, nil)
	if _, err := setupProcess(context.Background(), o, &stack); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("setupProcess = %v, want loopback refusal", err)
	}
}

func TestOpenStoreRecordsAgentlessEventsUntilShutdown(t *testing.T) {
	t.Parallel()
	o := phaseOptions(t)
	var stack shutdownStack
	proc, err := setupProcess(context.Background(), o, &stack)
	if err != nil {
		t.Fatal(err)
	}
	c, err := openStore(context.Background(), proc, &stack)
	if err != nil {
		t.Fatal(err)
	}
	c.rec.Record("", "phase_test", "{}")
	var n int
	if err := c.st.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = 'phase_test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("recorded events = %d, err = %v", n, err)
	}
	stack.run(proc.log, nil)
	if err := c.st.DB.Ping(); err == nil {
		t.Fatal("store still open after shutdown")
	}
}

func defaultQueueCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_queues WHERE prefix = 'TASK'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBuildTasksSeedsDefaultQueueAndBareImage(t *testing.T) {
	t.Parallel()
	proc, c, _ := phaseCore(t, phaseOptions(t))
	tk, err := buildTasks(context.Background(), proc, c)
	if err != nil {
		t.Fatal(err)
	}
	if tk.service == nil || tk.hub == nil {
		t.Fatal("task service or hub missing")
	}
	if n := defaultQueueCount(t, c.st.DB); n != 1 {
		t.Fatalf("default queues = %d, want 1", n)
	}
	if !tk.images.Exists(image.BareRef) {
		t.Fatal("bare image not seeded")
	}
}

// Startup seeding runs on the daemon context: a canceled start does no work.
func TestBuildTasksHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	proc, c, _ := phaseCore(t, phaseOptions(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buildTasks(ctx, proc, c); err != nil {
		t.Fatal(err)
	}
	if n := defaultQueueCount(t, c.st.DB); n != 0 {
		t.Fatalf("default queues = %d after canceled start, want 0", n)
	}
}

func TestBuildProxyDoesNotListen(t *testing.T) {
	t.Parallel()
	o := phaseOptions(t)
	proc, c, _ := phaseCore(t, o)
	px, err := buildProxy(context.Background(), o, proc, c)
	if err != nil {
		t.Fatal(err)
	}
	if px.proxy == nil || px.ingester == nil || px.catalog == nil {
		t.Fatal("proxy parts missing")
	}
	if addr := px.proxy.Addr(); addr != "" {
		t.Fatalf("buildProxy bound %s, want no listener", addr)
	}
}

func TestBuildPluginsAllowsCoreSubscribes(t *testing.T) {
	t.Parallel()
	proc, c, _ := phaseCore(t, phaseOptions(t))
	pl := buildPlugins(proc, c)
	if pl.host == nil || pl.api == nil {
		t.Fatal("plugin parts missing")
	}
	// The subscription hook and params validator treat a core channel as
	// non-provider, so the subscribe succeeds.
	if _, err := c.bus.Subscribe("phase-agent", bus.InboxChannel("phase-agent"), bus.Matcher{}, nil); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
}

func TestBuildManagerWiresRegistryContext(t *testing.T) {
	t.Parallel()
	o := phaseOptions(t)
	proc, c, _ := phaseCore(t, o)
	tk, err := buildTasks(context.Background(), proc, c)
	if err != nil {
		t.Fatal(err)
	}
	px, err := buildProxy(context.Background(), o, proc, c)
	if err != nil {
		t.Fatal(err)
	}
	mg := buildManager(context.Background(), o, proc, c, tk, px, buildPlugins(proc, c))
	if mg.manager == nil || mg.reg == nil {
		t.Fatal("manager parts missing")
	}
	if mg.cctx.Control != mg.manager || mg.cctx.Tasks != tk.service || mg.cctx.Store != c.st {
		t.Fatal("registry context not wired to the built parts")
	}
}

// A request deadline arms a one-shot timeout schedule in the caller's
// transaction, and the reply's cancel removes exactly that entry.
func TestDeadlineHooksArmAndCancelOneShot(t *testing.T) {
	t.Parallel()
	_, c, _ := phaseCore(t, phaseOptions(t))
	sched := schedule.NewStore(c.st, time.Now)
	arm, cancelDeadline := deadlineHooks(sched)
	tx, err := c.st.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := arm(tx, "phase-agent", "corr-1", "5m"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	list, err := sched.List("phase-agent")
	if err != nil || len(list) != 1 || list[0].Kind != "oneshot" || !strings.Contains(list[0].MessageTemplate, "corr-1") {
		t.Fatalf("schedules = %+v, err = %v", list, err)
	}
	tx, err = c.st.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := cancelDeadline(tx, "corr-1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if list, _ := sched.List("phase-agent"); len(list) != 0 {
		t.Fatalf("schedules after cancel = %+v", list)
	}
	if err := arm(nil, "phase-agent", "corr-2", "soon"); err == nil {
		t.Fatal("arm accepted a malformed deadline")
	}
}

// The workers step cancels every worker and returns only after they finished.
func TestStartWorkersShutdownWaitsForWorkers(t *testing.T) {
	t.Parallel()
	o := phaseOptions(t)
	proc, c, _ := phaseCore(t, o)
	tk, err := buildTasks(context.Background(), proc, c)
	if err != nil {
		t.Fatal(err)
	}
	px, err := buildProxy(context.Background(), o, proc, c)
	if err != nil {
		t.Fatal(err)
	}
	mg := buildManager(context.Background(), o, proc, c, tk, px, buildPlugins(proc, c))
	started := make(chan struct{})
	finished := false
	o.schedulerRun = func(ctx context.Context, _ *schedule.Scheduler) {
		close(started)
		<-ctx.Done()
		finished = true
	}
	var stack shutdownStack
	startWorkers(context.Background(), o, proc, c, tk, px, mg, &stack)
	<-started
	stack.run(proc.log, nil)
	if !finished {
		t.Fatal("workers step returned before the scheduler finished")
	}
}
