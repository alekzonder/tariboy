package telegramplugin

import (
	"github.com/alekzonder/tariboy/internal/tidy"

	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) Run(ctx context.Context) {
	go s.reconcileLoop(ctx)
	backoff := time.Second
	for ctx.Err() == nil {
		state := s.state.Snapshot()
		if state.Token == "" || s.bot == nil {
			if !waitContext(ctx, time.Second) {
				return
			}
			continue
		}
		updates, err := s.bot.GetUpdates(ctx, state.Token, state.Offset)
		if err != nil {
			var botErr *BotError
			if errors.As(err, &botErr) && botErr.Code == http.StatusTooManyRequests && botErr.RetryAfter > 0 {
				if !waitContext(ctx, time.Duration(botErr.RetryAfter)*time.Second) {
					return
				}
				continue
			}
			if !waitContext(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for _, update := range updates {
			if err := s.ProcessUpdate(ctx, update); err != nil {
				break
			}
		}
	}
}

func (s *Server) reconcileLoop(ctx context.Context) {
	tidy.Log("best effort: s.ReconcileTopics", s.ReconcileTopics(ctx))
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tidy.Log("best effort: s.ReconcileTopics", s.ReconcileTopics(ctx))
		}
	}
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type Update struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

type TelegramMessage struct {
	MessageID       int64          `json:"message_id"`
	MessageThreadID int64          `json:"message_thread_id,omitempty"`
	From            *TelegramUser  `json:"from,omitempty"`
	Chat            TelegramChat   `json:"chat"`
	Text            string         `json:"text,omitempty"`
	Caption         string         `json:"caption,omitempty"`
	ForwardOrigin   *MessageOrigin `json:"forward_origin,omitempty"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type TelegramChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// MessageOrigin is the Bot API forward_origin of a forwarded message.
type MessageOrigin struct {
	Type            string        `json:"type"`
	Date            int64         `json:"date"`
	SenderUser      *TelegramUser `json:"sender_user,omitempty"`
	SenderUserName  string        `json:"sender_user_name,omitempty"`
	SenderChat      *TelegramChat `json:"sender_chat,omitempty"`
	Chat            *TelegramChat `json:"chat,omitempty"`
	MessageID       int64         `json:"message_id,omitempty"`
	AuthorSignature string        `json:"author_signature,omitempty"`
}

// describe returns the header line shown to the agent and the
// telegram_forward data describing where a forwarded message came from.
func (o *MessageOrigin) describe() (string, map[string]any) {
	data := map[string]any{"type": o.Type, "date": o.Date}
	name, username, label := "", "", strings.ReplaceAll(o.Type, "_", " ")
	switch {
	case o.SenderUser != nil:
		name = strings.TrimSpace(o.SenderUser.FirstName + " " + o.SenderUser.LastName)
		username = o.SenderUser.Username
		data["user_id"] = o.SenderUser.ID
	case o.SenderUserName != "":
		name = o.SenderUserName
	default:
		chat := o.SenderChat
		if chat == nil {
			chat = o.Chat
		}
		if chat != nil {
			name, username = chat.Title, chat.Username
			data["chat_id"] = chat.ID
		}
	}
	header := "Forwarded from " + label
	if name != "" {
		data["name"] = name
		header += " " + strconv.Quote(name)
	}
	if username != "" {
		data["username"] = username
		header += " (@" + username + ")"
	}
	if o.MessageID != 0 {
		data["message_id"] = o.MessageID
		if o.Type == "channel" && username != "" {
			link := "https://t.me/" + username + "/" + strconv.FormatInt(o.MessageID, 10)
			data["link"] = link
			header += " " + link
		}
	}
	if o.AuthorSignature != "" {
		data["author_signature"] = o.AuthorSignature
	}
	return header + ":", data
}

func (s *Server) ProcessUpdate(ctx context.Context, update Update) error {
	state := s.state.Snapshot()
	if update.UpdateID < state.Offset {
		return nil
	}
	discard := func() error { return s.state.SetOffset(update.UpdateID + 1) }
	message := update.Message
	if len(state.AllowedUIDs) == 0 || message == nil || message.From == nil {
		return discard()
	}
	index := sort.Search(len(state.AllowedUIDs), func(i int) bool { return state.AllowedUIDs[i] >= message.From.ID })
	if index == len(state.AllowedUIDs) || state.AllowedUIDs[index] != message.From.ID || message.Chat.ID != state.ChatID {
		return discard()
	}
	if message.MessageThreadID == state.ManagementTopicID {
		if message.ForwardOrigin != nil {
			return discard()
		}
		handled, err := s.handleCommand(ctx, message, "", update.UpdateID)
		if err != nil {
			return err
		}
		if handled {
			return discard()
		}
		return discard()
	}
	agent := ""
	for _, topic := range state.AgentTopics {
		if topic.ThreadID == message.MessageThreadID {
			agent = topic.Name
			break
		}
	}
	text := message.Text
	if text == "" {
		text = message.Caption
	}
	if agent == "" || text == "" || s.daemon == nil {
		return discard()
	}
	agents, err := s.daemon.ListAgents(ctx)
	if err != nil {
		return err
	}
	live := false
	for _, candidate := range agents {
		if candidate.Name == agent {
			live = true
			break
		}
	}
	if !live {
		return discard()
	}
	var forward map[string]any
	if message.ForwardOrigin != nil {
		// Forwarded text was written by someone else and never runs as a command.
		var header string
		header, forward = message.ForwardOrigin.describe()
		text = header + "\n" + text
	} else if handled, err := s.handleCommand(ctx, message, agent, update.UpdateID); err != nil {
		return err
	} else if handled {
		return discard()
	}
	if err := s.daemon.Publish(ctx, PublishedMessage{
		Channel: "chat:telegram:" + agent, Agent: agent, Text: text, Forward: forward,
		UpdateID: update.UpdateID, ExternalID: "telegram:" + strconv.FormatInt(update.UpdateID, 10),
	}); err != nil {
		return err
	}
	return discard()
}
