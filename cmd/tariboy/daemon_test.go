package main

import (
	"bytes"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/testutil"
)

// isolate points the CLI at fresh base and runtime dirs, so no test can reach
// the live daemon, and returns the runtime dir that holds the socket.
func isolate(t *testing.T) string {
	t.Helper()
	runtime := testutil.ShortDir(t)
	t.Setenv("TARIBOY_BASE_DIR", t.TempDir())
	t.Setenv("TARIBOY_RUNTIME_DIR", runtime)
	return runtime
}

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(t.Context(), args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestDaemonStatusStoppedExitsNonZero(t *testing.T) {
	isolate(t)
	out, errOut, code := runCLI(t, "daemon", "status")
	if code == 0 || strings.TrimSpace(out) != "stopped" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want non-zero and stopped", code, out, errOut)
	}
}

func TestDaemonStatusJSONIsMachineReadableWhenStopped(t *testing.T) {
	isolate(t)
	for _, argv := range [][]string{{"daemon", "status", "--json"}, {"--json", "daemon", "status"}} {
		out, errOut, code := runCLI(t, argv...)
		if code == 0 || strings.TrimSpace(out) != `{"running":false}` {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", argv, code, out, errOut)
		}
	}
}

func TestDaemonStatusReportsLiveDaemon(t *testing.T) {
	runtime := isolate(t)
	listener, err := net.Listen("unix", filepath.Join(runtime, "tariboyd.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	body := `{"ok":true,"result":{"version":"1.2.3","pid":42,"http_addr":"127.0.0.1:9990"}}`
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			buf := make([]byte, 4096)
			_, _ = conn.Read(buf)
			_, _ = fmt.Fprintf(conn, "HTTP/1.0 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
			conn.Close()
		}
	}()
	out, errOut, code := runCLI(t, "daemon", "status", "--json")
	if code != 0 || strings.TrimSpace(out) != `{"version":"1.2.3","pid":42,"http_addr":"127.0.0.1:9990"}` {
		t.Fatalf("json: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runCLI(t, "daemon", "status")
	if code != 0 || strings.TrimSpace(out) != "running (pid 42, version 1.2.3)" {
		t.Fatalf("human: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestDaemonHelpComesFromRegistry(t *testing.T) {
	isolate(t)
	for _, argv := range [][]string{{"daemon"}, {"daemon", "--help"}, {"daemon", "help"}} {
		out, errOut, code := runCLI(t, argv...)
		if code != 0 {
			t.Fatalf("%v: code=%d stderr=%q", argv, code, errOut)
		}
		for _, want := range []string{"Command groups:", "Commands:", "agent-shell-script", "config", "start", "stop", "restart", "status", "logs", "reindex"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%v: help missing %q:\n%s", argv, want, out)
			}
		}
	}
}

func TestDaemonStopWhenNotRunning(t *testing.T) {
	isolate(t)
	out, errOut, code := runCLI(t, "daemon", "stop")
	if code != 0 || !strings.Contains(out, "not running") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestDaemonRestartRunsLocally(t *testing.T) {
	runtime := isolate(t)
	// A missing daemon binary makes the start half fail fast: no daemon spawns.
	t.Setenv("TARIBOY_DAEMON_BIN", filepath.Join(runtime, "no-such-daemon"))
	_, errOut, code := runCLI(t, "daemon", "restart")
	if code != 1 || !strings.HasPrefix(errOut, "daemon restart:") {
		t.Fatalf("code=%d stderr=%q, want the local restart failure", code, errOut)
	}
}

func TestDaemonLogsReadsLogFile(t *testing.T) {
	isolate(t)
	_, errOut, code := runCLI(t, "daemon", "logs", "--tail", "5")
	if code != 1 || !strings.HasPrefix(errOut, "daemon logs:") {
		t.Fatalf("code=%d stderr=%q, want the missing-log failure", code, errOut)
	}
}

// TestComposeFlagsBeforeVerb pins `compose -f FILE VERB`, used by the e2e and
// smoke scripts.
func TestComposeFlagsBeforeVerb(t *testing.T) {
	isolate(t)
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	for _, argv := range [][]string{
		{"compose", "-f", missing, "--no-start", "status"},
		{"compose", "status", "-f", missing},
	} {
		_, errOut, code := runCLI(t, argv...)
		if code != 1 || !strings.Contains(errOut, "compose: read "+missing) {
			t.Fatalf("%v: code=%d stderr=%q", argv, code, errOut)
		}
	}
}

func TestComposeHelpListsVerbs(t *testing.T) {
	isolate(t)
	out, _, code := runCLI(t, "compose", "--help")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	for _, verb := range []string{"up", "down", "start", "stop", "restart", "kill", "exec", "ps", "status", "logs", "build", "rm", "archive", "import"} {
		if !strings.Contains(out, "  "+verb+" ") {
			t.Fatalf("compose help missing %q:\n%s", verb, out)
		}
	}
}
