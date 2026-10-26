// Package mailcli provides the shared agent and operator mail client: tmail
// (tariboy-mail) for agents and tariboy mail for operators.
package mailcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/version"
)

type Caller interface {
	Call(method, route string, body any) (json.RawMessage, error)
}

var newCaller = func(socket string) Caller { return client.New(socket) }

// stdin is where --text - reads a body and clear reads its confirmation.
var stdin io.Reader = os.Stdin

// mode is how an entry point reaches the daemon: prog names the command in
// help, a non-empty toolsSocket selects agent mode, and socket resolves the
// operator daemon socket.
type mode struct {
	prog        string
	toolsSocket string
	socket      func() (string, error)
}

// Run dispatches mail commands to the identity-bound agent socket when one is
// present, otherwise to the operator daemon socket.
func Run(ctx context.Context, argv []string, getenv func(string) string, stdout, stderr io.Writer) int {
	return run(ctx, argv, mode{prog: "tmail", toolsSocket: getenv("TARIBOY_TOOLS_SOCKET"), socket: func() (string, error) {
		resolved, err := paths.Resolve(getenv)
		return resolved.Socket(), err
	}}, stdout, stderr)
}

// RunOperator runs the commands of Run in operator mode on socket whatever
// TARIBOY_TOOLS_SOCKET holds, so tariboy mail can read any mailbox.
func RunOperator(ctx context.Context, argv []string, socket string, stdout, stderr io.Writer) int {
	return run(ctx, argv, mode{prog: "tariboy mail", socket: func() (string, error) { return socket, nil }}, stdout, stderr)
}

func run(_ context.Context, argv []string, m mode, stdout, stderr io.Writer) int {
	jsonOut := false
	args := make([]string, 0, len(argv))
	for _, arg := range argv {
		if arg == "--json" {
			jsonOut = true
		} else {
			args = append(args, arg)
		}
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, version.Version)
		return 0
	}
	if len(args) == 1 && args[0] == "--help-json" {
		return runHelpJSON(stdout, stderr)
	}
	if code, handled := runTextHelp(args, m.prog, stdout, stderr); handled {
		return code
	}
	parsed, err := parse(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if m.toolsSocket != "" {
		if parsed.cmd.operatorOnly {
			fmt.Fprintf(stderr, "mail %s: operator-only; agent mode cannot run it%s\n", parsed.cmd.name, agentRefusalHint[parsed.cmd.name])
			return 2
		}
		if parsed.has("as") {
			fmt.Fprintf(stderr, "mail %s: --as is operator-only; agent mode always acts as the calling agent\n", parsed.cmd.name)
			return 2
		}
	}
	if code := resolveTexts(&parsed, stderr); code != 0 {
		return code
	}
	if m.toolsSocket != "" {
		return runAgent(parsed, newCaller(m.toolsSocket), m.prog, jsonOut, stdout, stderr)
	}
	socket, err := m.socket()
	if err != nil {
		fmt.Fprintf(stderr, "tariboy-mail: %v\n", err)
		return 2
	}
	return runOperator(parsed, newCaller(socket), m.prog, jsonOut, stdout, stderr)
}

var agentRefusalHint = map[string]string{
	"close": " (agents answer a letter with tmail reply)",
	"read":  " (the prompt marks the letters it shows as read)",
}

// resolveTexts reads a --text, or a reply TEXT, of "-" from standard input.
func resolveTexts(parsed *request, stderr io.Writer) int {
	for _, target := range []map[string]string{parsed.flags, parsed.pos} {
		if value, ok := target["text"]; ok {
			text, err := resolveText(value, stdin)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 2
			}
			target["text"] = text
		}
	}
	return 0
}

// payload is the JSON body of a command without as: the agent tool body, and
// the operator body before as is added.
func payload(parsed request) map[string]any {
	body := map[string]any{}
	if id, ok := parsed.pos["id"]; ok {
		body["id"] = id
	}
	if text, ok := parsed.pos["text"]; ok {
		body["text"] = text
	}
	for _, f := range parsed.cmd.flags {
		value, given := parsed.flags[f.name]
		switch {
		case !given || f.name == "as" || f.name == "yes":
		case f.boolean:
			if value == "true" {
				body[f.name] = true
			}
		case f.name == "limit":
			n, _ := strconv.Atoi(value)
			body[f.name] = n
		case f.name == "data":
			var data map[string]any
			_ = json.Unmarshal([]byte(value), &data)
			body[f.name] = data
		default:
			body[f.name] = value
		}
	}
	return body
}

func runAgent(parsed request, caller Caller, prog string, jsonOut bool, stdout, stderr io.Writer) int {
	raw, err := caller.Call("POST", "/tools/mail/"+parsed.cmd.name, payload(parsed))
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) {
			fmt.Fprintf(stderr, "error (%s): %s\n", apiErr.Code, apiErr.Msg)
			return 1
		}
		fmt.Fprintln(stderr, "tools: agent socket is not reachable")
		return 2
	}
	return printResult(parsed, raw, prog, jsonOut, stdout, stderr)
}

// runOperator calls the operator route of a command. The server acts as the
// customer when as is absent, so as is sent only when --as is given.
func runOperator(parsed request, caller Caller, prog string, jsonOut bool, stdout, stderr io.Writer) int {
	method, route, body := operatorRoute(parsed)
	if parsed.cmd.name == "clear" && !parsed.on("yes") && !confirmClear(parsed.flags["as"], stderr) {
		return 0
	}
	var requestBody any = body
	if method == "GET" {
		query := map[string]string{}
		for key, value := range body {
			query[key] = fmt.Sprint(value)
		}
		requestBody = query
	}
	raw, err := caller.Call(method, route, requestBody)
	if err != nil {
		if client.IsDaemonDown(err) {
			fmt.Fprintln(stderr, "tariboyd is not running (start it with: tariboyd)")
			return 2
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return printResult(parsed, raw, prog, jsonOut, stdout, stderr)
}

func operatorRoute(parsed request) (method, route string, body map[string]any) {
	body = payload(parsed)
	if as, ok := parsed.flags["as"]; ok && parsed.cmd.name != "clear" {
		body["as"] = as
	}
	id := url.PathEscape(fmt.Sprint(body["id"]))
	delete(body, "id")
	switch parsed.cmd.name {
	case "inbox", "sent", "archive":
		box := parsed.cmd.name
		if body["all"] == true {
			box = "all"
		}
		delete(body, "all")
		body["box"] = box
		return "GET", "/api/mail", body
	case "show":
		return "GET", "/api/mail/" + id, body
	case "send":
		return "POST", "/api/mail", body
	case "unread":
		return "GET", "/api/mail/unread", body
	case "read":
		body["ids"] = parsed.ids
		return "POST", "/api/mail/read", body
	case "clear":
		name := strings.TrimPrefix(parsed.flags["as"], "agent:")
		return "POST", "/api/agents/" + url.PathEscape(name) + "/mail/clear", body
	}
	// reply, close and requeue act on one letter.
	return "POST", "/api/mail/" + id + "/" + parsed.cmd.name, body
}

// confirmClear asks before deleting an agent's open letters, as compose
// import asks before it applies.
func confirmClear(agent string, stderr io.Writer) bool {
	fmt.Fprintf(stderr, "Delete every open letter in the inbox of %s? [y/N] ", agent)
	var answer string
	if _, err := fmt.Fscan(stdin, &answer); err != nil || (answer != "y" && answer != "Y" && answer != "yes") {
		fmt.Fprintln(stderr, "mail not cleared")
		return false
	}
	return true
}
