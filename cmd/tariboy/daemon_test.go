package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

func TestDispatchDaemonStatusStoppedExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	getenv := func(k string) string {
		switch k {
		case "HOME":
			return dir
		case "TARIBOY_RUNTIME_DIR":
			return dir
		}
		return ""
	}
	var out, errOut bytes.Buffer
	handled, code := dispatchDaemon(context.Background(), []string{"daemon", "status"}, getenv, &out, &errOut)
	if !handled {
		t.Fatal("status should be handled locally")
	}
	if code == 0 {
		t.Fatal("status exit code should be non-zero when stopped")
	}
	if !strings.Contains(out.String()+errOut.String(), "stopped") {
		t.Fatalf("expected 'stopped' in output, got %q / %q", out.String(), errOut.String())
	}
}

func TestDispatchDaemonStatusJSONIsMachineReadableWhenStopped(t *testing.T) {
	dir := t.TempDir()
	getenv := func(k string) string {
		if k == "HOME" || k == "TARIBOY_RUNTIME_DIR" {
			return dir
		}
		return ""
	}
	var out, errOut bytes.Buffer
	handled, code := dispatchDaemon(
		context.Background(),
		[]string{"daemon", "status", "--json"},
		getenv,
		&out,
		&errOut,
	)
	if !handled || code == 0 || strings.TrimSpace(out.String()) != `{"running":false}` {
		t.Fatalf("handled=%v code=%d stdout=%q stderr=%q", handled, code, out.String(), errOut.String())
	}
}

func TestDispatchDaemonStatusJSONReturnsLiveAPIStatus(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "tariboyd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	body := `{"ok":true,"result":{"version":"1.2.3","pid":42,"http_addr":"127.0.0.1:9990"}}`
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		_, _ = conn.Read(buf)
		_, _ = fmt.Fprintf(
			conn,
			"HTTP/1.0 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
			len(body),
			body,
		)
	}()
	getenv := func(k string) string {
		if k == "HOME" || k == "TARIBOY_RUNTIME_DIR" {
			return dir
		}
		return ""
	}
	var out, errOut bytes.Buffer
	handled, code := dispatchDaemon(
		context.Background(),
		[]string{"daemon", "status", "--json"},
		getenv,
		&out,
		&errOut,
	)
	<-done
	if !handled || code != 0 {
		t.Fatalf("handled=%v code=%d stderr=%q", handled, code, errOut.String())
	}
	got := strings.TrimSpace(out.String())
	if got != `{"version":"1.2.3","pid":42,"http_addr":"127.0.0.1:9990"}` {
		t.Fatalf("status JSON=%q", got)
	}
}

func TestDispatchDaemonPassesThroughUnknownSub(t *testing.T) {
	var out, errOut bytes.Buffer
	handled, _ := dispatchDaemon(context.Background(), []string{"daemon", "config", "get"}, func(string) string { return "" }, &out, &errOut)
	if handled {
		t.Fatal("daemon config must fall through to the registry (handled=false)")
	}
}

func TestDispatchDaemonUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	handled, code := dispatchDaemon(context.Background(), []string{"daemon"}, func(string) string { return "" }, &out, &errOut)
	if !handled || code != 0 {
		t.Fatalf("bare daemon: handled=%v code=%d, want handled=true code=0", handled, code)
	}
	if !strings.Contains(out.String(), "start") || !strings.Contains(out.String(), "stop") {
		t.Fatalf("usage should list start/stop, got:\n%s", out.String())
	}
}

func TestDispatchDaemonStopAliasHandled(t *testing.T) {
	dir := t.TempDir()
	getenv := func(k string) string {
		if k == "HOME" || k == "TARIBOY_RUNTIME_DIR" {
			return dir
		}
		return ""
	}
	var out, errOut bytes.Buffer
	// No daemon and no pidfile in the temp runtime dir => "stop" is a clean no-op.
	handled, code := dispatchDaemon(context.Background(), []string{"daemon", "stop"}, getenv, &out, &errOut)
	if !handled || code != 0 {
		t.Fatalf("daemon stop: handled=%v code=%d, want handled=true code=0", handled, code)
	}
	if !strings.Contains(out.String(), "not running") {
		t.Fatalf("expected 'not running', got %q", out.String())
	}
}

func TestDispatchDaemonRestartHandled(t *testing.T) {
	dir := t.TempDir()
	getenv := func(k string) string {
		switch k {
		case "HOME", "TARIBOY_RUNTIME_DIR":
			return dir
		case "TARIBOY_DAEMON_BIN":
			// Point at a nonexistent binary so EnsureUp fails fast (no daemon is
			// actually spawned); we only assert the verb is handled locally.
			return dir + "/no-such-daemon"
		}
		return ""
	}
	var out, errOut bytes.Buffer
	// No daemon and no pidfile => Down no-ops ("not running"), then EnsureUp is
	// attempted; either way "restart" must be handled locally (not passed through).
	handled, _ := dispatchDaemon(context.Background(), []string{"daemon", "restart"}, getenv, &out, &errOut)
	if !handled {
		t.Fatal("daemon restart should be handled locally (handled=true)")
	}
}

func TestDaemonUsageHeadings(t *testing.T) {
	var out, errOut bytes.Buffer
	handled, code := dispatchDaemon(context.Background(), []string{"daemon"}, func(string) string { return "" }, &out, &errOut)
	if !handled || code != 0 {
		t.Fatalf("handled=%v code=%d", handled, code)
	}
	got := out.String()
	for _, want := range []string{"Command groups:", "Commands:", "start", "stop", "restart", "status", "logs", "config", "reindex"} {
		if !strings.Contains(got, want) {
			t.Fatalf("daemon usage missing %q:\n%s", want, got)
		}
	}
}
