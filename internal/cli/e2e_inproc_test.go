package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	go func() {
		defer close(done)
		_ = daemon.Run(ctx, inProcessDaemonOptions(daemon.Options{BaseDir: base, Listen: "unix", LogLevel: "error"}))
	}()
	sock = filepath.Join(base, "tariboyd.sock")
	c = client.New(sock)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := c.Call("GET", "/api/daemon/status", nil); err == nil {
			return base, sock, c
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("daemon did not become ready")
	return
}

// syncBuf is a concurrency-safe writer so a follow goroutine can print while the
// test reads what it has produced so far.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestLogsNonFollowRunsInDaemon drives `logs <agent>` exactly as the real CLI
// does: a LOCAL Ctx whose Store is nil plus a client to a daemon that owns the
// Store. Before the fix, logs was a CLI-local command and this dereferenced the
// nil Store in the CLI process; now the non-follow read is remote.
func TestLogsNonFollowRunsInDaemon(t *testing.T) {
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

// TestChannelTailFollow verifies `channel tail <ch> -f` polls and prints newly
// published messages, then exits cleanly when its context is cancelled.
func TestChannelTailFollow(t *testing.T) {
	_, sock, c := startDaemon(t)

	send := func(text string) {
		var o, e bytes.Buffer
		code := cli.Run(context.Background(), commands.BuildRegistry(),
			[]string{"message", "send", "-c", "chat:room", "--type", "note", "--text", text}, c, nil, &o, &e)
		if code != 0 {
			t.Fatalf("send %q exit=%d err=%s", text, code, e.String())
		}
	}

	send("first")

	fctx, fcancel := context.WithCancel(context.Background())
	local := &registry.Ctx{Socket: sock}
	out := &syncBuf{}
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(fctx, commands.BuildRegistry(),
			[]string{"channel", "tail", "chat:room", "-f"}, c, local, out, io.Discard)
	}()

	// Let the first poll drain the backlog, then publish a new message.
	waitFor(t, func() bool { return strings.Contains(out.String(), "first") })
	send("second")
	waitFor(t, func() bool { return strings.Contains(out.String(), "second") })

	fcancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("tail -f exit=%d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tail -f did not exit after cancel")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestCLITalksToDaemon(t *testing.T) {
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go daemon.Run(ctx, inProcessDaemonOptions(daemon.Options{BaseDir: base, Listen: "unix", LogLevel: "error"}))

	sock := filepath.Join(base, "tariboyd.sock")
	c := client.New(sock)
	deadline := time.Now().Add(15 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if _, err := c.Call("GET", "/api/daemon/status", nil); err == nil {
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("daemon did not become ready")
	}

	// daemon.status is CLIHidden (served by the CLI-local dispatchDaemon, not
	// cli.Run); use a reachable registry GET to prove cli.Run reaches the socket.
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
