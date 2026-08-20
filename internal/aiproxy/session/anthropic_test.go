package session

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseAnthropicRequest(t *testing.T) {
	body, _ := os.ReadFile("testdata/anthropic_req.json")
	sys, msgs, err := parseAnthropicRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if sys == "" {
		t.Fatal("instructions not extracted")
	}
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	// assistant turn: text + tool_use
	if msgs[1].Role != "assistant" || len(msgs[1].Blocks) != 2 {
		t.Fatalf("bad assistant msg: %+v", msgs[1])
	}
	if msgs[1].Blocks[1].Type != "tool_use" || msgs[1].Blocks[1].ToolName != "Bash" {
		t.Fatalf("bad tool_use block: %+v", msgs[1].Blocks[1])
	}
	// tool_result
	if msgs[2].Blocks[0].Type != "tool_result" || msgs[2].Blocks[0].ToolUseID != "tu_1" {
		t.Fatalf("bad tool_result: %+v", msgs[2].Blocks[0])
	}
}

func TestParseAnthropicResponseStream(t *testing.T) {
	body, _ := os.ReadFile("testdata/anthropic_resp_stream.txt")
	resp, trunc, err := parseAnthropicResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if trunc {
		t.Fatal("unexpected truncation")
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("stop reason: %q", resp.StopReason)
	}
	if len(resp.Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d: %+v", len(resp.Blocks), resp.Blocks)
	}
	if resp.Blocks[0].Type != "thinking" || resp.Blocks[0].Text != "I should check the test config first." {
		t.Fatalf("bad thinking block: %+v", resp.Blocks[0])
	}
	if resp.Blocks[1].Type != "tool_use" || resp.Blocks[1].ToolName != "Read" {
		t.Fatalf("bad tool_use block: %+v", resp.Blocks[1])
	}
	if !json.Valid(resp.Blocks[1].Input) {
		t.Fatalf("reconstructed input is not valid JSON: %q", resp.Blocks[1].Input)
	}
	if string(resp.Blocks[1].Input) != `{"file_path":"Makefile"}` {
		t.Fatalf("bad reconstructed input: %q", resp.Blocks[1].Input)
	}
}

func TestParseAnthropicResponseJSON(t *testing.T) {
	body := []byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"hi"},{"type":"thinking","thinking":"hmm"},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"x"}}]}`)
	resp, trunc, err := parseAnthropicResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if trunc {
		t.Fatal("unexpected truncation")
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("stop reason: %q", resp.StopReason)
	}
	if len(resp.Blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d: %+v", len(resp.Blocks), resp.Blocks)
	}
	if resp.Blocks[0].Type != "text" || resp.Blocks[0].Text != "hi" {
		t.Fatalf("bad text block: %+v", resp.Blocks[0])
	}
	if resp.Blocks[1].Type != "thinking" || resp.Blocks[1].Text != "hmm" {
		t.Fatalf("bad thinking block: %+v", resp.Blocks[1])
	}
	if resp.Blocks[2].Type != "tool_use" || resp.Blocks[2].ToolName != "Read" || resp.Blocks[2].ToolUseID != "t1" {
		t.Fatalf("bad tool_use block: %+v", resp.Blocks[2])
	}
	if !json.Valid(resp.Blocks[2].Input) {
		t.Fatalf("input is not valid JSON: %q", resp.Blocks[2].Input)
	}
	if string(resp.Blocks[2].Input) != `{"file_path":"x"}` {
		t.Fatalf("bad input: %q", resp.Blocks[2].Input)
	}
}
