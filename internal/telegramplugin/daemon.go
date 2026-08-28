package telegramplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type AgentInfo struct {
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

type PublishedMessage struct {
	Channel    string
	Text       string
	UpdateID   int64
	ExternalID string
}

type DaemonAPI interface {
	ListAgents(context.Context) ([]AgentInfo, error)
	Subscribe(context.Context, string, string) error
	Publish(context.Context, PublishedMessage) error
	Call(context.Context, string, string, any, any) error
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

func (c *DaemonClient) Subscribe(ctx context.Context, agent, channel string) error {
	return c.Call(ctx, http.MethodPost, "/api/agents/"+url.PathEscape(agent)+"/subscriptions", map[string]any{
		"name": agent, "channel": channel,
	}, nil)
}

func (c *DaemonClient) Publish(ctx context.Context, message PublishedMessage) error {
	body := map[string]any{
		"channel": message.Channel, "type": "chat.message", "text": message.Text,
		"data": map[string]any{"telegram_update_id": message.UpdateID, "external_id": message.ExternalID},
	}
	return c.do(ctx, http.MethodPost, "/api/plugin/publish", body, nil, true)
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
			return fmt.Errorf("daemon %s: %s", envelope.Error.Code, envelope.Error.Message)
		}
		return fmt.Errorf("daemon request failed (%d)", response.StatusCode)
	}
	if result != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}
