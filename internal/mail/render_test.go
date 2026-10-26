package mail_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
)

func TestRenderBatchOrdersRequestsFirstAndCountsAttempts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	n1 := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "notice 1"})
	r1 := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "request 1", NeedsReply: true})
	n2 := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "notice 2"})
	r2 := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "request 2", NeedsReply: true})
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:bob", Body: "not alice's", NeedsReply: true})

	batch, err := f.m.RenderBatch("alice", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(batch), []string{r1.ID, r2.ID, n1.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("batch = %v; want %v", got, want)
	}
	for _, l := range batch {
		if l.Attempts != 1 {
			t.Fatalf("%s attempts = %d; want 1", l.ID, l.Attempts)
		}
	}
	if got := f.show(t, "agent:alice", n2.ID); got.Attempts != 0 {
		t.Fatalf("a letter held back by the limit was counted: %+v", got)
	}
	batch, err = f.m.RenderBatch("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(batch), []string{r1.ID, r2.ID, n1.ID, n2.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second batch = %v; want %v", got, want)
	}
	if batch[0].Attempts != 2 || batch[3].Attempts != 1 {
		t.Fatalf("attempts = %d / %d; want 2 / 1", batch[0].Attempts, batch[3].Attempts)
	}
}

func TestRenderBatchDeadLettersAfterFiveAttempts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "ignored", NeedsReply: true})
	for i := 1; i <= 5; i++ {
		batch, err := f.m.RenderBatch("alice", 10)
		if err != nil || len(batch) != 1 || batch[0].Attempts != i {
			t.Fatalf("render %d = %+v, %v", i, batch, err)
		}
	}
	batch, err := f.m.RenderBatch("alice", 10)
	if err != nil || len(batch) != 0 {
		t.Fatalf("sixth render = %v, %v; want nothing", ids(batch), err)
	}
	got := f.show(t, "agent:alice", l.ID)
	if got.DeadAt == "" || got.DeadReason != "attempts" || got.Attempts != 5 {
		t.Fatalf("letter after six renders = %+v", got)
	}
	if n, _ := f.m.OpenCount("alice"); n != 0 {
		t.Fatalf("open = %d; want 0", n)
	}
	if f.rec.auditKinds("mail_dead") != 1 {
		t.Fatalf("audits = %v", f.rec.audits)
	}
}

func TestRenderBatchDeadLettersUndecodableData(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	bad := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "bad", NeedsReply: true})
	good := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "good", NeedsReply: true})
	if _, err := f.st.DB.Exec(`UPDATE mail SET data = '{not json' WHERE id = ?`, bad.ID); err != nil {
		t.Fatal(err)
	}
	batch, err := f.m.RenderBatch("alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(batch); !reflect.DeepEqual(got, []string{good.ID}) {
		t.Fatalf("batch = %v; want only the good letter", got)
	}
	var reason string
	if err := f.st.DB.QueryRow(`SELECT dead_reason FROM mail WHERE id = ?`, bad.ID).Scan(&reason); err != nil || reason != "decode_error" {
		t.Fatalf("dead_reason = %q, %v; want decode_error", reason, err)
	}
}

func TestCloseReadClosesOnlyNotices(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	notice := f.sendOne(t, mail.Letter{From: "system:proxy", To: "agent:alice", Kind: "budget.warn", Body: "80%"})
	request := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "?", NeedsReply: true})
	other := f.sendOne(t, mail.Letter{From: "system:proxy", To: "agent:bob", Body: "bob's"})
	n, err := f.m.CloseRead("alice", []string{notice.ID, request.ID, other.ID})
	if err != nil || n != 1 {
		t.Fatalf("close read = %d, %v; want 1", n, err)
	}
	if got := f.show(t, "agent:alice", notice.ID); got.AnsweredAt == "" || got.CloseNote != "read" {
		t.Fatalf("notice = %+v", got)
	}
	if got := f.show(t, "agent:alice", request.ID); got.AnsweredAt != "" {
		t.Fatalf("request closed: %+v", got)
	}
	if got := f.show(t, "agent:bob", other.ID); got.AnsweredAt != "" {
		t.Fatalf("another agent's notice closed: %+v", got)
	}
}

func TestRequeueReopensDeadLetter(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "x", NeedsReply: true})
	for range 6 {
		if _, err := f.m.RenderBatch("alice", 10); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.m.Requeue("agent:bob", l.ID)
	wantCode(t, err, "forbidden")
	f.rec.wakes = nil
	got, err := f.m.Requeue("agent:alice", l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeadAt != "" || got.DeadReason != "" || got.Attempts != 0 || got.AnsweredAt != "" || got.AnswerID != "" {
		t.Fatalf("requeued = %+v", got)
	}
	if woken := f.rec.woken(); !reflect.DeepEqual(woken, []string{"alice"}) {
		t.Fatalf("woken = %v; want alice", woken)
	}
	if f.rec.auditKinds("mail_requeued") != 1 {
		t.Fatalf("audits = %v", f.rec.audits)
	}
	_, err = f.m.Requeue("agent:alice", l.ID)
	wantCode(t, err, "invalid_argument")
}

func TestClearInboxDeletesOpenOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "open 1", NeedsReply: true})
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "open 2"})
	closed := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "closed", NeedsReply: true})
	if _, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: closed.ID, Text: "done"}); err != nil {
		t.Fatal(err)
	}
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:bob", Body: "bob", NeedsReply: true})
	n, err := f.m.ClearInbox("alice")
	if err != nil || n != 2 {
		t.Fatalf("cleared = %d, %v; want 2", n, err)
	}
	if open, _ := f.m.OpenCount("alice"); open != 0 {
		t.Fatalf("alice open = %d", open)
	}
	if open, _ := f.m.OpenCount("bob"); open != 1 {
		t.Fatalf("bob open = %d", open)
	}
	f.show(t, "agent:alice", closed.ID)
	if f.rec.auditKinds("mail_cleared") != 1 {
		t.Fatalf("audits = %v", f.rec.audits)
	}
}

func TestListBoxesAndPaging(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var sent []mail.Letter
	for range 3 {
		sent = append(sent, f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Body: "x", NeedsReply: true}))
	}
	if _, err := f.m.Close("user:customer", sent[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	inbox, _, err := f.m.List(mail.BoxInbox, "user:customer", 10, "")
	if err != nil || !reflect.DeepEqual(ids(inbox), []string{sent[1].ID, sent[2].ID}) {
		t.Fatalf("inbox = %v, %v; want oldest first", ids(inbox), err)
	}
	page, next, err := f.m.List(mail.BoxSent, "agent:alice", 2, "")
	if err != nil || !reflect.DeepEqual(ids(page), []string{sent[2].ID, sent[1].ID}) || next == "" {
		t.Fatalf("sent page = %v next %q, %v", ids(page), next, err)
	}
	page, next, err = f.m.List(mail.BoxSent, "agent:alice", 2, next)
	if err != nil || !reflect.DeepEqual(ids(page), []string{sent[0].ID}) || next != "" {
		t.Fatalf("sent page 2 = %v next %q, %v", ids(page), next, err)
	}
	archive, _, err := f.m.List(mail.BoxArchive, "user:customer", 10, "")
	if err != nil || !reflect.DeepEqual(ids(archive), []string{sent[0].ID}) {
		t.Fatalf("archive = %v, %v", ids(archive), err)
	}
	_, _, err = f.m.List("trash", "user:customer", 10, "")
	wantCode(t, err, "invalid_argument")
}

func TestShowIsScopedToSenderAndRecipient(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	l := f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "x"})
	f.show(t, "agent:alice", l.ID)
	f.show(t, "agent:bob", l.ID)
	_, _, err := f.m.Show("agent:carol", l.ID)
	wantCode(t, err, "forbidden")
	_, _, err = f.m.Show("agent:carol", "m-none")
	wantCode(t, err, "not_found")
}

func TestUnreadCountsOpenLettersBySenderAndKind(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q := map[string]any{"task_key": "OPS-1"}
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Kind: "task.question", Body: "?", NeedsReply: true, Data: q})
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Kind: "task.question", Body: "?", NeedsReply: true, Data: q})
	read := f.sendOne(t, mail.Letter{From: "agent:bob", To: "user:customer", Body: "hi", NeedsReply: true})
	if _, _, err := f.m.MarkRead("user:customer", []string{read.ID}); err != nil {
		t.Fatal(err)
	}
	u, err := f.m.Unread("user:customer")
	if err != nil {
		t.Fatal(err)
	}
	want := mail.MailUnread{As: "user:customer", Open: 3, Unread: 2, BySender: []mail.SenderCount{
		{Sender: "agent:alice", Kind: "task.question", Count: 2},
		{Sender: "agent:bob", Kind: "message", Count: 1},
	}}
	if !reflect.DeepEqual(u, want) {
		t.Fatalf("unread = %+v; want %+v", u, want)
	}
}

func TestAwaitingListsUnansweredRequestsWithDeadline(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	waiting := f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "?", NeedsReply: true, Deadline: "10m"})
	answered := f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:carol", Body: "?", NeedsReply: true, Deadline: "10m"})
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "no deadline", NeedsReply: true})
	if _, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:carol", ID: answered.ID, Text: "done"}); err != nil {
		t.Fatal(err)
	}
	got, err := f.m.Awaiting("alice")
	if err != nil || !reflect.DeepEqual(ids(got), []string{waiting.ID}) {
		t.Fatalf("awaiting = %v, %v; want %s", ids(got), err, waiting.ID)
	}
}

func TestCloseTaskQuestionsTxClosesOnlyThatTask(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q1 := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Kind: "task.question", Body: "?",
		NeedsReply: true, Data: map[string]any{"task_key": "OPS-1"}})
	q2 := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Kind: "task.question", Body: "?",
		NeedsReply: true, Data: map[string]any{"task_key": "OPS-2"}})
	// Another asker's question on the same task stays open: its wait does.
	q3 := f.sendOne(t, mail.Letter{From: "agent:bob", To: "user:customer", Kind: "task.question", Body: "?",
		NeedsReply: true, Data: map[string]any{"task_key": "OPS-1"}})
	tx, err := f.st.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	n, err := mail.CloseTaskQuestionsTx(tx, "user:customer", "OPS-1", []string{"agent:alice"}, "task-comment:9", *f.now)
	if err != nil || n != 1 {
		_ = tx.Rollback()
		t.Fatalf("closed = %d, %v; want 1", n, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := f.show(t, "user:customer", q1.ID); got.AnsweredAt == "" || got.CloseNote != "task-comment:9" {
		t.Fatalf("q1 = %+v", got)
	}
	if got := f.show(t, "user:customer", q2.ID); got.AnsweredAt != "" {
		t.Fatalf("q2 closed: %+v", got)
	}
	if got := f.show(t, "user:customer", q3.ID); got.AnsweredAt != "" {
		t.Fatalf("q3 closed: %+v", got)
	}
}

// pageAll pages box of as with limit until next_before is empty and returns
// every letter id in page order.
func pageAll(t *testing.T, m *mail.Mail, box, as string, limit int) []string {
	t.Helper()
	var out []string
	before := ""
	for range 20 {
		page, next, err := m.List(box, as, limit, before)
		if err != nil {
			t.Fatalf("list %s before %q: %v", box, before, err)
		}
		out = append(out, ids(page)...)
		if next == "" {
			return out
		}
		before = next
	}
	t.Fatalf("list %s did not end", box)
	return nil
}

// fixedClockMail is a Mail over f's store whose clock never moves, so every
// letter it writes shares one created_at.
func fixedClockMail(f *fixture) *mail.Mail {
	at := time.Date(2026, 10, 26, 11, 0, 0, 0, time.UTC)
	return mail.New(f.st, func() time.Time { return at }, f.dir)
}

func TestListPagingKeepsRowsSharingCreatedAt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	m := fixedClockMail(f)
	// One send to three recipients: three rows with one created_at.
	out, err := m.Send(mail.Letter{From: "user:customer", To: "agent:alice,agent:bob,agent:carol", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := pageAll(t, m, mail.BoxSent, "user:customer", 2); !reflect.DeepEqual(got, []string{out[2].ID, out[1].ID, out[0].ID}) {
		t.Fatalf("sent pages = %v; want every row once, newest first", got)
	}
	// Three letters to one recipient in the same instant, closed: archive and all.
	var letters []string
	for range 3 {
		l, err := m.Send(mail.Letter{From: "agent:alice", To: "user:customer", Body: "y"})
		if err != nil {
			t.Fatal(err)
		}
		letters = append(letters, l[0].ID)
	}
	if _, _, err := m.MarkRead("user:customer", letters); err != nil {
		t.Fatal(err)
	}
	want := []string{letters[2], letters[1], letters[0]}
	for _, box := range []string{mail.BoxArchive, mail.BoxAll} {
		if got := pageAll(t, m, box, "user:customer", 2); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s pages = %v; want %v", box, got, want)
		}
	}
	_, _, err = m.List(mail.BoxSent, "user:customer", 2, "2026-10-26T11:00:00Z/x")
	wantCode(t, err, "invalid_argument")
}

func TestInboxPagesAscendingWithCursor(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	m := fixedClockMail(f)
	var want []string
	for range 5 {
		l, err := m.Send(mail.Letter{From: "agent:alice", To: "user:customer", Body: "?", NeedsReply: true})
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, l[0].ID)
	}
	if got := pageAll(t, m, mail.BoxInbox, "user:customer", 2); !reflect.DeepEqual(got, want) {
		t.Fatalf("inbox pages = %v; want %v oldest first", got, want)
	}
}
