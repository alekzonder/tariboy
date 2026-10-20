package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/coder/websocket"
)

// One socket per host tells the UI which agent changed state, so the agent
// list and status refresh at once instead of on a short poll. Terminal output
// and messages are not state changes and are not forwarded.
func TestAgentStatesWebSocketStreamsStateHints(t *testing.T) {
	t.Parallel()
	httpServer, hub := messagesWSServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/agent-states/ws"
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	// Emit until a frame arrives: the handler subscribes after the handshake.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			hub.Emit(events.Event{Agent: "noise", Type: "message", Time: "t", Data: map[string]any{"id": "m1"}})
			hub.Emit(events.Event{Agent: "worker", Type: "iteration", Time: "t", Data: map[string]any{"status": "running"}})
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	for i := 0; i < 3; i++ {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var hint map[string]any
		if err := json.Unmarshal(raw, &hint); err != nil {
			t.Fatal(err)
		}
		if hint["agent"] != "worker" || hint["type"] != "iteration" {
			t.Fatalf("hint = %#v", hint)
		}
	}
}
