package schedule

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// A one-shot keeps its exact instant and becomes due by time order within
// the same second: not at 10ns, but at 11ns.
func TestScheduleDueComparesSameSecondByTime(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	fire := now.Add(time.Minute + 11)
	sch, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot",
		Spec: fire.In(time.FixedZone("x", 3600)).Format(time.RFC3339Nano), Channel: "agent:smoke:inbox"})
	if err != nil {
		t.Fatal(err)
	}
	if sch.NextFireAt != store.FormatTime(fire) || sch.CreatedAt != store.FormatTime(now) {
		t.Fatalf("added = %+v", sch)
	}
	if due, err := st.DueBefore(now.Add(time.Minute + 10)); err != nil || len(due) != 0 {
		t.Fatalf("due at 10ns = %+v err=%v", due, err)
	}
	if due, err := st.DueBefore(fire); err != nil || len(due) != 1 {
		t.Fatalf("due at 11ns = %+v err=%v", due, err)
	}
}

func TestNextAfterIsCanonical(t *testing.T) {
	next, ok := NextAfter("*/5 * * * *", time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC))
	if !ok || next != "2026-07-06T10:05:00.000000000Z" {
		t.Fatalf("NextAfter = %q ok=%v", next, ok)
	}
}
