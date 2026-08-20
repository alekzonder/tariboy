package session

import (
	"bytes"
	"encoding/json"
)

// parseOpenAIRequest folds the OpenAI chat-completions request into instructions
// (the concatenated system-role messages) plus the remaining messages.
func parseOpenAIRequest(body []byte) (string, []Message, error) {
	var envelope struct {
		Instructions string          `json:"instructions"`
		Input        json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", nil, err
	}
	if envelope.Instructions != "" || len(envelope.Input) != 0 {
		return parseOpenAIResponsesRequest(envelope.Instructions, envelope.Input)
	}

	var req struct {
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", nil, err
	}
	instructions := ""
	var msgs []Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			instructions += openaiContentText(m.Content)
			continue
		}
		if m.Role == "tool" {
			msgs = append(msgs, Message{Role: "tool", Blocks: []Block{{Type: "tool_result", ToolUseID: m.ToolCallID, Text: openaiContentText(m.Content)}}})
			continue
		}
		var blocks []Block
		if t := openaiContentText(m.Content); t != "" {
			blocks = append(blocks, Block{Type: "text", Text: t})
		}
		for _, tc := range m.ToolCalls {
			blocks = append(blocks, Block{Type: "tool_use", ToolName: tc.Function.Name, ToolUseID: tc.ID, Input: json.RawMessage(tc.Function.Arguments)})
		}
		msgs = append(msgs, Message{Role: m.Role, Blocks: blocks})
	}
	return instructions, msgs, nil
}

// openaiContentText renders a message content (string or array of parts).
func openaiContentText(raw json.RawMessage) string {
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
		for _, p := range arr {
			out += p.Text
		}
		return out
	}
	return ""
}

// parseOpenAIResponse folds a chat-completions response (SSE or a single JSON
// object) into a normalized Response. reasoning_content maps to a thinking block.
func parseOpenAIResponse(body []byte) (Response, bool, error) {
	if bytes.Contains(body, []byte(`"response.output_`)) ||
		bytes.Contains(body, []byte(`"response.completed"`)) ||
		bytes.Contains(body, []byte(`"response.failed"`)) ||
		bytes.Contains(body, []byte(`"response.incomplete"`)) ||
		isOpenAIResponsesJSON(body) {
		return parseOpenAIResponsesResponse(body)
	}
	if !sniffStreamed(body) {
		return parseOpenAIJSON(body)
	}
	payloads, truncated := reassembleSSE(body)
	var thinking, text string
	tools := map[int]*Block{}
	toolOrder := []int{}
	stop := ""
	for _, p := range payloads {
		var ev struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal(p, &ev) != nil || len(ev.Choices) == 0 {
			continue
		}
		ch := ev.Choices[0]
		thinking += ch.Delta.ReasoningContent
		text += ch.Delta.Content
		for _, tc := range ch.Delta.ToolCalls {
			b := tools[tc.Index]
			if b == nil {
				b = &Block{Type: "tool_use"}
				tools[tc.Index] = b
				toolOrder = append(toolOrder, tc.Index)
			}
			if tc.ID != "" {
				b.ToolUseID = tc.ID
			}
			if tc.Function.Name != "" {
				b.ToolName = tc.Function.Name
			}
			b.Input = append(b.Input, []byte(tc.Function.Arguments)...)
		}
		if ch.FinishReason != "" {
			stop = ch.FinishReason
		}
	}
	resp := Response{StopReason: stop}
	if thinking != "" {
		resp.Blocks = append(resp.Blocks, Block{Type: "thinking", Text: thinking})
	}
	if text != "" {
		resp.Blocks = append(resp.Blocks, Block{Type: "text", Text: text})
	}
	for _, i := range toolOrder {
		resp.Blocks = append(resp.Blocks, *tools[i])
	}
	return resp, truncated, nil
}

func isOpenAIResponsesJSON(body []byte) bool {
	if sniffStreamed(body) {
		return false
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	_, hasOutput := value["output"]
	_, hasChoices := value["choices"]
	return hasOutput && !hasChoices
}

type responsesItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Name      string          `json:"name"`
	CallID    string          `json:"call_id"`
	Arguments string          `json:"arguments"`
	Input     json.RawMessage `json:"input"`
	Action    json.RawMessage `json:"action"`
	Output    json.RawMessage `json:"output"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Summary []struct {
		Text string `json:"text"`
	} `json:"summary"`
}

func parseOpenAIResponsesRequest(instructions string, raw json.RawMessage) (string, []Message, error) {
	var prompt string
	if json.Unmarshal(raw, &prompt) == nil {
		return instructions, []Message{{Role: "user", Blocks: []Block{{Type: "text", Text: prompt}}}}, nil
	}
	var items []responsesItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", nil, err
	}
	messages := make([]Message, 0, len(items))
	for _, item := range items {
		if message, ok := responsesItemMessage(item); ok {
			messages = append(messages, message)
		}
	}
	return instructions, messages, nil
}

func responsesItemMessage(item responsesItem) (Message, bool) {
	switch item.Type {
	case "message":
		blocks := []Block{}
		for _, part := range item.Content {
			if part.Text != "" {
				blocks = append(blocks, Block{Type: "text", Text: part.Text})
			}
		}
		return Message{Role: item.Role, Blocks: blocks}, true
	case "function_call", "custom_tool_call", "local_shell_call", "mcp_call":
		name := item.Name
		if name == "" && item.Type == "local_shell_call" {
			name = "local_shell"
		}
		return Message{Role: "assistant", Blocks: []Block{{
			Type: "tool_use", ToolName: name, ToolUseID: item.CallID, Input: responsesToolInput(item),
		}}}, true
	case "function_call_output", "custom_tool_call_output", "local_shell_call_output", "mcp_call_output":
		return Message{Role: "tool", Blocks: []Block{{
			Type: "tool_result", ToolUseID: item.CallID, Text: responseOutputText(item.Output),
		}}}, true
	case "reasoning":
		text := ""
		for _, part := range item.Summary {
			text += part.Text
		}
		if text != "" {
			return Message{Role: "assistant", Blocks: []Block{{Type: "thinking", Text: text}}}, true
		}
	}
	return Message{}, false
}

func responsesToolInput(item responsesItem) json.RawMessage {
	if item.Arguments != "" {
		return responseArguments(item.Arguments)
	}
	if len(item.Input) != 0 {
		var text string
		if json.Unmarshal(item.Input, &text) == nil {
			return responseArguments(text)
		}
		return item.Input
	}
	if len(item.Action) != 0 {
		return item.Action
	}
	return json.RawMessage(`{}`)
}

func responseArguments(value string) json.RawMessage {
	if json.Valid([]byte(value)) {
		return json.RawMessage(value)
	}
	raw, _ := json.Marshal(value)
	return raw
}

func responseOutputText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	if len(raw) != 0 {
		return string(raw)
	}
	return ""
}

func parseOpenAIResponsesResponse(body []byte) (Response, bool, error) {
	if !sniffStreamed(body) {
		var value struct {
			Status string          `json:"status"`
			Output []responsesItem `json:"output"`
		}
		if err := json.Unmarshal(body, &value); err != nil {
			return Response{}, false, err
		}
		return responsesItemsToResponse(value.Output, value.Status), false, nil
	}
	payloads, truncated := reassembleSSE(body)
	response := Response{Blocks: []Block{}}
	for _, payload := range payloads {
		var event struct {
			Type     string        `json:"type"`
			Text     string        `json:"text"`
			Item     responsesItem `json:"item"`
			Response struct {
				Status string `json:"status"`
			} `json:"response"`
		}
		if json.Unmarshal(payload, &event) != nil {
			continue
		}
		switch event.Type {
		case "response.output_item.done":
			parsed := responsesItemsToResponse([]responsesItem{event.Item}, "")
			for _, block := range parsed.Blocks {
				response.Blocks = appendResponsesBlock(response.Blocks, block)
			}
		case "response.output_text.done":
			if event.Text != "" {
				response.Blocks = appendResponsesBlock(response.Blocks, Block{Type: "text", Text: event.Text})
			}
		case "response.completed", "response.failed", "response.incomplete":
			response.StopReason = event.Response.Status
		}
	}
	return response, truncated, nil
}

func appendResponsesBlock(blocks []Block, candidate Block) []Block {
	if candidate.Type == "text" {
		for _, block := range blocks {
			if block.Type == candidate.Type && block.Text == candidate.Text {
				return blocks
			}
		}
	}
	return append(blocks, candidate)
}

func responsesItemsToResponse(items []responsesItem, status string) Response {
	response := Response{Blocks: []Block{}, StopReason: status}
	for _, item := range items {
		message, ok := responsesItemMessage(item)
		if !ok {
			continue
		}
		response.Blocks = append(response.Blocks, message.Blocks...)
	}
	return response
}

func parseOpenAIJSON(body []byte) (Response, bool, error) {
	var msg struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return Response{}, false, err
	}
	if len(msg.Choices) == 0 {
		return Response{}, false, nil
	}
	ch := msg.Choices[0]
	resp := Response{StopReason: ch.FinishReason}
	if ch.Message.ReasoningContent != "" {
		resp.Blocks = append(resp.Blocks, Block{Type: "thinking", Text: ch.Message.ReasoningContent})
	}
	if ch.Message.Content != "" {
		resp.Blocks = append(resp.Blocks, Block{Type: "text", Text: ch.Message.Content})
	}
	for _, tc := range ch.Message.ToolCalls {
		resp.Blocks = append(resp.Blocks, Block{Type: "tool_use", ToolName: tc.Function.Name, ToolUseID: tc.ID, Input: json.RawMessage(tc.Function.Arguments)})
	}
	return resp, false, nil
}
