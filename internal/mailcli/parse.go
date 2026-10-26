package mailcli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// flagSpec is one flag of a command: a switch when boolean, otherwise it takes
// a value.
type flagSpec struct {
	name    string
	boolean bool
	help    string
}

// command is one tmail command: its positional arguments in order (a name
// ending in "..." takes every remaining word), its flags, and whether agent
// mode refuses it.
type command struct {
	name         string
	positional   []string
	flags        []flagSpec
	operatorOnly bool
	help         commandHelp
}

type commandHelp struct {
	summary, usage, help, arguments, examples string
}

var (
	asFlag     = flagSpec{"as", false, "Principal to act as, user:login or agent:name (operator-only; default the customer)"}
	limitFlag  = flagSpec{"limit", false, "Most letters to list (default 100)"}
	beforeFlag = flagSpec{"before", false, "Cursor of the next page, from the next: line or next_before"}
)

// commands is the tmail command table, in the order help lists them.
var commands = []command{
	{name: "inbox", flags: []flagSpec{{"all", true, "Also list closed letters (boolean, default false)"}, limitFlag, beforeFlag, asFlag}, help: helpText["inbox"]},
	{name: "sent", flags: []flagSpec{limitFlag, beforeFlag, asFlag}, help: helpText["sent"]},
	{name: "archive", flags: []flagSpec{limitFlag, beforeFlag, asFlag}, help: helpText["archive"]},
	{name: "show", positional: []string{"id"}, flags: []flagSpec{asFlag}, help: helpText["show"]},
	{name: "send", flags: []flagSpec{
		{"to", false, "Recipient principals, comma separated: user:login, agent:name, group:name (required)"},
		{"subject", false, "Subject line"},
		{"text", false, "Body text; - reads it from stdin"},
		{"kind", false, "Letter kind (default message)"},
		{"deadline", false, "Reply deadline: a duration such as 5m or an RFC 3339 time"},
		{"notice", true, "Send a notice that needs no reply (boolean, default false)"},
		{"data", false, "Structured data as a JSON object"},
		asFlag,
	}, help: helpText["send"]},
	{name: "reply", positional: []string{"id", "text"}, flags: []flagSpec{
		{"text", false, "Reply text instead of positional TEXT; - reads it from stdin"},
		{"ask", true, "Make the reply a request that needs an answer (boolean, default false)"},
		{"as", false, "Principal to reply as (operator-only); agent:name replies on the agent's behalf"},
	}, help: helpText["reply"]},
	{name: "requeue", positional: []string{"id"}, flags: []flagSpec{asFlag}, help: helpText["requeue"]},
	{name: "close", positional: []string{"id"}, operatorOnly: true, flags: []flagSpec{{"note", false, "Archive note"}, asFlag}, help: helpText["close"]},
	{name: "read", positional: []string{"id..."}, operatorOnly: true, flags: []flagSpec{asFlag}, help: helpText["read"]},
	{name: "unread", operatorOnly: true, flags: []flagSpec{asFlag}, help: helpText["unread"]},
	{name: "clear", operatorOnly: true, flags: []flagSpec{
		{"as", false, "Agent whose open letters to delete, agent:name (required)"},
		{"yes", true, "Delete without asking (boolean, default false)"},
	}, help: helpText["clear"]},
}

func findCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// request is one parsed command line: the command, its positional arguments
// by name, and the flags given, by name. A switch holds "true" or "false".
type request struct {
	cmd   command
	pos   map[string]string
	ids   []string
	flags map[string]string
}

func (r request) has(name string) bool { _, ok := r.flags[name]; return ok }
func (r request) on(name string) bool  { return r.flags[name] == "true" }

type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

func usagef(cmd, format string, args ...any) error {
	return usageError{"mail " + cmd + ": " + fmt.Sprintf(format, args...)}
}

// parse reads a command line. Only a word with two dashes is a flag, so a
// text such as -42 is a positional argument; after "--" every word is.
func parse(argv []string) (request, error) {
	if len(argv) == 0 {
		return request{}, usageError{"mail: a command is required"}
	}
	cmd, ok := findCommand(argv[0])
	if !ok {
		return request{}, usageError{fmt.Sprintf("mail: unknown command %q", argv[0])}
	}
	r := request{cmd: cmd, pos: map[string]string{}, flags: map[string]string{}}
	var words []string
	args := argv[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			words = append(words, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") {
			words = append(words, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg[2:], "=")
		spec, ok := cmd.flag(name)
		if !ok {
			return request{}, usagef(cmd.name, "unknown flag --%s", name)
		}
		switch {
		case spec.boolean && !hasValue:
			value = "true"
		case spec.boolean:
			b, err := strconv.ParseBool(value)
			if err != nil {
				return request{}, usagef(cmd.name, "flag --%s must be true or false", name)
			}
			value = strconv.FormatBool(b)
		case !hasValue:
			if i+1 >= len(args) || (strings.HasPrefix(args[i+1], "--") && args[i+1] != "--") {
				return request{}, usagef(cmd.name, "flag --%s needs a value", name)
			}
			i++
			value = args[i]
		}
		r.flags[name] = value
	}
	return r, r.setPositional(words)
}

func (c command) flag(name string) (flagSpec, bool) {
	for _, f := range c.flags {
		if f.name == name {
			return f, true
		}
	}
	return flagSpec{}, false
}

// setPositional assigns words to the command's positional arguments and
// checks the rules a command has beyond its argument list.
func (r *request) setPositional(words []string) error {
	name := r.cmd.name
	for i, label := range r.cmd.positional {
		if rest, ok := strings.CutSuffix(label, "..."); ok {
			if len(words) <= i {
				return usagef(name, "%s is required", strings.ToUpper(rest))
			}
			r.ids, words = words[i:], nil
			break
		}
		if len(words) <= i {
			if label == "text" && r.has("text") {
				continue
			}
			return usagef(name, "%s is required", strings.ToUpper(label))
		}
		r.pos[label] = words[i]
	}
	if len(words) > len(r.cmd.positional) {
		return usagef(name, "unexpected argument: %s", words[len(r.cmd.positional)])
	}
	switch name {
	case "reply":
		if _, given := r.pos["text"]; given && r.has("text") {
			return usagef(name, "give the text as TEXT or --text, not both")
		}
	case "send":
		if strings.TrimSpace(r.flags["to"]) == "" {
			return usagef(name, "--to is required")
		}
		if r.has("data") {
			var data map[string]any
			if err := json.Unmarshal([]byte(r.flags["data"]), &data); err != nil || data == nil {
				return usagef(name, "--data must be a JSON object")
			}
		}
	case "clear":
		if !strings.HasPrefix(r.flags["as"], "agent:") || strings.TrimPrefix(r.flags["as"], "agent:") == "" {
			return usagef(name, "--as agent:NAME is required")
		}
	}
	for _, f := range []string{"limit"} {
		if r.has(f) {
			if n, err := strconv.Atoi(r.flags[f]); err != nil || n < 0 {
				return usagef(name, "--%s must be a non-negative number", f)
			}
		}
	}
	return nil
}

// resolveText replaces a text of "-" with standard input.
func resolveText(value string, in io.Reader) (string, error) {
	if value != "-" {
		return value, nil
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("mail: read stdin: %w", err)
	}
	return string(data), nil
}
