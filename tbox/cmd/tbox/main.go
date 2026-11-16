// Command tbox runs tariboy agent images in ordinary harness sessions.
package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/cli"
	"github.com/alekzonder/tariboy/tbox/internal/tui"
	"golang.org/x/term"
)

func main() {
	cwd, _ := os.Getwd()
	os.Exit(cli.Main(cli.Env{
		Args:     os.Args[1:],
		Getenv:   os.Getenv,
		Environ:  os.Environ,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Cwd:      cwd,
		Exec:     syscall.Exec,
		LookPath: exec.LookPath,
		Now:      time.Now,
		OpenTerminal: func() (tui.Terminal, error) {
			return tui.Open(os.Stdin, os.Stderr)
		},
		StdoutIsTerminal: func() bool { return term.IsTerminal(int(os.Stdout.Fd())) },
	}))
}
