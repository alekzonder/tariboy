package mail_test

import (
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/mail"
)

func TestReplyClosesOriginalAndIsNoticeByDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	orig := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Subject: "Question on TARI-42", Body: "?", NeedsReply: true})
	f.rec.wakes = nil
	reply, closed, err := f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: orig.ID, Text: "use S3"})
	if err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("reply did not report closing the original")
	}
	if reply.From != "agent:alice" || reply.To != "user:customer" || reply.Subject != "Re: Question on TARI-42" ||
		reply.InReplyTo != orig.ID || reply.NeedsReply || reply.Body != "use S3" || reply.Kind != "message" {
		t.Fatalf("reply = %+v", reply)
	}
	got, thread, err := f.m.Show("user:customer", orig.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AnsweredAt == "" || got.AnswerID != reply.ID {
		t.Fatalf("original after reply = %+v", got)
	}
	if thread == nil || thread.ID != reply.ID {
		t.Fatalf("show reply = %+v; want %s", thread, reply.ID)
	}
	if f.rec.auditKinds("mail_answered") != 1 {
		t.Fatalf("audits = %v", f.rec.audits)
	}
	// A reply to a reply keeps a single Re: prefix.
	back := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Subject: "Re: x", Body: "more?", NeedsReply: true})
	f.rec.wakes = nil
	again, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: back.ID, Text: "ok"})
	if err != nil || again.Subject != "Re: x" {
		t.Fatalf("subject = %q, %v; want Re: x", again.Subject, err)
	}
	// A reply to an already closed letter is recorded and reopens nothing.
	late, closed, err := f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: orig.ID, Text: "also"})
	if err != nil || closed {
		t.Fatalf("late reply closed=%v err=%v", closed, err)
	}
	got = f.show(t, "agent:alice", orig.ID)
	if got.AnswerID != reply.ID || got.AnsweredAt == "" {
		t.Fatalf("late reply changed the original: %+v", got)
	}
	if late.InReplyTo != orig.ID {
		t.Fatalf("late reply = %+v", late)
	}
	// A reply to the agent wakes it; the agent's notice reply to the customer wakes nobody.
	if got := f.rec.woken(); len(got) != 0 {
		t.Fatalf("replies to the customer woke %v", got)
	}
}

func TestReplyOnBehalfMarksOperator(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	orig := f.sendOne(t, mail.Letter{From: "agent:bob", To: "agent:alice", Body: "?", NeedsReply: true})
	reply, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: orig.ID, Text: "done", OnBehalf: true})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Data["on_behalf"] != "operator" {
		t.Fatalf("reply data = %v", reply.Data)
	}
	if got := f.rec.woken(); len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("woken = %v; want alice then bob", got)
	}
}

func TestReplyAskIsRequest(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	orig := f.sendOne(t, mail.Letter{From: "agent:bob", To: "agent:alice", Body: "?", NeedsReply: true})
	reply, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: orig.ID, Text: "which bucket?", Ask: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reply.NeedsReply {
		t.Fatalf("ask reply = %+v; want a request", reply)
	}
	if n, _ := f.m.OpenCount("bob"); n != 1 {
		t.Fatalf("bob open = %d; want the follow-up question", n)
	}
}

func TestReplyByNonRecipientForbidden(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	orig := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "?", NeedsReply: true})
	for _, as := range []string{"agent:bob", "user:customer"} {
		_, _, err := f.m.Reply(mail.ReplyRequest{As: as, ID: orig.ID, Text: "x"})
		wantCode(t, err, "forbidden")
	}
	_, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: "m-none", Text: "x"})
	wantCode(t, err, "not_found")
	if got := f.show(t, "agent:alice", orig.ID); got.AnsweredAt != "" {
		t.Fatalf("a refused reply closed the letter: %+v", got)
	}
}

func TestReplyToTaskQuestionRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Kind: "task.question", Body: "which db?",
		NeedsReply: true, Data: map[string]any{"task_key": "OPS-7"}})
	_, _, err := f.m.Reply(mail.ReplyRequest{As: "user:customer", ID: q.ID, Text: "postgres"})
	wantCode(t, err, "answer_on_task")
	if !strings.Contains(err.Error(), "ttasks comment OPS-7") {
		t.Fatalf("error %q does not name the ttasks command", err)
	}
	if got := f.show(t, "user:customer", q.ID); got.AnsweredAt != "" {
		t.Fatalf("question closed by a refused reply: %+v", got)
	}
}

func TestCloseArchivesWithNote(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	l := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Body: "fyi?", NeedsReply: true})
	closed, err := f.m.Close("user:customer", l.ID, "handled offline")
	if err != nil {
		t.Fatal(err)
	}
	if closed.AnsweredAt == "" || closed.AnswerID != "" || closed.CloseNote != "handled offline" {
		t.Fatalf("closed = %+v", closed)
	}
	_, err = f.m.Close("agent:bob", l.ID, "")
	wantCode(t, err, "forbidden")
	if f.rec.auditKinds("mail_closed") != 1 {
		t.Fatalf("audits = %v", f.rec.audits)
	}
}

func TestMarkReadClosesNoticesOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	notice := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Body: "fyi"})
	request := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Body: "?", NeedsReply: true})
	closedNotices, read, err := f.m.MarkRead("user:customer", []string{notice.ID, request.ID})
	if err != nil || closedNotices != 1 || read != 2 {
		t.Fatalf("mark read = %d %d %v; want 1 2", closedNotices, read, err)
	}
	if got := f.show(t, "user:customer", notice.ID); got.AnsweredAt == "" || got.CloseNote != "read" || got.ReadAt == "" {
		t.Fatalf("notice = %+v", got)
	}
	if got := f.show(t, "user:customer", request.ID); got.AnsweredAt != "" || got.ReadAt == "" {
		t.Fatalf("request = %+v", got)
	}
	_, _, err = f.m.MarkRead("agent:bob", []string{request.ID})
	wantCode(t, err, "forbidden")
}
