package mailcli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// instantTimeout bounds tmail instant: a hook must never hold up a tool call.
const instantTimeout = 1500 * time.Millisecond

// instantResult is the /tools/mail/instant result.
type instantResult struct {
	IDs  []string `json:"ids"`
	Text string   `json:"text"`
}

// runInstant is tmail instant, run by a harness hook after each tool call.
// Without the flag file it returns at once without calling the daemon. Every
// failure prints nothing and exits 0, and nothing goes to stderr, which a hook
// may show the model; only a bad --format is a usage error.
func runInstant(parsed request, m mode, stdout, stderr io.Writer) int {
	format := parsed.flags["format"]
	if !validInstantFormat(format) {
		fmt.Fprintln(stderr, usagef("instant", "--format must be one of %s, got %q", instantFormatList, format))
		return 2
	}
	if m.toolsSocket == "" || m.instantFlag == "" {
		return 0
	}
	if _, err := os.Stat(m.instantFlag); err != nil {
		return 0
	}
	type reply struct {
		raw json.RawMessage
		err error
	}
	done := make(chan reply, 1)
	caller := newCaller(m.toolsSocket)
	go func() {
		raw, err := caller.Call("POST", "/tools/mail/instant", map[string]any{})
		done <- reply{raw, err}
	}()
	var r reply
	select {
	case r = <-done:
	case <-time.After(instantTimeout):
		return 0
	}
	var res instantResult
	if r.err != nil || json.Unmarshal(r.raw, &res) != nil || len(res.IDs) == 0 {
		return 0
	}
	fmt.Fprint(stdout, formatInstant(format, res))
	return 0
}

// instantFormatList names the --format values of tmail instant.
const instantFormatList = "text, json, claude-post-tool-use, claude-post-tool-use-failure, claude-stop"

// claudeHookEvents maps a Claude Code PostToolUse-style format to the hook
// event name its hookSpecificOutput must carry.
var claudeHookEvents = map[string]string{
	"claude-post-tool-use":         "PostToolUse",
	"claude-post-tool-use-failure": "PostToolUseFailure",
}

func validInstantFormat(format string) bool {
	switch format {
	case "", "text", "json", "claude-stop":
		return true
	}
	_, ok := claudeHookEvents[format]
	return ok
}

// formatInstant renders taken letters. The claude-* formats are the JSON a
// Claude Code hook prints: tmail builds it itself, so the hook needs no jq,
// python or node.
func formatInstant(format string, res instantResult) string {
	var v any
	switch format {
	case "", "text":
		return res.Text
	case "json":
		v = res
	case "claude-stop":
		v = map[string]string{"decision": "block", "reason": res.Text}
	default:
		v = map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName":     claudeHookEvents[format],
			"additionalContext": res.Text,
		}}
	}
	out, _ := json.Marshal(v)
	return string(out) + "\n"
}
