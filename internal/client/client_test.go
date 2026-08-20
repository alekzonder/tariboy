package client

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func startDaemon(t *testing.T) string {
	t.Helper()
	reg := registry.New()
	reg.Register(registry.Command{
		Path: "daemon.status", Summary: "s",
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			return map[string]any{"version": "t", "q": p["q"]}, nil
		},
	})
	reg.Register(registry.Command{
		Path: "fail", Summary: "s",
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/fail"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			return nil, api.UserError{Code: "nope", Msg: "no way"}
		},
	})
	cctx := &registry.Ctx{Version: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := api.NewServer(reg, cctx)
	sock := filepath.Join(t.TempDir(), "d.sock")
	go srv.ServeUnix(sock)
	// wait
	c := New(sock)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := c.Call("GET", "/api/help.json", nil); err == nil {
			return sock
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no server")
	return ""
}

func TestCallOK(t *testing.T) {
	sock := startDaemon(t)
	raw, err := New(sock).Call("GET", "/api/daemon/status", map[string]string{"q": "42"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(raw, &m)
	if m["version"] != "t" || m["q"] != "42" {
		t.Fatalf("bad result %v", m)
	}
}

func TestCallAPIError(t *testing.T) {
	sock := startDaemon(t)
	_, err := New(sock).Call("POST", "/api/fail", nil)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "nope" {
		t.Fatalf("want APIError nope, got %v", err)
	}
}

func TestDaemonDown(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "absent.sock")).Call("GET", "/api/daemon/status", nil)
	if err == nil || !IsDaemonDown(err) {
		t.Fatalf("want daemon-down error, got %v", err)
	}
}
