package aiproxy

import "testing"

func TestUsageNormalizedClampsNegativeBuckets(t *testing.T) {
	u := Usage{InputTokens: -1, OutputTokens: -2, CacheWriteTokens: -3, CacheReadTokens: -4}.Normalized()
	if u != (Usage{}) {
		t.Fatalf("normalized usage = %+v, want all zero", u)
	}
}

func TestParseAnthropicUsage(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-8","usage":{"input_tokens":60,"output_tokens":50,
		"cache_creation_input_tokens":10,"cache_read_input_tokens":30}}`)
	u, model, ok := ParseAnthropicUsage(body)
	if !ok || model != "claude-opus-4-8" {
		t.Fatalf("ok=%v model=%q", ok, model)
	}
	if u.InputTokens != 60 || u.OutputTokens != 50 || u.CacheWriteTokens != 10 || u.CacheReadTokens != 30 {
		t.Fatalf("usage = %+v", u)
	}
	if _, _, ok := ParseAnthropicUsage([]byte(`{"error":"boom"}`)); ok {
		t.Fatal("a body with no usage should report ok=false")
	}
}

func TestParseOpenAIUsage(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","usage":{"prompt_tokens":100,"completion_tokens":40,
		"prompt_tokens_details":{"cached_tokens":40}}}`)
	u, model, ok := ParseOpenAIUsage(body)
	if !ok || model != "gpt-4o" || u.InputTokens != 60 || u.OutputTokens != 40 || u.CacheReadTokens != 40 {
		t.Fatalf("openai usage = %+v model=%q ok=%v", u, model, ok)
	}
}

func TestParseOpenAIResponsesUsage(t *testing.T) {
	body := []byte(`{"model":"gpt-5","usage":{"input_tokens":100,"output_tokens":7,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":10}}}`)
	u, model, ok := ParseOpenAIUsage(body)
	if !ok || model != "gpt-5" || u.InputTokens != 50 || u.OutputTokens != 7 || u.CacheWriteTokens != 10 || u.CacheReadTokens != 40 {
		t.Fatalf("responses usage = %+v model=%q ok=%v", u, model, ok)
	}
	p := &Pricing{table: map[string]ModelPrice{
		"gpt-5": {CacheReadPerMtok: 1_000_000},
	}}
	if cost := p.Cost(model, u); cost != 40 {
		t.Fatalf("cached-token cost = %v, want 40", cost)
	}
}

func TestParseOpenAIUsageClampsCacheTokensAboveInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5","usage":{"input_tokens":10,"output_tokens":7,"input_tokens_details":{"cached_tokens":7,"cache_write_tokens":5}}}`)
	u, _, ok := ParseOpenAIUsage(body)
	if !ok || u.InputTokens != 0 || u.CacheWriteTokens != 5 || u.CacheReadTokens != 7 {
		t.Fatalf("clamped OpenAI usage = %+v ok=%v", u, ok)
	}
}

func TestSSEAccumulator(t *testing.T) {
	a := &SSEAccumulator{}
	a.FeedData([]byte(`{"type":"message_start","message":{"model":"claude-opus-4-8",
		"usage":{"input_tokens":100,"cache_creation_input_tokens":10,"cache_read_input_tokens":5,"output_tokens":1}}}`))
	a.FeedData([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`))
	a.FeedData([]byte(`{"type":"message_delta","usage":{"output_tokens":7}}`))
	a.FeedData([]byte(`{"type":"message_delta","usage":{"output_tokens":42}}`)) // cumulative, last wins
	u, model := a.Result()
	if model != "claude-opus-4-8" {
		t.Fatalf("model = %q", model)
	}
	if u.InputTokens != 100 || u.CacheWriteTokens != 10 || u.CacheReadTokens != 5 || u.OutputTokens != 42 {
		t.Fatalf("accumulated usage = %+v", u)
	}
}

func TestSSEAccumulatorChatGPTResponseCompletedLastValidWins(t *testing.T) {
	a := &SSEAccumulator{}
	a.FeedData([]byte(`{"type":"response.completed","response":{"model":"gpt-5.6-preview","usage":{"input_tokens":1,"input_tokens_details":{"cache_write_tokens":2,"cached_tokens":3},"output_tokens":4}}}`))
	a.FeedData([]byte(`{"type":"response.output_text.delta","delta":"ignored"}`))
	a.FeedData([]byte(`{"type":"response.completed","response":`))
	a.FeedData([]byte(`{"type":"response.completed","response":{"model":"gpt-5.6-terra","usage":{"input_tokens":100,"input_tokens_details":{"cache_write_tokens":7,"cached_tokens":25},"output_tokens":40}}}`))
	a.FeedData([]byte(`{"type":"response.completed","response":{"model":"gpt-missing-usage"}}`))
	a.FeedData([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":999,"output_tokens":999}}}`))

	u, model := a.Result()
	if model != "gpt-5.6-terra" {
		t.Fatalf("model = %q, want gpt-5.6-terra", model)
	}
	// Responses input is inclusive: 100 minus 7 cache-write and 25 cache-read
	// tokens leaves 68 ordinary input tokens.
	if u.InputTokens != 68 || u.OutputTokens != 40 || u.CacheWriteTokens != 7 || u.CacheReadTokens != 25 {
		t.Fatalf("completed response usage = %+v", u)
	}
}

// TestSSEAccumulatorRawStream feeds a realistic multi-event Anthropic SSE
// stream (raw `event:`/`data:` lines, blank-line separators, and a final
// [DONE]-free stop) through the accumulator the way a proxy would: split the
// wire bytes into events and hand each data line's JSON to FeedData.
func TestSSEAccumulatorRawStream(t *testing.T) {
	raw := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-opus-4-8","role":"assistant","usage":{"input_tokens":100,"cache_creation_input_tokens":10,"cache_read_input_tokens":5,"output_tokens":1}}}` + "\n" +
		"\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n" +
		"\n" +
		"event: ping\n" +
		`data: {"type":"ping"}` + "\n" +
		"\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}` + "\n" +
		"\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}` + "\n" +
		"\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n" +
		"\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}` + "\n" +
		"\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":73}}` + "\n" +
		"\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n" +
		"\n"

	a := &SSEAccumulator{}
	for _, line := range splitLines(raw) {
		const prefix = "data: "
		if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			a.FeedData([]byte(line[len(prefix):]))
		}
	}
	u, model := a.Result()
	if model != "claude-opus-4-8" {
		t.Fatalf("model = %q", model)
	}
	// input/cache come from message_start; output is the LAST message_delta.
	if u.InputTokens != 100 || u.CacheWriteTokens != 10 || u.CacheReadTokens != 5 || u.OutputTokens != 73 {
		t.Fatalf("accumulated usage = %+v", u)
	}
}

// splitLines splits on "\n" without keeping the separators (a tiny local
// helper so the test does not pull in strings just for this).
func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
