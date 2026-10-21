package taskcli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/alekzonder/tariboy/internal/tasks"
)

type request struct {
	action string
	spec   tasks.Spec
	// input is the parsed request struct; payload is its tools-socket body.
	input   any
	payload map[string]any
	// noWait makes advance return a pending request at once instead of waiting
	// for its checks. It is a client option and never part of the payload.
	noWait bool
	// args are the command's own arguments, so list can print the command for
	// its next page.
	args []string
}
type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

// maxArtifactInput is one byte more than the daemon accepts, so an oversize
// value reaches the server and is refused there instead of being cut short.
const maxArtifactInput = 64<<10 + 1

// parse reads a task command from the tasks.Specs table: find its spec, parse
// its flags and positional arguments, then check its cross-field rules.
func parse(argv []string) (request, error) {
	spec, rest, err := findCommand(argv)
	if err != nil {
		return request{}, err
	}
	input, err := parseArgs(spec, rest)
	if err != nil {
		return request{}, err
	}
	parsed := request{action: spec.Action, spec: spec, input: input, payload: tasks.Payload(input), args: rest}
	if advance, ok := input.(*tasks.AdvanceRequest); ok {
		parsed.noWait = advance.NoWait
	}
	return parsed, nil
}

// cliSpec returns the command a ttasks word or word pair names.
func cliSpec(words ...string) (tasks.Spec, bool) {
	for _, spec := range tasks.Specs() {
		if len(spec.Words) > 0 && strings.Join(spec.Words, " ") == strings.Join(words, " ") {
			return spec, true
		}
	}
	return tasks.Spec{}, false
}

func findCommand(argv []string) (tasks.Spec, []string, error) {
	if len(argv) == 0 {
		return tasks.Spec{}, nil, usageError{"tasks: a command is required"}
	}
	if spec, ok := cliSpec(argv[0]); ok {
		return spec, argv[1:], nil
	}
	if !helpGroupHasCommands(argv[0]) {
		return tasks.Spec{}, nil, usageError{fmt.Sprintf("tasks: unknown command %q", argv[0])}
	}
	if len(argv) == 1 || strings.HasPrefix(argv[1], "--") {
		return tasks.Spec{}, nil, usageError{fmt.Sprintf("tasks %s: a command is required", argv[0])}
	}
	spec, ok := cliSpec(argv[0], argv[1])
	if !ok {
		return tasks.Spec{}, nil, usageError{fmt.Sprintf("tasks: unknown command %q", argv[0]+" "+argv[1])}
	}
	return spec, argv[2:], nil
}

// helpGroupHasCommands reports whether word starts a two-word command.
func helpGroupHasCommands(word string) bool {
	for _, spec := range tasks.Specs() {
		if len(spec.Words) == 2 && spec.Words[0] == word {
			return true
		}
	}
	return false
}

// parseArgs fills a new request of spec from args. Positional arguments are
// checked first, then flags in field order, then the request's Validate, then
// arguments left over.
func parseArgs(spec tasks.Spec, args []string) (any, error) {
	command := strings.Join(spec.Words, " ")
	fail := func(err error) error { return usageError{fmt.Sprintf("tasks %s: %v", command, err)} }
	input := spec.New()
	fields := tasks.Fields(input)
	texts, pos, err := scanFlags(command, fields, input, args)
	if err != nil {
		return nil, err
	}
	extra := 0
	for _, f := range fields {
		if f.Pos < 0 {
			continue
		}
		if err := setPositional(f, input, pos); err != nil {
			return nil, fail(err)
		}
		extra = max(extra, f.Pos+1)
		if f.Rest {
			extra = len(pos)
		}
	}
	for i, f := range fields {
		if text, given := texts[i]; given {
			if err := f.Set(input, text, "--"+f.Flag); err != nil {
				return nil, fail(err)
			}
		}
		if f.Flag != "" && f.CLIRequired && f.IsZero(input) {
			return nil, fail(fmt.Errorf("--%s is required", f.Flag))
		}
	}
	if v, ok := input.(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return nil, fail(err)
		}
	}
	if len(pos) > extra {
		return nil, fail(fmt.Errorf("unexpected argument: %s", pos[extra]))
	}
	return input, nil
}

func setPositional(f tasks.Field, input any, pos []string) error {
	if f.Rest {
		if len(pos) > f.Pos {
			f.SetWords(input, pos[f.Pos:])
		}
		return nil
	}
	if len(pos) <= f.Pos || (f.CLIRequired && strings.TrimSpace(pos[f.Pos]) == "") {
		if f.CLIRequired {
			return fmt.Errorf("%s is required", f.Label)
		}
		return nil
	}
	return f.Set(input, pos[f.Pos], f.Label)
}

// scanFlags parses args with a flag.FlagSet, taking a positional argument
// wherever the flags stop and parsing the rest again; after "--" every
// argument is positional. Only a word with two dashes is a flag: a word such
// as -42 or -v is text, so the FlagSet never sees it. A boolean flag is set at once; the text of any other
// flag is returned by field index, so it is checked after the positionals.
func scanFlags(command string, fields []tasks.Field, input any, args []string) (map[int]string, []string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	texts := map[int]string{}
	valueFlags := map[string]bool{}
	badBool := ""
	for i, f := range fields {
		switch {
		case f.Flag == "":
		case f.Kind == tasks.KindBool:
			fs.Var(boolFlag{field: f, input: input, failed: &badBool}, f.Flag, "")
		default:
			fs.Var(textFlag{texts: texts, index: i}, f.Flag, "")
			valueFlags[f.Flag] = true
		}
	}
	if err := checkFlagValues(fs, valueFlags, args); err != nil {
		return nil, nil, err
	}
	var pos []string
	for {
		end := textWord(args, valueFlags)
		if err := fs.Parse(args[:end]); err != nil {
			return nil, nil, flagError(err, badBool)
		}
		rest := append(fs.Args(), args[end:]...)
		if len(rest) == 0 {
			return texts, pos, nil
		}
		if consumed := end - len(fs.Args()); consumed > 0 && args[consumed-1] == "--" {
			return texts, append(pos, rest...), nil
		}
		pos, args = append(pos, rest[0]), rest[1:]
	}
}

// textWord returns the index of the first single-dash word in args that is
// not the value of a preceding flag, or len(args).
func textWord(args []string, valueFlags map[string]bool) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return len(args)
		case len(arg) > 1 && arg[0] == '-' && arg[1] != '-':
			return i
		case strings.HasPrefix(arg, "--") && !strings.Contains(arg, "=") && valueFlags[arg[2:]]:
			i++
		}
	}
	return len(args)
}

// checkFlagValues refuses a value flag followed by nothing or by another
// --flag, which flag.FlagSet would take as its value. It stops at the first
// flag the command does not have, which FlagSet reports.
func checkFlagValues(fs *flag.FlagSet, valueFlags map[string]bool, args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil
		}
		name := strings.TrimPrefix(arg, "--")
		if !strings.HasPrefix(arg, "--") || name == "" || strings.Contains(name, "=") {
			continue
		}
		if fs.Lookup(name) == nil {
			return nil
		}
		if valueFlags[name] {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return usageError{fmt.Sprintf("flag --%s needs a value", name)}
			}
			i++
		}
	}
	return nil
}

// flagError restates a flag.FlagSet error in the client's words.
func flagError(err error, badBool string) error {
	text := err.Error()
	switch {
	case badBool != "":
		return usageError{fmt.Sprintf("flag --%s must be true or false", badBool)}
	case errors.Is(err, flag.ErrHelp):
		return usageError{"unknown flag --help"}
	case strings.HasPrefix(text, "flag provided but not defined: "):
		return usageError{"unknown flag --" + strings.TrimLeft(strings.TrimPrefix(text, "flag provided but not defined: "), "-")}
	case strings.HasPrefix(text, "flag needs an argument: "):
		return usageError{fmt.Sprintf("flag --%s needs a value", strings.TrimLeft(strings.TrimPrefix(text, "flag needs an argument: "), "-"))}
	}
	return usageError{text}
}

// textFlag keeps the text of a value flag for parseArgs.
type textFlag struct {
	texts map[int]string
	index int
}

func (f textFlag) String() string { return "" }
func (f textFlag) Set(text string) error {
	f.texts[f.index] = text
	return nil
}

// boolFlag is a switch: --claim, or --claim=true or --claim=false.
type boolFlag struct {
	field  tasks.Field
	input  any
	failed *string
}

func (f boolFlag) String() string   { return "" }
func (f boolFlag) IsBoolFlag() bool { return true }
func (f boolFlag) Set(text string) error {
	if err := f.field.Set(f.input, text, f.field.Flag); err != nil {
		*f.failed = f.field.Flag
		return err
	}
	return nil
}

// commandFlags maps each flag of a command to whether it is a switch.
func commandFlags(spec tasks.Spec) map[string]bool {
	out := map[string]bool{}
	for _, f := range tasks.Fields(spec.Request) {
		if f.Flag != "" {
			out[f.Flag] = f.Kind == tasks.KindBool
		}
	}
	return out
}

// commandName is the command as the user types it: "artifacts set" for the
// action artifact_set.
func commandName(action string) string {
	return strings.Join(sharedHelpPath(action), " ")
}
