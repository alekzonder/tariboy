package api

import (
	"context"
	"net/http"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/operations"
	"github.com/coder/websocket"
)

// serveMail streams one hint per written letter, {id, from, to, kind, ts}, so
// a mailbox refreshes the moment a letter is sent instead of polling. A frame
// is only a hint: the client refetches over HTTP, which stays authoritative.
// Nothing is replayed, because a client refetches on connect and on every
// reconnect anyway.
func (s *Server) serveMail(w http.ResponseWriter, r *http.Request) {
	s.serveHints(w, r, []string{"mail"}, func(event events.Event) map[string]any {
		hint := map[string]any{"ts": event.Time}
		for _, key := range []string{"id", "from", "to", "kind"} {
			hint[key] = event.Data[key]
		}
		return hint
	})
}

// serveAgentStates streams a hint naming the agent whenever something that can
// change its state happens: an iteration event, or a "state" event from start,
// stop, a status line, a halt or an idle stop. The agents
// list and the agent status refetch on a hint, so they can poll slowly.
func (s *Server) serveAgentStates(w http.ResponseWriter, r *http.Request) {
	s.serveHints(w, r, []string{"iteration", "state"}, func(event events.Event) map[string]any {
		return map[string]any{"agent": event.Agent, "type": event.Type, "ts": event.Time}
	})
}

// serveOperations streams a hint, {id, status, ts}, whenever an operation
// starts, writes a log line or ends. An operation page refetches the
// operation and its log from the last offset on a hint for its id.
func (s *Server) serveOperations(w http.ResponseWriter, r *http.Request) {
	s.serveHints(w, r, []string{operations.EventType}, func(event events.Event) map[string]any {
		return map[string]any{"id": event.Data["id"], "status": event.Data["status"], "ts": event.Time}
	})
}

// serveHints forwards every agent's events of the given types as hint frames.
func (s *Server) serveHints(w http.ResponseWriter, r *http.Request, types []string, hint func(events.Event) map[string]any) {
	if rejectUntrustedOrigin(w, r) {
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ctx := ws.CloseRead(r.Context())
	// An empty agent watches every agent.
	stream, cancel := s.events.Subscribe("", types)
	defer cancel()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-stream:
			if !ok {
				return
			}
			if event.Type == "reset" {
				// The hub dropped events for this socket. The client refetches
				// on every reconnect, so closing recovers what it missed.
				ws.Close(websocket.StatusTryAgainLater, "events dropped")
				return
			}
			if err := ws.Write(ctx, websocket.MessageText, mustJSON(hint(event))); err != nil {
				return
			}
		case <-ping.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			err := ws.Ping(pingCtx)
			pingCancel()
			if err != nil {
				return
			}
		}
	}
}
