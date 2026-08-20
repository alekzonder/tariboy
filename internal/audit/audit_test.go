package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	return func() time.Time { return time.Unix(0, 0).UTC() }
}

func TestRecordSeqMonotonicAndPersisted(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	l := Open(p, fixedClock())
	if got := l.Record("iteration_started", "system", "it-1", nil); got != 1 {
		t.Fatalf("seq = %d, want 1", got)
	}
	if got := l.Record("shim", "shim", "it-1", map[string]any{"line": "x"}); got != 2 {
		t.Fatalf("seq = %d, want 2", got)
	}
	// Reopen: seq continues from the last line.
	l2 := Open(p, fixedClock())
	if got := l2.Record("iteration_finished", "system", "it-1", nil); got != 3 {
		t.Fatalf("reopened seq = %d, want 3", got)
	}
	evs, err := ReadEvents(p, 0, 0)
	if err != nil || len(evs) != 3 {
		t.Fatalf("read = %d evs err=%v", len(evs), err)
	}
	if evs[1].Type != "shim" || evs[1].Source != "shim" || evs[1].Data["line"] != "x" {
		t.Fatalf("event[1] = %+v", evs[1])
	}
	if evs[0].IterationID != "it-1" || evs[0].TS == "" {
		t.Fatalf("event[0] = %+v", evs[0])
	}
}

func TestReadEventsLimitSinceAndBadLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	os.WriteFile(p, []byte(`{"seq":1,"type":"a"}`+"\n"+`garbage`+"\n"+`{"seq":2,"type":"b"}`+"\n"), 0o600)
	evs, err := ReadEvents(p, 0, 1)
	if err != nil || len(evs) != 1 || evs[0].Type != "b" {
		t.Fatalf("since: %+v err=%v", evs, err)
	}
	all, _ := ReadEvents(p, 1, 0)
	if len(all) != 1 || all[0].Type != "b" {
		t.Fatalf("limit: %+v", all)
	}
}

func TestReadEventsMissingFile(t *testing.T) {
	evs, err := ReadEvents(filepath.Join(t.TempDir(), "nope.jsonl"), 0, 0)
	if err != nil || len(evs) != 0 {
		t.Fatalf("missing file = %+v err=%v", evs, err)
	}
}

func TestReadByIterationAndBefore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	l := Open(p, fixedClock())
	l.Record("iteration_started", "system", "it-1", nil)                       // seq 1
	l.Record("harness_output", "harness", "it-1", map[string]any{"line": "a"}) // seq 2
	l.Record("iteration_finished", "system", "it-1", nil)                      // seq 3
	l.Record("iteration_started", "system", "it-2", nil)                       // seq 4
	l.Record("harness_output", "harness", "it-2", map[string]any{"line": "b"}) // seq 5

	it1, err := ReadByIteration(p, "it-1")
	if err != nil || len(it1) != 3 {
		t.Fatalf("it-1 = %d evs err=%v", len(it1), err)
	}
	if it1[0].Seq != 1 || it1[2].Seq != 3 {
		t.Fatalf("it-1 seqs = %d..%d", it1[0].Seq, it1[2].Seq)
	}
	if evs, _ := ReadByIteration(p, "it-2"); len(evs) != 2 || evs[0].Seq != 4 {
		t.Fatalf("it-2 = %+v", evs)
	}

	// before=4 → the two events with seq < 4, limited to the last 2.
	before, err := ReadBefore(p, 4, 2)
	if err != nil || len(before) != 2 {
		t.Fatalf("before = %d evs err=%v", len(before), err)
	}
	if before[0].Seq != 2 || before[1].Seq != 3 {
		t.Fatalf("before seqs = %d,%d want 2,3", before[0].Seq, before[1].Seq)
	}
	// before<=0 → the last `limit` overall.
	if tail, _ := ReadBefore(p, 0, 2); len(tail) != 2 || tail[1].Seq != 5 {
		t.Fatalf("before<=0 tail = %+v", tail)
	}
	// Missing file → empty, no error.
	if evs, err := ReadByIteration(filepath.Join(t.TempDir(), "none.jsonl"), "x"); err != nil || len(evs) != 0 {
		t.Fatalf("missing = %d err=%v", len(evs), err)
	}
}

func TestFollowBackfillThenAppend(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	l := Open(p, fixedClock())
	l.Record("a", "system", "it", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Follow(ctx, p, 0, 5*time.Millisecond)
	if e := <-ch; e.Type != "a" {
		t.Fatalf("backfill = %+v", e)
	}
	l.Record("b", "system", "it", nil)
	if e := <-ch; e.Type != "b" {
		t.Fatalf("append = %+v", e)
	}
}
