package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

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
	version, force, err := parseUpdateArgs(args[1:])
	if err != nil {
		fmt.Fprintln(errOut, "update:", err)
		printUpdateUsage(errOut)
		return true, 2
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	err = selfupdate.Run(ctx, selfupdate.Options{
		Version: version,
		Force:   force,
		BaseURL: getenv("TARIBOY_UPDATE_BASE_URL"),
		Home:    getenv("HOME"),
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
		Getenv:  getenv,
		Out:     out,
	})
	if err != nil {
		fmt.Fprintln(errOut, "update:", err)
		return true, 1
	}
	return true, 0
}

// parseUpdateArgs reads the optional version argument and --force. An omitted
// version means the newest published release.
func parseUpdateArgs(args []string) (version string, force bool, err error) {
	for _, arg := range args {
		switch {
		case arg == "--force":
			force = true
		case len(arg) > 0 && arg[0] == '-':
			return "", false, fmt.Errorf("unknown flag: %s", arg)
		case version == "":
			version = arg
		default:
			return "", false, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	if version == "" {
		version = "latest"
	}
	return version, force, nil
}

func printUpdateUsage(out io.Writer) {
	fmt.Fprint(out, `Usage: tariboy update [VERSION|latest] [--force]

Download a published release from GitHub and install it over this
installer-managed installation, then restart a running daemon.

Arguments:
  VERSION      release version to install; 'latest' (the default) resolves the
               newest published release

Flags:
  --force      reinstall even when that version is already active

Only installer-managed linux-x86_64 installations can be updated. The download
root can be overridden with TARIBOY_UPDATE_BASE_URL.
`)
}
