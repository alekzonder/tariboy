package plugins

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// fixedClock returns the same instant every call so timestamp output is
// deterministic. 2026-07-13T09:08:07.006Z formats to "2026-07-13 09:08:07.006".
func fixedClock() func() time.Time {
	t := time.Date(2026, 7, 13, 9, 8, 7, 6_000_000, time.UTC)
	return func() time.Time { return t }
}

const wantTS = "2026-07-13 09:08:07.006"

func TestTSWriter_PrefixesEachLine(t *testing.T) {
	var buf bytes.Buffer
	w := newTSWriter(&buf, fixedClock())
	if _, err := w.Write([]byte("hello\nworld\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := buf.String()
	want := wantTS + " hello\n" + wantTS + " world\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A single logical line split across multiple Writes (io.Copy chunk boundaries)
// must be timestamped ONCE, when its newline finally arrives — not per chunk.
func TestTSWriter_BuffersPartialLineAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	w := newTSWriter(&buf, fixedClock())
	w.Write([]byte("abc"))
	if buf.Len() != 0 {
		t.Fatalf("partial line emitted early: %q", buf.String())
	}
	w.Write([]byte("def\n"))
	if got, want := buf.String(), wantTS+" abcdef\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A trailing partial line (no newline, e.g. plugin killed mid-write) is held
// until Flush, then emitted with a timestamp and a synthesized newline.
func TestTSWriter_FlushEmitsTrailingPartialLine(t *testing.T) {
	var buf bytes.Buffer
	w := newTSWriter(&buf, fixedClock())
	w.Write([]byte("tail-no-newline"))
	if buf.Len() != 0 {
		t.Fatalf("held line emitted before flush: %q", buf.String())
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got, want := buf.String(), wantTS+" tail-no-newline\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Flush is idempotent: a second flush writes nothing more.
	if err := w.Flush(); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if strings.Count(buf.String(), "tail-no-newline") != 1 {
		t.Fatalf("flush not idempotent: %q", buf.String())
	}
}

// A plugin that streams output with no newline must not grow tsWriter's buffer
// without bound. Past the cap, the held partial content is force-emitted as one
// timestamped line and the buffer stays bounded regardless of stream length.
func TestTSWriter_ForceEmitsNewlineLessOutputPastCap(t *testing.T) {
	var out bytes.Buffer
	w := newTSWriter(&out, fixedClock())

	// Feed well past the cap in small chunks (as io.Copy would), with no newline
	// ever. After every write the held buffer must stay strictly under the cap.
	const chunk = 8 << 10
	for wrote := 0; wrote < 5*maxLineBuf; wrote += chunk {
		if _, err := w.Write(bytes.Repeat([]byte("x"), chunk)); err != nil {
			t.Fatalf("write: %v", err)
		}
		if len(w.buf) >= maxLineBuf {
			t.Fatalf("buffer not bounded: holding %d bytes (cap %d)", len(w.buf), maxLineBuf)
		}
	}

	// Something was force-emitted (not all held in memory), and the first
	// emitted line is timestamped just like any normal line.
	if out.Len() == 0 {
		t.Fatal("newline-less stream past the cap was never force-emitted")
	}
	if !strings.HasPrefix(out.String(), wantTS+" ") {
		t.Fatalf("force-emitted output not timestamped: starts with %q", firstN(out.String(), 40))
	}
	// Every force-emitted line is exactly maxLineBuf payload bytes: "<ts> " +
	// maxLineBuf 'x' + "\n". Verify no line's payload exceeds the cap.
	for line := range strings.SplitSeq(strings.TrimRight(out.String(), "\n"), "\n") {
		payload := strings.TrimPrefix(line, wantTS+" ")
		if len(payload) > maxLineBuf {
			t.Fatalf("emitted line payload %d exceeds cap %d", len(payload), maxLineBuf)
		}
	}
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// Every captured line gets a timestamp regardless of its source content — a raw
// print with no time field is timestamped exactly like an slog line would be.
func TestTSWriter_TimestampsRawLines(t *testing.T) {
	var buf bytes.Buffer
	w := newTSWriter(&buf, fixedClock())
	w.Write([]byte("plain stdout print, no time= field\n"))
	if got, want := buf.String(), wantTS+" plain stdout print, no time= field\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
