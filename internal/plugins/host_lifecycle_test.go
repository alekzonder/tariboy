package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/testutil"
)

func TestStartAfterStopAllReturnsErrHostStopping(t *testing.T) {
	t.Parallel()
	h, _, _ := newHost(t, &nameRunner{})
	h.StopAll()
	if err := h.Start(sampleRecord("late")); !errors.Is(err, ErrHostStopping) {
		t.Fatalf("Start after StopAll = %v, want ErrHostStopping", err)
	}
	h.mu.Lock()
	_, reserved := h.running["late"]
	h.mu.Unlock()
	if reserved {
		t.Fatal("refused Start left a reservation in the running map")
	}
}

func TestInstallRollbackAfterStopAllDoesNotStartPlugin(t *testing.T) {
	t.Parallel()
	runner := &blockingUpgradeRunner{stopEntered: make(chan struct{}), releaseStop: make(chan struct{})}
	h, _, _ := newHost(t, runner)
	sock := h.SocketPath("single")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	status := 200
	healthServer(t, sock, &status)
	pluginSource := func(version string) string {
		dir := t.TempDir()
		manifest := fmt.Sprintf(`{"name":"single","version":%q,"protocol_version":1,"types":["tool"],"exec":"run.sh"}`, version)
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	if _, err := h.Install(pluginSource("1.0.0")); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, h, "single")
	timeout := make(chan time.Time, 1)
	timeout <- time.Now()
	h.cfg.DrainAfter = func(time.Duration) <-chan time.Time { return timeout }
	// The previous instance is stuck in its stop, so the drain times out and
	// Install leaves a rollback restart waiting for that instance to exit.
	if _, err := h.Install(pluginSource("2.0.0")); err == nil {
		t.Fatal("upgrade succeeded after drain timeout")
	}
	stopped := make(chan struct{})
	go func() { h.StopAll(); close(stopped) }()
	close(runner.releaseStop)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("StopAll did not return")
	}
	// A late restart would bump the count; give an untracked one a moment.
	testutil.WaitFor(200*time.Millisecond, func() bool { return runner.count() >= 2 })
	if got := runner.count(); got != 1 {
		t.Fatalf("rollback restarted the plugin after StopAll: starts=%d", got)
	}
}

func TestWaitGroupWaitsForRollbackRestart(t *testing.T) {
	t.Parallel()
	h, _, _ := newHost(t, nil)
	previous := &runningPlugin{done: make(chan struct{})}
	ran := make(chan struct{})
	h.afterDrain(previous, func() { close(ran) })
	close(previous.done)
	h.wg.Wait()
	select {
	case <-ran:
	default:
		t.Fatal("h.wg.Wait returned before the rollback restart ran")
	}
}

// sinkRecorder serves /health and /deliver; /deliver answers with the next
// status from statuses (200 once they run out) and reports every attempt.
func sinkRecorder(t *testing.T, sock string, statuses ...int) <-chan int {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	attempts := make(chan int, 16)
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/deliver", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		status := 200
		if len(statuses) > 0 {
			status, statuses = statuses[0], statuses[1:]
		}
		mu.Unlock()
		w.WriteHeader(status)
		attempts <- status
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return attempts
}

func nextAttempt(t *testing.T, attempts <-chan int) int {
	t.Helper()
	select {
	case status := <-attempts:
		return status
	case <-time.After(3 * time.Second):
		t.Fatal("sink received no delivery")
		return 0
	}
}

func startTestSink(t *testing.T, h *Host, ps *Store, timer <-chan time.Time) context.CancelFunc {
	t.Helper()
	h.cfg.SinkAfter = func(time.Duration) <-chan time.Time { return timer }
	rec := Record{Name: "echo", Version: "0.1.0", Types: []string{"mail-sink"}, Mail: MailCaps{Receive: true}}
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.startDrain(ctx, rec, h.SocketPath("echo"))
	t.Cleanup(func() { cancel(); h.wg.Wait() })
	return cancel
}

// TestSendToPluginWakesDrainerAndDeliverCloses: a letter to plugin:echo wakes
// its drainer without the fallback timer, and the delivered letter is closed
// as delivered.
func TestSendToPluginWakesDrainerAndDeliverCloses(t *testing.T) {
	t.Parallel()
	h, b, ps := newHost(t, nil)
	attempts := sinkRecorder(t, h.SocketPath("echo"))
	startTestSink(t, h, ps, make(chan time.Time)) // the fallback timer never fires
	sent := sendToPlugin(t, b, "echo", "hey")
	if status := nextAttempt(t, attempts); status != 200 {
		t.Fatalf("delivery status = %d", status)
	}
	if !testutil.WaitFor(2*time.Second, func() bool { return len(openLetters(t, b, "plugin:echo")) == 0 }) {
		t.Fatal("delivered letter was not closed")
	}
	got, _, err := b.Show("plugin:echo", sent.ID)
	if err != nil || got.CloseNote != mail.CloseNoteDelivered || got.Attempts != 1 {
		t.Fatalf("delivered letter = %+v, %v", got, err)
	}
}

func TestSinkFallbackTimerRetriesFailedDelivery(t *testing.T) {
	t.Parallel()
	h, b, ps := newHost(t, nil)
	attempts := sinkRecorder(t, h.SocketPath("echo"), 500)
	timer := make(chan time.Time)
	startTestSink(t, h, ps, timer)
	sendToPlugin(t, b, "echo", "hey")
	if status := nextAttempt(t, attempts); status != 500 {
		t.Fatalf("first delivery status = %d, want the failing 500", status)
	}
	select {
	case timer <- time.Now():
	case <-time.After(3 * time.Second):
		t.Fatal("drainer never waited on the fallback timer")
	}
	if status := nextAttempt(t, attempts); status != 200 {
		t.Fatalf("retry status = %d", status)
	}
	if !testutil.WaitFor(2*time.Second, func() bool { return len(openLetters(t, b, "plugin:echo")) == 0 }) {
		t.Fatal("retried letter was not closed")
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestOnStateLogsStatePersistFailure(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, nil)
	logs := &lockedBuffer{}
	h.cfg.Log = slog.New(slog.NewTextHandler(logs, nil))
	if _, err := ps.db.Exec(`DROP TABLE plugins`); err != nil {
		t.Fatal(err)
	}
	h.onState("echo", "error", map[string]any{"detail": "boom"})
	out := logs.String()
	if !strings.Contains(out, "plugin state persist") || !strings.Contains(out, "plugin=echo") || !strings.Contains(out, "state=error") {
		t.Fatalf("persist failure not logged with plugin and state: %q", out)
	}
}

func TestOnStateSurvivesHostRestart(t *testing.T) {
	t.Parallel()
	h, b, ps := newHost(t, nil)
	if err := ps.Upsert(sampleRecord("echo")); err != nil {
		t.Fatal(err)
	}
	h.onState("echo", "error", map[string]any{"detail": "boom"})
	restarted := NewHost(HostConfig{Store: ps, Mail: b, Tokens: NewTokenRegistry(nil), PluginsDir: h.cfg.PluginsDir, Clock: h.cfg.Clock})
	list, err := restarted.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["state"] != "error" {
		t.Fatalf("list after restart = %+v", list)
	}
	var health map[string]any
	if err := json.Unmarshal(list[0]["health"].(json.RawMessage), &health); err != nil || health["detail"] != "boom" {
		t.Fatalf("health after restart = %v, %v", health, err)
	}
}
