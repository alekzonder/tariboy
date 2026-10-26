package cli_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/cli"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/daemon"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

var errInProcessPricingRefreshDisabled = errors.New("pricing refresh disabled in CLI daemon test")

type pricingHTTPClientFunc func(*http.Request) (*http.Response, error)

func (f pricingHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func inProcessDaemonOptions(options daemon.Options) daemon.Options {
	options.UserPathResolver = func(context.Context, string) (string, error) {
		return os.Getenv("PATH"), nil
	}
	if options.PricingHTTPClient == nil {
		options.PricingHTTPClient = pricingHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return nil, errInProcessPricingRefreshDisabled
		})
	}
	return options
}

func TestInProcessDaemonOptionsDisablesPricingNetwork(t *testing.T) {
	t.Parallel()
	opts := inProcessDaemonOptions(daemon.Options{})
	if opts.PricingHTTPClient == nil {
		t.Fatal("PricingHTTPClient = nil, would use the production GitHub client")
	}
	req, err := http.NewRequest(http.MethodGet, "https://catalog.test/prices.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := opts.PricingHTTPClient.Do(req)
	if resp != nil || !errors.Is(err, errInProcessPricingRefreshDisabled) {
		t.Fatalf("pricing client response=%v error=%v, want nil deterministic disabled error", resp, err)
	}
}

// startDaemon boots an in-process daemon on a temp base dir and returns its
// socket path once it answers, mirroring the real CLI<->daemon split.
func startDaemon(t *testing.T) (base, sock string, c *client.Client) {
	t.Helper()
	base = t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	ready := make(chan struct{})
	go func() {
		defer close(done)
		_ = daemon.Run(ctx, inProcessDaemonOptions(daemon.Options{BaseDir: base, Listen: "unix", LogLevel: "error",
			Ready: func() { close(ready) }}))
	}()
	sock = filepath.Join(base, "tariboyd.sock")
	c = client.New(sock)
	waitReady(t, ready, done)
	return base, sock, c
}

// waitReady waits until the daemon called Options.Ready, failing if it exited
// (done closed) first.
func waitReady(t *testing.T, ready, done <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-done:
		t.Fatal("daemon exited before it became ready")
	case <-time.After(30 * time.Second):
		t.Fatal("daemon did not become ready")
	}
}

// TestLogsNonFollowRunsInDaemon drives `logs <agent>` exactly as the real CLI
// does: a LOCAL Ctx whose Store is nil plus a client to a daemon that owns the
// Store. Before the fix, logs was a CLI-local command and this dereferenced the
// nil Store in the CLI process; now the non-follow read is remote.
func TestLogsNonFollowRunsInDaemon(t *testing.T) {
	t.Parallel()
	base, sock, c := startDaemon(t)

	// Seed an agent-scoped audit event into the agent's audit.jsonl (the durable
	// audit log the daemon now reads for `logs <agent>`).
	auditPath := agentdir.New(paths.New(base).AgentsDir(), "agent-x").AuditLog()
	audit.Open(auditPath, nil).Record("iteration_done", "system", "", map[string]any{"n": 1})

	local := &registry.Ctx{Socket: sock} // Store stays nil, like the real CLI.
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), commands.BuildRegistry(),
		[]string{"logs", "agent-x"}, c, local, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "count:") || !strings.Contains(out.String(), "iteration_done") {
		t.Fatalf("logs did not print recent events: out=%q", out.String())
	}
}

func TestCLITalksToDaemon(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done // stop the daemon before TempDir removes its base
	})
	ready := make(chan struct{})
	go func() {
		defer close(done)
		_ = daemon.Run(ctx, inProcessDaemonOptions(daemon.Options{BaseDir: base, Listen: "unix", LogLevel: "error",
			Ready: func() { close(ready) }}))
	}()

	sock := filepath.Join(base, "tariboyd.sock")
	c := client.New(sock)
	waitReady(t, ready, done)

	// daemon status answers locally from the socket in TARIBOY_RUNTIME_DIR, not
	// through this caller; use a registry GET to prove cli.Run reaches the socket.
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), commands.BuildRegistry(), []string{"agent", "ps"}, c, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "count:") {
		t.Fatalf("out=%q", out.String())
	}

	code = cli.Run(context.Background(), commands.BuildRegistry(),
		[]string{"daemon", "config", "set", "log_level", "debug"}, c, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("config set exit=%d err=%s", code, errOut.String())
	}
	out.Reset()
	code = cli.Run(context.Background(), commands.BuildRegistry(),
		[]string{"daemon", "config", "get", "--key", "log_level"}, c, nil, &out, &errOut)
	if code != 0 || !strings.Contains(out.String(), "log_level: debug") {
		t.Fatalf("config get exit=%d out=%q", code, out.String())
	}
}
