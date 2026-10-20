package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/daemon"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// TestHTTPAPIEndToEnd boots the real daemon with a loopback listener and drives
// the JSON API + one control POST over http://127.0.0.1:PORT, the exact surface
// the desktop app's webview uses. Discriminant: it fails if the listener, the
// host allowlist, or the mutating routes regress. No external network.
func TestHTTPAPIEndToEnd(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback listener: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release loopback listener: %v", err)
	}

	done := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("daemon.Run: %v", err)
		}
	})
	go func() {
		done <- daemon.Run(ctx, inProcessDaemonOptions(daemon.Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: addr}))
	}()

	client := &http.Client{Timeout: 2 * time.Second}

	// Wait for the listener, then assert the envelope.
	var raw string
	testutil.WaitFor(7500*time.Millisecond, func() bool {
		resp, err := client.Get("http://" + addr + "/api/agents")
		if err != nil {
			return false
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		raw = string(b)
		return resp.StatusCode == 200
	})
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Count int `json:"count"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil || !env.OK {
		t.Fatalf("GET /api/agents envelope = %q (err %v)", raw, err)
	}

	// A control POST returns the JSON envelope (this agent does not exist, so the
	// daemon replies ok:false — the point is the mutating route is reachable).
	preq, _ := http.NewRequest("POST", "http://"+addr+"/api/agents/ghost/stop", bytes.NewReader([]byte("{}")))
	preq.Header.Set("Content-Type", "application/json")
	presp, err := client.Do(preq)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	pb, _ := io.ReadAll(presp.Body)
	presp.Body.Close()
	if !strings.Contains(string(pb), `"ok":`) {
		t.Fatalf("control POST did not return a JSON envelope: %q", pb)
	}

	// A spoofed Host is rejected (421) — the allowlist guards the mutating surface.
	sreq, _ := http.NewRequest("GET", "http://"+addr+"/api/agents", nil)
	sreq.Host = "evil.com"
	sresp, err := client.Do(sreq)
	if err != nil {
		t.Fatalf("spoofed host: %v", err)
	}
	sresp.Body.Close()
	if sresp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("spoofed Host status = %d, want 421", sresp.StatusCode)
	}
}
