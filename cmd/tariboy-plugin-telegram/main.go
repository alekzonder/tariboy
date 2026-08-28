package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/telegramplugin"
	"github.com/alekzonder/tariboy/internal/version"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(version.Version)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "telegram plugin:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	socket := os.Getenv("TARIBOY_PLUGIN_SOCKET")
	workdir := os.Getenv("TARIBOY_PLUGIN_WORKDIR")
	daemonSocket := os.Getenv("TARIBOY_DAEMON_SOCKET")
	if socket == "" || workdir == "" || daemonSocket == "" {
		return errors.New("plugin socket, workdir, and daemon socket are required")
	}
	state, err := telegramplugin.OpenState(workdir)
	if err != nil {
		return err
	}
	daemon := telegramplugin.NewDaemonClient(daemonSocket, os.Getenv("TARIBOY_PLUGIN_TOKEN"))
	server := telegramplugin.NewServer(state, telegramplugin.NewBotClient(os.Getenv("TARIBOY_TELEGRAM_API_BASE")), daemon)
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0o600); err != nil {
		return err
	}
	handshake := plugins.Handshake{
		Name: "telegram", Version: version.Version, ProtocolVersion: plugins.ProtocolVersion,
		Types: []string{"channel-source", "channel-sink"}, Socket: socket,
	}
	if err := json.NewEncoder(os.Stdout).Encode(handshake); err != nil {
		return err
	}
	httpServer := &http.Server{Handler: server}
	go server.Run(ctx)
	go func() {
		<-ctx.Done()
		_ = httpServer.Shutdown(context.Background())
	}()
	err = httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
