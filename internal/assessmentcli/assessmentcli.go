// Package assessmentcli is the `tariboy assessment` client. With the agent
// tools socket (TARIBOY_TOOLS_SOCKET) every command goes to the agent's
// routes and add records an assessment as that agent. Without it ls and show
// read through the operator API and add refuses: operators cannot record
// assessments.
package assessmentcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/client"
)

// Caller is the daemon transport.
type Caller interface {
	Call(method, route string, body any) (json.RawMessage, error)
}

var newCaller = func(socket string) Caller { return client.New(socket) }

const usage = `Usage:
  tariboy assessment add --file PATH        record an assessment (agent tools socket only; PATH - reads stdin)
  tariboy assessment ls [filters] [--limit N] [--history]
      filters: --iteration ID --task KEY --agent NAME --assessor NAME --image REF --image-version VERSION
      lists the latest assessment of each iteration and task; --history lists every assessment
  tariboy assessment show ID

Every command accepts --json to print the daemon's result as is.
`

// lsFlags maps the ls flags to their query parameters.
var lsFlags = map[string]string{
	"--iteration": "iteration", "--task": "task", "--agent": "agent", "--assessor": "assessor",
	"--image": "image", "--image-version": "image_version", "--limit": "limit",
}

// Run runs one assessment command. toolsSocket selects agent mode when set;
// otherwise operatorSocket is the daemon socket.
func Run(argv []string, toolsSocket, operatorSocket string, stdin io.Reader, stdout, stderr io.Writer) int {
	jsonOut := false
	args := make([]string, 0, len(argv))
	for _, a := range argv {
		if a == "--json" {
			jsonOut = true
		} else {
			args = append(args, a)
		}
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	method, route, body, code := request(args, toolsSocket != "", stdin, stderr)
	if code != 0 {
		return code
	}
	socket := operatorSocket
	if toolsSocket != "" {
		socket = toolsSocket
	}
	raw, err := newCaller(socket).Call(method, route, body)
	if err != nil {
		var apiErr *client.APIError
		switch {
		case errors.As(err, &apiErr):
			fmt.Fprintf(stderr, "error (%s): %s\n", apiErr.Code, apiErr.Msg)
			return 1
		case toolsSocket != "":
			fmt.Fprintln(stderr, "tools: agent socket is not reachable")
			return 2
		case client.IsDaemonDown(err):
			fmt.Fprintln(stderr, "tariboyd is not running (start it with: tariboyd)")
			return 2
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if jsonOut {
		fmt.Fprintln(stdout, string(raw))
		return 0
	}
	if err := printResult(args[0], raw, stdout); err != nil {
		fmt.Fprintf(stderr, "assessment %s: %v\n", args[0], err)
		return 1
	}
	return 0
}

// request parses args into the daemon call, or returns a non-zero exit code
// after printing why.
func request(args []string, agent bool, stdin io.Reader, stderr io.Writer) (method, route string, body any, code int) {
	fail := func(format string, a ...any) (string, string, any, int) {
		fmt.Fprintf(stderr, format+"\n", a...)
		return "", "", nil, 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "add":
		if !agent {
			return fail("assessment add: needs the agent tools socket (TARIBOY_TOOLS_SOCKET); operators cannot record assessments")
		}
		if len(rest) != 2 || rest[0] != "--file" {
			return fail("usage: tariboy assessment add --file PATH (- reads stdin)")
		}
		in, err := readInput(rest[1], stdin)
		if err != nil {
			return fail("assessment add: %v", err)
		}
		return "POST", "/tools/assessment/add", in, 0
	case "ls":
		query := map[string]string{}
		for i := 0; i < len(rest); i += 2 {
			if rest[i] == "--history" { // the one flag without a value
				query["history"] = "true"
				i-- // the loop step skips one argument only
				continue
			}
			key, ok := lsFlags[rest[i]]
			if !ok || i+1 >= len(rest) {
				return fail("assessment ls: unknown flag or missing value %q\n%s", rest[i], usage)
			}
			if key == "limit" {
				if _, err := strconv.Atoi(rest[i+1]); err != nil {
					return fail("assessment ls: --limit must be a number")
				}
			}
			query[key] = rest[i+1]
		}
		if agent {
			return "GET", "/tools/assessment/ls", query, 0
		}
		return "GET", "/api/assessments", query, 0
	case "show":
		if len(rest) != 1 || strings.HasPrefix(rest[0], "-") {
			return fail("usage: tariboy assessment show ID")
		}
		if agent {
			return "GET", "/tools/assessment/show/" + url.PathEscape(rest[0]), nil, 0
		}
		return "GET", "/api/assessments/" + url.PathEscape(rest[0]), nil, 0
	}
	return fail("assessment: unknown command %q\n%s", cmd, usage)
}

// readInput reads the JSON object of an add from path, or from stdin for -.
func readInput(path string, stdin io.Reader) (map[string]any, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var in map[string]any
	if err := json.Unmarshal(data, &in); err != nil || in == nil {
		return nil, errors.New("the input must be a JSON object")
	}
	return in, nil
}
