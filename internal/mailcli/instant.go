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
	if format != "" && format != "text" && format != "json" {
		fmt.Fprintln(stderr, usagef("instant", "--format must be text or json, got %q", format))
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
	if format == "json" {
		out, _ := json.Marshal(res)
		fmt.Fprintln(stdout, string(out))
		return 0
	}
	fmt.Fprint(stdout, res.Text)
	return 0
}
