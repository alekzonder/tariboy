package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type pricingHTTPClientFunc func(*http.Request) (*http.Response, error)

func (f pricingHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

// daemonTestOptions keeps in-process daemon fixtures deterministic. Production
// startup intentionally resolves the account's login-shell PATH, but that
// external probe is neither part of daemon readiness tests nor safe to put on
// their critical path. userpath_test.go passes its own resolver when it needs
// to exercise that production behavior.
func daemonTestOptions(options Options) Options {
	if options.UserPathResolver == nil {
		options.UserPathResolver = func(context.Context, string) (string, error) {
			return os.Getenv("PATH"), nil
		}
	}
	if options.PricingHTTPClient == nil {
		options.PricingHTTPClient = pricingHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("pricing refresh disabled in daemon test")
		})
	}
	if options.Ready == nil {
		ready := readyChannel(options.BaseDir)
		options.Ready = func() { close(ready) }
	}
	return options
}

// daemonReady maps a base dir to the Ready channel of its next daemon start.
// Either side may create the entry first: Run often configures its options
// inside the goroutine that the test waits on.
var daemonReady sync.Map

func readyChannel(base string) chan struct{} {
	ready, _ := daemonReady.LoadOrStore(base, make(chan struct{}))
	return ready.(chan struct{})
}

// waitDaemonReady waits until the next daemon started on base called Ready,
// and consumes that start so a restart on the same base waits again.
func waitDaemonReady(t *testing.T, base string) {
	t.Helper()
	select {
	case <-readyChannel(base):
		daemonReady.Delete(base)
	case <-time.After(30 * time.Second):
		t.Fatal("daemon never became ready")
	}
}

// freeLoopbackAddr returns a loopback host:port that was free a moment ago, so
// parallel daemons never contend for one fixed port.
func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func TestDaemonTestOptionsBypassExternalUserPathProbe(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "external-path-probe-called")
	probe := filepath.Join(root, "probe-shell")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\ntouch \""+marker+"\"\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", probe)

	// Force Run to return immediately after PATH setup. This isolates the
	// assertion from unrelated daemon startup duration, including -race load.
	baseFile := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(baseFile, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), daemonTestOptions(Options{BaseDir: baseFile, Listen: "unix", LogLevel: "error"})); err == nil {
		t.Fatal("Run error = nil, want isolated invalid-base failure")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("external PATH probe ran despite injected resolver: stat error = %v", err)
	}
}
