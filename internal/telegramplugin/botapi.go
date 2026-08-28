package telegramplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBotAPIBase = "https://api.telegram.org"

type BotClient struct {
	base string
	http *http.Client
}

type BotUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Chat struct {
	ID      int64  `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	IsForum bool   `json:"is_forum"`
}

type ChatMember struct {
	Status          string `json:"status"`
	CanManageTopics bool   `json:"can_manage_topics"`
}

type ForumTopic struct {
	MessageThreadID int64  `json:"message_thread_id"`
	Name            string `json:"name"`
}

type SentMessage struct {
	MessageID int64 `json:"message_id"`
}

type BotError struct {
	Method     string
	Code       int
	RetryAfter int
	Reason     string
}

func (e *BotError) Error() string {
	return fmt.Sprintf("telegram %s failed (%d)", e.Method, e.Code)
}

func NewBotClient(base string) *BotClient {
	if base == "" {
		base = defaultBotAPIBase
	}
	return &BotClient{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 40 * time.Second}}
}

func (c *BotClient) GetMe(ctx context.Context, token string) (BotUser, error) {
	var bot BotUser
	err := c.call(ctx, token, "getMe", struct{}{}, &bot)
	return bot, err
}

func (c *BotClient) GetChat(ctx context.Context, token string, chatID int64) (Chat, error) {
	var chat Chat
	err := c.call(ctx, token, "getChat", map[string]any{"chat_id": chatID}, &chat)
	return chat, err
}

func (c *BotClient) GetChatMember(ctx context.Context, token string, chatID, userID int64) (ChatMember, error) {
	var member ChatMember
	err := c.call(ctx, token, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &member)
	return member, err
}

func (c *BotClient) CreateForumTopic(ctx context.Context, token string, chatID int64, name string) (ForumTopic, error) {
	var topic ForumTopic
	err := c.call(ctx, token, "createForumTopic", map[string]any{"chat_id": chatID, "name": name}, &topic)
	return topic, err
}

func (c *BotClient) GetUpdates(ctx context.Context, token string, offset int64) ([]Update, error) {
	updates := []Update{}
	err := c.call(ctx, token, "getUpdates", map[string]any{
		"offset": offset, "timeout": 30, "allowed_updates": []string{"message"},
	}, &updates)
	return updates, err
}

func (c *BotClient) SendMessage(ctx context.Context, token string, chatID, threadID int64, text string) (SentMessage, error) {
	var message SentMessage
	err := c.call(ctx, token, "sendMessage", map[string]any{
		"chat_id": chatID, "message_thread_id": threadID, "text": text,
	}, &message)
	return message, err
}

func (c *BotClient) call(ctx context.Context, token, method string, body, result any) error {
	if token == "" || strings.Contains(token, "/") {
		return &BotError{Method: method, Code: 401}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := c.base + "/bot" + url.PathEscape(token) + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s request failed", method)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("telegram %s response failed", method)
	}
	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("telegram %s returned invalid JSON", method)
	}
	if response.StatusCode/100 != 2 || !envelope.OK {
		code := envelope.ErrorCode
		if code == 0 {
			code = response.StatusCode
		}
		reason := ""
		if strings.Contains(strings.ToLower(envelope.Description), "message thread not found") {
			reason = "message_thread_not_found"
		}
		return &BotError{Method: method, Code: code, RetryAfter: envelope.Parameters.RetryAfter, Reason: reason}
	}
	if result != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, result); err != nil {
			return fmt.Errorf("telegram %s returned invalid result", method)
		}
	}
	return nil
}
