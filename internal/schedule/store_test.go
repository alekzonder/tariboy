package schedule

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func newStore(t *testing.T, now time.Time) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s, func() time.Time { return now })
}

func TestScheduleOneshotFires(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	fireAt := now.Add(time.Minute).Format(time.RFC3339)
	sch, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: fireAt,
		Channel: "agent:smoke:inbox", MessageTemplate: `{"type":"wake"}`})
	if err != nil {
		t.Fatal(err)
	}
	if sch.ID == "" || sch.NextFireAt != fireAt || !sch.Enabled {
		t.Fatalf("added = %+v", sch)
	}
	// Not due yet.
	if due, _ := st.DueBefore(now); len(due) != 0 {
		t.Fatalf("premature due: %+v", due)
	}
	// Due after the fire time.
	due, err := st.DueBefore(now.Add(2 * time.Minute))
	if err != nil || len(due) != 1 {
		t.Fatalf("due = %+v err=%v", due, err)
	}
	// Firing a oneshot disables it.
	sch = due[0]
	sch.Enabled = false
	if err := st.MarkFired(sch); err != nil {
		t.Fatal(err)
	}
	if due, _ := st.DueBefore(now.Add(time.Hour)); len(due) != 0 {
		t.Fatalf("disabled oneshot still due: %+v", due)
	}
}

// TestScheduleAddSameInstantDistinctIDs uses a frozen clock (returns the SAME
// instant on every call) to prove Add cannot rely on wall-clock resolution for
// id uniqueness: two Add calls for the same agent at the same clock instant
// must still succeed with distinct ids, and both rows must persist.
func TestScheduleAddSameInstantDistinctIDs(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	fireAt := now.Add(time.Minute).Format(time.RFC3339)

	sch1, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: fireAt,
		Channel: "agent:smoke:inbox", MessageTemplate: `{"type":"wake"}`})
	if err != nil {
		t.Fatal(err)
	}
	sch2, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: fireAt,
		Channel: "agent:smoke:inbox", MessageTemplate: `{"type":"wake"}`})
	if err != nil {
		t.Fatal(err)
	}
	if sch1.ID == sch2.ID {
		t.Fatalf("same-instant Add calls collided on id: %s", sch1.ID)
	}
	list, err := st.List("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected both rows to persist, got %+v", list)
	}
}

func TestScheduleCronRecurs(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	sch, err := st.Add(Schedule{Agent: "smoke", Kind: "cron", Spec: "*/5 * * * *",
		Channel: "agent:smoke:inbox", MessageTemplate: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if sch.NextFireAt != "2026-07-06T10:05:00Z" {
		t.Fatalf("initial next_fire_at = %q", sch.NextFireAt)
	}
	// List and Cancel.
	list, _ := st.List("smoke")
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}
	// A different agent cannot cancel this agent's schedule.
	if err := st.Cancel("other", sch.ID); err != ErrNotFound {
		t.Fatalf("cross-agent cancel = %v, want ErrNotFound", err)
	}
	if err := st.Cancel("smoke", sch.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = st.List("smoke"); len(list) != 0 {
		t.Fatalf("cancel failed: %+v", list)
	}
}

func TestCancelByCorrelation(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	fireAt := now.Add(time.Minute).Format(time.RFC3339)
	// Two deadline one-shots for different correlations, plus a plain schedule.
	if _, err := st.Add(Schedule{Agent: "alice", Kind: "oneshot", Spec: fireAt,
		Channel: "agent:alice:inbox", MessageTemplate: `{"type":"timeout"}`, CorrelationID: "corr-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Add(Schedule{Agent: "alice", Kind: "oneshot", Spec: fireAt,
		Channel: "agent:alice:inbox", MessageTemplate: `{"type":"timeout"}`, CorrelationID: "corr-2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Add(Schedule{Agent: "alice", Kind: "oneshot", Spec: fireAt,
		Channel: "agent:alice:inbox", MessageTemplate: "{}"}); err != nil {
		t.Fatal(err)
	}
	// Cancelling one correlation leaves the other deadline and the plain schedule.
	if err := st.CancelByCorrelation("corr-1"); err != nil {
		t.Fatal(err)
	}
	list, _ := st.List("alice")
	if len(list) != 2 {
		t.Fatalf("after cancel corr-1, list = %+v", list)
	}
	// Idempotent: cancelling an unknown / already-cleared correlation is a no-op.
	if err := st.CancelByCorrelation("corr-1"); err != nil {
		t.Fatalf("re-cancel should be nil: %v", err)
	}
	// Empty correlation matches nothing (must not wipe the plain / other rows).
	if err := st.CancelByCorrelation(""); err != nil {
		t.Fatal(err)
	}
	if list, _ = st.List("alice"); len(list) != 2 {
		t.Fatalf("empty-correlation cancel touched rows: %+v", list)
	}
	// Cancelling the remaining correlation drops only it.
	if err := st.CancelByCorrelation("corr-2"); err != nil {
		t.Fatal(err)
	}
	if list, _ = st.List("alice"); len(list) != 1 {
		t.Fatalf("after cancel corr-2, list = %+v", list)
	}
}
