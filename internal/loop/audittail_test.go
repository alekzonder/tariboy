package loop

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type capturedEvent struct {
	typ, source, iterID string
	data                map[string]any
}

type captureRecorder struct {
	mu     sync.Mutex
	events []capturedEvent
	seq    int64
}

func (c *captureRecorder) Record(typ, source, iterID string, data map[string]any) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	c.events = append(c.events, capturedEvent{typ, source, iterID, data})
	return c.seq
}

func (c *captureRecorder) snapshot() []capturedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedEvent, len(c.events))
	copy(out, c.events)
	return out
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, rec *captureRecorder, n int) []capturedEvent {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if evs := rec.snapshot(); len(evs) >= n {
			return evs
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d events, got %d", n, len(rec.snapshot()))
	return nil
}

func TestTailerTeesShimAndHarnessLines(t *testing.T) {
	dir := t.TempDir()
	rec := &captureRecorder{}
	tl := StartTailer(rec, "it-1", dir, 5*time.Millisecond, false)

	appendLine(t, filepath.Join(dir, "shim.log"), "launch tmux session=manager")
	appendLine(t, filepath.Join(dir, "harness.stdout.log"), "hello from claude")
	appendLine(t, filepath.Join(dir, "harness.stderr.log"), "a warning")

	evs := waitFor(t, rec, 3)
	tl.Stop()

	byType := map[string]capturedEvent{}
	for _, e := range evs {
		key := e.typ
		if s, ok := e.data["stream"].(string); ok {
			key += ":" + s
		}
		byType[key] = e
	}
	if e, ok := byType["shim"]; !ok || e.source != "shim" || e.data["line"] != "launch tmux session=manager" {
		t.Fatalf("shim event wrong: %+v", byType["shim"])
	}
	if e, ok := byType["harness_output:stdout"]; !ok || e.source != "harness" || e.data["line"] != "hello from claude" {
		t.Fatalf("stdout event wrong: %+v", byType["harness_output:stdout"])
	}
	if e, ok := byType["harness_output:stderr"]; !ok || e.data["line"] != "a warning" {
		t.Fatalf("stderr event wrong: %+v", byType["harness_output:stderr"])
	}
}

func TestTailerOffsetNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	rec := &captureRecorder{}
	tl := StartTailer(rec, "it-1", dir, 5*time.Millisecond, false)
	p := filepath.Join(dir, "shim.log")
	appendLine(t, p, "line one")
	waitFor(t, rec, 1)
	appendLine(t, p, "line two")
	evs := waitFor(t, rec, 2)
	tl.Stop()
	if len(evs) != 2 {
		t.Fatalf("want exactly 2 events (no re-read), got %d: %+v", len(evs), evs)
	}
	if evs[0].data["line"] != "line one" || evs[1].data["line"] != "line two" {
		t.Fatalf("lines out of order/duplicated: %+v", evs)
	}
}

func TestTailerFinalDrainOnStop(t *testing.T) {
	dir := t.TempDir()
	rec := &captureRecorder{}
	// Long poll so the line is only picked up by the Stop() drain, not a tick.
	tl := StartTailer(rec, "it-1", dir, time.Hour, false)
	appendLine(t, filepath.Join(dir, "harness.stdout.log"), "trailing answer")
	tl.Stop()
	evs := rec.snapshot()
	if len(evs) != 1 || evs[0].data["line"] != "trailing answer" {
		t.Fatalf("final drain missed the trailing line: %+v", evs)
	}
}

func TestTailerStopIdempotent(t *testing.T) {
	tl := StartTailer(&captureRecorder{}, "it", t.TempDir(), 5*time.Millisecond, false)
	tl.Stop()
	tl.Stop() // must not panic on a double close
}

func TestTailerInteractiveSkipsHarnessOutput(t *testing.T) {
	dir := t.TempDir()
	rec := &captureRecorder{}
	tl := StartTailer(rec, "it-1", dir, 5*time.Millisecond, true) // interactive

	appendLine(t, filepath.Join(dir, "harness.stdout.log"), "tui redraw \x1b[2J noise")
	appendLine(t, filepath.Join(dir, "shim.log"), "launch tmux session=manager")

	evs := waitFor(t, rec, 1)
	tl.Stop()

	for _, e := range evs {
		if e.typ == "harness_output" {
			t.Fatalf("interactive tailer must not tee harness output: %+v", e)
		}
	}
	if len(evs) != 1 || evs[0].typ != "shim" {
		t.Fatalf("interactive tailer should tee only shim.log, got %+v", evs)
	}
}
