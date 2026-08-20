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
	var body []byte
	c := NewClient(serveUnixPlugin(t, 200, 200, &body))
	msg := MessageDTO{ID: "m1", Channel: "chat:in", Type: "chat.msg", Text: "hi", Source: "user:ops"}
	if err := c.Deliver(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Message MessageDTO `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("deliver body not {message:...}: %v (%s)", err, body)
	}
	if env.Message.ID != "m1" || env.Message.Text != "hi" {
		t.Fatalf("delivered msg = %+v", env.Message)
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
	c := NewClient(serveHungPlugin(t))

	start := time.Now()
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("hung plugin health check should error, not hang forever")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Health took %v; client timeout should have fired well before that", elapsed)
	}

	start = time.Now()
	msg := MessageDTO{ID: "m1", Channel: "chat:in", Type: "chat.msg"}
	if err := c.Deliver(context.Background(), msg); err == nil {
		t.Fatal("hung plugin deliver should error, not hang forever")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Deliver took %v; client timeout should have fired well before that", elapsed)
	}
}

func TestClientPushWatches(t *testing.T) {
	var got ChannelWatchesDTO
	mux := http.NewServeMux()
	mux.HandleFunc("/watches", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(200)
	})
	err := NewClient(serveUnixMux(t, mux)).PushWatches(context.Background(), "issue-provider:query",
		[]WatchDTO{{Watch: "a1b2", Params: map[string]any{"query": "PROJ"}, Subscribers: []string{"dev-manager"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Channel != "issue-provider:query" || len(got.Watches) != 1 {
		t.Fatalf("plugin received %+v", got)
	}
	if got.Watches[0].Watch != "a1b2" || got.Watches[0].Subscribers[0] != "dev-manager" {
		t.Fatalf("watch payload = %+v", got.Watches[0])
	}
}

func TestClientPushWatchesNon2xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/watches", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) })
	err := NewClient(serveUnixMux(t, mux)).PushWatches(context.Background(), "issue-provider:query", nil)
	if err == nil {
		t.Fatal("non-2xx must be an error so the daemon retries")
	}
}

func TestProvidedChannelNames(t *testing.T) {
	if got := providedChannelNames(nil); got != nil {
		t.Fatalf("nil provide → nil, got %v", got)
	}
	got := providedChannelNames([]Provided{{Channel: "issue-provider:query"}, {Channel: "issue-provider:ticket"}})
	if len(got) != 2 || got[0] != "issue-provider:query" || got[1] != "issue-provider:ticket" {
		t.Fatalf("names = %v", got)
	}
}
