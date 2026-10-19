package loop

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/store"
)

// Script run times keep the full clock precision in the stored format; the
// supervisor used to write finished_at with whole seconds.
func TestScriptSupervisorWritesCanonicalRunTimes(t *testing.T) {
	m, as, _, raw := newManager(t, &fakeRunner{})
	at := time.Date(2026, 9, 10, 2, 0, 0, 123456789, time.UTC)
	m.cfg.Clock = func() time.Time { return at }
	st := script.NewStore(raw, m.cfg.Clock)
	m.cfg.Scripts, m.cfg.Bus = st, bus.New(raw, m.cfg.Clock)
	if err := as.Create(agent.Agent{Name: "worker", ImageRef: "basic:latest", Cwd: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	startScriptTestSupervisor(t, m)
	_, r, err := m.RunOnce("worker", script.CreateOnce{Name: "ok", Description: "test", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	r = awaitScriptRun(t, st, "worker", r.ID, func(r script.Run) bool { return r.Status == script.RunSucceeded })
	want := store.FormatTime(at)
	if r.StartedAt != want || r.FinishedAt != want {
		t.Fatalf("started_at=%q finished_at=%q, want %q", r.StartedAt, r.FinishedAt, want)
	}
}

// Live events carry the stored format, converted to UTC.
func TestManagerEventTimesAreCanonical(t *testing.T) {
	m, _, _, _ := newManager(t, &fakeRunner{})
	at := time.Date(2026, 9, 10, 4, 0, 0, 10, time.FixedZone("x", 7200))
	m.cfg.Clock = func() time.Time { return at }
	var got []events.Event
	m.cfg.Emit = func(e events.Event) { got = append(got, e) }
	m.emitState("worker")
	if len(got) != 1 || got[0].Time != store.FormatTime(at) {
		t.Fatalf("events = %+v, want time %q", got, store.FormatTime(at))
	}
}

// messageAge reads the canonical bus ts.
func TestMessageAgeReadsCanonicalTS(t *testing.T) {
	now := time.Date(2026, 9, 10, 2, 1, 30, 0, time.UTC)
	if d, ok := messageAge(store.FormatTime(now.Add(-90*time.Second)), now); !ok || d != 90*time.Second {
		t.Fatalf("age = %s ok=%v", d, ok)
	}
}
