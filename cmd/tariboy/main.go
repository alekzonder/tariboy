package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/cli"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/taskcli"
	"github.com/alekzonder/tariboy/internal/version"
)

func main() {
	// Ctrl-C / SIGTERM cancels follow-mode composites (logs -f, channel tail -f)
	// and local verbs (daemon logs -f, compose) so they exit cleanly instead of
	// being hard-killed mid-print.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(out, version.Version)
		return 0
	}
	sock := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--version":
			fmt.Fprintln(out, version.Version)
			return 0
		case args[i] == "--socket" && i+1 < len(args):
			i++
			sock = args[i]
		default:
			rest = append(rest, args[i])
		}
	}
	p, err := paths.Resolve(os.Getenv)
	if err != nil {
		fmt.Fprintln(errOut, "tariboy:", err)
		return 2
	}
	if sock == "" {
		sock = p.Socket()
	}
	if argv, ok := tasksArgs(rest); ok {
		return taskcli.RunOperator(ctx, argv, sock, out, errOut)
	}
	// compose has always accepted its flags before the verb
	// (`compose -f FILE up`); the registry resolves the verb first.
	if len(rest) > 0 && rest[0] == "compose" {
		rest = append([]string{"compose"}, compose.VerbFirst(rest[1:])...)
	}
	local := &registry.Ctx{
		Log:       slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelWarn})),
		BaseDir:   p.Base,
		Socket:    sock,
		Version:   version.Version,
		StartedAt: time.Now(),
		// Store stays nil: CLI-local commands touch the filesystem, not the DB.
	}
	reg := commands.BuildRegistry()
	caller := client.New(sock)
	// Plugin discovery is best-effort so core help and local commands still work
	// while the daemon is down. A contributed command itself necessarily needs
	// the daemon that supplied its schema. A core command skips it: plugins
	// cannot claim a core name, and `daemon stop` must not wait on a wedged
	// daemon.
	if !isCoreCommand(reg, rest) {
		_ = cli.LoadPluginCommands(reg, caller, os.Stdin)
	}
	return cli.Run(ctx, reg, rest, caller, local, out, errOut)
}

// tasksArgs reports whether argv's first word is tasks and returns argv
// without it: tariboy tasks is the ttasks client, always in operator mode.
func tasksArgs(argv []string) ([]string, bool) {
	for i, a := range argv {
		if strings.HasPrefix(a, "-") || a == "help" {
			continue
		}
		if a != "tasks" {
			return nil, false
		}
		return append(slices.Clone(argv[:i]), argv[i+1:]...), true
	}
	return nil, false
}

// isCoreCommand reports whether argv's first word names a core group or
// command.
func isCoreCommand(reg *registry.Registry, argv []string) bool {
	for _, a := range argv {
		if strings.HasPrefix(a, "-") || a == "help" {
			continue
		}
		_, group := reg.Group(a)
		_, command := reg.Get(a)
		return group || command
	}
	return false
}
