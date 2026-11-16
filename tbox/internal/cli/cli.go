// Package cli dispatches tbox commands.
package cli

import (
	"fmt"
	"io"
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
	// StdoutIsTerminal reports whether stdout is a terminal. Nil means it
	// is not.
	StdoutIsTerminal func() bool
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
	switch name {
	case "--version":
		name = "version"
	case "--help", "-h":
		name = "help"
	}
	if dir := imageDir(&env); dir != "" && name != "version" && name != "help" {
		return agentmode.Run(agentHost(&env), dir, env.Args)
	}
	cmd, ok := commands[name]
	if !ok {
		return usage(&env, name)
	}
	if name != "help" && wantsHelp(env.Args[1:]) {
		fmt.Fprint(env.Stdout, commandUsage(name, env.Args[1:]))
		return 0
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
  version                                  print the version
  help                                     print this help
`

const mainUsage = `usage: tbox <command> [args]
commands:
  build [STORE/IMAGE | --path DIR]         build an image (default: the current directory)
  images                                   list built images and the images of the Stores
  image show|rm|export|import|new          show, remove, export or import an image; create a source
  run [IMAGE [HARNESS]] [-- ARGS...]       start a harness session with an image
  store add|list|show|refresh|ref|remove   manage Stores
  skills                                   list the skills of all Stores
  skill STORE/SKILL                        show a Store skill
  prompts                                  list the prompts of all Stores
  prompt STORE/PROMPT                      print a Store prompt
  clean [IMAGE]                            remove run directories and harness files
  version                                  print the version (also --version)
  help                                     print this help (also --help, -h)
Run tbox <command> --help for the usage of a command.
`

const (
	buildUsage  = "usage: tbox build [STORE/IMAGE | --path DIR] [--name N] [--tag T]... [--update]\n"
	cleanUsage  = "usage: tbox clean [IMAGE]\n"
	imageUsage  = "usage: tbox image show REF | rm REF | export REF [-o FILE] [--force] | import FILE | new NAME\n"
	exportUsage = "usage: tbox image export REF [-o FILE] [--force]\n"
	importUsage = "usage: tbox image import FILE\n"
)

// commandUsages is the usage text of each command for --help.
var commandUsages = map[string]string{
	"build":   buildUsage,
	"clean":   cleanUsage,
	"image":   imageUsage,
	"images":  "usage: tbox images\n",
	"run":     runUsage,
	"store":   storeUsage,
	"skills":  "usage: tbox skills\n",
	"skill":   "usage: tbox skill STORE/SKILL\n",
	"prompts": "usage: tbox prompts\n",
	"prompt":  "usage: tbox prompt STORE/PROMPT\n",
	"version": "usage: tbox version\n",
}

// commandUsage is the usage of a command, or of an image subcommand.
func commandUsage(name string, args []string) string {
	if name == "image" && len(args) > 0 {
		switch args[0] {
		case "new":
			return imageNewUsage + "\n"
		case "export":
			return exportUsage
		case "import":
			return importUsage
		}
	}
	if u, ok := commandUsages[name]; ok {
		return u
	}
	return mainUsage
}

// wantsHelp reports whether args ask for help before any "--".
func wantsHelp(args []string) bool {
	for _, a := range args {
		switch a {
		case "--":
			return false
		case "-h", "-help", "--help":
			return true
		}
	}
	return false
}

// usage prints the command list to stderr, after the unknown command when
// one is given, and returns the usage exit code.
func usage(env *Env, unknown string) int {
	if unknown != "" {
		fmt.Fprintf(env.Stderr, "tbox: unknown command %q\n", unknown)
	}
	if imageDir(env) != "" {
		fmt.Fprint(env.Stderr, sessionUsage)
	} else {
		fmt.Fprint(env.Stderr, mainUsage)
	}
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
	if imageDir(env) != "" {
		fmt.Fprint(env.Stdout, sessionUsage)
	} else {
		fmt.Fprint(env.Stdout, mainUsage)
	}
	return 0
}
