// Command tariboy-tools is the agent-facing CLI (spec §3/§8).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/alekzonder/tariboy/internal/toolscli"
	"github.com/alekzonder/tariboy/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv("TARIBOY_TOOLS_SOCKET"), os.Stdout, os.Stderr))
}

func run(args []string, socket string, out, errOut io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(out, version.Version)
		return 0
	}
	return toolscli.Run(socket, args, out, errOut)
}
