package session

import "encoding/json"

// parseAnthropicRequest extracts the system prompt and the message list from an
// Anthropic Messages API request body. `system` may be a string or an array of
// text blocks; `content` may be a string or an array of typed blocks.
func parseAnthropicRequest(body []byte) (string, []Message, error) {
	var req struct {
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", nil, err
	}
	instructions := anthropicSystemText(req.System)
	var msgs []Message
	for _, m := range req.Messages {
		msgs = append(msgs, Message{Role: m.Role, Blocks: anthropicContentBlocks(m.Content)})
	}
	return instructions, msgs, nil
}

func anthropicSystemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var arr []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &arr) == nil {
		out := ""
		for _, b := range arr {
			out += b.Text
		}
		return out
	}
	return ""
}

// anthropicContentBlocks normalizes a message's content (string or block array).
func anthropicContentBlocks(raw json.RawMessage) []Block {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []Block{{Type: "text", Text: s}}
	}
	var arr []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Thinking  string          `json:"thinking"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
	}
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	var out []Block
	for _, b := range arr {
		switch b.Type {
		case "text":
			out = append(out, Block{Type: "text", Text: b.Text})
		case "thinking":
			out = append(out, Block{Type: "thinking", Text: b.Thinking})
		case "tool_use":
			out = append(out, Block{Type: "tool_use", ToolName: b.Name, Input: b.Input, ToolUseID: b.ID})
		case "tool_result":
			out = append(out, Block{Type: "tool_result", ToolUseID: b.ToolUseID, Text: rawText(b.Content), IsError: b.IsError})
		}
	}
	return out
}

// rawText renders a tool_result content field (string or block array) to text.
func rawText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var arr []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &arr) == nil {
		out := ""
		for _, b := range arr {
			out += b.Text
		}
		return out
	}
	return string(raw)
}

// parseAnthropicResponse folds a response body (SSE stream or a single JSON
// message) into a normalized Response.
func parseAnthropicResponse(body []byte) (Response, bool, error) {
	if !sniffStreamed(body) {
		return parseAnthropicJSON(body)
	}
	payloads, truncated := reassembleSSE(body)
	blocks := map[int]*Block{}
	order := []int{}
	stop := ""
	for _, p := range payloads {
		var ev struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type     string          `json:"type"`
				Name     string          `json:"name"`
				ID       string          `json:"id"`
				Input    json.RawMessage `json:"input"`
				Thinking string          `json:"thinking"`
				Text     string          `json:"text"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
		}
		if json.Unmarshal(p, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "content_block_start":
			b := &Block{}
			switch ev.ContentBlock.Type {
			case "thinking":
				b.Type, b.Text = "thinking", ev.ContentBlock.Thinking
			case "text":
				b.Type, b.Text = "text", ev.ContentBlock.Text
			case "tool_use":
				b.Type, b.ToolName, b.ToolUseID = "tool_use", ev.ContentBlock.Name, ev.ContentBlock.ID
			default:
				b.Type = ev.ContentBlock.Type
			}
			blocks[ev.Index] = b
			order = append(order, ev.Index)
		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				b.Text += ev.Delta.Text
			case "thinking_delta":
				b.Text += ev.Delta.Thinking
			case "input_json_delta":
				b.Input = append(b.Input, []byte(ev.Delta.PartialJSON)...)
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stop = ev.Delta.StopReason
			}
		}
	}
	resp := Response{StopReason: stop}
	for _, i := range order {
		resp.Blocks = append(resp.Blocks, *blocks[i])
	}
	return resp, truncated, nil
}

func parseAnthropicJSON(body []byte) (Response, bool, error) {
	var msg struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Thinking string          `json:"thinking"`
			Name     string          `json:"name"`
			ID       string          `json:"id"`
			Input    json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return Response{}, false, err
	}
	resp := Response{StopReason: msg.StopReason}
	for _, c := range msg.Content {
		switch c.Type {
		case "text":
			resp.Blocks = append(resp.Blocks, Block{Type: "text", Text: c.Text})
		case "thinking":
			resp.Blocks = append(resp.Blocks, Block{Type: "thinking", Text: c.Thinking})
		case "tool_use":
			resp.Blocks = append(resp.Blocks, Block{Type: "tool_use", ToolName: c.Name, ToolUseID: c.ID, Input: c.Input})
		}
	}
	return resp, false, nil
}
