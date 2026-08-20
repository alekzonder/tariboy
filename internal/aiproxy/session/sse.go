package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

const truncMarker = "[transcript truncated at cap]"

// sniffStreamed reports whether body is an SSE stream (vs a single JSON object).
// The proxy captures either a raw JSON response body or the raw SSE stream; the
// first non-space byte distinguishes them: '{' starts JSON, 'e'/'d' start an
// "event:"/"data:" line.
func sniffStreamed(body []byte) bool {
	b := bytes.TrimLeft(body, " \t\r\n")
	if len(b) == 0 {
		return false
	}
	if b[0] == '{' {
		return false
	}
	return bytes.HasPrefix(b, []byte("event:")) || bytes.HasPrefix(b, []byte("data:"))
}

// reassembleSSE returns the JSON payloads of each `data:` line in order, and
// whether the capture hit the transcript cap. Non-JSON data lines (e.g. the
// literal "[DONE]") are skipped. Best-effort: malformed lines are ignored.
func reassembleSSE(body []byte) ([]json.RawMessage, bool) {
	var out []json.RawMessage
	truncated := false
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, truncMarker) {
			truncated = true
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if json.Valid([]byte(payload)) {
			out = append(out, json.RawMessage(payload))
		}
	}
	return out, truncated
}
