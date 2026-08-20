package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// MessageDTO is the wire form of a bus.Message the daemon delivers to a
// channel-sink plugin (spec §7.3). Kept here so bus gains no HTTP knowledge.
type MessageDTO struct {
	ID                  string         `json:"id"`
	Channel             string         `json:"channel"`
	TS                  string         `json:"ts"`
	Source              string         `json:"source"`
	Type                string         `json:"type"`
	Subject             map[string]any `json:"subject,omitempty"`
	Text                string         `json:"text,omitempty"`
	Data                map[string]any `json:"data,omitempty"`
	ProducedByAgent     string         `json:"produced_by_agent,omitempty"`
	ProducedInIteration string         `json:"produced_in_iteration,omitempty"`
	ProducedByPlugin    string         `json:"produced_by_plugin,omitempty"`
	// Routing fields (spec §4) — carried so a channel-sink can implement the
	// reply-forwarding contract (§6.4): a kind=reply delivery on the plugin's own
	// channel is mapped back to its external entity via in_reply_to/correlation_id.
	Kind          string `json:"kind,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	InReplyTo     string `json:"in_reply_to,omitempty"`
	ReplyTo       string `json:"reply_to,omitempty"`
	Deadline      string `json:"deadline,omitempty"`
}

// WatchDTO is the wire form of one unit of provider demand on a channel: the
// watch fingerprint, the params that produced it, and the subscribing agents
// (spec §6.2). Carried by both the push (POST /watches) and pull
// (GET /api/plugin/watches) paths so a provider always sees full current state.
type WatchDTO struct {
	Watch       string         `json:"watch"`
	Params      map[string]any `json:"params,omitempty"`
	Subscribers []string       `json:"subscribers"`
}

// ChannelWatchesDTO is the full current watch list for one provided channel.
// It is the POST /watches body, and the element type of the GET
// /api/plugin/watches "channels" array.
type ChannelWatchesDTO struct {
	Channel string     `json:"channel"`
	Watches []WatchDTO `json:"watches"`
}

// Client is the daemon-side HTTP/JSON client of one plugin's unix socket.
type Client struct {
	http *http.Client
	base string
}

// NewClient dials the plugin over its unix socket. The base URL host is a fixed
// placeholder; the transport ignores it and dials socketPath.
func NewClient(socketPath string) *Client {
	return &Client{
		base: "http://plugin",
		http: &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// Health is a liveness probe; any error or non-2xx is a failure.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("plugin /health returned %d", resp.StatusCode)
	}
	return nil
}

// ActionError is a non-2xx response from a plugin's /action, carrying the
// plugin's status and its "error" code so the daemon maps it to a clean error.
type ActionError struct {
	Status int
	Code   string
}

func (e *ActionError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("plugin action failed (%d): %s", e.Status, e.Code)
	}
	return fmt.Sprintf("plugin action failed (%d)", e.Status)
}

// NewClientWithTimeout is NewClient with an explicit per-request timeout (create
// chat may hit a slow upstream, so callers use a longer bound than the 2s default).
func NewClientWithTimeout(socketPath string, timeout time.Duration) *Client {
	c := NewClient(socketPath)
	c.http.Timeout = timeout
	return c
}

// Routes GETs the plugin's /routes (messenger: current chat->channel bindings).
func (c *Client) Routes(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/routes", nil)
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

// Action POSTs a body to the plugin's /action and returns the parsed response.
// A non-2xx is returned as *ActionError (with the parsed "error" code).
func (c *Client) Action(ctx context.Context, body map[string]any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/action", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doJSON(req)
}

func (c *Client) doJSON(req *http.Request) (map[string]any, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if resp.StatusCode/100 != 2 {
		code := ""
		if out != nil {
			code, _ = out["error"].(string)
		}
		return out, &ActionError{Status: resp.StatusCode, Code: code}
	}
	return out, nil
}

// PushWatches POSTs the full current watch list for a provided channel to the
// plugin's /watches (spec §6.2). A non-2xx is an error so the daemon can retry.
func (c *Client) PushWatches(ctx context.Context, channel string, watches []WatchDTO) error {
	body, err := json.Marshal(ChannelWatchesDTO{Channel: channel, Watches: watches})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/watches", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("plugin /watches returned %d", resp.StatusCode)
	}
	return nil
}

// Deliver POSTs {message:...} to the plugin's /deliver (channel-sink). A non-2xx
// is a typed error so the drainer can leave the message unacked for redelivery.
func (c *Client) Deliver(ctx context.Context, msg MessageDTO) error {
	body, err := json.Marshal(map[string]any{"message": msg})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/deliver", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("plugin /deliver returned %d", resp.StatusCode)
	}
	return nil
}
