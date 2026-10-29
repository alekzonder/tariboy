package daemon

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// startDaemon runs the daemon and returns once Options.Ready fired. done
// carries Run's result; the daemon stops when ctx is canceled.
func startDaemon(t *testing.T, ctx context.Context, opts Options) <-chan error {
	t.Helper()
	ready := make(chan struct{})
	opts.Ready = func() { close(ready) }
	done := make(chan error, 1)
	go func() { done <- Run(ctx, daemonTestOptions(opts)) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("Run returned before Ready: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("daemon never became ready")
	}
	return done
}

// shutdownRecorder collects the names of the shutdown steps Run executes.
type shutdownRecorder struct {
	mu    sync.Mutex
	steps []string
}

func (r *shutdownRecorder) observe(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, name)
}

func (r *shutdownRecorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.steps)
}

func TestShutdownStackRunsLIFOOnceAndSurvivesPanics(t *testing.T) {
	t.Parallel()
	var got []string
	var stack shutdownStack
	stack.push("first", func() { got = append(got, "first") })
	stack.push("panics", func() { panic("boom") })
	stack.push("last", func() { got = append(got, "last") })
	var observed []string
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	stack.run(log, func(name string) { observed = append(observed, name) })
	stack.run(log, func(name string) { observed = append(observed, name) })
	if want := []string{"last", "first"}; !slices.Equal(got, want) {
		t.Fatalf("ran %v, want %v", got, want)
	}
	if want := []string{"last", "panics", "first"}; !slices.Equal(observed, want) {
		t.Fatalf("observed %v, want %v", observed, want)
	}
	if !strings.Contains(buf.String(), "step=panics") {
		t.Fatalf("panic not logged: %q", buf.String())
	}
}

// The shutdown order is data: every resource stops before the ones it uses.
func TestRunShutdownOrder(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	var rec shutdownRecorder
	ctx, cancel := context.WithCancel(context.Background())
	done := startDaemon(t, ctx, Options{BaseDir: base, Listen: "unix", LogLevel: "error",
		HTTPAddr: freeLoopbackAddr(t), onShutdownStep: rec.observe})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
	want := []string{"api-servers", "access-log", "ai-proxy", "workers", "operations", "plugins", "manager",
		"ai-proxy-listener", "telemetry", "store", "pidfile"}
	if got := rec.get(); !slices.Equal(got, want) {
		t.Fatalf("shutdown steps = %v, want %v", got, want)
	}
}

func socketAnswers(sock string) bool {
	c, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
	if err == nil {
		_ = c.Close()
	}
	return err == nil
}

// A web listener that cannot bind stops the already bound unix API server
// before the store closes, instead of leaving it serving over a closed store.
func TestRunWebBindFailureShutsDownUnixServer(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	var rec shutdownRecorder
	err = Run(context.Background(), daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error",
		HTTPAddr: occupied.Addr().String(), onShutdownStep: rec.observe}))
	if err == nil || !strings.Contains(err.Error(), "http listener") {
		t.Fatalf("Run = %v, want an http listener error", err)
	}
	steps := rec.get()
	if i, j := slices.Index(steps, "api-servers"), slices.Index(steps, "store"); i < 0 || j < 0 || i > j {
		t.Fatalf("shutdown steps = %v, want api-servers before store", steps)
	}
	if socketAnswers(paths.New(base).Socket()) {
		t.Fatal("unix API socket still answers after Run returned")
	}
}

// ServeWeb failing after the unix listener already serves takes the same path:
// Run returns the error after every server stopped and before the store closed.
func TestRunWebServeFailureAfterReadyShutsDownUnixServer(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	sock := paths.New(base).Socket()
	fail := make(chan struct{})
	var rec shutdownRecorder
	var unixServedAtFailure bool
	opts := Options{BaseDir: base, Listen: "unix", LogLevel: "error",
		HTTPAddr: freeLoopbackAddr(t), onShutdownStep: rec.observe,
		serveHook: func(name string, serve func() error) func() error {
			if name != "web" {
				return serve
			}
			return func() error {
				<-fail
				unixServedAtFailure = socketAnswers(sock)
				return errors.New("web listener broke")
			}
		}}
	done := startDaemon(t, context.Background(), opts)
	close(fail)
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "web listener broke") {
			t.Fatalf("Run = %v, want the web serve error", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after the web listener failed")
	}
	if !unixServedAtFailure {
		t.Fatal("unix socket was not serving when the web listener failed")
	}
	steps := rec.get()
	if i, j := slices.Index(steps, "api-servers"), slices.Index(steps, "store"); i < 0 || j < 0 || i > j {
		t.Fatalf("shutdown steps = %v, want api-servers before store", steps)
	}
	if socketAnswers(sock) {
		t.Fatal("unix API socket still answers after Run returned")
	}
}
