package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alekzonder/tariboy/internal/daemonctl"
	"github.com/alekzonder/tariboy/internal/registry"
)

// The daemon lifecycle verbs run in the CLI process: they spawn or signal the
// daemon and tail its log file, so they must work while the daemon is down.

func daemonStart() registry.Command {
	return daemonLifecycle("daemon.start", "Start tariboyd detached (no-op if already running)", func(c *registry.Ctx, p registry.Request, cfg daemonctl.Config) error {
		_, err := daemonctl.EnsureUp(p.Ctx, cfg, c.Out)
		return err
	})
}

func daemonStop() registry.Command {
	return daemonLifecycle("daemon.stop", "Stop the daemon (SIGTERM, then SIGKILL on timeout)", func(c *registry.Ctx, p registry.Request, cfg daemonctl.Config) error {
		return daemonctl.Down(p.Ctx, cfg, c.Out)
	})
}

func daemonRestart() registry.Command {
	return daemonLifecycle("daemon.restart", "Restart the daemon (stop then start)", func(c *registry.Ctx, p registry.Request, cfg daemonctl.Config) error {
		return daemonctl.Restart(p.Ctx, cfg, c.Out)
	})
}

func daemonLogs() registry.Command {
	c := daemonLifecycle("daemon.logs", "Tail ~/.tariboyd/tariboyd.log", func(c *registry.Ctx, p registry.Request, cfg daemonctl.Config) error {
		return daemonctl.TailLog(p.Ctx, cfg, p.IntOr("tail", 200), p.Bool("follow"), c.Out)
	})
	c.Args = []registry.Arg{
		{Name: "follow", Flag: "follow", Short: "f", Type: registry.Bool, Help: "keep printing new lines"},
		{Name: "tail", Flag: "tail", Type: registry.Int, Default: 200, Help: "how many last lines to print"},
	}
	return c
}

// daemonLifecycle is a local-only daemon verb that reports a failure as
// "daemon VERB: error" and exits 1.
func daemonLifecycle(path, summary string, run func(*registry.Ctx, registry.Request, daemonctl.Config) error) registry.Command {
	display := "daemon " + path[len("daemon."):]
	return registry.Command{
		Path: path, Summary: summary,
		Local: func(c *registry.Ctx, p registry.Request) (any, error) {
			cfg, err := daemonctl.ResolveConfig(os.Getenv)
			if err != nil {
				fmt.Fprintln(c.ErrOut, "daemon:", err)
				return nil, registry.ExitCode(1)
			}
			if err := run(c, p, cfg); err != nil {
				fmt.Fprintf(c.ErrOut, "%s: %v\n", display, err)
				return nil, registry.ExitCode(1)
			}
			return nil, nil
		},
	}
}

// daemonStatusLocal probes the daemon socket from the CLI. A running daemon
// prints its pid and version, or with --json its full status; a stopped one
// prints "stopped", or {"running":false}, and exits 1.
func daemonStatusLocal(c *registry.Ctx, p registry.Request) (any, error) {
	cfg, err := daemonctl.ResolveConfig(os.Getenv)
	if err != nil {
		fmt.Fprintln(c.ErrOut, "daemon:", err)
		return nil, registry.ExitCode(1)
	}
	s := daemonctl.GetStatus(cfg)
	if !s.Running {
		if c.JSON {
			fmt.Fprintln(c.Out, `{"running":false}`)
		} else {
			fmt.Fprintln(c.Out, "stopped")
		}
		return map[string]any{"running": false}, nil
	}
	if c.JSON {
		if len(s.Raw) == 0 {
			s.Raw = json.RawMessage(`{}`)
		}
		fmt.Fprintln(c.Out, string(s.Raw))
	} else {
		fmt.Fprintf(c.Out, "running (pid %d, version %s)\n", s.Pid, s.Version)
	}
	return s.Raw, nil
}

// stoppedExitCode is 1 for a {"running":false} status.
func stoppedExitCode(raw json.RawMessage) int {
	var status struct {
		Running *bool `json:"running"`
	}
	if json.Unmarshal(raw, &status) == nil && status.Running != nil && !*status.Running {
		return 1
	}
	return 0
}
