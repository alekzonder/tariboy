package api

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// serveMessages streams live message hints for every agent on this host over one
// socket, so a chat list and an open conversation both refresh the moment
// something is published instead of polling. A frame is only a hint: the client
// refetches over HTTP, which stays authoritative. Nothing is replayed, because a
// client refetches on connect and on every reconnect anyway.
func (s *Server) serveMessages(w http.ResponseWriter, r *http.Request) {
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
	// An empty agent watches every agent; "message" excludes terminal stream
	// bytes, which are not conversation.
	stream, cancel := s.events.Subscribe("", []string{"message"})
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
			hint := map[string]any{"agent": event.Agent, "ts": event.Time}
			for _, key := range []string{"id", "channel", "chat", "type", "from"} {
				if value, present := event.Data[key]; present {
					hint[key] = value
				}
			}
			if err := ws.Write(ctx, websocket.MessageText, mustJSON(hint)); err != nil {
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
