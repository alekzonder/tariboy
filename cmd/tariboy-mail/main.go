package main

import (
	"context"
	"os"

	"github.com/alekzonder/tariboy/internal/mailcli"
)

func main() {
	os.Exit(mailcli.Run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
