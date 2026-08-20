package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/storesvc"
	"github.com/alekzonder/tariboy/internal/storeui"
	"github.com/alekzonder/tariboy/internal/version"
)

func main() {
	fs := flag.NewFlagSet("tariboy-store", flag.ExitOnError)
	addr := fs.String("addr", ":8443", "listen address host:port")
	dataDir := fs.String("data-dir", "", "image blob storage directory (required)")
	dbPath := fs.String("db", "", "SQLite catalog/token DB path (default <data-dir>/store.db)")
	tlsCert := fs.String("tls-cert", "", "TLS certificate file (PEM)")
	tlsKey := fs.String("tls-key", "", "TLS private key file (PEM)")
	tokenFile := fs.String("token-file", "", "file holding a bootstrap readwrite bearer token")
	anonPull := fs.Bool("anon-pull", false, "allow unauthenticated GET/HEAD (pull)")
	allowInsecure := fs.Bool("allow-insecure", false, "serve plain HTTP (local dev only, NEVER production)")
	showVer := fs.Bool("version", false, "print version and exit")
	fs.Parse(os.Args[1:])

	if *showVer {
		fmt.Println(version.Version)
		return
	}
	if *dataDir == "" {
		fmt.Fprintln(os.Stderr, "tariboy-store: --data-dir is required")
		os.Exit(2)
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "tariboy-store:", err)
		os.Exit(1)
	}
	db := *dbPath
	if db == "" {
		db = filepath.Join(*dataDir, "store.db")
	}
	srv, err := storesvc.New(storesvc.Config{
		Addr: *addr, TLSCert: *tlsCert, TLSKey: *tlsKey,
		AllowInsecure: *allowInsecure, AnonPull: *anonPull,
		DataDir: *dataDir, DBPath: db, Version: version.Version,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "tariboy-store:", err)
		os.Exit(1)
	}
	uiHandler, err := storeui.Handler()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tariboy-store: store ui:", err)
		os.Exit(1)
	}
	srv.SetUI(uiHandler)
	if *tokenFile != "" {
		b, err := os.ReadFile(*tokenFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tariboy-store: token file:", err)
			os.Exit(1)
		}
		tok := strings.TrimSpace(string(b))
		if tok == "" {
			fmt.Fprintln(os.Stderr, "tariboy-store: token file is empty")
			os.Exit(1)
		}
		if err := srv.SeedToken(tok, storesvc.ScopeReadWrite); err != nil {
			fmt.Fprintln(os.Stderr, "tariboy-store: seed token:", err)
			os.Exit(1)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	fmt.Fprintf(os.Stderr, "tariboy-store listening on %s (tls=%v anon_pull=%v)\n", *addr, !*allowInsecure, *anonPull)
	if err := srv.ListenAndServeTLS(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "tariboy-store:", err)
		os.Exit(1)
	}
}
