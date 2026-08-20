package client

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

// startStampedDaemon serves the minimal ok-envelope over a unix socket and
// stamps the given version header (empty header => stamps nothing, which is how
// a pre-SUPER-224 daemon answers).
func startStampedDaemon(t *testing.T, header string) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if header != "" {
			w.Header().Set(version.Header, header)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"hi": "there"}})
	}))
	return sock
}

// captureWarnings redirects the one-shot mismatch warning and rearms it, so
// each case starts from a fresh process-like state.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prevOut, prevWarned := versionWarnOut, versionWarned.Load()
	versionWarnOut = buf
	versionWarned.Store(false)
	t.Cleanup(func() { versionWarnOut = prevOut; versionWarned.Store(prevWarned) })
	return buf
}

func TestCallSilentWhenVersionsMatch(t *testing.T) {
	warn := captureWarnings(t)
	sock := startStampedDaemon(t, version.Version)
	if _, err := New(sock).Call("GET", "/api/daemon/status", nil); err != nil {
		t.Fatal(err)
	}
	if warn.Len() != 0 {
		t.Fatalf("matching versions warned: %q", warn.String())
	}
}

func TestCallSilentWhenDaemonSendsNoVersion(t *testing.T) {
	warn := captureWarnings(t)
	sock := startStampedDaemon(t, "")
	raw, err := New(sock).Call("GET", "/api/daemon/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "there") {
		t.Fatalf("result mangled: %s", raw)
	}
	if warn.Len() != 0 {
		t.Fatalf("headerless daemon warned: %q", warn.String())
	}
}

func TestCallWarnsOncePerProcessOnVersionMismatch(t *testing.T) {
	warn := captureWarnings(t)
	sock := startStampedDaemon(t, "0.0.1-old")
	c := New(sock)
	for i := 0; i < 3; i++ {
		raw, err := c.Call("GET", "/api/daemon/status", nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		// The warning must not disturb the payload the caller parses.
		if !strings.Contains(string(raw), "there") {
			t.Fatalf("call %d result mangled: %s", i, raw)
		}
	}
	// A fresh Client shares the process-wide latch.
	if _, err := New(sock).Call("GET", "/api/daemon/status", nil); err != nil {
		t.Fatal(err)
	}
	got := warn.String()
	if n := strings.Count(got, "\n"); n != 1 {
		t.Fatalf("warned %d times, want 1: %q", n, got)
	}
	if !strings.Contains(got, version.Version) || !strings.Contains(got, "0.0.1-old") {
		t.Fatalf("warning names neither both versions: %q", got)
	}
}
