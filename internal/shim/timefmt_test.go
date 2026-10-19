package shim

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// The shim cannot import the store (it would link SQLite into the shim
// binary), so it keeps its own copy of the layout. The copy must not drift.
func TestShimTimeLayoutMatchesStore(t *testing.T) {
	if timeLayout != store.TimeLayout {
		t.Fatalf("shim timeLayout = %q, store.TimeLayout = %q", timeLayout, store.TimeLayout)
	}
	at := time.Date(2026, 9, 10, 2, 0, 0, 10, time.FixedZone("x", 3600))
	if got := formatTime(at); got != store.FormatTime(at) {
		t.Fatalf("formatTime = %q, want %q", got, store.FormatTime(at))
	}
}

// The daemon sends canonical deadlines, but an older daemon still on the other
// end of the socket sends RFC 3339 with an offset and no fraction.
func TestShimDeadlineAcceptsAnyRFC3339(t *testing.T) {
	want := time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC)
	for _, in := range []string{
		store.FormatTime(want),
		want.In(time.FixedZone("x", 3600)).Format(time.RFC3339),
		want.Format(time.RFC3339Nano),
	} {
		if got := initialDeadline(Options{HardDeadline: in}); !got.Equal(want) {
			t.Errorf("initialDeadline(%q) = %s", in, got)
		}
		if got := tmuxDeadline(Options{HardDeadline: in}); !got.Equal(want) {
			t.Errorf("tmuxDeadline(%q) = %s", in, got)
		}
		w := newDeadlineWatchdog(time.Time{}, func() {})
		if err := w.Set(in); err != nil {
			t.Errorf("Set(%q): %v", in, err)
		}
		w.Stop()
	}
}
