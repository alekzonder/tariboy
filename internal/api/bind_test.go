package api

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

func bindTestServer() *Server {
	return NewServer(registry.New(), &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

// A bound unix socket accepts connections before its serve function runs, so a
// caller can report readiness as soon as bind returns.
func TestBindUnixAcceptsBeforeServe(t *testing.T) {
	t.Parallel()
	srv := bindTestServer()
	sock := filepath.Join(t.TempDir(), "d.sock")
	serve, err := srv.BindUnix(sock)
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial bound socket: %v", err)
	}
	_ = c.Close()
	done := make(chan error, 1)
	go func() { done <- serve() }()
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve after shutdown = %v, want nil", err)
	}
}

func TestBindWebFailsOnBusyAddress(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if _, err := bindTestServer().BindWeb(ln.Addr().String()); err == nil {
		t.Fatal("BindWeb on a busy address succeeded")
	}
}

// Shutdown releases listeners whose serve function never ran: a startup that
// fails after one bind must not leave that socket accepting connections.
func TestShutdownClosesBoundUnservedListeners(t *testing.T) {
	t.Parallel()
	srv := bindTestServer()
	sock := filepath.Join(t.TempDir(), "d.sock")
	if _, err := srv.BindUnix(sock); err != nil {
		t.Fatal(err)
	}
	tcpServe, err := srv.BindTCP("127.0.0.1:0", "token")
	if err != nil {
		t.Fatal(err)
	}
	webServe, err := srv.BindWeb("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c, err := net.Dial("unix", sock); err == nil {
		_ = c.Close()
		t.Fatal("unix socket still accepts after Shutdown")
	}
	if err := tcpServe(); err != nil {
		t.Fatalf("tcp serve after shutdown = %v, want nil", err)
	}
	if err := webServe(); err != nil {
		t.Fatalf("web serve after shutdown = %v, want nil", err)
	}
}
