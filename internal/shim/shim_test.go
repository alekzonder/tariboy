package shim

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/harness"
)

const shimTestProxyToken = "sk-tariboy-0123456789abcdef0123456789abcdef0123456789abcdef"

func TestMain(m *testing.M) {
	if len(os.Args) >= 6 && os.Args[1] == TmuxSupervisorMode && os.Args[4] == "--" {
		if RunTmuxSupervisor(os.Args[2], os.Args[3], os.Args[5:]) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func codexHarnessArgv(t *testing.T, binDir string) []string {
	t.Helper()
	prompt := filepath.Join(t.TempDir(), "PROMPT.md")
	if err := os.WriteFile(prompt, []byte("test prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	adapter, err := harness.Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	argv, _, err := adapter.Command(t.TempDir(), prompt, harness.Config{
		ProxyURL: "http://127.0.0.1:5555/_tariboy/" + shimTestProxyToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	return argv
}

func assertSafeCodexShimLog(t *testing.T, iterDir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(iterDir, "logs", "shim.log"))
	if err != nil {
		t.Fatal(err)
	}
	logText := string(data)
	if strings.Contains(logText, shimTestProxyToken) {
		t.Fatal("shim.log contains the raw proxy token")
	}
	if !strings.Contains(logText, "model_providers.tariboy.base_url=") ||
		!strings.Contains(logText, "/_tariboy/***") {
		t.Fatal("shim.log does not preserve the redacted Codex provider URL shape")
	}
}

func readNULTerminatedArgs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != 0 {
		t.Fatal("captured argv is not NUL-terminated")
	}
	parts := strings.Split(string(data[:len(data)-1]), "\x00")
	return parts
}

func TestRedactProxyTokenSegmentsPreservesURLBoundaries(t *testing.T) {
	const tokenA = "sk-tariboy-0123456789abcdef0123456789abcdef0123456789abcdef"
	const tokenB = "sk-tariboy-fedcba9876543210fedcba9876543210fedcba9876543210"
	const base = "http://127.0.0.1:5555/_tariboy/"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "exact", in: base + tokenA, want: base + "***"},
		{name: "quoted config", in: `model_providers.tariboy.base_url="` + base + tokenA + `"`, want: `model_providers.tariboy.base_url="` + base + `***"`},
		{name: "path suffix", in: base + tokenA + "/responses", want: base + "***/responses"},
		{name: "query fragment", in: base + tokenA + "?stream=true#result", want: base + "***?stream=true#result"},
		{name: "punctuation separated", in: base + tokenA + "," + base + tokenB + ";" + base + tokenA + "&x=" + base + tokenB + ")", want: base + "***," + base + "***;" + base + "***&x=" + base + "***)"},
		{name: "empty segment", in: base, want: base},
		{name: "wrong prefix", in: base + "token-0123456789abcdef0123456789abcdef0123456789abcdef", want: base + "token-0123456789abcdef0123456789abcdef0123456789abcdef"},
		{name: "short payload", in: base + "sk-tariboy-0123456789abcdef", want: base + "sk-tariboy-0123456789abcdef"},
		{name: "non hex payload", in: base + "sk-tariboy-0123456789abcdef0123456789abcdef0123456789abcdeg", want: base + "sk-tariboy-0123456789abcdef0123456789abcdef0123456789abcdeg"},
		{name: "long identifier", in: base + tokenA + "0", want: base + tokenA + "0"},
		{name: "multiple tokens", in: base + tokenA + " " + base + tokenB, want: base + "*** " + base + "***"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactProxyTokenSegments(tc.in); got != tc.want {
				t.Fatal("proxy token redaction did not preserve the expected structure")
			}
		})
	}
}

func TestTmuxShimAttachEcho(t *testing.T) {
	// Stub tmux with `cat`: attach-session becomes an echo loop so we can
	// verify the PTY bridge copies bytes both directions without a real tmux.
	old := execCommand
	execCommand = func(name string, arg ...string) *exec.Cmd { return exec.Command("cat") }
	defer func() { execCommand = old }()

	s := &tmuxShim{session: "sess"}
	c1, c2 := net.Pipe()
	attachDone := make(chan struct{})
	go func() { _ = s.Attach(c2, AttachParams{Cols: 80, Rows: 24}); close(attachDone) }()

	// Wait for the PTY to be registered so Resize can find it.
	var ptmx *os.File
	for i := 0; i < 200; i++ {
		activePTYsMu.Lock()
		for f := range activePTYs["sess"] {
			ptmx = f
		}
		activePTYsMu.Unlock()
		if ptmx != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ptmx == nil {
		t.Fatal("PTY was never registered in activePTYs")
	}

	// Resize must retarget the live PTY without error.
	if err := s.Resize(ResizeParams{Cols: 100, Rows: 40}); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	if _, err := c1.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(c1, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("got %q, want %q", buf, "hello")
	}

	// Closing the tunnel must tear down the attach (no goroutine leak) and
	// deregister the PTY.
	c1.Close()
	select {
	case <-attachDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Attach did not return after conn close")
	}
	activePTYsMu.Lock()
	leaked := activePTYs["sess"]
	activePTYsMu.Unlock()
	if leaked != nil {
		t.Fatal("PTY still registered after Attach returned")
	}
}

// TestTmuxShimAttachMultiClientResize proves the fix for concurrent same-
// session attaches: two viewers on one session must each register/deregister
// only their own ptmx, so one client detaching does not knock the survivor
// out of activePTYs (and break its Resize) as it did when activePTYs was a
// single-value map keyed by session.
func TestTmuxShimAttachMultiClientResize(t *testing.T) {
	old := execCommand
	execCommand = func(name string, arg ...string) *exec.Cmd { return exec.Command("cat") }
	defer func() { execCommand = old }()

	s := &tmuxShim{session: "sess"}

	a1, b1 := net.Pipe()
	a2, b2 := net.Pipe()
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	go func() { _ = s.Attach(b1, AttachParams{Cols: 80, Rows: 24}); close(done1) }()
	go func() { _ = s.Attach(b2, AttachParams{Cols: 80, Rows: 24}); close(done2) }()

	waitForCount := func(n int) {
		t.Helper()
		for i := 0; i < 200; i++ {
			activePTYsMu.Lock()
			got := len(activePTYs["sess"])
			activePTYsMu.Unlock()
			if got == n {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for activePTYs[sess] to reach %d entries", n)
	}

	// Both clients registered.
	waitForCount(2)

	if err := s.Resize(ResizeParams{Cols: 100, Rows: 40}); err != nil {
		t.Fatalf("Resize with two active clients: %v", err)
	}

	// Close the first client; it must deregister only its own ptmx.
	a1.Close()
	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("first Attach did not return after conn close")
	}
	waitForCount(1)

	// Regression check: the survivor must still be registered and Resize
	// must still succeed (this failed before the fix: the first Attach's
	// teardown deleted the whole session entry, orphaning the second).
	if err := s.Resize(ResizeParams{Cols: 120, Rows: 50}); err != nil {
		t.Fatalf("Resize with one surviving client: %v", err)
	}

	// Close the second client; the session key must be fully removed.
	a2.Close()
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("second Attach did not return after conn close")
	}
	waitForCount(0)

	activePTYsMu.Lock()
	_, present := activePTYs["sess"]
	activePTYsMu.Unlock()
	if present {
		t.Fatal("session key still present in activePTYs after both attaches returned")
	}

	if err := s.Resize(ResizeParams{Cols: 80, Rows: 24}); err == nil {
		t.Fatal("Resize after both clients detached should return an error")
	}
}

func isTmuxAttachCommand(args []string) bool {
	return len(args) > 1 && args[0] == "-u" && args[1] == "attach-session"
}

func TestTmuxShimResizeNoActivePTY(t *testing.T) {
	s := &tmuxShim{session: "no-such-session"}
	if err := s.Resize(ResizeParams{Cols: 80, Rows: 24}); err == nil {
		t.Fatal("Resize with no active PTY should return an error, not panic")
	}
}

func TestTmuxShimAttachMissingSessionDoesNotStartAttachClient(t *testing.T) {
	old := execCommand
	var attachStarted bool
	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "has-session" {
			return exec.Command("/bin/sh", "-c", "exit 1")
		}
		if isTmuxAttachCommand(args) {
			attachStarted = true
			return exec.Command("/bin/sh", "-c", "printf 'no sessions\\n'")
		}
		return exec.Command("/bin/true")
	}
	defer func() { execCommand = old }()

	s := &tmuxShim{session: "gone"}
	client, server := net.Pipe()
	errCh := make(chan error, 1)
	readCh := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(client)
		readCh <- data
	}()
	go func() { errCh <- s.Attach(server, AttachParams{Cols: 80, Rows: 24}) }()

	if err := <-errCh; err == nil {
		t.Fatal("Attach should reject a missing tmux session")
	}
	if attachStarted {
		t.Fatal("Attach started tmux attach-session after has-session failed")
	}

	data := <-readCh
	if len(data) != 0 {
		t.Fatalf("missing session leaked tmux diagnostics into terminal: %q", data)
	}
}

func TestTmuxShimAttachSetsChildTerminalWithoutMutatingParent(t *testing.T) {
	oldTerm, hadTerm := os.LookupEnv("TERM")
	if err := os.Unsetenv("TERM"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadTerm {
			_ = os.Setenv("TERM", oldTerm)
		} else {
			_ = os.Unsetenv("TERM")
		}
	})

	old := execCommand
	execCommand = func(_ string, args ...string) *exec.Cmd {
		if isTmuxAttachCommand(args) {
			return exec.Command("/bin/sh", "-c", `printf %s "$TERM"`)
		}
		return exec.Command("/bin/true")
	}
	t.Cleanup(func() { execCommand = old })

	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- (&tmuxShim{session: "sess"}).Attach(server, AttachParams{Cols: 80, Rows: 24})
	}()

	got, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if string(got) != "xterm-256color" {
		t.Fatalf("attach TERM = %q, want xterm-256color", got)
	}
	if _, ok := os.LookupEnv("TERM"); ok {
		t.Fatal("Attach mutated the parent TERM")
	}
}

func TestTmuxShimAttachForcesUTF8Client(t *testing.T) {
	old := execCommand
	var attachArgs []string
	execCommand = func(_ string, args ...string) *exec.Cmd {
		for _, arg := range args {
			if arg == "attach-session" {
				attachArgs = append([]string(nil), args...)
				return exec.Command("/bin/true")
			}
		}
		return exec.Command("/bin/true")
	}
	t.Cleanup(func() { execCommand = old })

	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- (&tmuxShim{session: "sess"}).Attach(
			server,
			AttachParams{Cols: 80, Rows: 24},
		)
	}()
	_, _ = io.ReadAll(client)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	want := []string{"-u", "attach-session", "-t", "sess"}
	if !reflect.DeepEqual(attachArgs, want) {
		t.Fatalf("attach args = %#v, want %#v", attachArgs, want)
	}
}

func TestTmuxShimAttachFailureLogsSanitizedCategory(t *testing.T) {
	iterDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(iterDir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}

	old := execCommand
	execCommand = func(_ string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "has-session" {
			return exec.Command("/bin/sh", "-c", "printf 'raw-secret-stderr' >&2; exit 1")
		}
		return exec.Command("/bin/true")
	}
	t.Cleanup(func() { execCommand = old })

	client, server := net.Pipe()
	err := (&tmuxShim{session: "sess", iterationDir: iterDir}).Attach(
		server,
		AttachParams{Cols: 80, Rows: 24},
	)
	_ = client.Close()
	if err == nil {
		t.Fatal("Attach succeeded after tmux has-session failed")
	}

	data, readErr := os.ReadFile(filepath.Join(iterDir, "logs", "shim.log"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	logText := string(data)
	if !strings.Contains(logText, "ERROR terminal attach category=session_missing session=sess") {
		t.Fatalf("shim.log missing sanitized attach category: %q", logText)
	}
	if strings.Contains(logText, "raw-secret-stderr") {
		t.Fatalf("shim.log contains discarded tmux stderr: %q", logText)
	}
}

func TestTmuxShimAttachRaceDoesNotLeakAttachError(t *testing.T) {
	old := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		switch {
		case len(args) > 0 && args[0] == "has-session":
			return exec.Command("/bin/true")
		case isTmuxAttachCommand(args):
			// Model the TOCTOU window: preflight saw the session, but it
			// disappeared before the attach client started. Real tmux writes
			// "no sessions" to stderr in this case.
			return exec.Command("/bin/sh", "-c", "printf 'no sessions\\n' >&2; exit 1")
		default:
			return exec.Command("/bin/true")
		}
	}
	defer func() { execCommand = old }()

	s := &tmuxShim{session: "raced-away"}
	client, server := net.Pipe()
	errCh := make(chan error, 1)
	readCh := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(client)
		readCh <- data
	}()
	go func() { errCh <- s.Attach(server, AttachParams{Cols: 80, Rows: 24}) }()

	if err := <-errCh; err != nil {
		t.Fatalf("Attach stream teardown: %v", err)
	}
	if data := <-readCh; len(data) != 0 {
		t.Fatalf("attach race leaked tmux diagnostics into terminal: %q", data)
	}
}

func TestSendKeysArgs_ItemsRawNoEnter(t *testing.T) {
	items := []KeyItem{{Text: "hello world"}, {Key: "Enter"}, {Key: "C-c"}}
	got := sendKeysCommands("sess", SendKeysParams{Items: items})
	want := [][]string{
		{"send-keys", "-t", "sess", "-l", "--", "hello world"},
		{"send-keys", "-t", "sess", "--", "Enter"},
		{"send-keys", "-t", "sess", "--", "C-c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items path:\n got %v\nwant %v", got, want)
	}
}

func TestSendKeysArgs_LegacyKeysAppendEnter(t *testing.T) {
	got := sendKeysCommands("sess", SendKeysParams{Keys: "ls -la"})
	want := [][]string{{"send-keys", "-t", "sess", "--", "ls -la", "Enter"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy path:\n got %v\nwant %v", got, want)
	}
}

func TestCaptureArgs_AnsiScrollback(t *testing.T) {
	got := captureArgs("sess", 200)
	want := []string{"capture-pane", "-e", "-p", "-S", "-200", "-t", "sess"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRunProcessModeWritesResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A trivial child that prints and exits 0.
	err := Run(Options{
		IterationDir: dir, Agent: "smoke", IterationID: "smoke-1", HardTimeoutS: 10,
		HarnessArgv: []string{"/bin/sh", "-c", "echo hello; echo oops 1>&2; exit 0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatalf("result.json missing: %v", err)
	}
	var r IterationResult
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 || r.EndedAt == "" || r.OOM {
		t.Fatalf("result = %+v", r)
	}
	if out, _ := os.ReadFile(filepath.Join(dir, "logs", "harness.stdout.log")); string(out) != "hello\n" {
		t.Fatalf("stdout log = %q", out)
	}
	if errl, _ := os.ReadFile(filepath.Join(dir, "logs", "harness.stderr.log")); string(errl) != "oops\n" {
		t.Fatalf("stderr log = %q", errl)
	}
	// shim.sock is removed when Run returns
	if _, err := os.Stat(filepath.Join(dir, "shim.sock")); err == nil {
		t.Fatal("shim.sock should be gone after Run")
	}
}

func TestRunProcessRedactsCodexProxyTokenOnlyFromShimLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	capture := filepath.Join(t.TempDir(), "codex.argv")
	stdinCapture := filepath.Join(t.TempDir(), "codex.stdin")
	fakeCodex := filepath.Join(binDir, "codex")
	if err := os.WriteFile(fakeCodex, []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$SHIM_TEST_CAPTURE\"\ncat > \"$SHIM_TEST_STDIN_CAPTURE\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHIM_TEST_CAPTURE", capture)
	t.Setenv("SHIM_TEST_STDIN_CAPTURE", stdinCapture)
	argv := codexHarnessArgv(t, binDir)
	original := append([]string(nil), argv...)

	if err := Run(Options{
		IterationDir: dir, Agent: "codex-agent", IterationID: "codex-exec-1", HardTimeoutS: 10,
		HarnessArgv: argv,
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(argv, original) {
		t.Fatal("shim logging mutated the original exec harness argv")
	}
	// The prompt reaches codex on stdin, never in argv: a single argv element is
	// capped at MAX_ARG_STRLEN and an oversized one fails execve with E2BIG.
	executed := readNULTerminatedArgs(t, capture)
	wantExecuted := original[6:]
	if !reflect.DeepEqual(executed, wantExecuted) {
		t.Fatalf("executed Codex argv differs from the adapter argv: got_count=%d want_count=%d", len(executed), len(wantExecuted))
	}
	if got, _ := os.ReadFile(stdinCapture); string(got) != "test prompt" {
		t.Fatalf("codex stdin = %q, want the prompt body", got)
	}
	assertSafeCodexShimLog(t, dir)
}

func TestRunNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "logs"), 0o700)
	if err := Run(Options{
		IterationDir: dir, Agent: "a", IterationID: "i", HardTimeoutS: 10,
		HarnessArgv: []string{"/bin/sh", "-c", "exit 7"},
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	var r IterationResult
	json.Unmarshal(data, &r)
	if r.ExitCode != 7 {
		t.Fatalf("exit_code = %d, want 7", r.ExitCode)
	}
}

func TestRunHardTimeoutKills(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "logs"), 0o700)
	start := time.Now()
	// HardTimeoutS is a whole number; use a fast override via the unexported knob.
	err := runWith(Options{
		IterationDir: dir, Agent: "a", IterationID: "i", HardTimeoutS: 1,
		HarnessArgv: []string{"/bin/sh", "-c", "sleep 30"},
	}, 300*time.Millisecond, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("hard timeout did not fire promptly: %v", time.Since(start))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	var r IterationResult
	json.Unmarshal(data, &r)
	if r.ExitCode == 0 {
		t.Fatalf("killed child should not report exit 0: %+v", r)
	}
}

func TestRunProcessForcedStopKillsSignalResistantHarness(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(dir, "harness.pid")
	forceStop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runProcessWithStop(Options{
			IterationDir: dir, Agent: "a", IterationID: "i", HardTimeoutS: 30,
			HarnessArgv: []string{
				"/bin/sh", "-c",
				`trap '' TERM; printf '%s' "$$" > "$1"; while :; do sleep 1; done`,
				"signal-resistant-harness", ready,
			},
		}, 100*time.Millisecond, 20*time.Millisecond, forceStop)
	}()

	var pid int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(ready)
		if err == nil {
			pid, err = strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid <= 0 {
		t.Fatal("signal-resistant harness did not start")
	}
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })

	close(forceStop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("forced stop did not terminate signal-resistant harness")
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("harness process group still exists after forced stop")
}

func TestInitialDeadlinePrefersPersistedAbsoluteDeadline(t *testing.T) {
	persisted := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339Nano)
	got := initialDeadline(Options{HardTimeoutS: 1, HardDeadline: persisted})
	want, err := time.Parse(time.RFC3339Nano, persisted)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Fatalf("initial deadline = %s, want persisted deadline %s", got, want)
	}
}

// A zero deadline means "no hard timeout" — the watchdog must never fire on its
// own, yet must still accept a later Set() (e.g. a resync that arms a real
// deadline). This is what lets an interactive agent run until it exits or is
// stopped instead of being killed by the default 60s timeout.
func TestDeadlineWatchdogZeroDeadlineNeverFiresButCanBeArmed(t *testing.T) {
	fired := make(chan struct{}, 1)
	w := newDeadlineWatchdog(time.Time{}, func() { fired <- struct{}{} })
	defer w.Stop()
	select {
	case <-fired:
		t.Fatal("watchdog with a zero (disabled) deadline fired on its own")
	case <-time.After(120 * time.Millisecond):
	}
	// A subsequent Set must still arm the watchdog.
	deadline := time.Now().Add(40 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	if err := w.Set(deadline); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fired:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("watchdog did not fire after being armed via Set")
	}
}

// tmuxDeadline (the interactive path) must NOT apply the 60s default: with no
// explicit hard timeout/deadline it returns the zero time (no deadline). An
// explicit HardTimeoutS or HardDeadline still arms it.
func TestTmuxDeadlineNoDefaultForInteractive(t *testing.T) {
	if got := tmuxDeadline(Options{}); !got.IsZero() {
		t.Fatalf("tmuxDeadline with no timeout = %s, want zero (no deadline)", got)
	}
	if got := tmuxDeadline(Options{HardTimeoutS: 30}); got.IsZero() {
		t.Fatal("tmuxDeadline with HardTimeoutS should arm a deadline, got zero")
	}
	persisted := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339Nano)
	want, _ := time.Parse(time.RFC3339Nano, persisted)
	if got := tmuxDeadline(Options{HardDeadline: persisted}); !got.Equal(want) {
		t.Fatalf("tmuxDeadline with HardDeadline = %s, want %s", got, want)
	}
}

func TestDeadlineWatchdogResetHonorsLatestAbsoluteDeadline(t *testing.T) {
	fired := make(chan struct{}, 1)
	w := newDeadlineWatchdog(time.Now().Add(40*time.Millisecond), func() { fired <- struct{}{} })
	defer w.Stop()
	deadline := time.Now().Add(150 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	if err := w.Set(deadline); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fired:
		t.Fatal("watchdog fired at stale deadline after reset")
	case <-time.After(80 * time.Millisecond):
	}
	select {
	case <-fired:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("watchdog did not fire at updated deadline")
	}
}

func TestShellJoinQuotesArgs(t *testing.T) {
	got := shellJoin([]string{"claude", "--name", "a b", "it's ok", ""})
	want := `'claude' '--name' 'a b' 'it'"'"'s ok' ''`
	if got != want {
		t.Fatalf("shellJoin = %q, want %q", got, want)
	}
}

func TestRunTmuxSupervisorPreservesExitAndArgv(t *testing.T) {
	binDir := t.TempDir()
	harness := filepath.Join(binDir, "harness with spaces")
	script := "#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$SHIM_TEST_ARGV\"\nexit \"$SHIM_TEST_EXIT\"\n"
	if err := os.WriteFile(harness, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeTmux := filepath.Join(binDir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/bin/sh\n[ \"$1\" = list-sessions ] && printf 'supervised\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	wantArgv := []string{"argument with spaces", "it's quoted", ""}

	for _, wantExit := range []int{0, 127} {
		t.Run(strconv.Itoa(wantExit), func(t *testing.T) {
			logsDir := t.TempDir()
			statusPath := filepath.Join(logsDir, "harness.exit-status")
			capturePath := filepath.Join(t.TempDir(), "argv")
			t.Setenv("SHIM_TEST_ARGV", capturePath)
			t.Setenv("SHIM_TEST_EXIT", strconv.Itoa(wantExit))
			t.Setenv("PATH", binDir)

			if err := RunTmuxSupervisor("supervised", statusPath, append([]string{harness}, wantArgv...)); err != nil {
				t.Fatal(err)
			}
			if gotArgv := readNULTerminatedArgs(t, capturePath); !reflect.DeepEqual(gotArgv, wantArgv) {
				t.Fatalf("harness argv = %#v, want %#v", gotArgv, wantArgv)
			}
			gotStatus, err := os.ReadFile(statusPath)
			if err != nil {
				t.Fatalf("read status: %v", err)
			}
			if want := strconv.Itoa(wantExit) + "\n"; string(gotStatus) != want {
				t.Fatalf("status = %q, want %q", gotStatus, want)
			}
			info, err := os.Stat(statusPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != 0o600 {
				t.Fatalf("status mode = %o, want 600", got)
			}
			temps, err := filepath.Glob(statusPath + ".tmp.*")
			if err != nil {
				t.Fatal(err)
			}
			if len(temps) != 0 {
				t.Fatalf("temporary status files remain after atomic rename: %#v", temps)
			}
		})
	}
}

func TestRunTmuxSupervisorIgnoresStoppedHarness(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, "stoppable-harness.sh")
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
printf '%s\n' "$$" > "$1"
trap ':' TERM
while [ ! -e "$2" ]; do /bin/sleep 0.01; done
exit 23
`), 0o700); err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(dir, "harness.pid")
	releasePath := filepath.Join(dir, "release")
	statusPath := filepath.Join(dir, "status")
	cmd := exec.Command(os.Args[0], TmuxSupervisorMode, "supervised", statusPath, "--", harness, pidPath, releasePath)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	waited := false

	var pid int
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(pidPath)
		if err == nil {
			pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 1 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid <= 1 {
		t.Fatal("harness did not write a valid pid")
	}
	t.Cleanup(func() {
		_ = os.WriteFile(releasePath, nil, 0o600)
		_ = syscall.Kill(pid, syscall.SIGCONT)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		if !waited {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("supervisor did not exit during cleanup")
			}
		}
	})

	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		waited = true
		t.Fatalf("supervisor treated stop/continue as exit: %v; status %d", err, readSupervisorStatus(t, statusPath))
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Fatalf("status written before exit: %v", err)
	}
	if err := os.WriteFile(releasePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := <-done
	waited = true
	if err != nil {
		t.Fatal(err)
	}
	if got := readSupervisorStatus(t, statusPath); got != 23 {
		t.Fatalf("status = %d, want 23", got)
	}
}

func readSupervisorStatus(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	status, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func TestRunTmuxSupervisorOperationalFailuresDoNotWriteStatus(t *testing.T) {
	testErr := errors.New("test syscall failure")
	tests := []struct {
		name string
		wait func(int) error
		kill func(int, syscall.Signal) error
		reap func(int, *syscall.WaitStatus, int, *syscall.Rusage) (int, error)
	}{
		{
			name: "observer failure",
			wait: func(int) error { return testErr },
		},
		{
			name: "group signal failure",
			wait: func(int) error { return nil },
			kill: func(int, syscall.Signal) error { return testErr },
		},
		{
			name: "reap failure",
			wait: func(int) error { return nil },
			reap: func(int, *syscall.WaitStatus, int, *syscall.Rusage) (int, error) {
				return -1, testErr
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldWait, oldKill, oldReap := waitTmuxChildExit, killTmuxGroup, reapTmuxChild
			waitTmuxChildExit = tc.wait
			pid := 0
			killTmuxGroup = func(target int, signal syscall.Signal) error {
				pid = -target
				if tc.kill != nil {
					return tc.kill(target, signal)
				}
				return syscall.Kill(target, signal)
			}
			if tc.reap != nil {
				reapTmuxChild = tc.reap
			}
			statusPath := filepath.Join(t.TempDir(), "status")
			start := time.Now()
			err := RunTmuxSupervisor("supervised", statusPath, []string{"/bin/sh", "-c", "while :; do /bin/sleep 1; done"})
			waitTmuxChildExit, killTmuxGroup, reapTmuxChild = oldWait, oldKill, oldReap
			if pid > 1 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
				var status syscall.WaitStatus
				_, _ = syscall.Wait4(pid, &status, 0, nil)
			}
			if !errors.Is(err, testErr) {
				t.Fatalf("error = %v, want test syscall failure", err)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("operational failure returned after %v", elapsed)
			}
			if _, statErr := os.Stat(statusPath); !os.IsNotExist(statErr) {
				t.Fatalf("status written after operational failure: %v", statErr)
			}
		})
	}
}

func TestRunTmuxSupervisorHUPPreservesCooperativeExit(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "harness.pid")
	statusPath := filepath.Join(dir, "status")
	harness := filepath.Join(dir, "cooperative-harness.sh")
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
printf '%s\n' "$$" > "$1"
trap 'exit 23' TERM
while :; do /bin/sleep 1; done
`), 0o700); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- RunTmuxSupervisor("supervised", statusPath, []string{harness, pidPath}) }()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(pidPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatal("harness did not become ready")
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cooperative harness did not exit after HUP")
	}
	if got := readSupervisorStatus(t, statusPath); got != 23 {
		t.Fatalf("status = %d, want 23", got)
	}
}

func TestRunTmuxSupervisorBoundedExitWaits(t *testing.T) {
	t.Run("TERM wait expires before KILL observes exit", func(t *testing.T) {
		oldWait, oldKill := waitTmuxChildExit, killTmuxGroup
		killed := make(chan struct{})
		var once sync.Once
		waitTmuxChildExit = func(int) error {
			<-killed
			return nil
		}
		killTmuxGroup = func(pid int, signal syscall.Signal) error {
			err := syscall.Kill(pid, signal)
			if signal == syscall.SIGKILL {
				once.Do(func() { close(killed) })
			}
			return err
		}
		t.Cleanup(func() {
			waitTmuxChildExit, killTmuxGroup = oldWait, oldKill
		})

		dir := t.TempDir()
		pidPath := filepath.Join(dir, "harness.pid")
		statusPath := filepath.Join(dir, "status")
		done := make(chan error, 1)
		go func() {
			done <- RunTmuxSupervisor("supervised", statusPath, []string{"/bin/sh", "-c", `trap '' TERM; printf '%s' "$$" > "$1"; while :; do /bin/sleep 1; done`, "harness", pidPath})
		}()
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			if _, err := os.Stat(pidPath); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if _, err := os.Stat(pidPath); err != nil {
			t.Fatal("harness did not become ready")
		}
		start := time.Now()
		if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed < 2*time.Second || elapsed > 3*time.Second {
			t.Fatalf("TERM wait elapsed %v, want two-second expiry", elapsed)
		}
		if got := readSupervisorStatus(t, statusPath); got != 137 {
			t.Fatalf("status = %d, want 137", got)
		}
	})

	t.Run("KILL wait expires without status", func(t *testing.T) {
		oldWait := waitTmuxChildExit
		release := make(chan struct{})
		waitTmuxChildExit = func(int) error {
			<-release
			return nil
		}
		t.Cleanup(func() {
			close(release)
			waitTmuxChildExit = oldWait
		})

		dir := t.TempDir()
		pidPath := filepath.Join(dir, "harness.pid")
		statusPath := filepath.Join(dir, "status")
		done := make(chan error, 1)
		go func() {
			done <- RunTmuxSupervisor("supervised", statusPath, []string{"/bin/sh", "-c", `trap '' TERM; printf '%s' "$$" > "$1"; while :; do /bin/sleep 1; done`, "harness", pidPath})
		}()
		var pid int
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			data, err := os.ReadFile(pidPath)
			if err == nil {
				pid, _ = strconv.Atoi(string(data))
				if pid > 1 {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		if pid <= 1 {
			t.Fatal("harness did not write a valid pid")
		}
		start := time.Now()
		if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
			t.Fatal(err)
		}
		err := <-done
		if elapsed := time.Since(start); elapsed < 4*time.Second || elapsed > 5*time.Second {
			t.Fatalf("bounded waits elapsed %v, want four seconds", elapsed)
		}
		if err == nil {
			t.Fatal("supervisor succeeded without confirmed child exit")
		}
		if _, statErr := os.Stat(statusPath); !os.IsNotExist(statErr) {
			t.Fatalf("status written after exit wait expiry: %v", statErr)
		}
		if pid > 1 {
			var status syscall.WaitStatus
			_, _ = syscall.Wait4(pid, &status, 0, nil)
		}
	})
}

func TestRunTmuxSupervisorPreservesExitedHarnessStatusAndKillsDescendant(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, "race-harness.sh")
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
/bin/sh -c 'printf '\''%s\n'\'' "$$" > "$1"; trap '\'''\'' HUP INT TERM; /bin/sleep 0.3; printf survived > "$2"' child "$3" "$4" &
printf ready > "$1"
while [ ! -e "$2" ]; do /bin/sleep 0.01; done
exit 23
`), 0o700); err != nil {
		t.Fatal(err)
	}
	readyPath := filepath.Join(dir, "ready")
	releasePath := filepath.Join(dir, "release")
	childPIDPath := filepath.Join(dir, "child.pid")
	survivedPath := filepath.Join(dir, "survived")
	statusPath := filepath.Join(dir, "status")
	cmd := exec.Command(os.Args[0], TmuxSupervisorMode, "supervised", statusPath, "--", harness, readyPath, releasePath, childPIDPath, survivedPath)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	waited := false
	t.Cleanup(func() {
		_ = os.WriteFile(releasePath, nil, 0o600)
		_ = cmd.Process.Signal(syscall.SIGCONT)
		if !waited {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("bounded supervisor did not exit during cleanup")
			}
		}
		if data, err := os.ReadFile(childPIDPath); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 1 {
				for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil; {
					time.Sleep(10 * time.Millisecond)
				}
			}
		}
	})

	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(readyPath); err != nil {
		t.Fatal("harness did not become ready")
	}
	childPID := 0
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(childPIDPath)
		if err == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && childPID > 1 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID <= 1 {
		t.Fatal("descendant did not write a valid pid")
	}
	if err := os.WriteFile(releasePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		waited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not reap the exited harness")
	}
	data, err := os.ReadFile(statusPath)
	if err != nil || string(data) != "23\n" {
		t.Fatalf("status = %q, error = %v; want 23", data, err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && syscall.Kill(childPID, 0) == nil; {
		time.Sleep(10 * time.Millisecond)
	}
	if syscall.Kill(childPID, 0) == nil {
		t.Fatal("descendant survived the leader's SIGCHLD")
	}
	if _, err := os.Stat(survivedPath); !os.IsNotExist(err) {
		t.Fatalf("descendant completed instead of being killed: %v", err)
	}
}

func TestReadTmuxExitStatus(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for _, tc := range []struct {
			content string
			want    int
		}{
			{content: "0\n", want: 0},
			{content: "127\n", want: 127},
			{content: "255\n", want: 255},
		} {
			path := filepath.Join(t.TempDir(), "status")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := readTmuxExitStatus(path)
			if err != nil {
				t.Fatalf("readTmuxExitStatus(%q): %v", tc.content, err)
			}
			if got != tc.want {
				t.Fatalf("readTmuxExitStatus(%q) = %d, want %d", tc.content, got, tc.want)
			}
		}
	})

	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			name    string
			content string
		}{
			{name: "empty"},
			{name: "malformed", content: "not-a-status\n"},
			{name: "negative", content: "-1\n"},
			{name: "out of range", content: "256\n"},
			{name: "leading whitespace", content: " 1\n"},
			{name: "trailing data", content: "1\n2\n"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "status")
				if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
				if got, err := readTmuxExitStatus(path); err == nil || got != -1 {
					t.Fatalf("readTmuxExitStatus(%q) = (%d, %v), want (-1, error)", tc.content, got, err)
				}
			})
		}
	})

	t.Run("missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")
		if got, err := readTmuxExitStatus(path); err == nil || got != -1 {
			t.Fatalf("readTmuxExitStatus(missing) = (%d, %v), want (-1, error)", got, err)
		}
	})
}

func TestTmuxBootstrapSessionArgsPassesEnv(t *testing.T) {
	got := tmuxBootstrapSessionArgs("agent-iter", []string{
		"ANTHROPIC_BASE_URL=http://127.0.0.1:1234/token",
		"",
		"=invalid",
		"TARIBOY_TOOLS_SOCKET=/tmp/tools.sock",
	})
	want := []string{
		"new-session", "-d", "-P", "-F", "#{window_id}", "-s", "agent-iter",
		"-e", "ANTHROPIC_BASE_URL=http://127.0.0.1:1234/token",
		"-e", "TARIBOY_TOOLS_SOCKET=/tmp/tools.sock",
	}
	if len(got) != len(want) {
		t.Fatalf("tmux args len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tmux args[%d] = %q, want %q; all args: %#v", i, got[i], want[i], got)
		}
	}
}

func TestRunTmuxLogsNewSessionError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Fake tmux on PATH whose new-session fails (mimics "duplicate session").
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "tmux")
	script := "#!/bin/sh\nif [ \"$1\" = new-session ]; then echo 'duplicate session: x' 1>&2; exit 1; fi\nexit 0\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := runTmux(Options{
		IterationDir: dir, Agent: "manager", IterationID: "manager-x-1",
		TmuxSession: "manager", HarnessArgv: []string{"claude"},
	}, 10*time.Millisecond)
	if err == nil {
		t.Fatal("runTmux should return the new-session error")
	}
	log, _ := os.ReadFile(filepath.Join(dir, "logs", "shim.log"))
	if !strings.Contains(string(log), "ERROR tmux new-session") {
		t.Fatalf("shim.log missing new-session error line:\n%s", log)
	}
}

func TestRunTmuxCreatesHarnessWindowAfterSettingSessionHistoryLimit(t *testing.T) {
	old := execCommand
	defer func() { execCommand = old }()

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "tmux" {
			t.Fatalf("command = %q, want tmux", name)
		}
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "new-session":
			return exec.Command("/bin/sh", "-c", "printf '@42'")
		case "has-session":
			return exec.Command("/bin/false")
		default:
			return exec.Command("/bin/true")
		}
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		IterationDir: dir, Agent: "manager", IterationID: "manager-history-1",
		TmuxSession: "managed-history", HarnessArgv: []string{"claude", "prompt with spaces"},
	}
	if err := runTmux(opts, time.Millisecond); err != nil {
		t.Fatalf("runTmux() error = %v", err)
	}

	var historyIndex, newWindowIndex, killIndex = -1, -1, -1
	for i, call := range calls {
		for _, arg := range call {
			if arg == "-g" {
				t.Fatalf("tmux call %q must not set a global option: %#v", call[0], call)
			}
		}
		switch call[0] {
		case "set-option":
			if reflect.DeepEqual(call, []string{"set-option", "-t", opts.TmuxSession, "history-limit", "10000"}) {
				historyIndex = i
			}
		case "new-window":
			newWindowIndex = i
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			wantCommand := shellJoin(tmuxSupervisorCommand(executable, opts.TmuxSession, filepath.Join(dir, "logs", tmuxExitStatusFilename), opts.HarnessArgv))
			if !reflect.DeepEqual(call, []string{"new-window", "-t", opts.TmuxSession, wantCommand}) {
				t.Fatalf("new-window args = %#v, want harness command byte-identical to prior new-session command", call)
			}
		case "kill-window":
			killIndex = i
			if !reflect.DeepEqual(call, []string{"kill-window", "-t", "@42"}) {
				t.Fatalf("kill-window args = %#v, want captured bootstrap id", call)
			}
		}
	}
	if historyIndex == -1 || newWindowIndex == -1 || killIndex == -1 {
		t.Fatalf("tmux calls = %#v, want history-limit, new-window, and bootstrap kill-window", calls)
	}
	if historyIndex >= newWindowIndex {
		t.Fatalf("history-limit call index = %d, new-window index = %d; limit must precede harness window creation", historyIndex, newWindowIndex)
	}
	if killIndex <= newWindowIndex {
		t.Fatalf("kill-window call index = %d, new-window index = %d; bootstrap must survive until harness window exists", killIndex, newWindowIndex)
	}
}

func TestRunTmuxKeepsBootstrapWindowWhenHarnessWindowCreationFails(t *testing.T) {
	old := execCommand
	defer func() { execCommand = old }()

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "new-session":
			return exec.Command("/bin/sh", "-c", "printf '@42'")
		case "new-window":
			return exec.Command("/bin/false")
		default:
			return exec.Command("/bin/true")
		}
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := runTmux(Options{
		IterationDir: dir, Agent: "manager", IterationID: "manager-history-failure-1",
		TmuxSession: "managed-history-failure", HarnessArgv: []string{"claude"},
	}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "tmux new-window") {
		t.Fatalf("runTmux() error = %v, want new-window error", err)
	}
	for _, call := range calls {
		if call[0] == "kill-window" {
			t.Fatalf("bootstrap window must not be killed after new-window failure: %#v", calls)
		}
	}
}

func TestRunTmuxDisablesMouseForEachManagedSession(t *testing.T) {
	old := execCommand
	defer func() { execCommand = old }()

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "tmux" {
			t.Fatalf("command = %q, want tmux", name)
		}
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "new-session":
			return exec.Command("/bin/sh", "-c", "printf '@42'")
		case "set-option":
			if len(args) == 5 && args[3] == "history-limit" {
				return exec.Command("/bin/true")
			}
			return exec.Command("/bin/false")
		case "has-session":
			return exec.Command("/bin/false")
		default:
			return exec.Command("/bin/true")
		}
	}

	for _, session := range []string{"managed-alpha", "managed-beta"} {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
			t.Fatal(err)
		}
		err := runTmux(Options{
			IterationDir: dir, Agent: session, IterationID: session + "-1",
			TmuxSession: session, HarnessArgv: []string{"claude"},
		}, time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "tmux set-option") {
			t.Fatalf("runTmux(%q) error = %v, want set-option failure", session, err)
		}
		log, readErr := os.ReadFile(filepath.Join(dir, "logs", "shim.log"))
		if readErr != nil || !strings.Contains(string(log), "ERROR tmux set-option session="+session) {
			t.Fatalf("shim.log = %q, read error = %v; want set-option error for %q", log, readErr, session)
		}
	}

	var targets []string
	for _, call := range calls {
		for _, arg := range call {
			if arg == "-g" {
				t.Fatalf("tmux call %q must not set a global option: %#v", call[0], call)
			}
		}
		if call[0] == "set-option" {
			if call[3] != "mouse" {
				continue
			}
			if len(call) != 5 || call[1] != "-t" || call[3] != "mouse" || call[4] != "off" {
				t.Fatalf("set-option args = %#v, want exactly session-scoped mouse off", call)
			}
			targets = append(targets, call[2])
		}
	}
	if !reflect.DeepEqual(targets, []string{"managed-alpha", "managed-beta"}) {
		t.Fatalf("set-option targets = %#v, want distinct managed sessions", targets)
	}
}

func TestRunTmuxRedactsCodexProxyTokenOnlyFromShimLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	capture := filepath.Join(t.TempDir(), "tmux.argv")
	fakeTmux := filepath.Join(binDir, "tmux")
	script := `#!/bin/sh
case "$1" in
new-session)
	printf '@42'
	;;
set-option)
	;;
new-window)
	printf '%s\000' "$@" > "$SHIM_TEST_TMUX_CAPTURE"
	exit 1
	;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHIM_TEST_TMUX_CAPTURE", capture)

	argv := codexHarnessArgv(t, binDir)
	original := append([]string(nil), argv...)
	err := runTmux(Options{
		IterationDir: dir, Agent: "codex-agent", IterationID: "codex-tmux-1",
		TmuxSession: "codex-agent", HarnessArgv: argv,
	}, 10*time.Millisecond)
	if err == nil {
		t.Fatal("runTmux should return the fake new-session error")
	}
	if !reflect.DeepEqual(argv, original) {
		t.Fatal("shim logging mutated the original tmux harness argv")
	}
	launched := readNULTerminatedArgs(t, capture)
	if len(launched) == 0 {
		t.Fatal("tmux launch argv capture is empty")
	}
	statusPath := filepath.Join(dir, "logs", tmuxExitStatusFilename)
	executable, executableErr := os.Executable()
	if executableErr != nil {
		t.Fatal(executableErr)
	}
	wantCommand := shellJoin(tmuxSupervisorCommand(executable, "codex-agent", statusPath, original))
	if launched[len(launched)-1] != wantCommand {
		t.Fatalf("tmux command differs from the original harness argv: arg_count=%d", len(launched))
	}
	assertSafeCodexShimLog(t, dir)
}

func TestKillTmuxSessionTerminatesAllPanesAndPreservesPrefixedSession(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", t.TempDir())

	dir := t.TempDir()
	shimBin := filepath.Join(dir, "tariboy-shim")
	build := exec.Command("go", "build", "-o", shimBin, "./cmd/tariboy-shim")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build tariboy-shim: %v\n%s", err, output)
	}
	harness := filepath.Join(dir, "bounded-harness.sh")
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
printf '%s\n' "$$" > "$1"
/bin/sh -c 'printf '\''%s\n'\'' "$$" > "$1"; trap '\'''\'' TERM HUP INT; i=0; while [ "$i" -lt 120 ]; do /bin/sleep 0.05; i=$((i + 1)); done' child "$2" &
trap '' TERM HUP INT
wait
`), 0o700); err != nil {
		t.Fatal(err)
	}
	managed, decoy := "shim-kill", "shim-kill-decoy"
	markers := []string{
		filepath.Join(dir, "managed-1.pid"), filepath.Join(dir, "managed-1-child.pid"),
		filepath.Join(dir, "managed-2.pid"), filepath.Join(dir, "managed-2-child.pid"),
	}
	statuses := []string{filepath.Join(dir, "managed-1.status"), filepath.Join(dir, "managed-2.status")}
	runTmux := func(args ...string) error { return exec.Command(tmux, args...).Run() }
	supervisorBinDir := t.TempDir()
	supervisorTmuxCalled := filepath.Join(dir, "supervisor-tmux-called")
	if err := os.WriteFile(filepath.Join(supervisorBinDir, "tmux"), []byte("#!/bin/sh\nprintf called > \"$SHIM_TEST_SUPERVISOR_TMUX_CALLED\"\nexit 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	supervisorCommand := func(status string, paneMarkers []string) string {
		return shellJoin([]string{"/usr/bin/env", "PATH=" + supervisorBinDir, "SHIM_TEST_SUPERVISOR_TMUX_CALLED=" + supervisorTmuxCalled, shimBin, "__tmux-supervisor", managed, status, "--", harness, paneMarkers[0], paneMarkers[1]})
	}
	readPID := func(marker string) (int, bool) {
		data, err := os.ReadFile(marker)
		if err != nil {
			return 0, false
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		return pid, err == nil && pid > 1
	}
	var pids []int
	decoyPID := 0
	t.Cleanup(func() {
		_ = runTmux("kill-session", "-t", "="+managed)
		_ = runTmux("kill-session", "-t", "="+decoy)
		_ = runTmux("kill-server")
		cleanupPIDs := append([]int(nil), pids...)
		if decoyPID > 1 {
			cleanupPIDs = append(cleanupPIDs, decoyPID)
		}
		deadline := time.Now().Add(9 * time.Second)
		for _, pid := range cleanupPIDs {
			for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
				time.Sleep(10 * time.Millisecond)
			}
			if syscall.Kill(pid, 0) == nil {
				t.Errorf("bounded owned process %d survived cleanup", pid)
			}
		}
	})
	if err := runTmux("new-session", "-d", "-s", managed, supervisorCommand(statuses[0], markers[:2])); err != nil {
		t.Fatal(err)
	}
	if err := runTmux("split-window", "-d", "-t", "="+managed+":", supervisorCommand(statuses[1], markers[2:])); err != nil {
		t.Fatal(err)
	}
	if err := runTmux("new-session", "-d", "-s", decoy, "/bin/sleep 8"); err != nil {
		t.Fatal(err)
	}

	for _, marker := range markers {
		deadline := time.Now().Add(3 * time.Second)
		found := false
		for time.Now().Before(deadline) {
			if pid, ok := readPID(marker); ok {
				pids = append(pids, pid)
				found = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !found {
			t.Fatalf("pane did not write owned marker %s", marker)
		}
	}
	output, err := exec.Command(tmux, "list-panes", "-t", "="+managed, "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range strings.Fields(string(output)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 1 {
			t.Fatalf("invalid supervisor pid %q", field)
		}
		pids = append(pids, pid)
	}
	decoyOutput, err := exec.Command(tmux, "list-panes", "-t", "="+decoy, "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	decoyPID, err = strconv.Atoi(strings.TrimSpace(string(decoyOutput)))
	if err != nil || decoyPID <= 1 {
		t.Fatalf("invalid decoy pid %q", decoyOutput)
	}

	if err := KillTmuxSession(managed); err != nil {
		t.Fatal(err)
	}
	for _, pid := range pids {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
			time.Sleep(10 * time.Millisecond)
		}
		if syscall.Kill(pid, 0) == nil {
			t.Fatalf("managed supervisor or harness process %d survived Kill", pid)
		}
	}
	for _, statusPath := range statuses {
		status, err := os.ReadFile(statusPath)
		if err != nil || string(status) != "137\n" {
			t.Fatalf("supervisor status = %q, error = %v; want 137", status, err)
		}
	}
	if _, err := os.Stat(supervisorTmuxCalled); !os.IsNotExist(err) {
		t.Fatalf("pane supervisor invoked tmux: %v", err)
	}
	if err := KillTmuxSession(managed); err != nil {
		t.Fatalf("second Kill of absent exact session: %v", err)
	}
	if err := runTmux("has-session", "-t", "="+decoy); err != nil {
		t.Fatal("Kill of absent managed session removed prefixed decoy")
	}
	if err := syscall.Kill(decoyPID, 0); err != nil {
		t.Fatal("prefixed decoy pane process was killed")
	}
}

func TestTmuxSupervisorGivesHarnessForegroundTerminal(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", t.TempDir())

	dir := t.TempDir()
	shimBin := filepath.Join(dir, "tariboy-shim")
	build := exec.Command("go", "build", "-o", shimBin, "./cmd/tariboy-shim")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build tariboy-shim: %v\n%s", err, output)
	}
	harness := filepath.Join(dir, "interactive-harness.sh")
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
printf '%s\n' "$$" > "$1"
trap 'printf int > "$3"; exit 23' INT
trap 'printf ttin > "$4"; exit 24' TTIN
IFS= read -r line
printf '%s\n' "$line" > "$2"
i=0
while [ "$i" -lt 120 ]; do /bin/sleep 0.05; i=$((i + 1)); done
`), 0o700); err != nil {
		t.Fatal(err)
	}
	session := "shim-foreground"
	pidPath := filepath.Join(dir, "harness.pid")
	inputPath := filepath.Join(dir, "input")
	intPath := filepath.Join(dir, "int")
	ttinPath := filepath.Join(dir, "ttin")
	statusPath := filepath.Join(dir, "harness.status")
	supervisorErrPath := filepath.Join(dir, "supervisor.stderr")
	runTmux := func(args ...string) error { return exec.Command(tmux, args...).Run() }

	var pids []int
	t.Cleanup(func() {
		if output, err := exec.Command(tmux, "list-panes", "-t", "="+session, "-F", "#{pane_pid}").Output(); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(output))); err == nil && pid > 1 {
				pids = append(pids, pid)
			}
		}
		_ = runTmux("kill-session", "-t", "="+session)
		_ = runTmux("kill-server")
		deadline := time.Now().Add(8 * time.Second)
		for _, pid := range pids {
			for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
				time.Sleep(10 * time.Millisecond)
			}
			if syscall.Kill(pid, 0) == nil {
				t.Errorf("bounded interactive process %d survived cleanup", pid)
			}
		}
	})
	command := shellJoin([]string{shimBin, TmuxSupervisorMode, session, statusPath, "--", harness, pidPath, inputPath, intPath, ttinPath}) + " 2>" + shellQuote(supervisorErrPath)
	if err := runTmux("new-session", "-d", "-s", session, command); err != nil {
		t.Fatal(err)
	}

	readPID := func(path string) int {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			data, err := os.ReadFile(path)
			if err == nil {
				pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
				if err == nil && pid > 1 {
					return pid
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("missing valid pid marker %s", path)
		return 0
	}
	harnessPID := readPID(pidPath)
	pids = append(pids, harnessPID)
	supervisorOutput, err := exec.Command(tmux, "list-panes", "-t", "="+session, "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	supervisorPID, err := strconv.Atoi(strings.TrimSpace(string(supervisorOutput)))
	if err != nil || supervisorPID <= 1 || supervisorPID == harnessPID {
		t.Fatalf("supervisor pid = %q, harness pid = %d", supervisorOutput, harnessPID)
	}
	pids = append(pids, supervisorPID)
	paneTarget := "=" + session + ":"

	sendErr := runTmux("send-keys", "-t", paneTarget, "-l", "--", "typed input")
	if sendErr == nil {
		sendErr = runTmux("send-keys", "-t", paneTarget, "Enter")
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if data, err := os.ReadFile(inputPath); err == nil && string(data) == "typed input\n" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if data, err := os.ReadFile(inputPath); err != nil || string(data) != "typed input\n" {
		ttin, _ := os.ReadFile(ttinPath)
		supervisorErr, _ := os.ReadFile(supervisorErrPath)
		status, _ := os.ReadFile(statusPath)
		pane, _ := exec.Command(tmux, "list-panes", "-t", "="+session, "-F", "dead=#{pane_dead} status=#{pane_dead_status} signal=#{pane_dead_signal}").Output()
		t.Fatalf("harness did not read pane input (input=%q, ttin=%q, status=%q, read_error=%v, send_error=%v, supervisor_error=%q, pane=%q)", data, ttin, status, err, sendErr, supervisorErr, pane)
	}
	if syscall.Kill(supervisorPID, 0) != nil {
		t.Fatal("supervisor exited before its harness")
	}
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Fatalf("supervisor wrote status before harness exit: %v", err)
	}

	if err := runTmux("send-keys", "-t", paneTarget, "C-c"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		data, statusErr := os.ReadFile(statusPath)
		if _, intErr := os.Stat(intPath); intErr == nil && statusErr == nil && string(data) == "23\n" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, statusErr := os.ReadFile(statusPath)
	_, intErr := os.Stat(intPath)
	t.Fatalf("terminal C-c did not reach harness (status=%q, status_error=%v, int_error=%v)", data, statusErr, intErr)
}

func TestKillTmuxSessionPropagatesTmuxOperationalError(t *testing.T) {
	old := execCommand
	t.Cleanup(func() { execCommand = old })
	execCommand = func(string, ...string) *exec.Cmd {
		return exec.Command("/bin/sh", "-c", "printf 'permission denied\\n' >&2; exit 1")
	}

	err := KillTmuxSession("managed")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("KillTmuxSession error = %v, want tmux operational error", err)
	}
}

func TestRunTmuxSupervisorKillsOwnedHarnessGroupOnHUP(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, "bounded-harness.sh")
	if err := os.WriteFile(harness, []byte(`#!/bin/sh
printf '%s\n' "$$" > "$1"
/bin/sh -c 'printf '\''%s\n'\'' "$$" > "$1"; trap '\'''\'' TERM HUP INT; i=0; while [ "$i" -lt 100 ]; do sleep 0.05; i=$((i + 1)); done' child "$2" &
trap '' TERM HUP INT
wait
`), 0o755); err != nil {
		t.Fatal(err)
	}
	leaderMarker := filepath.Join(dir, "leader.pid")
	childMarker := filepath.Join(dir, "child.pid")
	statusPath := filepath.Join(dir, tmuxExitStatusFilename)
	done := make(chan struct{})
	var supervisorErr error
	go func() {
		supervisorErr = RunTmuxSupervisor("supervised", statusPath, []string{harness, leaderMarker, childMarker})
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("bounded supervisor cleanup did not finish")
		}
	})

	readPID := func(path string) int {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			data, err := os.ReadFile(path)
			if err == nil {
				pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
				if err == nil && pid > 1 {
					return pid
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("missing valid pid marker %s", path)
		return 0
	}
	leaderPID := readPID(leaderMarker)
	childPID := readPID(childMarker)
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop after HUP")
	}
	if supervisorErr != nil {
		t.Fatal(supervisorErr)
	}
	for _, pid := range []int{leaderPID, childPID} {
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil; {
			time.Sleep(10 * time.Millisecond)
		}
		if syscall.Kill(pid, 0) == nil {
			t.Fatalf("owned harness process %d survived supervisor", pid)
		}
	}
	data, err := os.ReadFile(statusPath)
	if err != nil || string(data) != "137\n" {
		t.Fatalf("status = %q, error = %v; want 137", data, err)
	}
}

func TestRunTmuxPreservesHarnessExit(t *testing.T) {
	binDir := t.TempDir()
	fakeTmux := filepath.Join(binDir, "tmux")
	script := `#!/bin/sh
case "$1" in
new-session)
	: > "$SHIM_TEST_TMUX_SESSION"
	printf '@42'
	;;
new-window)
	shift
	last=
	for arg do
		last=$arg
	done
	if [ "${SHIM_TEST_KEEP_SESSION:-}" = 1 ]; then
		:
	elif [ "${SHIM_TEST_SKIP_COMMAND:-}" = 1 ]; then
		(sleep 0.02; rm -f "$SHIM_TEST_TMUX_SESSION") &
	elif [ -n "${SHIM_TEST_TMUX_OUTPUT:-}" ]; then
		(/bin/sh -c "$last" > "$SHIM_TEST_TMUX_OUTPUT" 2>&1; rm -f "$SHIM_TEST_TMUX_SESSION") &
	else
		(/bin/sh -c "$last"; rm -f "$SHIM_TEST_TMUX_SESSION") &
	fi
	;;
pipe-pane)
	if [ "${SHIM_TEST_PIPE_FAIL:-}" = 1 ]; then
		exit 1
	fi
	;;
set-option)
	;;
list-panes)
	printf '999999\n'
	;;
list-sessions)
	if [ -e "$SHIM_TEST_TMUX_SESSION" ]; then printf 'manager\n'; fi
	;;
kill-window)
	;;
has-session)
	test -e "$SHIM_TEST_TMUX_SESSION"
	;;
kill-session)
	rm -f "$SHIM_TEST_TMUX_SESSION"
	;;
*)
	exit 1
	;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, tc := range []struct {
		name        string
		skipCommand bool
		keepSession bool
		hardTimeout int
		wantExit    int
		wantReason  string
		harnessArgv []string
		capturePane bool
		omitLogs    bool
		pipeFail    bool
	}{
		{name: "exit 127", wantExit: 127},
		{name: "exit 127 after pipe failure", wantExit: 127, pipeFail: true},
		{name: "missing status", skipCommand: true, wantExit: -1},
		{name: "hard timeout", keepSession: true, hardTimeout: 1, wantExit: -1, wantReason: "hard_timeout"},
		{
			name: "status write failure", wantExit: -1, capturePane: true, omitLogs: true,
			harnessArgv: []string{"/bin/sh", "-c", "printf 'harness stderr retained\\n' >&2; exit 0"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			logsDir := filepath.Join(dir, "logs")
			if !tc.omitLogs {
				if err := os.MkdirAll(logsDir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("SHIM_TEST_TMUX_SESSION", filepath.Join(t.TempDir(), "session"))
			if tc.skipCommand {
				t.Setenv("SHIM_TEST_SKIP_COMMAND", "1")
			} else {
				t.Setenv("SHIM_TEST_SKIP_COMMAND", "")
			}
			if tc.keepSession {
				t.Setenv("SHIM_TEST_KEEP_SESSION", "1")
			} else {
				t.Setenv("SHIM_TEST_KEEP_SESSION", "")
			}
			outputPath := filepath.Join(t.TempDir(), "pane-output")
			if tc.capturePane {
				t.Setenv("SHIM_TEST_TMUX_OUTPUT", outputPath)
			} else {
				t.Setenv("SHIM_TEST_TMUX_OUTPUT", "")
			}
			if tc.pipeFail {
				t.Setenv("SHIM_TEST_PIPE_FAIL", "1")
			} else {
				t.Setenv("SHIM_TEST_PIPE_FAIL", "")
			}
			harnessArgv := tc.harnessArgv
			if harnessArgv == nil {
				harnessArgv = []string{"/bin/sh", "-c", "exit 127"}
			}

			err := runTmux(Options{
				IterationDir: dir, Agent: "manager", IterationID: "manager-exit-1",
				TmuxSession: "manager", HardTimeoutS: tc.hardTimeout,
				HarnessArgv: harnessArgv,
			}, 5*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "result.json"))
			if err != nil {
				t.Fatal(err)
			}
			var result IterationResult
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.ExitCode != tc.wantExit {
				t.Fatalf("exit_code = %d, want %d", result.ExitCode, tc.wantExit)
			}
			if result.TerminationReason != tc.wantReason {
				t.Fatalf("termination_reason = %q, want %q", result.TerminationReason, tc.wantReason)
			}
			statusPath := filepath.Join(logsDir, tmuxExitStatusFilename)
			if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
				t.Fatalf("transient tmux status file was not removed: %v", err)
			}
			if tc.skipCommand {
				logData, err := os.ReadFile(filepath.Join(logsDir, "shim.log"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(logData), "tmux exit status category=missing") {
					t.Fatalf("shim.log missing safe status category: %s", logData)
				}
				if strings.Contains(string(logData), statusPath) {
					t.Fatal("shim.log disclosed the private tmux status path")
				}
			}
			if tc.pipeFail {
				logData, err := os.ReadFile(filepath.Join(logsDir, "shim.log"))
				if err != nil {
					t.Fatal(err)
				}
				const warning = "WARN tmux pipe logs unavailable"
				if !strings.Contains(string(logData), warning) {
					t.Fatalf("shim.log missing fixed pipe warning: %s", logData)
				}
				if strings.Contains(string(logData), statusPath) {
					t.Fatal("shim.log disclosed the private tmux status path")
				}
			}
			if tc.capturePane {
				output, err := os.ReadFile(outputPath)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(output), "harness stderr retained") {
					t.Fatalf("wrapper suppressed harness stderr: %q", output)
				}
				if strings.Contains(string(output), statusPath) {
					t.Fatalf("wrapper leaked private status path into pane output: %q", output)
				}
			}
		})
	}
}
