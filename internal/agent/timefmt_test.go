package agent

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Two requests in the same second keep time order: 11ns is later than 10ns
// although their RFC3339Nano text has different widths.
func TestRecordAIRequestOrdersSameSecondRequests(t *testing.T) {
	s := openStore(t)
	if err := s.Create(Agent{Name: "worker", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateIteration(Iteration{ID: "worker-1", Agent: "worker", Status: "running",
		StartedAt: "2026-09-10T02:00:00.000000000Z"}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 10, 2, 4, 0, 0, time.UTC)
	for _, at := range []time.Time{base.Add(10), base.Add(11), base.Add(10)} {
		if err := s.RecordAIRequest("worker", "worker-1", at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetIteration("worker", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if want := store.FormatTime(base.Add(11)); got.LastAIRequestAt != want {
		t.Fatalf("last AI request = %q, want %q", got.LastAIRequestAt, want)
	}
}

// A started-at bound with an offset names the same instant as the stored UTC
// value; the text comparison must not see "04:00+02:00" as later than 02:00Z.
func TestFindIterationsNormalizesOffsetBounds(t *testing.T) {
	s := tagStore(t)
	ids := func(f IterationFilter) []string {
		t.Helper()
		its, err := s.FindIterations("worker", f)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(its))
		for _, it := range its {
			out = append(out, it.ID)
		}
		return out
	}
	if got := ids(IterationFilter{StartedAfter: "2026-09-11T04:00:00+02:00"}); len(got) != 2 || got[0] != "b" {
		t.Fatalf("after with offset = %v, want [b c]", got)
	}
	if got := ids(IterationFilter{StartedBefore: "2026-09-11T04:00:00+02:00"}); len(got) != 2 || got[1] != "b" {
		t.Fatalf("before with offset = %v, want [a b]", got)
	}
}

// Timeout deadlines are written in the stored format.
func TestIterationTimeoutDeadlinesAreCanonical(t *testing.T) {
	s := openStore(t)
	if err := s.Create(Agent{Name: "worker", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateIteration(Iteration{ID: "worker-1", Agent: "worker", Status: "running",
		StartedAt: "2026-09-10T02:00:00.000000000Z"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC)
	if err := s.InitializeIterationTimeout("worker-1", 30, 90, now); err != nil {
		t.Fatal(err)
	}
	it, err := s.ExtendIterationTimeout("worker", "worker-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if *it.TimeoutDeadline != store.FormatTime(now.Add(60*time.Second)) ||
		*it.HardTimeoutDeadline != store.FormatTime(now.Add(120*time.Second)) {
		t.Fatalf("deadlines = %q / %q", *it.TimeoutDeadline, *it.HardTimeoutDeadline)
	}
}
