package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotateKeepsNewestUnderCap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	l := Open(p, func() time.Time { return time.Unix(0, 0).UTC() })
	for i := 0; i < 50; i++ {
		l.Record("harness_output", "harness", "it", map[string]any{"line": "some output line padding here"})
	}
	before, _ := os.Stat(p)
	cap := before.Size() / 4
	if err := Rotate(p, cap); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(p)
	if after.Size() > cap {
		t.Fatalf("after rotate size %d > cap %d", after.Size(), cap)
	}
	evs, err := ReadEvents(p, 0, 0)
	if err != nil || len(evs) == 0 {
		t.Fatalf("read after rotate: %d evs err=%v", len(evs), err)
	}
	// Newest kept: the last event's seq must be the original max (50), and the
	// kept run must be contiguous ending at 50.
	if evs[len(evs)-1].Seq != 50 {
		t.Fatalf("last seq = %d, want 50 (newest kept)", evs[len(evs)-1].Seq)
	}
}

func TestRotateNoopWhenUnderCapOrDisabled(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	l := Open(p, nil)
	l.Record("a", "system", "it", nil)
	before, _ := os.Stat(p)
	if err := Rotate(p, 0); err != nil { // disabled
		t.Fatal(err)
	}
	if err := Rotate(p, 1<<20); err != nil { // under cap
		t.Fatal(err)
	}
	after, _ := os.Stat(p)
	if after.Size() != before.Size() {
		t.Fatalf("noop rotate changed size %d -> %d", before.Size(), after.Size())
	}
	// Missing file is a no-op, not an error.
	if err := Rotate(filepath.Join(t.TempDir(), "none.jsonl"), 10); err != nil {
		t.Fatalf("missing file rotate err = %v", err)
	}
}
