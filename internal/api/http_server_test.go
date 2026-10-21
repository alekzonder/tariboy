package api

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
)

// A client that sends a partial request header and then stalls (slowloris)
// is disconnected once the header timeout passes.
func TestServerDropsSlowHeaderClient(t *testing.T) {
	old := readHeaderTimeout
	readHeaderTimeout = 100 * time.Millisecond
	t.Cleanup(func() { readHeaderTimeout = old })

	srv := NewServer(registry.New(), &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	sock := filepath.Join(t.TempDir(), "d.sock")
	go srv.ServeUnix(sock)
	t.Cleanup(func() { srv.Shutdown(context.Background()) })

	var conn net.Conn
	var err error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if conn, err = net.Dial("unix", sock); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET /api/help.json HTTP/1.1\r\nHost: unix\r\n")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	start := time.Now()
	_, err = io.ReadAll(conn)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("server kept a stalled header open for %v", time.Since(start))
	}
}

func TestNewHTTPServerSetsLimits(t *testing.T) {
	s := NewHTTPServer(nil)
	if s.ReadHeaderTimeout <= 0 || s.IdleTimeout <= 0 || s.MaxHeaderBytes <= 0 {
		t.Fatalf("server limits = header %v idle %v max header %d", s.ReadHeaderTimeout, s.IdleTimeout, s.MaxHeaderBytes)
	}
}

// Every command without its own MaxBodyBytes gets the default body limit, both
// for a declared length and for a body that streams past it.
func TestDispatchAppliesDefaultBodyLimit(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(registry.Command{
		Path: "echo", Summary: "e",
		HTTP:    &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/echo"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) { return p, nil },
	})
	srv := NewServer(reg, &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	big := `{"text":"` + strings.Repeat("x", defaultMaxBodyBytes) + `"}`

	for _, declared := range []bool{true, false} {
		req := httptest.NewRequest(http.MethodPost, "/api/echo", strings.NewReader(big))
		if !declared {
			req.ContentLength = -1
		}
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rr.Body.String(), `"too_large"`) {
			t.Fatalf("declared=%v: status = %d, body = %.200s", declared, rr.Code, rr.Body.String())
		}
	}

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/echo", strings.NewReader(`{"text":"ok"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("small body: status = %d, body = %s", rr.Code, rr.Body.String())
	}
}
