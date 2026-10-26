package mail_test

import (
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/schedule"
)

func TestDeadlineSchedulesAndReplyCancels(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sched := schedule.NewStore(f.st, func() time.Time { return *f.now })
	f.m.SetDeadlines(schedule.Deadlines{Store: sched})

	req := f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "help", NeedsReply: true, Deadline: "5m"})
	list, err := sched.List("alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("schedules = %d, %v; want one deadline", len(list), err)
	}
	if list[0].Recipient != "agent:alice" || list[0].Kind != "oneshot" || !strings.Contains(list[0].MessageTemplate, req.ID) {
		t.Fatalf("deadline schedule = %+v", list[0])
	}
	var keyed int
	if err := f.st.DB.QueryRow(`SELECT COUNT(*) FROM schedules WHERE correlation_id = ?`, req.ID).Scan(&keyed); err != nil || keyed != 1 {
		t.Fatalf("schedules keyed on the letter id = %d, %v", keyed, err)
	}
	// A notice and a request without a deadline arm nothing.
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "fyi"})
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "?", NeedsReply: true})
	if list, _ := sched.List("alice"); len(list) != 1 {
		t.Fatalf("schedules = %d; want 1", len(list))
	}

	if _, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:bob", ID: req.ID, Text: "done"}); err != nil {
		t.Fatal(err)
	}
	if list, _ := sched.List("alice"); len(list) != 0 {
		t.Fatalf("the reply left %d deadline schedules", len(list))
	}
}

func TestRequeueRespectsCapAndDeadline(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sched := schedule.NewStore(f.st, func() time.Time { return *f.now })
	f.m.SetDeadlines(schedule.Deadlines{Store: sched})
	f.dir.agents["bob"] = 1
	first := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:bob", Body: "fills the inbox"})
	// Over the cap: stored dead with queue_limit, and no deadline is armed.
	req := f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "help", NeedsReply: true, Deadline: "1h"})
	if req.DeadReason != mail.DeadQueueLimit {
		t.Fatalf("request = %+v; want dead queue_limit", req)
	}
	_, err := f.m.Requeue("agent:bob", req.ID)
	wantCode(t, err, "queue_limit")
	if mail.ErrorStatus(err) != 409 || !strings.Contains(err.Error(), "close letters first") {
		t.Fatalf("requeue over the cap = %v (status %d)", err, mail.ErrorStatus(err))
	}
	if got := f.show(t, "agent:bob", req.ID); got.DeadAt == "" {
		t.Fatalf("a refused requeue reopened the letter: %+v", got)
	}
	if _, err := f.m.Close("agent:bob", first.ID, ""); err != nil {
		t.Fatal(err)
	}
	// Under the cap with the deadline ahead: reopened and the deadline armed.
	got, err := f.m.Requeue("agent:bob", req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Deadline != req.Deadline {
		t.Fatalf("requeued deadline = %q; want %q", got.Deadline, req.Deadline)
	}
	if list, _ := sched.List("alice"); len(list) != 1 || !strings.Contains(list[0].MessageTemplate, req.ID) {
		t.Fatalf("schedules after requeue = %+v; want the deadline armed once", list)
	}
	// Dead again by attempts, then requeued after the deadline passed: the
	// deadline is cleared and nothing stays armed.
	for range 6 {
		if _, err := f.m.RenderBatch("bob", 10); err != nil {
			t.Fatal(err)
		}
	}
	*f.now = f.now.Add(2 * time.Hour)
	got, err = f.m.Requeue("agent:bob", req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Deadline != "" || f.show(t, "agent:bob", req.ID).Deadline != "" {
		t.Fatalf("requeued past deadline = %q; want it cleared", got.Deadline)
	}
	if list, _ := sched.List("alice"); len(list) != 0 {
		t.Fatalf("schedules after a requeue past the deadline = %+v; want none", list)
	}
}
