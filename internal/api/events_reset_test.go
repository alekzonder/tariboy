package api

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/coder/websocket"
)

// The SSE stream names every event by its hub sequence and forwards the
// hub reset marker, so a client can tell it missed events.
func TestSSEStreamWritesSeqIDAndReset(t *testing.T) {
	src := &fakeEventSource{ch: make(chan events.Event, 2)}
	srv := NewServer(registry.New(), &registry.Ctx{Version: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	srv.SetEventSource(src)

	req := httptest.NewRequest("GET", "/api/agents/smoke/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { srv.Handler().ServeHTTP(rr, req); close(done) }()
	src.ch <- events.Event{Seq: 8, Agent: "smoke", Type: "reset", Data: map[string]any{"dropped": 3}}
	src.ch <- events.Event{Seq: 8, Agent: "smoke", Type: "message"}
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
	body := rr.Body.String()
	if !strings.Contains(body, "id: 8\nevent: reset\n") || !strings.Contains(body, "id: 8\nevent: message\n") {
		t.Fatalf("SSE body = %q", body)
	}
}

// A hint socket replays nothing; the client refetches on every reconnect. So
// when the hub reports dropped events the server closes the socket, and the
// reconnect refetch recovers what was missed.
func TestHintSocketClosesOnReset(t *testing.T) {
	src := &fakeEventSource{ch: make(chan events.Event, 1)}
	server := NewServer(registry.New(), &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	server.SetEventSource(src)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, messagesWSURL(httpServer.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	src.ch <- events.Event{Seq: 2, Agent: "worker", Type: "reset", Data: map[string]any{"dropped": 1}}
	_, _, err = ws.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusTryAgainLater {
		t.Fatalf("read after reset = %v, want close %v", err, websocket.StatusTryAgainLater)
	}
}
