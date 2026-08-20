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
)

// The loopback listener carries the JSON API with no auth, and nothing else: a
// non-/api path is a 404 envelope now that the daemon ships no UI.
func TestHTTPListenerServesAPIAndNotUI(t *testing.T) {
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := "127.0.0.1:8799"
	errc := make(chan error, 1)
	go func() {
		errc <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: addr}))
	}()

	client := &http.Client{Timeout: time.Second}
	var body string
	ok := false
	for i := 0; i < 100; i++ {
		resp, err := client.Get("http://" + addr + "/api/daemon/status")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				body = string(b)
				ok = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("http listener never answered /api/daemon/status")
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

	resp, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
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
	base := t.TempDir()
	err := Run(context.Background(), daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: "0.0.0.0:8798"}))
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("Run err = %v, want a non-loopback refusal", err)
	}
}
