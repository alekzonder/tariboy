package telegramplugin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/plugins"
)

func (s *Server) deliver(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message plugins.MessageDTO `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_delivery"})
		return
	}
	message := body.Message
	if message.ProducedByAgent == "" || message.ProducedByPlugin == "telegram" {
		writeJSON(w, http.StatusOK, map[string]any{"delivered": false})
		return
	}
	agent := strings.TrimPrefix(message.Channel, "chat:telegram:")
	if agent == message.Channel || agent != message.ProducedByAgent {
		writeJSON(w, http.StatusOK, map[string]any{"delivered": false})
		return
	}
	state := s.state.Snapshot()
	topic, ok := state.AgentTopics[agent]
	if !ok || state.Token == "" || state.ChatID == 0 || s.bot == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "telegram_not_configured"})
		return
	}
	for _, part := range splitTelegramText(message.Text) {
		if _, err := s.bot.SendMessage(r.Context(), state.Token, state.ChatID, topic.ThreadID, part); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "telegram_send_failed"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivered": true})
}

func splitTelegramText(text string) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return []string{""}
	}
	parts := make([]string, 0, (len(runes)+4095)/4096)
	for len(runes) > 0 {
		end := min(4096, len(runes))
		parts = append(parts, string(runes[:end]))
		runes = runes[end:]
	}
	return parts
}
