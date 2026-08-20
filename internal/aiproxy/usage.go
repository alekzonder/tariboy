package aiproxy

import (
	"bytes"
	"encoding/json"
)

type openAIUsageResponse struct {
	Model string `json:"model"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
		PromptDetails    struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		InputDetails struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

func (r *openAIUsageResponse) parsedUsage() Usage {
	in, out := r.Usage.PromptTokens, r.Usage.CompletionTokens
	if in == 0 && out == 0 {
		// The Responses API used by Codex reports input/output rather than
		// prompt/completion token names.
		in, out = r.Usage.InputTokens, r.Usage.OutputTokens
	}
	cacheRead := r.Usage.PromptDetails.CachedTokens
	if cacheRead == 0 {
		// The Responses API reports cache reads in input_tokens_details,
		// whereas Chat Completions uses prompt_tokens_details.
		cacheRead = r.Usage.InputDetails.CachedTokens
	}
	cacheWrite := r.Usage.InputDetails.CacheWriteTokens
	in -= cacheRead + cacheWrite
	if in < 0 {
		in = 0
	}
	return Usage{
		InputTokens:      in,
		OutputTokens:     out,
		CacheWriteTokens: cacheWrite,
		CacheReadTokens:  cacheRead,
	}.Normalized()
}

// ParseAnthropicUsage extracts usage + model from a non-streaming /v1/messages
// response. ok=false when the body carries no usage block.
func ParseAnthropicUsage(body []byte) (Usage, string, bool) {
	var r struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens         int `json:"input_tokens"`
			OutputTokens        int `json:"output_tokens"`
			CacheCreationTokens int `json:"cache_creation_input_tokens"`
			CacheReadTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &r) != nil || r.Usage == nil {
		return Usage{}, "", false
	}
	return Usage{
		InputTokens:      r.Usage.InputTokens,
		OutputTokens:     r.Usage.OutputTokens,
		CacheWriteTokens: r.Usage.CacheCreationTokens,
		CacheReadTokens:  r.Usage.CacheReadTokens,
	}.Normalized(), r.Model, true
}

// ParseOpenAIUsage extracts usage + model from a non-streaming
// /v1/chat/completions response.
func ParseOpenAIUsage(body []byte) (Usage, string, bool) {
	var r openAIUsageResponse
	if json.Unmarshal(body, &r) != nil || r.Usage == nil {
		return Usage{}, "", false
	}
	return r.parsedUsage(), r.Model, true
}

// ParseSSEUsage extracts usage from an SSE-framed body when an upstream sends
// the wrong Content-Type. It is used only after ordinary JSON parsing fails.
func ParseSSEUsage(body []byte) (Usage, string, bool) {
	a := &SSEAccumulator{}
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) > 0 && !bytes.Equal(payload, []byte("[DONE]")) {
			a.FeedData(payload)
		}
	}
	u, model := a.Result()
	return u, model, model != "" || u != (Usage{})
}

// SSEAccumulator folds Anthropic and OpenAI streaming responses into a Usage
// total. Anthropic message_delta and OpenAI response.completed usage are
// cumulative, so the last valid event wins.
type SSEAccumulator struct {
	u     Usage
	model string
}

func (a *SSEAccumulator) FeedData(jsonLine []byte) {
	var ev struct {
		Type     string               `json:"type"`
		Response *openAIUsageResponse `json:"response"`
		Message  *struct {
			Model string `json:"model"`
			Usage struct {
				InputTokens         int `json:"input_tokens"`
				OutputTokens        int `json:"output_tokens"`
				CacheCreationTokens int `json:"cache_creation_input_tokens"`
				CacheReadTokens     int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage *struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(jsonLine, &ev) != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		if ev.Message != nil {
			a.model = ev.Message.Model
			a.u.InputTokens = ev.Message.Usage.InputTokens
			a.u.CacheWriteTokens = ev.Message.Usage.CacheCreationTokens
			a.u.CacheReadTokens = ev.Message.Usage.CacheReadTokens
			a.u.OutputTokens = ev.Message.Usage.OutputTokens
		}
	case "message_delta":
		if ev.Usage != nil {
			a.u.OutputTokens = ev.Usage.OutputTokens
		}
	case "response.completed":
		if ev.Response != nil && ev.Response.Model != "" && ev.Response.Usage != nil {
			a.model = ev.Response.Model
			a.u = ev.Response.parsedUsage()
		}
	}
}

func (a *SSEAccumulator) Result() (Usage, string) { return a.u.Normalized(), a.model }
