// Package cli dispatches tbox commands.
package cli

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/agentmode"
	"github.com/alekzonder/tariboy/tbox/internal/tui"
	"github.com/alekzonder/tariboy/tbox/internal/version"
)

// Env carries every external dependency of a command so tests can fake them.
type Env struct {
	Args     []string
	Getenv   func(string) string
	Environ  func() []string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Cwd      string
	Exec     func(argv0 string, argv, env []string) error
	LookPath func(string) (string, error)
	Now      func() time.Time
	// OpenTerminal puts the terminal into raw mode for an interactive list.
	// Nil, or tui.ErrNoTerminal, means there is no terminal.
	OpenTerminal func() (tui.Terminal, error)
}

// Command runs one subcommand with the arguments after its name.
type Command func(*Env, []string) int

// commands is the dispatch table; later tasks register commands here.
var commands = map[string]Command{
	"version": runVersion,
}

func init() { commands["help"] = runHelp }

// Main dispatches on the first argument and returns the exit code.
func Main(env Env) int {
	if len(env.Args) == 0 {
		return usage(&env, "")
	}
	name := env.Args[0]
	if name == "--version" {
		name = "version"
	}
	if dir := imageDir(&env); dir != "" && name != "version" && name != "help" {
		return agentmode.Run(agentHost(&env), dir, env.Args)
	}
	cmd, ok := commands[name]
	if !ok {
		return usage(&env, name)
	}
	return cmd(&env, env.Args[1:])
}

func runVersion(env *Env, _ []string) int {
	fmt.Fprintln(env.Stdout, version.Version)
	return 0
}

const sessionUsage = `usage: tbox <command> [args]
commands:
  skills                                   list the skills of the image
  skill NAME                               show a skill
  skill exec NAME SCRIPT [ARGS...]         run a script of a skill
  version
  help
`

func usage(env *Env, unknown string) int {
	if imageDir(env) != "" {
		if unknown != "" {
			fmt.Fprintf(env.Stderr, "tbox: unknown command %q\n", unknown)
		}
		fmt.Fprint(env.Stderr, sessionUsage)
		return 2
	}
	if unknown != "" {
		fmt.Fprintf(env.Stderr, "tbox: unknown command %q\n", unknown)
	}
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintf(env.Stderr, "usage: tbox <command> [args]\ncommands: %v\n", names)
	return 2
}

// imageDir is the session image directory. A non-empty value switches every
// command to agent mode.
func imageDir(env *Env) string {
	if env.Getenv == nil {
		return ""
	}
	return env.Getenv(agentmode.EnvImageDir)
}

func agentHost(env *Env) agentmode.Host {
	var environ []string
	if env.Environ != nil {
		environ = env.Environ()
	}
	return agentmode.Host{Stdout: env.Stdout, Stderr: env.Stderr, Environ: environ, Exec: env.Exec}
}

func runHelp(env *Env, _ []string) int {
	usage(env, "")
	return 0
}
