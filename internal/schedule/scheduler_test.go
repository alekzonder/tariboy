package schedule

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	basestore "github.com/alekzonder/tariboy/internal/store"
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

func TestSchedulerRetryPublishesOnce(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	db, err := basestore.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	b := bus.New(db, func() time.Time { return now })
	st := NewStore(db, func() time.Time { return now })
	channel := bus.InboxChannel("smoke")
	if _, err := b.Subscribe("smoke", channel, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: now.Add(time.Minute).Format(time.RFC3339), Channel: channel}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`CREATE TRIGGER fail_schedule_mark BEFORE UPDATE ON schedules BEGIN SELECT RAISE(ABORT, 'fail'); END`); err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(st, b, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now }, nil)
	_, _ = scheduler.fireDue(now.Add(2 * time.Minute))
	if _, err := db.DB.Exec(`DROP TRIGGER fail_schedule_mark`); err != nil {
		t.Fatal(err)
	}
	_, _ = scheduler.fireDue(now.Add(2 * time.Minute))
	items, err := b.Inbox("smoke", "pending", 10, "")
	if err != nil || len(items) != 1 {
		t.Fatalf("scheduled retry delivered %d messages: %+v err=%v", len(items), items, err)
	}
}

type cancelingPublisher struct {
	b      *bus.Bus
	cancel func()
}

func (p *cancelingPublisher) Publish(m bus.Message) (bus.Message, error) { return p.b.Publish(m) }
func (p *cancelingPublisher) PublishWithGuard(m bus.Message, guard func(*sql.Tx, time.Time) error) (bus.Message, error) {
	p.cancel()
	return p.b.PublishWithGuard(m, guard)
}

func TestSchedulerDoesNotPublishStaleCancelledSchedule(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	db, err := basestore.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	b := bus.New(db, func() time.Time { return now })
	st := NewStore(db, func() time.Time { return now })
	channel := bus.InboxChannel("smoke")
	if _, err := b.Subscribe("smoke", channel, nil, nil); err != nil {
		t.Fatal(err)
	}
	sch, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: now.Add(time.Minute).Format(time.RFC3339), Channel: channel})
	if err != nil {
		t.Fatal(err)
	}
	pub := &cancelingPublisher{b: b, cancel: func() { _ = st.Cancel("smoke", sch.ID) }}
	scheduler := NewScheduler(st, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now }, nil)
	if n, err := scheduler.fireDue(now.Add(2 * time.Minute)); err != nil || n != 0 {
		t.Fatalf("cancelled schedule fired: n=%d err=%v", n, err)
	}
	items, err := b.Inbox("smoke", "pending", 10, "")
	if err != nil || len(items) != 0 {
		t.Fatalf("cancelled schedule delivered: %+v err=%v", items, err)
	}
}
