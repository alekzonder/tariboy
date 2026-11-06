package main

import (
	"os"

	"github.com/alekzonder/tariboy/internal/scriptcli"
)

func main() {
	os.Exit(scriptcli.Run(os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr))
}
