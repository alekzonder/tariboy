package client

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"testing"
)

// rawServer serves handler on a unix socket and returns the socket path.
func rawServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "raw.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return sock
}

func TestDownloadCopiesTheRawBody(t *testing.T) {
	t.Parallel()
	sock := rawServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/images/dev:latest/export" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Write([]byte("archive bytes"))
	})
	var buf bytes.Buffer
	n, err := New(sock).Download("/api/images/dev:latest/export", &buf)
	if err != nil || n != 13 || buf.String() != "archive bytes" {
		t.Fatalf("download = %d %q, %v", n, buf.String(), err)
	}
}

func TestDownloadDecodesAnErrorEnvelope(t *testing.T) {
	t.Parallel()
	sock := rawServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ok":false,"error":{"code":"image_not_exportable","message":"no such image"}}`))
	})
	var buf bytes.Buffer
	_, err := New(sock).Download("/api/images/x:latest/export", &buf)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "image_not_exportable" || buf.Len() != 0 {
		t.Fatalf("err = %v, body %q", err, buf.String())
	}
}
