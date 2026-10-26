package telegramplugin

import (
	"context"
	"encoding/json"
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
		case "/api/plugin/mail":
			if r.Header.Get("Authorization") != "Bearer life-token" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			var body struct {
				To             string         `json:"to"`
				Subject        string         `json:"subject"`
				Text           string         `json:"text"`
				Notice         bool           `json:"notice"`
				IdempotencyKey string         `json:"idempotency_key"`
				Data           map[string]any `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.To != "agent:worker" || body.Text != "hello" || body.Subject != "Telegram: hello" || body.Notice || body.IdempotencyKey != "telegram:9" {
				t.Fatalf("letter = %#v", body)
			}
			if forward, ok := body.Data["telegram_forward"].(map[string]any); !ok || forward["name"] != "News" || body.Data["telegram_update_id"] != float64(9) {
				t.Fatalf("letter data = %#v", body.Data)
			}
			published = true
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"letters": []any{}}})
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
	if err := client.Send(context.Background(), InboundLetter{Agent: "worker", Text: "hello", UpdateID: 9, ExternalID: "telegram:9", Forward: map[string]any{"type": "channel", "name": "News"}}); err != nil {
		t.Fatal(err)
	}
	if !published {
		t.Fatal("letter was not sent")
	}
	if info, err := os.Stat(socket); err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket missing: %v", err)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendReportsInboxFull(t *testing.T) {
	client := &DaemonClient{token: "token", http: &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error":{"code":"inbox_full","message":"full"}}`)), Header: make(http.Header)}, nil
	})}}
	if err := client.Send(context.Background(), InboundLetter{Agent: "worker", Text: "hello"}); !IsInboxFull(err) {
		t.Fatalf("send to a full inbox = %v; want inbox_full", err)
	}
}
