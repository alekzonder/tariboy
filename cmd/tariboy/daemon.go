package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alekzonder/tariboy/internal/daemonctl"
)

// dispatchDaemon handles the CLI-local daemon lifecycle verbs (up/down/status/
// logs). It returns handled=false for anything else (config, reindex, ...) so
// those keep routing through the daemon registry.
func dispatchDaemon(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) (bool, int) {
	if len(args) < 2 {
		printDaemonUsage(out)
		return true, 0
	}
	sub := args[1]
	switch sub {
	case "-h", "--help", "help":
		printDaemonUsage(out)
		return true, 0
	case "start", "stop", "restart", "status", "logs":
	default:
		return false, 0
	}
	cfg, err := daemonctl.ResolveConfig(getenv)
	if err != nil {
		fmt.Fprintln(errOut, "daemon:", err)
		return true, 1
	}
	switch sub {
	case "start":
		if _, err := daemonctl.EnsureUp(ctx, cfg, out); err != nil {
			fmt.Fprintln(errOut, "daemon start:", err)
			return true, 1
		}
		return true, 0
	case "stop":
		if err := daemonctl.Down(ctx, cfg, out); err != nil {
			fmt.Fprintln(errOut, "daemon stop:", err)
			return true, 1
		}
		return true, 0
	case "restart":
		if err := daemonctl.Restart(ctx, cfg, out); err != nil {
			fmt.Fprintln(errOut, "daemon restart:", err)
			return true, 1
		}
		return true, 0
	case "status":
		s := daemonctl.GetStatus(cfg)
		jsonOutput := false
		for _, arg := range args[2:] {
			if arg == "--json" {
				jsonOutput = true
			}
		}
		if s.Running {
			if jsonOutput {
				if len(s.Raw) == 0 {
					s.Raw = json.RawMessage(`{}`)
				}
				fmt.Fprintln(out, string(s.Raw))
				return true, 0
			}
			fmt.Fprintf(out, "running (pid %d, version %s)\n", s.Pid, s.Version)
			return true, 0
		}
		if jsonOutput {
			fmt.Fprintln(out, `{"running":false}`)
			return true, 1
		}
		fmt.Fprintln(out, "stopped")
		return true, 1
	case "logs":
		follow, n := parseLogFlags(args[2:])
		if err := daemonctl.TailLog(ctx, cfg, n, follow, out); err != nil {
			fmt.Fprintln(errOut, "daemon logs:", err)
			return true, 1
		}
		return true, 0
	}
	return true, 0
}

func printDaemonUsage(out io.Writer) {
	fmt.Fprint(out, `Usage: tariboy daemon <command> [args]

Control the tariboy daemon

Command groups:
  config                 read/set daemon config (get, set)

Commands:
  start                  start tariboyd detached (no-op if already running)
  stop                   stop the daemon (SIGTERM, then SIGKILL on timeout)
  restart                restart the daemon (stop then start)
  status                 running/stopped (exit 0 = running, non-zero = stopped)
  logs [-f] [--tail N]   tail ~/.tariboyd/tariboyd.log
  reindex                rebuild ai_requests metadata
`)
}

func parseLogFlags(args []string) (follow bool, tail int) {
	tail = 200
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-f" || args[i] == "--follow":
			follow = true
		case args[i] == "--tail" && i+1 < len(args):
			i++
			fmt.Sscanf(args[i], "%d", &tail)
		}
	}
	return follow, tail
}
