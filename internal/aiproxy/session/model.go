package session

import "encoding/json"

// Block is one piece of a message or response, provider-agnostic.
type Block struct {
	Type      string          `json:"type"` // text | thinking | tool_use | tool_result
	Text      string          `json:"text,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type Message struct {
	Role   string  `json:"role"`
	Blocks []Block `json:"blocks"`
}

type Response struct {
	Blocks     []Block `json:"blocks"`
	StopReason string  `json:"stop_reason,omitempty"`
}

type Usage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
}

// Call is one LLM exchange in an iteration, normalized for the audit UI.
type Call struct {
	Seq                 int       `json:"seq"`
	Ts                  string    `json:"ts"`
	Provider            string    `json:"provider"`
	Model               string    `json:"model"`
	Usage               Usage     `json:"usage"`
	CostUSD             float64   `json:"cost_usd"`
	LatencyMs           int       `json:"latency_ms"`
	Status              string    `json:"status"`
	Instructions        string    `json:"instructions"`
	InstructionsChanged bool      `json:"instructions_changed"`
	Delta               []Message `json:"delta"`
	Response            Response  `json:"response"`
	Truncated           bool      `json:"truncated"`
	ParseError          string    `json:"parse_error,omitempty"`
}

type SessionTimeline struct {
	Calls []Call `json:"calls"`
}
