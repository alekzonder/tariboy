package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"

	"github.com/alekzonder/tariboy/internal/selfupdate"
)

// dispatchUpdate handles the CLI-local `update` verb. Like the daemon lifecycle
// verbs it must not route through the daemon: it replaces the daemon binary
// itself and has to work while the daemon is down.
func dispatchUpdate(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) (bool, int) {
	if len(args) > 1 && (args[1] == "-h" || args[1] == "--help" || args[1] == "help") {
		printUpdateUsage(out)
		return true, 0
	}
	opts, err := parseUpdateArgs(args[1:])
	if err != nil {
		fmt.Fprintln(errOut, "update:", err)
		printUpdateUsage(errOut)
		return true, 2
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	opts.BaseURL = getenv("TARIBOY_UPDATE_BASE_URL")
	opts.Home = getenv("HOME")
	opts.GOOS = runtime.GOOS
	opts.GOARCH = runtime.GOARCH
	opts.Getenv = getenv
	opts.Out = out
	if err := selfupdate.Run(ctx, opts); err != nil {
		fmt.Fprintln(errOut, "update:", err)
		return true, 1
	}
	return true, 0
}

// parseUpdateArgs reads the optional version argument and the flags. An omitted
// version means the newest published release.
func parseUpdateArgs(args []string) (selfupdate.Options, error) {
	var opts selfupdate.Options
	keepSet := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--force":
			opts.Force = true
		case arg == "--cleanup":
			opts.Cleanup = true
		case arg == "--keep":
			if i+1 == len(args) {
				return opts, fmt.Errorf("--keep requires a value")
			}
			i++
			keep, err := strconv.Atoi(args[i])
			if err != nil || keep < 1 {
				return opts, fmt.Errorf("--keep must be a positive number, got %q", args[i])
			}
			opts.Keep, keepSet = keep, true
		case len(arg) > 0 && arg[0] == '-':
			return opts, fmt.Errorf("unknown flag: %s", arg)
		case opts.Version == "":
			opts.Version = arg
		default:
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	if keepSet && !opts.Cleanup {
		return opts, fmt.Errorf("--keep requires --cleanup")
	}
	if opts.Cleanup && !keepSet {
		opts.Keep = selfupdate.DefaultKeep
	}
	if opts.Version == "" {
		opts.Version = "latest"
	}
	return opts, nil
}

func printUpdateUsage(out io.Writer) {
	fmt.Fprint(out, `Usage: tariboy update [VERSION|latest] [--force] [--cleanup [--keep N]]

Download a published release from GitHub and install it over this
installer-managed installation, then restart a running daemon.

Arguments:
  VERSION      release version to install; 'latest' (the default) resolves the
               newest published release

Flags:
  --force      reinstall even when that version is already active
  --cleanup    after a successful update, delete older releases from
               ~/.local/lib/tariboy; the active release is always kept
  --keep N     with --cleanup, how many of the newest releases to keep
               (default 5)

Only installer-managed linux-x86_64 and linux-aarch64 installations can be
updated. The download root can be overridden with TARIBOY_UPDATE_BASE_URL.
`)
}
