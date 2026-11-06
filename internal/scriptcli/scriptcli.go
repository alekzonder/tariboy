// Package scriptcli is tscripts (tariboy-scripts): the agent client of the
// /tools/script/* API on TARIBOY_TOOLS_SOCKET. It replaces scripts.py of the
// store skill `scripts` and keeps its output, errors and exit codes byte for
// byte; testdata/golden.json holds the reference output.
package scriptcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/version"
)

const usage = "usage: scripts.sh <run|schedule|ls|runs|logs|rerun|cancel|rm> ... [--json]"

// usageError is a malformed command line: exit code 2.
type usageError string

func (e usageError) Error() string { return string(e) }

var errNoSocket = errors.New("tools: TARIBOY_TOOLS_SOCKET is not set (are you running inside an agent?)")

type cli struct {
	lookupEnv func(string) (string, bool)
	jsonOut   bool
	stdout    io.Writer
}

// Run runs one tscripts command and returns its exit code. lookupEnv tells an
// unset TARIBOY_TOOLS_SOCKET from an empty one, as scripts.py does.
func Run(argv []string, lookupEnv func(string) (string, bool), stdout, stderr io.Writer) int {
	args := slices.Clone(argv)
	jsonEnv, _ := lookupEnv("TARIBOY_TOOLS_JSON")
	jsonOut := jsonEnv == "1"
	if i := slices.Index(args, "--json"); i >= 0 {
		args = slices.Delete(args, i, i+1)
		jsonOut = true
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(stdout, usage)
		return 0
	}
	// --version is not in scripts.py; release checks call it on every binary.
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, version.Version)
		return 0
	}
	c := cli{lookupEnv: lookupEnv, jsonOut: jsonOut, stdout: stdout}
	err := c.run(args)
	if err == nil {
		return 0
	}
	var usageErr usageError
	var apiErr *client.APIError
	var urlErr *url.Error
	switch {
	case errors.Is(err, errNoSocket), errors.As(err, &usageErr):
		fmt.Fprintln(stderr, err)
		return 2
	case errors.As(err, &urlErr):
		fmt.Fprintln(stderr, "tools: agent socket is not reachable")
		return 2
	case errors.As(err, &apiErr):
		msg := apiErr.Msg
		if msg == "" {
			msg = "daemon returned failure without error detail"
		}
		fmt.Fprintln(stderr, msg)
		return 1
	default:
		fmt.Fprintln(stderr, err)
		return 1
	}
}

func (c cli) run(args []string) error {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch {
	case cmd == "run":
		return c.create(args, false)
	case cmd == "schedule":
		return c.create(args, true)
	case cmd == "ls" && len(args) == 1:
		return c.call("GET", "/tools/script/ls", nil)
	case cmd == "rerun" || cmd == "cancel" || cmd == "rm" || cmd == "runs" || cmd == "logs":
		_, pos, err := parseFlags(args, 1)
		if err != nil {
			return err
		}
		if len(pos) == 0 {
			return usageError("tools script " + cmd + ": <id> is required")
		}
		if cmd == "runs" || cmd == "logs" {
			// The ID goes into the path unescaped, as in scripts.py.
			return c.call("GET", "/tools/script/"+cmd+"/"+pos[0], nil)
		}
		return c.call("POST", "/tools/script/"+cmd, map[string]any{"id": pos[0]})
	}
	return usageError("tools script: unknown command " + strings.Join(args, " "))
}

func (c cli) create(args []string, scheduled bool) error {
	action := "run"
	allowed := []string{"description"}
	if scheduled {
		action = "schedule"
		allowed = append(allowed, "every")
	}
	sep := -1
	if len(args) >= 3 {
		if i := slices.Index(args[2:], "--"); i >= 0 {
			sep = i + 2
		}
	}
	if sep < 0 {
		return usageError("tools script " + action + ": NAME [options] -- COMMAND required")
	}
	name := args[1]
	if scheduled && slices.ContainsFunc(args[2:sep], func(arg string) bool {
		return arg == "--quiet-exit" || strings.HasPrefix(arg, "--quiet-exit=")
	}) {
		return usageError("tools script schedule: --quiet-exit is not supported; a recurring command exits with $TARIBOY_QUIET_EXIT (111) to stay quiet")
	}
	flags, pos, err := parseFlags(args[:sep], 2, allowed...)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageError(fmt.Sprintf("tools script %s: unexpected argument \"%s\" before --", action, pos[0]))
	}
	command := strings.Join(args[sep+1:], " ")
	if strings.TrimSpace(command) == "" {
		return usageError("tools script " + action + ": command is required after --")
	}
	description, ok := flags["description"]
	if !ok {
		description = name
	}
	body := map[string]any{"name": name, "description": description, "command": command}
	if scheduled {
		// strconv.Atoi takes the sign and digits Python int() takes, except underscores.
		every, err := strconv.Atoi(strings.TrimSpace(flags["every"]))
		if err != nil || every <= 0 {
			return usageError("tools script schedule: --every must be a positive number of seconds")
		}
		body["interval_seconds"] = every
	}
	return c.call("POST", "/tools/script/"+action, body)
}

// parseFlags is parse_flags of scripts.py: --name=value or --name value, a
// bare flag is "true", and a flag outside allowed is an error.
func parseFlags(args []string, start int, allowed ...string) (map[string]string, []string, error) {
	values := map[string]string{}
	var positionals []string
	for i := start; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg[2:], "=")
		if !slices.Contains(allowed, name) {
			return nil, nil, usageError("unknown flag --" + name)
		}
		if !hasValue {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				value = args[i]
			} else {
				value = "true"
			}
		}
		values[name] = value
	}
	return values, positionals, nil
}

func (c cli) call(method, route string, body any) error {
	socket, ok := c.lookupEnv("TARIBOY_TOOLS_SOCKET")
	if !ok {
		return errNoSocket
	}
	raw, err := client.New(socket).Call(method, route, body)
	if err != nil {
		return err
	}
	out, err := formatResult(raw, c.jsonOut)
	if err != nil {
		return err
	}
	_, err = io.WriteString(c.stdout, out)
	return err
}

// formatResult is print_result of scripts.py.
func formatResult(raw json.RawMessage, jsonOut bool) (string, error) {
	if jsonOut {
		s, err := pyDumps(raw)
		return s + "\n", err
	}
	var value any
	if err := decodeNumber(raw, &value); err != nil {
		return "", err
	}
	switch v := value.(type) {
	case map[string]any:
		var b strings.Builder
		for _, key := range sortedKeys(v) {
			b.WriteString(key + ": " + formatValue(v[key]) + "\n")
		}
		return b.String(), nil
	case string:
		s := pyString(v)
		return s[1:len(s)-1] + "\n", nil
	}
	s, err := pyDumps(raw)
	return s + "\n", err
}
