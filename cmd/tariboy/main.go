package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/cli"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/version"
)

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(version.Version)
		return
	}
	sock := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--version":
			fmt.Println(version.Version)
			return
		case args[i] == "--socket" && i+1 < len(args):
			i++
			sock = args[i]
		default:
			rest = append(rest, args[i])
		}
	}
	p, err := paths.Resolve(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tariboy:", err)
		os.Exit(2)
	}
	if sock == "" {
		sock = p.Socket()
	}
	if len(rest) > 0 && rest[0] == "compose" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := compose.Main(ctx, client.New(sock), p.ImagesDir(), rest[1:], os.Stdout, os.Stderr)
		stop()
		os.Exit(code)
	}
	// Daemon lifecycle verbs (up/down/status/logs) are CLI-local: they spawn or
	// signal the daemon and tail its log file, so they must not route over the
	// socket (the daemon may be down). Other daemon.* verbs (config, reindex)
	// fall through to the registry.
	if len(rest) > 0 && rest[0] == "daemon" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		if handled, code := dispatchDaemon(ctx, rest, os.Getenv, os.Stdout, os.Stderr); handled {
			stop()
			os.Exit(code)
		}
		stop()
	}
	local := &registry.Ctx{
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		BaseDir:   p.Base,
		Socket:    sock,
		Version:   version.Version,
		StartedAt: time.Now(),
		// Store stays nil: CLI-local commands touch the filesystem, not the DB.
	}
	// Ctrl-C / SIGTERM cancels follow-mode composites (logs -f, channel tail -f)
	// so they exit cleanly instead of being hard-killed mid-print.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, commands.BuildRegistry(), rest, client.New(sock), local, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
