package telegramplugin

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
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
	_ = s.ReconcileTopics(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.ReconcileTopics(ctx)
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
	MessageID       int64         `json:"message_id"`
	MessageThreadID int64         `json:"message_thread_id,omitempty"`
	From            *TelegramUser `json:"from,omitempty"`
	Chat            TelegramChat  `json:"chat"`
	Text            string        `json:"text,omitempty"`
}

type TelegramUser struct {
	ID int64 `json:"id"`
}

type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
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
	if agent == "" || message.Text == "" || s.daemon == nil {
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
	if handled, err := s.handleCommand(ctx, message, agent, update.UpdateID); err != nil {
		return err
	} else if handled {
		return discard()
	}
	if err := s.daemon.Publish(ctx, PublishedMessage{
		Channel: "chat:telegram:" + agent, Text: message.Text,
		UpdateID: update.UpdateID, ExternalID: "telegram:" + strconv.FormatInt(update.UpdateID, 10),
	}); err != nil {
		return err
	}
	return discard()
}
