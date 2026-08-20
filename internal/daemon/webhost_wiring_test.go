package daemon

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A request with a spoofed Host is rejected (421) by the web listener; a request
// with the real loopback Host is served. The primary unix API listener has no
// such check (it is unreachable from a browser and its posture is unchanged).
func TestWebListenerRejectsForeignHost(t *testing.T) {
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := "127.0.0.1:8796"
	errc := make(chan error, 1)
	go func() {
		errc <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: addr}))
	}()

	client := &http.Client{Timeout: time.Second}
	// Wait until the listener is up (correct Host succeeds).
	ok := false
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequest("GET", "http://"+addr+"/api/daemon/status", nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ok = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok {
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
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Occupy the port first so the web listener's bind fails.
	occupied := "127.0.0.1:8795"
	ln, lerr := net.Listen("tcp", occupied)
	if lerr != nil {
		t.Skipf("cannot occupy %s: %v", occupied, lerr)
	}
	defer ln.Close()

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
