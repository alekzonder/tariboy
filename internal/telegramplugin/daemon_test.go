package telegramplugin

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDaemonClientUsesOperatorAndPluginAPIs(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	published := false
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/agents":
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"agents": []any{map[string]any{"name": "worker", "alias": "Worker"}}, "count": 1}})
		case "/api/agents/worker/subscriptions":
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": "s1"}})
		case "/api/plugin/publish":
			if r.Header.Get("Authorization") != "Bearer life-token" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			published = true
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": "m1", "delivered_agents": []string{"worker"}}})
		default:
			t.Fatalf("unexpected daemon path %s", r.URL.Path)
		}
	})}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	client := NewDaemonClient(socket, "life-token")
	agents, err := client.ListAgents(context.Background())
	if err != nil || len(agents) != 1 || agents[0].Name != "worker" {
		t.Fatalf("agents=%+v err=%v", agents, err)
	}
	if err := client.Subscribe(context.Background(), "worker", "chat:telegram:worker"); err != nil {
		t.Fatal(err)
	}
	if err := client.Publish(context.Background(), PublishedMessage{Channel: "chat:telegram:worker", Agent: "worker", Text: "hello", UpdateID: 9}); err != nil {
		t.Fatal(err)
	}
	if !published {
		t.Fatal("message was not published")
	}
	if info, err := os.Stat(socket); err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket missing: %v", err)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPublishRejectsMissingAgentDelivery(t *testing.T) {
	client := &DaemonClient{token: "token", http: &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"delivered_agents":[]}}`)), Header: make(http.Header)}, nil
	})}}
	if err := client.Publish(context.Background(), PublishedMessage{Channel: "chat:telegram:worker", Agent: "worker", Text: "hello"}); err == nil {
		t.Fatal("publish without an agent delivery succeeded")
	}
}
