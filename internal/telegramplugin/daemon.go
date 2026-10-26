package telegramplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type AgentInfo struct {
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

// InboundLetter is one topic message the plugin sends to the topic's agent.
type InboundLetter struct {
	Agent      string
	Text       string
	UpdateID   int64
	ExternalID string
	Forward    map[string]any
}

type DaemonAPI interface {
	ListAgents(context.Context) ([]AgentInfo, error)
	Send(context.Context, InboundLetter) error
	Call(context.Context, string, string, any, any) error
}

// DaemonError is an error answer of the daemon with its code.
type DaemonError struct {
	Code    string
	Message string
}

func (e *DaemonError) Error() string { return "daemon " + e.Code + ": " + e.Message }

// IsInboxFull reports whether err is the daemon refusing a letter because the
// recipient's inbox is full.
func IsInboxFull(err error) bool {
	var de *DaemonError
	return errors.As(err, &de) && de.Code == "inbox_full"
}

type DaemonClient struct {
	token string
	http  *http.Client
}

func NewDaemonClient(socket, token string) *DaemonClient {
	return &DaemonClient{token: token, http: &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		}},
	}}
}

func (c *DaemonClient) ListAgents(ctx context.Context) ([]AgentInfo, error) {
	var result struct {
		Agents []AgentInfo `json:"agents"`
	}
	if err := c.Call(ctx, http.MethodGet, "/api/agents", nil, &result); err != nil {
		return nil, err
	}
	return result.Agents, nil
}

// Send sends a topic message to agent:<Agent> as a request from
// plugin:telegram: the customer wrote and expects an answer.
func (c *DaemonClient) Send(ctx context.Context, letter InboundLetter) error {
	data := map[string]any{"telegram_update_id": letter.UpdateID}
	if len(letter.Forward) > 0 {
		data["telegram_forward"] = letter.Forward
	}
	body := map[string]any{
		"to": "agent:" + letter.Agent, "subject": letterSubject(letter.Text), "text": letter.Text,
		"kind": "message", "idempotency_key": letter.ExternalID, "data": data,
	}
	return c.do(ctx, http.MethodPost, "/api/plugin/mail", body, nil, true)
}

// letterSubject is the first line of text, cut to 60 characters.
func letterSubject(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if runes := []rune(line); len(runes) > 60 {
		line = string(runes[:60]) + "…"
	}
	return "Telegram: " + line
}

func (c *DaemonClient) Call(ctx context.Context, method, path string, body, result any) error {
	return c.do(ctx, method, path, body, result, false)
}

func (c *DaemonClient) do(ctx context.Context, method, path string, body, result any, pluginAuth bool) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://daemon"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if pluginAuth {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("daemon returned invalid JSON")
	}
	if response.StatusCode/100 != 2 || !envelope.OK {
		if envelope.Error != nil {
			return &DaemonError{Code: envelope.Error.Code, Message: envelope.Error.Message}
		}
		return fmt.Errorf("daemon request failed (%d)", response.StatusCode)
	}
	if result != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}
