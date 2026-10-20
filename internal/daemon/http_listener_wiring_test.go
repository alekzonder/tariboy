package daemon

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// The loopback listener carries the JSON API with no auth, and nothing else: a
// non-/api path is a 404 envelope now that the daemon ships no UI.
func TestHTTPListenerServesAPIAndNotUI(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := freeLoopbackAddr(t)
	errc := make(chan error, 1)
	go func() {
		errc <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: addr}))
	}()

	client := &http.Client{Timeout: time.Second}
	waitDaemonReady(t, base)
	resp, err := client.Get("http://" + addr + "/api/daemon/status")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	if resp.StatusCode != 200 {
		t.Fatalf("http listener /api/daemon/status = %d %q", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"ok":true`) {
		t.Fatalf("status body = %q", body)
	}
	// http_addr round-trips so the desktop app can adopt this daemon.
	if !strings.Contains(body, `"http_addr":"`+addr+`"`) {
		t.Fatalf("status body missing http_addr=%s: %q", addr, body)
	}

	supportResponse, err := client.Get("http://" + addr + "/api/daemon/support-bundle")
	if err != nil {
		t.Fatalf("GET support bundle: %v", err)
	}
	supportBody, _ := io.ReadAll(supportResponse.Body)
	supportResponse.Body.Close()
	if supportResponse.StatusCode != http.StatusOK {
		t.Fatalf("support bundle = %d %q", supportResponse.StatusCode, supportBody)
	}
	supportZIP, err := zip.NewReader(bytes.NewReader(supportBody), int64(len(supportBody)))
	if err != nil {
		t.Fatalf("support response is not a ZIP: %v", err)
	}
	if len(supportZIP.File) != 2 ||
		supportZIP.File[0].Name != "diagnostics.json" ||
		supportZIP.File[1].Name != "logs/tariboyd.log" {
		t.Fatalf("default support entries = %+v", supportZIP.File)
	}

	resp, err = client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 404 || !strings.Contains(string(b), `"not_found"`) {
		t.Fatalf("GET / = %d %q, want a 404 JSON envelope", resp.StatusCode, b)
	}

	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Run returned: %v", err)
	}
}

func TestNonLoopbackHTTPAddrRefused(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	err := Run(context.Background(), daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: "0.0.0.0:8798"}))
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("Run err = %v, want a non-loopback refusal", err)
	}
}
