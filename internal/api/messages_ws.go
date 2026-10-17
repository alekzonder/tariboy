package api

import (
	"context"
	"net/http"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/coder/websocket"
)

// serveMessages streams live message hints for every agent on this host over one
// socket, so a chat list and an open conversation both refresh the moment
// something is published instead of polling. A frame is only a hint: the client
// refetches over HTTP, which stays authoritative. Nothing is replayed, because a
// client refetches on connect and on every reconnect anyway.
func (s *Server) serveMessages(w http.ResponseWriter, r *http.Request) {
	// "message" excludes terminal stream bytes, which are not conversation.
	s.serveHints(w, r, []string{"message"}, func(event events.Event) map[string]any {
		hint := map[string]any{"agent": event.Agent, "ts": event.Time}
		for _, key := range []string{"id", "channel", "chat", "type", "from"} {
			if value, present := event.Data[key]; present {
				hint[key] = value
			}
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

// serveHints forwards every agent's events of the given types as hint frames.
func (s *Server) serveHints(w http.ResponseWriter, r *http.Request, types []string, hint func(events.Event) map[string]any) {
	// The tokenless loopback listener relies on the browser Origin boundary,
	// exactly as the Tasks socket does.
	if !requestAuthenticated(r) {
		if origin := r.Header.Get("Origin"); origin != "" && !isAllowedWebOrigin(origin) {
			WriteErr(w, http.StatusForbidden, "forbidden_origin", "websocket origin is not allowed")
			return
		}
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
