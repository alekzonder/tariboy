// Command tariboy-shim runs one harness iteration under a watchdog (spec §4).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/alekzonder/tariboy/internal/version"
)

func main() {
	fs := flag.NewFlagSet("tariboy-shim", flag.ExitOnError)
	iterDir := fs.String("iteration-dir", "", "iteration directory (holds logs/, result.json, shim.sock)")
	agent := fs.String("agent", "", "agent name")
	iterID := fs.String("iteration-id", "", "iteration id")
	hardTimeout := fs.Int("hard-timeout-s", 0, "hard timeout seconds (0 = 60s default)")
	hardDeadline := fs.String("hard-deadline", "", "absolute hard deadline (RFC3339)")
	tmuxSession := fs.String("tmux-session", "", "tmux session name (empty = process mode)")
	shimSock := fs.String("shim-sock", "", "shim RPC socket path (empty = <iteration-dir>/shim.sock)")
	showVer := fs.Bool("version", false, "print version and exit")

	// Split argv at "--": everything after is the harness command.
	args := os.Args[1:]
	var flagArgs, harnessArgv []string
	for i, a := range args {
		if a == "--" {
			flagArgs = args[:i]
			harnessArgv = args[i+1:]
			break
		}
	}
	if harnessArgv == nil {
		flagArgs = args
	}
	fs.Parse(flagArgs)

	if *showVer {
		fmt.Println(version.Version)
		return
	}
	if *iterDir == "" || len(harnessArgv) == 0 {
		fmt.Fprintln(os.Stderr, "tariboy-shim: --iteration-dir and a '-- <harness argv>' are required")
		os.Exit(2)
	}

	err := shim.Run(shim.Options{
		IterationDir: *iterDir,
		Agent:        *agent,
		IterationID:  *iterID,
		HardTimeoutS: *hardTimeout,
		HardDeadline: *hardDeadline,
		TmuxSession:  *tmuxSession,
		ShimSock:     *shimSock,
		HarnessArgv:  harnessArgv,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "tariboy-shim:", err)
		os.Exit(1)
	}
}
