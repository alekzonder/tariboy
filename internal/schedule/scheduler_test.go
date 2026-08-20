package schedule

import (
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
)

type fakePublisher struct{ msgs []bus.Message }

func (f *fakePublisher) Publish(m bus.Message) (bus.Message, error) {
	m.ID = "m"
	f.msgs = append(f.msgs, m)
	return m, nil
}
func (f *fakePublisher) PublishWithGuard(m bus.Message, _ func(*sql.Tx, time.Time) error) (bus.Message, error) {
	return f.Publish(m)
}

func TestSchedulerFiresOneshotAndCron(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	// oneshot due at 10:01; cron every 5 min.
	one, _ := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: now.Add(time.Minute).Format(time.RFC3339),
		Channel: bus.InboxChannel("smoke"), MessageTemplate: `{"type":"wake","text":"rise"}`})
	cron, _ := st.Add(Schedule{Agent: "smoke", Kind: "cron", Spec: "*/5 * * * *",
		Channel: bus.InboxChannel("smoke"), MessageTemplate: `{"type":"tick"}`})

	pub := &fakePublisher{}
	sc := NewScheduler(st, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now }, time.After)

	// Fire everything due by 10:06: oneshot (10:01) and one cron tick (10:05).
	n, err := sc.fireDue(now.Add(6 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("fired %d, want 2", n)
	}
	if len(pub.msgs) != 2 {
		t.Fatalf("published %d messages", len(pub.msgs))
	}
	// The oneshot published its templated type/text with schedule attribution.
	var sawWake bool
	for _, m := range pub.msgs {
		if m.Type == "wake" && m.Text == "rise" && m.Source == "schedule" && m.ProducedByAgent == "smoke" {
			sawWake = true
		}
	}
	if !sawWake {
		t.Fatalf("templated oneshot message not published: %+v", pub.msgs)
	}
	// oneshot disabled; cron re-armed (next_fire_at advanced past 10:05).
	list, _ := st.List("smoke")
	byID := map[string]Schedule{}
	for _, s := range list {
		byID[s.ID] = s
	}
	if byID[one.ID].Enabled {
		t.Fatal("oneshot not disabled after firing")
	}
	if !byID[cron.ID].Enabled || byID[cron.ID].NextFireAt <= now.Add(5*time.Minute).UTC().Format(time.RFC3339) {
		t.Fatalf("cron not re-armed: %+v", byID[cron.ID])
	}
}

func TestSchedulerFailsClosedWithoutAtomicGuardPublisher(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	_, _ = st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: now.Add(time.Minute).Format(time.RFC3339), Channel: "chat:ops"})
	pub := &unguardedPublisher{}
	sc := NewScheduler(st, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now }, time.After)
	if n, err := sc.fireDue(now.Add(2 * time.Minute)); err != nil || n != 0 || len(pub.msgs) != 0 {
		t.Fatalf("fail-closed fire n=%d msgs=%v err=%v", n, pub.msgs, err)
	}
}

type unguardedPublisher struct{ msgs []bus.Message }

func (f *unguardedPublisher) Publish(m bus.Message) (bus.Message, error) {
	f.msgs = append(f.msgs, m)
	return m, nil
}
