package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alekzonder/tariboy/internal/taskcli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(taskcli.Run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
