package main

import (
	"context"
	"os"

	"github.com/alekzonder/tariboy/internal/taskcli"
)

func main() {
	os.Exit(taskcli.Run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
