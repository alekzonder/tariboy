package daemon

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// A request with a spoofed Host is rejected (421) by the web listener; a request
// with the real loopback Host is served. The primary unix API listener has no
// such check (it is unreachable from a browser and its posture is unchanged).
func TestWebListenerRejectsForeignHost(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := freeLoopbackAddr(t)
	errc := make(chan error, 1)
	go func() {
		errc <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: addr}))
	}()

	client := &http.Client{Timeout: time.Second}
	// Wait until the listener is up (correct Host succeeds).
	if !testutil.WaitFor(5*time.Second, func() bool {
		resp, err := client.Get("http://" + addr + "/api/daemon/status")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == 200
	}) {
		t.Fatal("web listener never served the API with the real Host")
	}

	// Spoofed Host -> 421.
	req, _ := http.NewRequest("GET", "http://"+addr+"/api/daemon/status", nil)
	req.Host = "evil.com"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("spoofed-host request: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest || !strings.Contains(string(b), "bad_host") {
		t.Fatalf("spoofed host = %d %q, want 421 bad_host", resp.StatusCode, b)
	}

	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Run returned: %v", err)
	}
}

// A web-bind failure is fatal: Run returns the bind error instead of hanging.
func TestWebBindFailureIsFatal(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Occupy a port first so the web listener's bind fails.
	ln, lerr := net.Listen("tcp", "127.0.0.1:0")
	if lerr != nil {
		t.Fatal(lerr)
	}
	defer ln.Close()
	occupied := ln.Addr().String()

	errc := make(chan error, 1)
	go func() {
		errc <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: occupied}))
	}()

	select {
	case rerr := <-errc:
		if rerr == nil || !strings.Contains(rerr.Error(), "http listener serve") {
			t.Fatalf("Run err = %v, want an http-listener bind failure", rerr)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return on web-bind failure (not routed through errc)")
	}
}
