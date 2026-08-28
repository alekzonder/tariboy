package telegramplugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

type Server struct {
	state  *StateStore
	bot    BotAPI
	daemon DaemonAPI
}

type BotAPI interface {
	GetMe(context.Context, string) (BotUser, error)
	GetChat(context.Context, string, int64) (Chat, error)
	GetChatMember(context.Context, string, int64, int64) (ChatMember, error)
	CreateForumTopic(context.Context, string, int64, string) (ForumTopic, error)
	GetUpdates(context.Context, string, int64) ([]Update, error)
	SendMessage(context.Context, string, int64, int64, string) (SentMessage, error)
}

func NewServer(state *StateStore, bot BotAPI, daemon ...DaemonAPI) *Server {
	server := &Server{state: state, bot: bot}
	if len(daemon) > 0 {
		server.daemon = daemon[0]
	}
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodGet && r.URL.Path == "/routes":
		writeJSON(w, http.StatusOK, map[string]any{"routes": s.routes(), "status": s.status()})
	case r.Method == http.MethodPost && r.URL.Path == "/action":
		s.action(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/deliver":
		s.deliver(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
	}
}

func (s *Server) routes() map[string]any {
	state := s.state.Snapshot()
	routes := make(map[string]any, len(state.AgentTopics))
	for _, topic := range state.AgentTopics {
		routes["chat:telegram:"+topic.Name] = map[string]any{"chat_id": state.ChatID, "thread_id": topic.ThreadID}
	}
	return routes
}

func (s *Server) status() map[string]any {
	state := s.state.Snapshot()
	return map[string]any{
		"token_configured":    state.Token != "",
		"allowlist_count":     len(state.AllowedUIDs),
		"chat_id":             state.ChatID,
		"management_topic_id": state.ManagementTopicID,
	}
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	action, _ := body["action"].(string)
	if action == "status" {
		writeJSON(w, http.StatusOK, map[string]any{"result": s.status()})
		return
	}
	if action == "configure" {
		token, _ := body["token"].(string)
		if token != "" {
			if s.bot == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bot_api_unavailable"})
				return
			}
			if _, err := s.bot.GetMe(r.Context(), token); err != nil {
				var botErr *BotError
				if errors.As(err, &botErr) && botErr.Code == http.StatusUnauthorized {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_token"})
				} else {
					writeJSON(w, http.StatusBadGateway, map[string]any{"error": "telegram_unavailable"})
				}
				return
			}
		}
		allowed, err := int64List(body["allowed_uids"])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_allowed_uids"})
			return
		}
		if err := s.state.Configure(token, allowed); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": s.status()})
		return
	}
	if action == "chat_setup" {
		s.chatSetup(w, r, body)
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown_action"})
}

func (s *Server) chatSetup(w http.ResponseWriter, r *http.Request, body map[string]any) {
	chatID, err := int64Value(body["chat_id"])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_chat_id"})
		return
	}
	state := s.state.Snapshot()
	if state.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "token_required"})
		return
	}
	if len(state.AllowedUIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "allowed_uids_required"})
		return
	}
	chat, err := s.bot.GetChat(r.Context(), state.Token, chatID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "telegram_unavailable"})
		return
	}
	if chat.Type != "supergroup" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chat_must_be_supergroup"})
		return
	}
	if !chat.IsForum {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "topics_must_be_enabled"})
		return
	}
	bot, err := s.bot.GetMe(r.Context(), state.Token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "telegram_unavailable"})
		return
	}
	member, err := s.bot.GetChatMember(r.Context(), state.Token, chatID, bot.ID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "telegram_unavailable"})
		return
	}
	if member.Status != "creator" && (member.Status != "administrator" || !member.CanManageTopics) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bot_must_manage_topics"})
		return
	}
	managementID := state.ManagementTopicID
	if state.ChatID != chatID || managementID == 0 {
		topic, err := s.bot.CreateForumTopic(r.Context(), state.Token, chatID, "tariboyd")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "create_topic_failed"})
			return
		}
		managementID = topic.MessageThreadID
	}
	if err := s.state.BindChat(chatID, managementID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save_failed"})
		return
	}
	if err := s.ReconcileTopics(r.Context()); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "create_agent_topics_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": s.status()})
}

func int64List(value any) ([]int64, error) {
	if value == nil {
		return []int64{}, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, errors.New("not a list")
	}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		switch value := value.(type) {
		case json.Number:
			n, err := value.Int64()
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		case string:
			n, err := json.Number(value).Int64()
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		default:
			return nil, errors.New("not an integer")
		}
	}
	return out, nil
}

func int64Value(value any) (int64, error) {
	switch value := value.(type) {
	case json.Number:
		return value.Int64()
	case string:
		return json.Number(value).Int64()
	case int64:
		return value, nil
	default:
		return 0, errors.New("not an integer")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
