package session

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseOpenAIRequest(t *testing.T) {
	body, _ := os.ReadFile("testdata/openai_req.json")
	sys, msgs, err := parseOpenAIRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if sys != "You are a careful engineer." {
		t.Fatalf("instructions: %q", sys)
	}
	// system role folded into instructions, not messages: user, assistant, tool
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "assistant" || msgs[1].Blocks[1].Type != "tool_use" || msgs[1].Blocks[1].ToolName != "Bash" {
		t.Fatalf("bad assistant tool_use: %+v", msgs[1])
	}
	toolInput := msgs[1].Blocks[1].Input
	if !json.Valid(toolInput) {
		t.Fatalf("tool_use Input not valid JSON: %q", toolInput)
	}
	if string(toolInput) != `{"command":"make test"}` {
		t.Fatalf("tool_use Input: %q", toolInput)
	}
	if msgs[2].Role != "tool" || msgs[2].Blocks[0].Type != "tool_result" || msgs[2].Blocks[0].ToolUseID != "call_1" {
		t.Fatalf("bad tool msg: %+v", msgs[2])
	}
}

func TestParseOpenAIResponseStream(t *testing.T) {
	body, _ := os.ReadFile("testdata/openai_resp_stream.txt")
	resp, _, err := parseOpenAIResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != "tool_calls" {
		t.Fatalf("stop reason: %q", resp.StopReason)
	}
	// expect: thinking, text, tool_use
	if len(resp.Blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d: %+v", len(resp.Blocks), resp.Blocks)
	}
	if resp.Blocks[0].Type != "thinking" || resp.Blocks[0].Text != "Check the config first." {
		t.Fatalf("bad thinking: %+v", resp.Blocks[0])
	}
	if resp.Blocks[1].Type != "text" || resp.Blocks[1].Text != "Reading the Makefile." {
		t.Fatalf("bad text: %+v", resp.Blocks[1])
	}
	if resp.Blocks[2].Type != "tool_use" || resp.Blocks[2].ToolName != "Read" {
		t.Fatalf("bad tool_use: %+v", resp.Blocks[2])
	}
	toolInput := resp.Blocks[2].Input
	if !json.Valid(toolInput) {
		t.Fatalf("tool_use Input not valid JSON: %q", toolInput)
	}
	if string(toolInput) != `{"file_path":"Makefile"}` {
		t.Fatalf("tool_use Input: %q", toolInput)
	}
}

func TestParseOpenAIResponseJSON(t *testing.T) {
	body := []byte(`{"choices":[{"finish_reason":"tool_calls","message":{"content":"done","reasoning_content":"think","tool_calls":[{"id":"c1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"x\"}"}}]}}]}`)
	resp, _, err := parseOpenAIResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != "tool_calls" {
		t.Fatalf("stop reason: %q", resp.StopReason)
	}
	// expect: thinking, text, tool_use
	if len(resp.Blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d: %+v", len(resp.Blocks), resp.Blocks)
	}
	if resp.Blocks[0].Type != "thinking" || resp.Blocks[0].Text != "think" {
		t.Fatalf("bad thinking: %+v", resp.Blocks[0])
	}
	if resp.Blocks[1].Type != "text" || resp.Blocks[1].Text != "done" {
		t.Fatalf("bad text: %+v", resp.Blocks[1])
	}
	if resp.Blocks[2].Type != "tool_use" || resp.Blocks[2].ToolName != "Read" {
		t.Fatalf("bad tool_use: %+v", resp.Blocks[2])
	}
	toolInput := resp.Blocks[2].Input
	if !json.Valid(toolInput) {
		t.Fatalf("tool_use Input not valid JSON: %q", toolInput)
	}
	if string(toolInput) != `{"file_path":"x"}` {
		t.Fatalf("tool_use Input: %q", toolInput)
	}
	var obj map[string]any
	if err := json.Unmarshal(toolInput, &obj); err != nil {
		t.Fatalf("tool_use Input did not unmarshal as object: %v", err)
	}
}
