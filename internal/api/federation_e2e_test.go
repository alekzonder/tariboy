package api

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
)

// startDaemon boots an api.Server on a fresh loopback TCP port with the given
// token via ServeTCP (the federated-daemon wiring). Returns the base URL.
func startDaemon(t *testing.T, token, version string) string {
	t.Helper()
	reg := registry.New()
	reg.Register(registry.Command{
		Path: "daemon.status", Summary: "status",
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			return map[string]any{"version": c.Version}, nil
		},
	})
	cctx := &registry.Ctx{Version: version, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now()}
	srv := NewServer(reg, cctx)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	go func() { _ = srv.ServeTCP(addr, token) }()
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

func getStatus(t *testing.T, url, bearer string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestTwoDaemonFederation(t *testing.T) {
	a := startDaemon(t, "token-a", "A")
	b := startDaemon(t, "token-b", "B")

	// Each daemon is reachable with ITS OWN token.
	if s := getStatus(t, a+"/api/daemon/status", "token-a"); s != 200 {
		t.Fatalf("A with token-a = %d, want 200", s)
	}
	if s := getStatus(t, b+"/api/daemon/status", "token-b"); s != 200 {
		t.Fatalf("B with token-b = %d, want 200", s)
	}
	// Trust isolation: A's token is rejected by B and vice versa.
	if s := getStatus(t, b+"/api/daemon/status", "token-a"); s != 401 {
		t.Fatalf("B with token-a = %d, want 401 (per-daemon trust)", s)
	}
	if s := getStatus(t, a+"/api/daemon/status", "token-b"); s != 401 {
		t.Fatalf("A with token-b = %d, want 401 (per-daemon trust)", s)
	}
	// No token at all → 401 on both.
	if s := getStatus(t, a+"/api/daemon/status", ""); s != 401 {
		t.Fatalf("A with no token = %d, want 401", s)
	}

	// Cross-origin preflight answered (before auth) on both.
	for _, base := range []string{a, b} {
		req, _ := http.NewRequest(http.MethodOptions, base+"/api/daemon/status", nil)
		req.Header.Set("Origin", "http://ui.example")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("OPTIONS %s: %v", base, err)
		}
		if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("preflight %s = %d acao=%q", base, resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
		}
		_ = resp.Body.Close()
	}

	// SSE query-token auth: a /events request with the right ?token= authenticates
	// (there is no events source registered, so it 404s AFTER passing auth — the
	// point is it is NOT 401). A wrong token IS 401.
	if s := getStatus(t, a+"/api/agents/foo/events?token=token-a", ""); s == 401 {
		t.Fatalf("A /events?token=token-a = 401, want auth to pass (any non-401)")
	}
	if s := getStatus(t, a+"/api/agents/foo/events?token=token-b", ""); s != 401 {
		t.Fatalf("A /events?token=token-b = %d, want 401", s)
	}
}
