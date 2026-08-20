package session

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
)

func entry(seq int, provider, sys string, req, resp string) aiproxy.TranscriptEntry {
	return aiproxy.TranscriptEntry{
		Meta:     aiproxy.AIRequest{TS: "2026-07-10T00:00:0" + string(rune('0'+seq)) + "Z", Provider: provider, Model: "m", InputTokens: 10, OutputTokens: 5},
		Request:  []byte(req),
		Response: []byte(resp),
	}
}

func TestBuildDeltaAndInstructionsChanged(t *testing.T) {
	// call 0: system S1, one user message
	r0 := `{"system":"S1","messages":[{"role":"user","content":"a"}]}`
	// call 1: SAME system, adds tool_result — delta should be just the new message
	r1 := `{"system":"S1","messages":[{"role":"user","content":"a"},{"role":"user","content":"b"}]}`
	// call 2: system CHANGED to S2
	r2 := `{"system":"S2","messages":[{"role":"user","content":"a"},{"role":"user","content":"b"},{"role":"user","content":"c"}]}`
	resp := `{"stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}`
	tl := Build([]aiproxy.TranscriptEntry{
		entry(0, "anthropic", "S1", r0, resp),
		entry(1, "anthropic", "S1", r1, resp),
		entry(2, "anthropic", "S2", r2, resp),
	})
	if len(tl.Calls) != 3 {
		t.Fatalf("want 3 calls, got %d", len(tl.Calls))
	}
	if !tl.Calls[0].InstructionsChanged {
		t.Fatal("call 0 should flag instructions changed (first)")
	}
	if tl.Calls[1].InstructionsChanged {
		t.Fatal("call 1 should NOT flag (same system)")
	}
	if !tl.Calls[2].InstructionsChanged {
		t.Fatal("call 2 should flag (S1->S2)")
	}
	if len(tl.Calls[1].Delta) != 1 || tl.Calls[1].Delta[0].Blocks[0].Text != "b" {
		t.Fatalf("call 1 delta should be just message 'b': %+v", tl.Calls[1].Delta)
	}
	if tl.Calls[0].Usage.Input != 10 || tl.Calls[0].Response.Blocks[0].Text != "ok" {
		t.Fatalf("metadata/response not carried: %+v", tl.Calls[0])
	}
}

func TestBuildParseErrorDegrades(t *testing.T) {
	tl := Build([]aiproxy.TranscriptEntry{
		entry(0, "anthropic", "", "not json", "also not json"),
	})
	if len(tl.Calls) != 1 || tl.Calls[0].ParseError == "" {
		t.Fatalf("bad body should degrade to ParseError, got %+v", tl.Calls[0])
	}
}

func TestBuildNeverEmitsNullBlocks(t *testing.T) {
	// A nil []Block marshals to JSON `null`, and the UI does
	// response.blocks.filter(...) / message.blocks.map(...) unconditionally —
	// a `null` there throws "Cannot read properties of null" and blanks the
	// whole page. Build must always emit `[]`, never `null`, even when the
	// response fails to parse or a message carries no renderable content.
	req := `{"system":"S1","messages":[{"role":"user","content":[]}]}`
	tl := Build([]aiproxy.TranscriptEntry{
		entry(0, "anthropic", "S1", req, "not json"),
	})
	c := tl.Calls[0]
	if c.Response.Blocks == nil {
		t.Fatal("Response.Blocks must be non-nil on parse failure")
	}
	if len(c.Delta) != 1 || c.Delta[0].Blocks == nil {
		t.Fatalf("Delta message blocks must be non-nil: %+v", c.Delta)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte(`"blocks":null`)) {
		t.Fatalf("call JSON must never contain blocks:null: %s", b)
	}
}

func TestBuildParsesOpenAIResponsesConversationAndActions(t *testing.T) {
	request := `{
		"model":"gpt-5.6-sol",
		"instructions":"You are a coding agent.",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the repository"}]},
			{"type":"function_call_output","call_id":"call-1","output":"README.md\nui/"}
		]
	}`
	response := "event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"I should inspect the files first."}]}}` + "\n\n" +
		"event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","item":{"type":"function_call","name":"exec_command","call_id":"call-2","arguments":"{\"cmd\":\"rg --files\"}"}}` + "\n\n" +
		"event: response.output_text.done\n" +
		`data: {"type":"response.output_text.done","text":"I found the relevant files."}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n"

	tl := Build([]aiproxy.TranscriptEntry{entry(0, "openai", "", request, response)})
	if len(tl.Calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(tl.Calls))
	}
	call := tl.Calls[0]
	if call.Instructions != "You are a coding agent." {
		t.Fatalf("instructions = %q", call.Instructions)
	}
	if len(call.Delta) != 2 || call.Delta[0].Role != "user" || call.Delta[0].Blocks[0].Text != "Inspect the repository" {
		t.Fatalf("delta = %#v", call.Delta)
	}
	if got := call.Delta[1].Blocks[0]; got.Type != "tool_result" || got.ToolUseID != "call-1" || got.Text != "README.md\nui/" {
		t.Fatalf("tool result = %#v", got)
	}
	if len(call.Response.Blocks) != 3 {
		t.Fatalf("response blocks = %#v", call.Response.Blocks)
	}
	if got := call.Response.Blocks[0]; got.Type != "thinking" || got.Text != "I should inspect the files first." {
		t.Fatalf("thinking = %#v", got)
	}
	if got := call.Response.Blocks[1]; got.Type != "tool_use" || got.ToolName != "exec_command" || got.ToolUseID != "call-2" || string(got.Input) != `{"cmd":"rg --files"}` {
		t.Fatalf("tool use = %#v", got)
	}
	if got := call.Response.Blocks[2]; got.Type != "text" || got.Text != "I found the relevant files." {
		t.Fatalf("assistant text = %#v", got)
	}
	if call.Response.StopReason != "completed" {
		t.Fatalf("stop reason = %q", call.Response.StopReason)
	}
}

func TestBuildParsesOpenAIResponsesLocalShellAndCustomSkill(t *testing.T) {
	response := `{
		"status":"completed",
		"output":[
			{"type":"local_shell_call","call_id":"shell-1","action":{"type":"exec","command":["bash","-lc","make check"]}},
			{"type":"custom_tool_call","name":"Skill","call_id":"skill-1","input":"{\"skill\":\"brainstorming\"}"}
		]
	}`
	tl := Build([]aiproxy.TranscriptEntry{entry(0, "openai", "", `{"instructions":"S","input":"work"}`, response)})
	blocks := tl.Calls[0].Response.Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %#v", blocks)
	}
	if got := blocks[0]; got.Type != "tool_use" || got.ToolName != "local_shell" || got.ToolUseID != "shell-1" || string(got.Input) != `{"type":"exec","command":["bash","-lc","make check"]}` {
		t.Fatalf("local shell = %#v", got)
	}
	if got := blocks[1]; got.Type != "tool_use" || got.ToolName != "Skill" || string(got.Input) != `{"skill":"brainstorming"}` {
		t.Fatalf("custom skill = %#v", got)
	}
}

func TestBuildDeduplicatesResponsesOutputTextEvents(t *testing.T) {
	response := "event: response.output_text.done\n" +
		`data: {"type":"response.output_text.done","text":"Done."}` + "\n\n" +
		"event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done."}]}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n"

	tl := Build([]aiproxy.TranscriptEntry{entry(0, "openai", "", `{"instructions":"S","input":"work"}`, response)})
	blocks := tl.Calls[0].Response.Blocks
	if len(blocks) != 1 || blocks[0].Type != "text" || blocks[0].Text != "Done." {
		t.Fatalf("duplicate response text was not collapsed: %#v", blocks)
	}
}

func TestBuildParsesResponsesFailureWithoutOutputItems(t *testing.T) {
	response := "event: response.failed\n" +
		`data: {"type":"response.failed","response":{"status":"failed"}}` + "\n\n"
	tl := Build([]aiproxy.TranscriptEntry{entry(0, "openai", "", `{"instructions":"S","input":"work"}`, response)})
	call := tl.Calls[0]
	if call.ParseError != "" || call.Response.StopReason != "failed" {
		t.Fatalf("failed Responses stream was not parsed: %#v", call)
	}
}

func TestReadEntriesMissing(t *testing.T) {
	dir := t.TempDir()
	entries, err := ReadEntries(dir, "agent1", "iter1")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if entries != nil {
		t.Fatalf("want nil entries, got %+v", entries)
	}
}

func TestReadEntriesGzipFallback(t *testing.T) {
	agentsDir := t.TempDir()
	dir := agentdir.New(agentsDir, "agent1").IterationDir("iter1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	e := aiproxy.TranscriptEntry{
		Meta:     aiproxy.AIRequest{TS: "2026-07-10T00:00:00Z", Provider: "anthropic", Model: "m", InputTokens: 10, OutputTokens: 5},
		Request:  []byte(`{"system":"S1","messages":[{"role":"user","content":"a"}]}`),
		Response: []byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}`),
	}
	line, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	gzPath := filepath.Join(dir, "proxy-transcript.jsonl.gz")
	f, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("create gz: %v", err)
	}
	gw := gzip.NewWriter(f)
	if _, err := gw.Write(append(line, '\n')); err != nil {
		t.Fatalf("write gz: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	entries, err := ReadEntries(agentsDir, "agent1", "iter1")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d: %+v", len(entries), entries)
	}
}
