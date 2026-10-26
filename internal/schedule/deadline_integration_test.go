package schedule

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// TestRequestDeadlineTimeoutFires is the deadline path against real mail, the
// schedule store and the scheduler: a request with a deadline that gets no
// reply sends the requester a deadline notice naming the request once the
// deadline passes, and only once.
func TestRequestDeadlineTimeoutFires(t *testing.T) {
	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	now := base
	clock := func() time.Time { return now }
	s := storetest.Open(t)
	m := newMail(s, clock, "alice", "carol")
	sched := NewStore(s, clock)
	m.SetDeadlines(Deadlines{Store: sched})

	reqs, err := m.Send(mail.Letter{From: "agent:alice", To: "agent:carol", Subject: "help", Body: "help", NeedsReply: true, Deadline: "5m"})
	if err != nil {
		t.Fatal(err)
	}
	req := reqs[0]
	scheduler := NewScheduler(sched, m, slog.New(slog.NewTextHandler(io.Discard, nil)), clock, nil)
	if n, _ := scheduler.fireDue(base.Add(4 * time.Minute)); n != 0 {
		t.Fatalf("deadline fired early: %d", n)
	}
	now = base.Add(6 * time.Minute)
	if n, err := scheduler.fireDue(now); err != nil || n != 1 {
		t.Fatalf("deadline fire n=%d err=%v", n, err)
	}
	inbox, _, err := m.List(mail.BoxInbox, "agent:alice", 10, "")
	if err != nil || len(inbox) != 1 {
		t.Fatalf("alice inbox = %+v, %v", inbox, err)
	}
	got := inbox[0]
	if got.Kind != "deadline" || got.From != SenderSchedule || got.NeedsReply || got.Data["request_id"] != req.ID {
		t.Fatalf("deadline letter = %+v", got)
	}
	if n, _ := scheduler.fireDue(base.Add(time.Hour)); n != 0 {
		t.Fatalf("one-shot re-fired: %d", n)
	}
}

// TestRequestDeadlineReplyCancelsTimeout: a reply before the deadline cancels
// the armed schedule, so nothing fires later.
func TestRequestDeadlineReplyCancelsTimeout(t *testing.T) {
	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	now := base
	clock := func() time.Time { return now }
	s := storetest.Open(t)
	m := newMail(s, clock, "alice", "carol")
	sched := NewStore(s, clock)
	m.SetDeadlines(Deadlines{Store: sched})

	reqs, err := m.Send(mail.Letter{From: "agent:alice", To: "agent:carol", Body: "help", NeedsReply: true, Deadline: "5m"})
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := sched.List("alice"); len(list) != 1 {
		t.Fatalf("armed schedules = %+v", list)
	}
	if _, _, err := m.Reply(mail.ReplyRequest{As: "agent:carol", ID: reqs[0].ID, Text: "on it"}); err != nil {
		t.Fatal(err)
	}
	if list, _ := sched.List("alice"); len(list) != 0 {
		t.Fatalf("reply did not cancel the deadline: %+v", list)
	}
	if awaiting, _ := m.Awaiting("alice"); len(awaiting) != 0 {
		t.Fatalf("reply left the request awaiting: %+v", awaiting)
	}
	scheduler := NewScheduler(sched, m, slog.New(slog.NewTextHandler(io.Discard, nil)), clock, nil)
	now = base.Add(time.Hour)
	if n, _ := scheduler.fireDue(now); n != 0 {
		t.Fatalf("cancelled deadline still fired: %d", n)
	}
}
