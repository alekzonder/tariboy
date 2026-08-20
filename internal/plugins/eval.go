package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ErrNoEvalPlugin means no installed plugin declares the "eval" type and is
// currently running. The eval runner records an "error" verdict rather than
// crashing the iteration.
var ErrNoEvalPlugin = errors.New("no running eval plugin")

// EvalRequestDTO is the daemon->plugin /evaluate request (protocol v1). For
// llm-judge, ProxyBaseURL+ProxyToken let the plugin make ONE accounted AI call
// through the in-daemon proxy; the real upstream key never leaves the daemon.
type EvalRequestDTO struct {
	Iteration    string `json:"iteration"`
	Agent        string `json:"agent"`
	ImageName    string `json:"image_name"`
	ImageTag     string `json:"image_tag"`
	ImageDigest  string `json:"image_digest"`
	EvalName     string `json:"eval_name"`
	EvalType     string `json:"eval_type"` // llm-judge | script
	Prompt       string `json:"prompt"`    // criteria text (llm-judge) or script body (script)
	Status       string `json:"status"`    // iteration outcome
	Workdir      string `json:"workdir"`   // agent workdir (script checks run here)
	ProxyBaseURL string `json:"proxy_base_url,omitempty"`
	ProxyToken   string `json:"proxy_token,omitempty"`
	Model        string `json:"model,omitempty"`
}

// EvalVerdictDTO is the plugin's reply.
type EvalVerdictDTO struct {
	Verdict string  `json:"verdict"` // pass | fail | error
	Score   float64 `json:"score"`
	Detail  string  `json:"detail"`
}

// NewEvalClient is NewClient with a long timeout: an llm-judge eval calls an LLM
// through the proxy, which the 2s default would abort.
func NewEvalClient(socketPath string) *Client {
	return &Client{
		base: "http://plugin",
		http: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// Evaluate POSTs the request to the plugin's /evaluate and decodes the verdict.
func (c *Client) Evaluate(ctx context.Context, req EvalRequestDTO) (EvalVerdictDTO, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return EvalVerdictDTO{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/evaluate", bytes.NewReader(body))
	if err != nil {
		return EvalVerdictDTO{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return EvalVerdictDTO{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return EvalVerdictDTO{}, fmt.Errorf("plugin /evaluate returned %d", resp.StatusCode)
	}
	var v EvalVerdictDTO
	if err := json.Unmarshal(raw, &v); err != nil {
		return EvalVerdictDTO{}, fmt.Errorf("decode eval verdict: %w", err)
	}
	return v, nil
}

// Evaluate dispatches to a running eval-type plugin (spec §7.3). Mirrors the
// channel-sink dispatch: find the plugin by type, dial its socket on demand.
func (h *Host) Evaluate(ctx context.Context, req EvalRequestDTO) (EvalVerdictDTO, error) {
	name, ok := h.findEvalPlugin()
	if !ok {
		return EvalVerdictDTO{}, ErrNoEvalPlugin
	}
	return NewEvalClient(h.SocketPath(name)).Evaluate(ctx, req)
}

// findEvalPlugin returns the name of a running plugin that declares "eval".
func (h *Host) findEvalPlugin() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for name, rp := range h.running {
		if !manifestFromRecord(rp.rec).HasType("eval") {
			continue
		}
		rp.mu.Lock()
		state := rp.state
		rp.mu.Unlock()
		if state == "running" {
			return name, true
		}
	}
	return "", false
}
