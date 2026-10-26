package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
)

// serveUnixMux starts an HTTP server on a fresh unix socket bound to mux and
// returns its path. Used by the /routes and /action client tests.
func serveUnixMux(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "p.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close(); _ = os.Remove(sock) })
	return sock
}

func TestClientRoutes(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/routes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"routes": map[string]any{"c1": "chat:orders"}, "has_token": true})
	})
	out, err := NewClient(serveUnixMux(t, mux)).Routes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out["has_token"] != true {
		t.Fatalf("has_token: %v", out["has_token"])
	}
}

func TestClientActionSuccess(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"chat_id": body["chat_id"], "channel": body["channel"]})
	})
	out, err := NewClient(serveUnixMux(t, mux)).Action(context.Background(),
		map[string]any{"action": "bind", "chat_id": "c1", "channel": "chat:orders"})
	if err != nil {
		t.Fatal(err)
	}
	if out["channel"] != "chat:orders" {
		t.Fatalf("channel: %v", out["channel"])
	}
}

func TestClientActionError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/action", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "no_token"})
	})
	_, err := NewClient(serveUnixMux(t, mux)).Action(context.Background(), map[string]any{"action": "create"})
	var ae *ActionError
	if !errors.As(err, &ae) {
		t.Fatalf("want *ActionError, got %v", err)
	}
	if ae.Status != 409 || ae.Code != "no_token" {
		t.Fatalf("got status=%d code=%q", ae.Status, ae.Code)
	}
}

// serveUnixPlugin starts a tiny HTTP server on a unix socket and returns its
// path. It records the last /deliver body.
func serveUnixPlugin(t *testing.T, health int, deliver int, gotBody *[]byte) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "plugin.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(health) })
	mux.HandleFunc("/deliver", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if gotBody != nil {
			*gotBody = b
		}
		w.WriteHeader(deliver)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

func TestClientHealth(t *testing.T) {
	t.Parallel()
	c := NewClient(serveUnixPlugin(t, 200, 200, nil))
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("healthy plugin: %v", err)
	}
	bad := NewClient(serveUnixPlugin(t, 503, 200, nil))
	if err := bad.Health(context.Background()); err == nil {
		t.Fatal("unhealthy plugin should error")
	}
	// A socket that isn't listenable.
	if err := NewClient(filepath.Join(t.TempDir(), "nope.sock")).Health(context.Background()); err == nil {
		t.Fatal("dead socket should error")
	}
}

func TestClientDeliver(t *testing.T) {
	t.Parallel()
	var body []byte
	c := NewClient(serveUnixPlugin(t, 200, 200, &body))
	msg := mail.Letter{ID: "m1", From: "agent:alice", To: "plugin:echo", Body: "hi"}
	if err := c.Deliver(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Letter mail.Letter `json:"letter"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("deliver body not {letter:...}: %v (%s)", err, body)
	}
	if env.Letter.ID != "m1" || env.Letter.Body != "hi" || env.Letter.From != "agent:alice" {
		t.Fatalf("delivered letter = %+v", env.Letter)
	}
	// Non-2xx from the plugin is an error the drainer can act on.
	fail := NewClient(serveUnixPlugin(t, 200, 500, nil))
	if err := fail.Deliver(context.Background(), msg); err == nil {
		t.Fatal("500 from /deliver should error")
	}
}

// serveHungPlugin starts a unix-socket HTTP server whose handlers accept the
// connection but never respond, simulating a wedged plugin.
func serveHungPlugin(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "hung.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	mux := http.NewServeMux()
	hang := func(w http.ResponseWriter, r *http.Request) { <-block }
	mux.HandleFunc("/health", hang)
	mux.HandleFunc("/deliver", hang)
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

// TestClientHungPluginTimes out verifies that a wedged plugin cannot stall the
// caller indefinitely: the client's per-call timeout must fire.
func TestClientHungPluginTimesOut(t *testing.T) {
	t.Parallel()
	c := NewClient(serveHungPlugin(t))

	// Each call waits out the full client timeout, so the two run side by side.
	t.Run("health", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		if err := c.Health(context.Background()); err == nil {
			t.Fatal("hung plugin health check should error, not hang forever")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("Health took %v; client timeout should have fired well before that", elapsed)
		}
	})
	t.Run("deliver", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		msg := mail.Letter{ID: "m1", From: "agent:alice", To: "plugin:echo"}
		if err := c.Deliver(context.Background(), msg); err == nil {
			t.Fatal("hung plugin deliver should error, not hang forever")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("Deliver took %v; client timeout should have fired well before that", elapsed)
		}
	})
}
