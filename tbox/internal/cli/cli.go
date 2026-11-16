// Package cli dispatches tbox commands.
package cli

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/version"
)

// Env carries every external dependency of a command so tests can fake them.
type Env struct {
	Args     []string
	Getenv   func(string) string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Cwd      string
	Exec     func(argv0 string, argv, env []string) error
	LookPath func(string) (string, error)
	Now      func() time.Time
}

// Command runs one subcommand with the arguments after its name.
type Command func(*Env, []string) int

// commands is the dispatch table; later tasks register commands here.
var commands = map[string]Command{
	"version": runVersion,
}

// Main dispatches on the first argument and returns the exit code.
func Main(env Env) int {
	if len(env.Args) == 0 {
		return usage(&env, "")
	}
	name := env.Args[0]
	if name == "--version" {
		name = "version"
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

func usage(env *Env, unknown string) int {
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
