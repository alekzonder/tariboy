package plugins

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"
)

// logTimeFormat is the human-readable date+time prepended to every captured
// plugin log line. UTC (matching the daemon's other timestamps, e.g. health
// checked_at) keeps it unambiguous across machines; millisecond precision
// preserves ordering of bursty log output.
const logTimeFormat = "2006-01-02 15:04:05.000"

// maxLineBuf caps how many bytes tsWriter holds for a single not-yet-terminated
// line. The post-handshake io.Copy that feeds tsWriter is NOT length-bounded, so
// a plugin streaming newline-less output (binary garbage on stdout, a \r-only
// progress line, a stuck stream) would otherwise grow t.buf without limit and
// leak daemon memory. When the held buffer reaches this cap with no newline seen,
// the partial content is force-emitted as one timestamped line and the buffer is
// reset, keeping memory bounded regardless of stream length. 64 KiB is far larger
// than any real log line yet small enough that a runaway stream cannot bloat the
// daemon.
const maxLineBuf = 64 << 10

// tsWriter prefixes every captured plugin log LINE with a wall-clock timestamp
// at the moment it is captured. It is the single choke point for a plugin's log
// (the supervisor tees the plugin's stdout tail + stderr through here into the
// per-plugin log file), so wrapping it here guarantees EVERY line carries a
// date+time — regardless of whether the plugin uses slog with a time field, a
// handler configured without one, or raw prints to stdout/stderr.
//
// Plugin output arrives in arbitrary chunks (io.Copy imposes no line
// boundaries), so writes are buffered until a newline and each complete line is
// emitted as "<ts> <line>\n". A trailing partial line (no newline, e.g. a plugin
// killed mid-write) is held until Flush, which the host calls once when the
// plugin life ends and the log file is about to close.
type tsWriter struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time
	buf []byte
}

// newTSWriter wraps w so every line written through it is timestamp-prefixed.
// now supplies the capture time (injected for deterministic tests); it falls
// back to time.Now when nil.
func newTSWriter(w io.Writer, now func() time.Time) *tsWriter {
	if now == nil {
		now = time.Now
	}
	return &tsWriter{w: w, now: now}
}

func (t *tsWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	for {
		if i := bytes.IndexByte(t.buf, '\n'); i >= 0 {
			line := t.buf[:i]
			if err := t.emit(line); err != nil {
				// Drop the emitted prefix from the buffer regardless so a failing
				// write does not re-emit the same line on the next call.
				t.buf = t.buf[i+1:]
				return len(p), err
			}
			t.buf = t.buf[i+1:]
			continue
		}
		// No newline in the buffer. Hold the partial line for the next Write /
		// Flush unless it has grown past the cap — then force-emit maxLineBuf
		// bytes as one timestamped line so newline-less output cannot grow
		// memory without bound. No delimiter byte is consumed here (unlike the
		// newline path), so no data is dropped.
		if len(t.buf) < maxLineBuf {
			break
		}
		line := t.buf[:maxLineBuf]
		if err := t.emit(line); err != nil {
			t.buf = t.buf[maxLineBuf:]
			return len(p), err
		}
		t.buf = t.buf[maxLineBuf:]
	}
	return len(p), nil
}

// Flush emits any buffered trailing partial line (one that never got a newline).
// It is idempotent: after flushing, the buffer is empty.
func (t *tsWriter) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.buf) == 0 {
		return nil
	}
	line := t.buf
	t.buf = nil
	return t.emit(line)
}

// emit writes one timestamped line. Caller holds t.mu.
func (t *tsWriter) emit(line []byte) error {
	_, err := fmt.Fprintf(t.w, "%s %s\n", t.now().UTC().Format(logTimeFormat), line)
	return err
}
