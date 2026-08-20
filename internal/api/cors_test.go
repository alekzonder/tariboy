package api

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
)

// tcpServer starts an api.Server on a real loopback TCP port with the given auth
// token (via ServeTCP) and returns its base URL. Mirrors the ServeTCP wiring the
// federated daemon uses.
func tcpServer(t *testing.T, token string) string {
	t.Helper()
	reg := registry.New()
	reg.Register(registry.Command{
		Path: "daemon.status", Summary: "status",
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			return map[string]any{"version": c.Version}, nil
		},
	})
	cctx := &registry.Ctx{Version: "test", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now()}
	srv := NewServer(reg, cctx)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // ServeTCP re-binds this addr; race window is acceptable in-test
	go func() { _ = srv.ServeTCP(addr, token) }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	base := "http://" + addr
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", base+"/api/daemon/status", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return base
}

func TestCORSPreflightNoAuth(t *testing.T) {
	base := tcpServer(t, "sekret")

	// A credential-less preflight OPTIONS must be answered 204 (before auth) with
	// permissive-but-scoped CORS headers.
	req, _ := http.NewRequest(http.MethodOptions, base+"/api/daemon/status", nil)
	req.Header.Set("Origin", "http://daemon-a.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("ACAO = %q, want *", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatalf("ACAH empty, want Authorization/Content-Type")
	}
}

func TestCORSPreflightAllowsPatchForTaskSave(t *testing.T) {
	base := tcpServer(t, "sekret")
	req, _ := http.NewRequest(http.MethodOptions, base+"/api/tasks/TEST-1", nil)
	req.Header.Set("Origin", "http://daemon-a.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPatch) {
		t.Fatalf("ACAM = %q, want PATCH", got)
	}
}

func TestCORSHeaderOnAuthedResponse(t *testing.T) {
	base := tcpServer(t, "sekret")
	req, _ := http.NewRequest("GET", base+"/api/daemon/status", nil)
	req.Header.Set("Authorization", "Bearer sekret")
	req.Header.Set("Origin", "http://daemon-a.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("ACAO on real response = %q, want *", got)
	}
}

// TestCORSRealRequestStillRequiresBearer asserts CORS does NOT bypass auth: a
// real (non-preflight) request without the bearer is still 401. The middleware
// order is cors(auth(handler)) — cors only short-circuits OPTIONS.
func TestCORSRealRequestStillRequiresBearer(t *testing.T) {
	base := tcpServer(t, "sekret")
	req, _ := http.NewRequest("GET", base+"/api/daemon/status", nil)
	req.Header.Set("Origin", "http://daemon-a.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET without bearer = %d, want 401", resp.StatusCode)
	}
}

// TestCORSScopedToTCP asserts the wildcard `*` CORS of ServeTCP reaches neither
// the unix listener (no CORS at all) nor the web listener (loopback-origin echo
// only, and nothing when the request carries no Origin).
func TestCORSScopedToTCP(t *testing.T) {
	newSrv := func() *Server {
		reg := registry.New()
		reg.Register(registry.Command{
			Path: "daemon.status", Summary: "status",
			HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
			Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
				return map[string]any{"version": c.Version}, nil
			},
		})
		cctx := &registry.Ctx{Version: "test", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now()}
		return NewServer(reg, cctx)
	}

	// Unix listener: no CORS header.
	t.Run("unix", func(t *testing.T) {
		srv := newSrv()
		sock := t.TempDir() + "/d.sock"
		go func() { _ = srv.ServeUnix(sock) }()
		t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
		cl := &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		}}
		var resp *http.Response
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			r, err := cl.Get("http://unix/api/daemon/status")
			if err == nil {
				resp = r
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if resp == nil {
			t.Fatal("unix server never came up")
		}
		defer resp.Body.Close()
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("unix listener emitted ACAO = %q, want none", got)
		}
	})

	// Web (loopback) listener: a request with no Origin gets no CORS header.
	t.Run("web", func(t *testing.T) {
		srv := newSrv()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		addr := ln.Addr().String()
		_ = ln.Close()
		go func() { _ = srv.ServeWeb(addr) }()
		t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
		base := "http://" + addr
		var resp *http.Response
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			r, err := http.Get(base + "/api/daemon/status")
			if err == nil {
				resp = r
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if resp == nil {
			t.Fatal("web server never came up")
		}
		defer resp.Body.Close()
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("web listener emitted ACAO = %q, want none", got)
		}
	})
}

// webServer starts an api.Server on a real loopback TCP port via ServeWeb (no
// bearer auth, Host+CORS middleware) and returns its base URL.
func webServer(t *testing.T) string {
	t.Helper()
	reg := registry.New()
	reg.Register(registry.Command{
		Path: "daemon.status", Summary: "status",
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			return map[string]any{"version": c.Version}, nil
		},
	})
	cctx := &registry.Ctx{Version: "test", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now()}
	srv := NewServer(reg, cctx)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // ServeWeb re-binds this addr; race window is acceptable in-test
	go func() { _ = srv.ServeWeb(addr) }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	base := "http://" + addr
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(base + "/api/daemon/status"); err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return base
}

// TestWebCORSLoopbackOrigin covers the port-forward case: a UI served by daemon
// A on http://localhost:9990 fetching daemon B forwarded to another localhost
// port. The origin is echoed back (never `*`), and a preflight is answered 204.
func TestWebCORSLoopbackOrigin(t *testing.T) {
	base := webServer(t)

	for _, origin := range []string{
		"http://localhost:9990",
		"http://127.0.0.1:9993",
		"http://[::1]:9990",
	} {
		t.Run(origin, func(t *testing.T) {
			req, _ := http.NewRequest("GET", base+"/api/daemon/status", nil)
			req.Header.Set("Origin", origin)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET = %d, want 200", resp.StatusCode)
			}
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("ACAO = %q, want %q", got, origin)
			}
			if got := resp.Header.Get("Vary"); !strings.Contains(got, "Origin") {
				t.Fatalf("Vary = %q, want it to contain Origin", got)
			}
		})
	}

	t.Run("preflight", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, base+"/api/daemon/status", nil)
		req.Header.Set("Origin", "http://localhost:9990")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("OPTIONS: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("preflight = %d, want 204", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:9990" {
			t.Fatalf("preflight ACAO = %q", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Headers"); got == "" {
			t.Fatalf("ACAH empty, want Authorization/Content-Type")
		}
	})
}

// TestWebCORSRejectsNonLoopbackOrigin is the security half: the web listener is
// unauthenticated, so a page on any non-loopback origin must get NO ACAO and
// have its read blocked by the browser.
func TestWebCORSRejectsNonLoopbackOrigin(t *testing.T) {
	base := webServer(t)

	for _, origin := range []string{
		"http://evil.example",
		"https://localhost:9990", // https is not the loopback UI origin
		"null",                   // sandboxed/file:// document
		"http://localhost.evil.example:9990",
	} {
		t.Run(origin, func(t *testing.T) {
			req, _ := http.NewRequest("GET", base+"/api/daemon/status", nil)
			req.Header.Set("Origin", origin)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("ACAO for %q = %q, want none", origin, got)
			}
		})
	}
}

// The macOS Tauri webview serves the bundled SPA from tauri://localhost. Every
// fetch/EventSource it makes to the daemon's loopback listener is cross-origin
// and carries that Origin, so the web listener must echo it back.
func TestWebCORSAllowsDesktopOrigin(t *testing.T) {
	base := webServer(t)
	const origin = "tauri://localhost"

	req, _ := http.NewRequest("GET", base+"/api/daemon/status", nil)
	req.Header.Set("Origin", origin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("ACAO = %q, want %q", got, origin)
	}
	if got := resp.Header.Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want it to contain Origin", got)
	}
}

func TestWebCORSDesktopPreflightAllowsPatchForTaskSave(t *testing.T) {
	base := webServer(t)
	req, _ := http.NewRequest(http.MethodOptions, base+"/api/tasks/TEST-1", nil)
	req.Header.Set("Origin", desktopOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != desktopOrigin {
		t.Fatalf("ACAO = %q, want %q", got, desktopOrigin)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPatch) {
		t.Fatalf("ACAM = %q, want PATCH", got)
	}
}

// Only the exact desktop origin is allowed: no other tauri:// host, and no
// look-alike that merely embeds the string.
func TestWebCORSRejectsLookalikeDesktopOrigins(t *testing.T) {
	base := webServer(t)
	for _, origin := range []string{
		"tauri://evil",
		"tauri://localhost.evil.example",
		"https://tauri.localhost",
	} {
		t.Run(origin, func(t *testing.T) {
			req, _ := http.NewRequest("GET", base+"/api/daemon/status", nil)
			req.Header.Set("Origin", origin)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("ACAO for %q = %q, want none", origin, got)
			}
		})
	}
}
