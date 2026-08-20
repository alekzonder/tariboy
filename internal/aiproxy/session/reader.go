package session

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
)

// ReadEntries loads the proxy transcript for one iteration. It prefers the plain
// proxy-transcript.jsonl and falls back to the gzipped .gz written at iteration
// close. A missing transcript (no AI calls) returns (nil, nil).
func ReadEntries(agentsDir, agent, iteration string) ([]aiproxy.TranscriptEntry, error) {
	dir := agentdir.New(agentsDir, agent).IterationDir(iteration)
	plain := filepath.Join(dir, "proxy-transcript.jsonl")
	f, err := os.Open(plain)
	if err == nil {
		defer f.Close()
		return scanEntries(f)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	gzPath := plain + ".gz"
	gf, err := os.Open(gzPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer gf.Close()
	gz, err := gzip.NewReader(gf)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return scanEntries(gz)
}

func scanEntries(r io.Reader) ([]aiproxy.TranscriptEntry, error) {
	var out []aiproxy.TranscriptEntry
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e aiproxy.TranscriptEntry
		if json.Unmarshal(line, &e) != nil {
			continue // skip a corrupt line, keep the rest
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// Build folds transcript entries into one SessionTimeline: per-call metadata,
// instruction-change flags, message deltas (each message shown once), and the
// parsed response. A parse failure degrades that single call to ParseError.
func Build(entries []aiproxy.TranscriptEntry) SessionTimeline {
	tl := SessionTimeline{Calls: []Call{}}
	prevInstr := ""
	prevMsgCount := 0
	for i, e := range entries {
		call := Call{
			Seq: i, Ts: e.Meta.TS, Provider: e.Meta.Provider, Model: e.Meta.Model,
			Usage: Usage{Input: e.Meta.InputTokens, Output: e.Meta.OutputTokens,
				CacheRead: e.Meta.CacheReadTokens, CacheWrite: e.Meta.CacheWriteTokens},
			CostUSD: e.Meta.CostUSD, LatencyMs: e.Meta.LatencyMs, Status: e.Meta.Status,
		}
		instr, msgs, reqErr := parseRequest(e.Meta.Provider, e.Request)
		resp, truncated, respErr := parseResponse(e.Meta.Provider, e.Response)
		if reqErr != nil || respErr != nil {
			call.ParseError = errText(reqErr, respErr)
			// Still surface what parsed (instructions/response may be usable).
		}
		call.Instructions = instr
		call.InstructionsChanged = i == 0 || instr != prevInstr
		if len(msgs) >= prevMsgCount {
			call.Delta = msgs[prevMsgCount:]
		} else {
			call.Delta = msgs // history shrank (unexpected) — show all
		}
		call.Response = resp
		call.Truncated = truncated
		// A nil []Block marshals to JSON `null`; the UI calls .filter/.map on
		// these unconditionally, so a `null` blanks the whole page. Always emit
		// `[]` — this happens on a failed/empty response or a contentless message.
		if call.Response.Blocks == nil {
			call.Response.Blocks = []Block{}
		}
		for j := range call.Delta {
			if call.Delta[j].Blocks == nil {
				call.Delta[j].Blocks = []Block{}
			}
		}
		prevInstr = instr
		prevMsgCount = len(msgs)
		tl.Calls = append(tl.Calls, call)
	}
	return tl
}

func parseRequest(provider string, body []byte) (string, []Message, error) {
	switch provider {
	case "openai":
		return parseOpenAIRequest(body)
	default:
		return parseAnthropicRequest(body)
	}
}

func parseResponse(provider string, body []byte) (Response, bool, error) {
	switch provider {
	case "openai":
		return parseOpenAIResponse(body)
	default:
		return parseAnthropicResponse(body)
	}
}

func errText(a, b error) string {
	switch {
	case a != nil && b != nil:
		return "request: " + a.Error() + "; response: " + b.Error()
	case a != nil:
		return "request: " + a.Error()
	case b != nil:
		return "response: " + b.Error()
	}
	return ""
}

// RawCalls returns the decoded request/response bytes per call for ?raw=1.
func RawCalls(entries []aiproxy.TranscriptEntry) []map[string]any {
	out := []map[string]any{}
	for i, e := range entries {
		out = append(out, map[string]any{
			"seq": i, "ts": e.Meta.TS,
			"request": string(e.Request), "response": string(e.Response),
		})
	}
	return out
}
