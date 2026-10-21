package commands

import (
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/registry"
)

// composeVerbs describes each `tariboy compose VERB`. The verbs run in the CLI
// process: they read a compose file next to the operator and drive the daemon
// API, and `up` starts the daemon first.
var composeVerbs = map[string]struct {
	summary string
	args    []registry.Arg
}{
	"up":      {summary: "Build, create and start the file's images, groups and agents; start the daemon first"},
	"down":    {summary: "Remove the file's agents and groups; --volumes also deletes their data"},
	"start":   {summary: "Start the file's agents, or the named agents and groups", args: composeMembers()},
	"stop":    {summary: "Stop the file's agents, or the named agents and groups", args: composeMembers()},
	"restart": {summary: "Restart the file's agents, or the named agents and groups", args: composeMembers()},
	"kill":    {summary: "Kill the current iteration of the file's agents, or the named agents and groups", args: composeMembers()},
	"rm":      {summary: "Remove the file's stopped agents, or the named agents and groups", args: composeMembers()},
	"logs":    {summary: "Print recent events of the file's agents, or the named agents and groups", args: composeMembers()},
	"exec": {summary: "Run an iteration of one of the file's agents now, with an optional prompt", args: []registry.Arg{
		{Name: "member", Type: registry.String, Required: true, Help: "agent in the compose file"},
		{Name: "prompt", Type: registry.String, Repeatable: true, Rest: true, Help: "one-shot prompt; the words are joined by spaces"},
	}},
	"ps":      {summary: "List the file's agents and their live state"},
	"status":  {summary: "Compare the file with the daemon and report drift"},
	"build":   {summary: "Build the file's images"},
	"archive": {summary: "Create a compose-only portable team archive; transfer runnable images separately"},
	"import":  {summary: "Preview and import compose-only team/runtime configuration"},
}

func composeMembers() []registry.Arg {
	return []registry.Arg{{Name: "members", Type: registry.String, Repeatable: true, Rest: true, Help: "agent or group names (default: every agent in the file)"}}
}

// composeFlags are accepted by every verb, as they always were; each flag
// names the verb that reads it.
func composeFlags() []registry.Arg {
	return []registry.Arg{
		{Name: "file", Flag: "file", Short: "f", Type: registry.String, Default: compose.DefaultFile, Help: "compose file"},
		{Name: "no_start", Flag: "no-start", Type: registry.Bool, Help: "up: do not start the daemon (also TARIBOY_NO_AUTOSTART)"},
		{Name: "volumes", Flag: "volumes", Short: "v", Type: registry.Bool, Help: "down: also delete agent data and group shared dirs"},
		{Name: "tail", Flag: "tail", Type: registry.Int, Default: compose.DefaultTail, Help: "logs: events per agent"},
		{Name: "output", Flag: "output", Type: registry.String, Help: "archive: archive file to create (required)"},
		{Name: "archive", Flag: "archive", Type: registry.String, Help: "import: archive file to upload (required)"},
		{Name: "yes", Flag: "yes", Type: registry.Bool, Help: "import: apply without asking"},
	}
}

func composeCommands() []registry.Command {
	out := make([]registry.Command, 0, len(compose.Verbs))
	for _, verb := range compose.Verbs {
		spec := composeVerbs[verb]
		out = append(out, registry.Command{
			Path: "compose." + verb, Summary: spec.summary,
			Args:  append(append([]registry.Arg{}, spec.args...), composeFlags()...),
			Local: composeLocal(verb),
		})
	}
	return out
}

func composeLocal(verb string) registry.HandlerFunc {
	return func(c *registry.Ctx, p registry.Request) (any, error) {
		opts := compose.Options{
			Verb:    verb,
			File:    p.String("file"),
			Output:  p.String("output"),
			Archive: p.String("archive"),
			Yes:     p.Bool("yes"),
			Volumes: p.Bool("volumes"),
			NoStart: p.Bool("no_start"),
			Tail:    p.IntOr("tail", compose.DefaultTail),
		}
		for _, name := range []string{"member", "members", "prompt"} {
			opts.Args = append(opts.Args, positionals(p.Params, name)...)
		}
		images := filepath.Join(c.BaseDir, "images")
		if code := compose.Run(p.Ctx, client.New(c.Socket), images, opts, c.Out, c.ErrOut); code != 0 {
			return nil, registry.ExitCode(code)
		}
		return nil, nil
	}
}

// positionals reads a positional argument as it was given, without the comma
// splitting of Params.Strings.
func positionals(p registry.Params, name string) []string {
	switch v := p[name].(type) {
	case string:
		return []string{v}
	case []string:
		return v
	}
	return nil
}
