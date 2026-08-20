package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alekzonder/tariboy/internal/daemon"
	"github.com/alekzonder/tariboy/internal/version"
)

// parseFlags builds daemon.Options from argv. It is a separate function so the
// --web-addr → --http-addr aliasing (the only non-obvious bit) is unit-testable.
func parseFlags(args []string) (daemon.Options, bool, error) {
	fs := flag.NewFlagSet("tariboyd", flag.ContinueOnError)
	baseDir := fs.String("base-dir", "", "base directory (default $TARIBOY_BASE_DIR or ~/.tariboy)")
	listen := fs.String("listen", "unix", "listen spec: unix | unix:/path.sock | tcp:HOST:PORT")
	authFile := fs.String("auth-token-file", "", "bearer token file; REQUIRED for non-loopback tcp")
	logLevel := fs.String("log-level", "info", "debug|info|warn|error")
	httpAddr := fs.String("http-addr", "127.0.0.1:9990", "loopback address for the JSON API/WS listener (empty disables)")
	// Deprecated alias kept for one release: the daemon no longer serves a web
	// UI, but wrappers (scripts/tariboy-smoke.sh among them) still pass
	// --web-addr "" to switch the listener off.
	webAddr := fs.String("web-addr", "127.0.0.1:9990", "deprecated alias for --http-addr")
	showVer := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return daemon.Options{}, false, err
	}

	// flag.Visit only reports flags that were actually PRESENT, which is what
	// separates "--web-addr ''" (disable) from "not passed" (keep the default).
	setHTTP, setWeb := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "http-addr":
			setHTTP = true
		case "web-addr":
			setWeb = true
		}
	})
	addr := *httpAddr
	if setWeb && !setHTTP {
		addr = *webAddr
		fmt.Fprintln(os.Stderr, "tariboyd: --web-addr is deprecated, use --http-addr")
	}

	return daemon.Options{
		BaseDir: *baseDir, Listen: *listen,
		AuthTokenFile: *authFile, LogLevel: *logLevel, HTTPAddr: addr,
	}, *showVer, nil
}

func main() {
	opts, showVer, err := parseFlags(os.Args[1:])
	if err != nil {
		// -h/--help is a successful request for usage, not a usage error. The
		// flag package has already printed it; exiting non-zero here would break
		// any wrapper that runs `tariboyd --help` as a health check.
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}
	if showVer {
		fmt.Println(version.Version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := daemon.Run(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, "tariboyd:", err)
		os.Exit(1)
	}
}
