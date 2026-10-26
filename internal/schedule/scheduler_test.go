package schedule

import (
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	basestore "github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

type fakeSender struct{ letters []mail.Letter }

func (f *fakeSender) SendGuarded(l mail.Letter, _ func(*sql.Tx) error) ([]mail.Letter, error) {
	l.ID = "m"
	f.letters = append(f.letters, l)
	return []mail.Letter{l}, nil
}

// agentsDirectory knows a fixed set of agents and nothing else.
type agentsDirectory map[string]bool

func (d agentsDirectory) AgentExists(name string) (bool, error)     { return d[name], nil }
func (agentsDirectory) InboxCap(string) (int, error)                { return 0, nil }
func (agentsDirectory) CustomerLogin() string                       { return "customer" }
func (agentsDirectory) PluginInstalled(string) (bool, error)        { return false, nil }
func (agentsDirectory) QueueExists(string) (bool, error)            { return false, nil }
func (agentsDirectory) GroupMembers(string) ([]string, bool, error) { return nil, false, nil }

func newMail(st *basestore.Store, clock func() time.Time, agents ...string) *mail.Mail {
	dir := agentsDirectory{}
	for _, a := range agents {
		dir[a] = true
	}
	return mail.New(st, clock, dir)
}

func TestSchedulerFiresOneshotAndCron(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t, now)
	// oneshot due at 10:01; cron every 5 min.
	one, _ := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: now.Add(time.Minute).Format(time.RFC3339),
		Recipient: "agent:smoke", MessageTemplate: `{"type":"wake","text":"rise"}`})
	cron, _ := st.Add(Schedule{Agent: "smoke", Kind: "cron", Spec: "*/5 * * * *",
		Recipient: "agent:smoke", MessageTemplate: `{"kind":"tick","notice":true}`})

	pub := &fakeSender{}
	sc := NewScheduler(st, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now }, time.After)

	// Fire everything due by 10:06: oneshot (10:01) and one cron tick (10:05).
	n, err := sc.fireDue(now.Add(6 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("fired %d, want 2", n)
	}
	if len(pub.letters) != 2 {
		t.Fatalf("sent %d letters", len(pub.letters))
	}
	// The oneshot sent its templated kind and text from system:schedule as a
	// request; the cron template asked for a notice.
	var sawWake, sawTick bool
	for _, l := range pub.letters {
		if l.Kind == "wake" && l.Body == "rise" && l.From == SenderSchedule && l.To == "agent:smoke" && l.NeedsReply {
			sawWake = true
		}
		if l.Kind == "tick" && !l.NeedsReply && l.Data["schedule_id"] == cron.ID {
			sawTick = true
		}
	}
	if !sawWake || !sawTick {
		t.Fatalf("templated letters not sent: %+v", pub.letters)
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
	if !byID[cron.ID].Enabled || byID[cron.ID].NextFireAt <= basestore.FormatTime(now.Add(5*time.Minute)) {
		t.Fatalf("cron not re-armed: %+v", byID[cron.ID])
	}
}

func TestSchedulerRetrySendsOnce(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	db := storetest.Open(t)
	m := newMail(db, func() time.Time { return now }, "smoke")
	st := NewStore(db, func() time.Time { return now })
	if _, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: now.Add(time.Minute).Format(time.RFC3339), Recipient: "agent:smoke"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`CREATE TRIGGER fail_schedule_mark BEFORE UPDATE ON schedules BEGIN SELECT RAISE(ABORT, 'fail'); END`); err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(st, m, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now }, nil)
	_, _ = scheduler.fireDue(now.Add(2 * time.Minute))
	if _, err := db.DB.Exec(`DROP TRIGGER fail_schedule_mark`); err != nil {
		t.Fatal(err)
	}
	_, _ = scheduler.fireDue(now.Add(2 * time.Minute))
	if n, err := m.OpenCount("smoke"); err != nil || n != 1 {
		t.Fatalf("scheduled retry left %d open letters, err=%v; want 1", n, err)
	}
}

// cancelingSender cancels the schedule between the tick's read and its send.
type cancelingSender struct {
	m      *mail.Mail
	cancel func()
}

func (p *cancelingSender) SendGuarded(l mail.Letter, guard func(*sql.Tx) error) ([]mail.Letter, error) {
	p.cancel()
	return p.m.SendGuarded(l, guard)
}

func TestSchedulerDoesNotSendStaleCancelledSchedule(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	db := storetest.Open(t)
	m := newMail(db, func() time.Time { return now }, "smoke")
	st := NewStore(db, func() time.Time { return now })
	sch, err := st.Add(Schedule{Agent: "smoke", Kind: "oneshot", Spec: now.Add(time.Minute).Format(time.RFC3339), Recipient: "agent:smoke"})
	if err != nil {
		t.Fatal(err)
	}
	pub := &cancelingSender{m: m, cancel: func() { _ = st.Cancel("smoke", sch.ID) }}
	scheduler := NewScheduler(st, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now }, nil)
	if n, err := scheduler.fireDue(now.Add(2 * time.Minute)); err != nil || n != 0 {
		t.Fatalf("cancelled schedule fired: n=%d err=%v", n, err)
	}
	if n, err := m.OpenCount("smoke"); err != nil || n != 0 {
		t.Fatalf("cancelled schedule sent %d letters, err=%v", n, err)
	}
}

func TestRecipientForChannelFollowsTheMigrationRule(t *testing.T) {
	for channel, want := range map[string]string{
		"":                     "agent:owner",
		"chat:service:bob":     "agent:bob",
		"agent:bob:inbox":      "agent:bob",
		"chat:ops":             "agent:owner",
		"agent:bob:inbox:more": "agent:owner",
		"chat:service:a:b":     "agent:owner",
	} {
		if got := RecipientForChannel("owner", channel); got != want {
			t.Errorf("RecipientForChannel(%q) = %q; want %q", channel, got, want)
		}
	}
}
