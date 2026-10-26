package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/testutil"
	"github.com/coder/websocket"
)

func messagesWSURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + "/api/mail/ws"
}

func messagesWSServer(t *testing.T) (*httptest.Server, *events.Hub) {
	t.Helper()
	hub := events.NewHub()
	server := NewServer(registry.New(), &registry.Ctx{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	server.SetEventSource(hub)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return httpServer, hub
}

// One socket per host carries a hint for every letter, so a mailbox refetches
// immediately instead of polling.
func TestMailWebSocketStreamsLetterHints(t *testing.T) {
	t.Parallel()
	httpServer, hub := messagesWSServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, messagesWSURL(httpServer.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	// The handler subscribes after the handshake completes, so keep emitting
	// until one frame is read rather than racing the accept with one event.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			hub.Emit(events.Event{Agent: "worker", Type: "mail", Time: "t", Data: map[string]any{
				"id": "m1", "from": "agent:worker", "to": "user:customer", "kind": "task.question",
			}})
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	_, raw, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var hint map[string]any
	if err := json.Unmarshal(raw, &hint); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"id": "m1", "from": "agent:worker", "to": "user:customer", "kind": "task.question", "ts": "t"}
	if len(hint) != len(want) {
		t.Fatalf("hint = %#v; want exactly %#v", hint, want)
	}
	for k, v := range want {
		if hint[k] != v {
			t.Fatalf("hint = %#v; want %#v", hint, want)
		}
	}
}

func TestMailWebSocketWatchesEveryAgentAndOnlyMail(t *testing.T) {
	t.Parallel()
	recorder := &recordingEvents{}
	server := NewServer(registry.New(), &registry.Ctx{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	server.SetEventSource(recorder)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, messagesWSURL(httpServer.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	testutil.WaitFor(3*time.Second, func() bool { return recorder.calls() > 0 })
	agent, types := recorder.subscription()
	if agent != "" || len(types) != 1 || types[0] != "mail" {
		t.Fatalf("subscribed as agent %q with types %v; want every agent and [mail]", agent, types)
	}
}

type recordingEvents struct {
	mu    sync.Mutex
	agent string
	types []string
	count int
}

func (r *recordingEvents) Subscribe(agent string, types []string) (<-chan events.Event, func()) {
	r.mu.Lock()
	r.agent, r.types, r.count = agent, types, r.count+1
	r.mu.Unlock()
	return make(chan events.Event), func() {}
}

func (r *recordingEvents) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func (r *recordingEvents) subscription() (string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.agent, r.types
}

func TestMailWebSocketRejectsUntrustedOriginWithoutBearer(t *testing.T) {
	t.Parallel()
	httpServer, _ := messagesWSServer(t)
	_, response, err := websocket.Dial(context.Background(), messagesWSURL(httpServer.URL),
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil {
		t.Fatal("untrusted tokenless origin unexpectedly opened the mail websocket")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("response = %#v, want HTTP 403", response)
	}
}
