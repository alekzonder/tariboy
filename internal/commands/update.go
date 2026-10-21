package commands

import (
	"fmt"
	"os"
	"runtime"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/selfupdate"
)

// runSelfUpdate is the install step of `update`; tests replace it.
var runSelfUpdate = selfupdate.Run

// update runs in the CLI process: it replaces the daemon and CLI binaries
// themselves, so it must work while the daemon is down.
func update() registry.Command {
	return registry.Command{
		Path:    "update",
		Summary: "Install a published release over this installer-managed installation",
		Help: `Usage: tariboy update [VERSION|latest] [--force] [--cleanup [--keep N]]

Download a published release from GitHub and install it over this
installer-managed installation, then restart a running daemon.

Only installer-managed linux-x86_64 and linux-aarch64 installations can be
updated. The download root can be overridden with TARIBOY_UPDATE_BASE_URL.`,
		Args: []registry.Arg{
			{Name: "release", Type: registry.String, Default: "latest", Help: "release version to install, given as the first argument; 'latest' resolves the newest published release"},
			{Name: "force", Flag: "force", Type: registry.Bool, Help: "reinstall even when that version is already active"},
			{Name: "cleanup", Flag: "cleanup", Type: registry.Bool, Help: "after a successful update, delete older releases from ~/.local/lib/tariboy; the active release is always kept"},
			{Name: "keep", Flag: "keep", Type: registry.Int, Help: fmt.Sprintf("with --cleanup, how many of the newest releases to keep (default %d)", selfupdate.DefaultKeep)},
		},
		Local: func(c *registry.Ctx, p registry.Request) (any, error) {
			opts, err := updateOptions(p.Params)
			if err != nil {
				fmt.Fprintln(c.ErrOut, "update:", err)
				return nil, registry.ExitCode(2)
			}
			opts.BaseURL = os.Getenv("TARIBOY_UPDATE_BASE_URL")
			opts.Home = os.Getenv("HOME")
			opts.GOOS = runtime.GOOS
			opts.GOARCH = runtime.GOARCH
			opts.Getenv = os.Getenv
			opts.Out = c.Out
			if err := runSelfUpdate(p.Ctx, opts); err != nil {
				fmt.Fprintln(c.ErrOut, "update:", err)
				return nil, registry.ExitCode(1)
			}
			return nil, nil
		},
	}
}

// updateOptions checks the flag combination. An omitted version means the
// newest published release.
func updateOptions(p registry.Params) (selfupdate.Options, error) {
	opts := selfupdate.Options{
		Version: p.String("release"),
		Force:   p.Bool("force"),
		Cleanup: p.Bool("cleanup"),
	}
	keep, keepSet := p["keep"]
	if keepSet {
		opts.Keep = p.Int("keep")
		if opts.Keep < 1 {
			return opts, fmt.Errorf("--keep must be a positive number, got %v", keep)
		}
		if !opts.Cleanup {
			return opts, fmt.Errorf("--keep requires --cleanup")
		}
	}
	if opts.Cleanup && !keepSet {
		opts.Keep = selfupdate.DefaultKeep
	}
	if opts.Version == "" {
		opts.Version = "latest"
	}
	return opts, nil
}
