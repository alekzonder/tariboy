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
const instantFormatList = "text, json, claude-post-tool-use, claude-post-tool-use-failure, claude-stop, codex-post-tool-use, codex-stop, opencode-tool-after"

// hookContextEvents maps a PostToolUse-style hook format to the hook event
// name its hookSpecificOutput must carry. Codex takes the same JSON as Claude
// Code today; its formats keep their own names so either harness can change
// its contract alone.
var hookContextEvents = map[string]string{
	"claude-post-tool-use":         "PostToolUse",
	"claude-post-tool-use-failure": "PostToolUseFailure",
	"codex-post-tool-use":          "PostToolUse",
}

func validInstantFormat(format string) bool {
	switch format {
	case "", "text", "json", "claude-stop", "codex-stop", "opencode-tool-after":
		return true
	}
	_, ok := hookContextEvents[format]
	return ok
}

// formatInstant renders taken letters. opencode-tool-after is plain text the
// OpenCode plugin appends to a tool result. The claude-* and codex-* formats
// are the JSON a harness hook prints: tmail builds it itself, so the hook needs
// no jq, python or node.
func formatInstant(format string, res instantResult) string {
	var v any
	switch format {
	case "", "text":
		return res.Text
	case "opencode-tool-after":
		// The OpenCode plugin appends this to the tool result as is.
		return "\n\n" + res.Text
	case "json":
		v = res
	case "claude-stop", "codex-stop":
		v = map[string]string{"decision": "block", "reason": res.Text}
	default:
		v = map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName":     hookContextEvents[format],
			"additionalContext": res.Text,
		}}
	}
	out, _ := json.Marshal(v)
	return string(out) + "\n"
}
