package mail_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
)

// instantDirectory adds the instant sender settings to fakeDirectory.
type instantDirectory struct {
	*fakeDirectory
	senders map[string][]string // agent -> mail_instant_senders; absent: not set
	leads   map[string]string   // agent -> lead of its group
}

func (d *instantDirectory) InstantSenders(agent string) ([]string, bool, error) {
	s, ok := d.senders[agent]
	return s, ok, nil
}

func (d *instantDirectory) GroupLead(agent string) (string, error) { return d.leads[agent], nil }

// fakeRuntime is the loop state the tests control.
type fakeRuntime struct {
	mu         sync.Mutex
	iterations map[string]string // agent -> running iteration
	iterates   map[string]bool   // agent -> starts iterations on its own
	marks      []string          // "agent iteration"
}

func (r *fakeRuntime) InstantState(agent string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.iterations[agent], r.iterates[agent]
}

func (r *fakeRuntime) MarkInstant(agent, iteration string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.marks = append(r.marks, agent+" "+iteration)
}

type instantFixture struct {
	*fixture
	idir *instantDirectory
	rt   *fakeRuntime
}

// newInstantFixture is newFixture with an instant directory (bob's lead is
// alice, alice leads herself) and a runtime where alice runs it-1, bob is
// idle but iterates and carol is stopped.
func newInstantFixture(t *testing.T) *instantFixture {
	t.Helper()
	f := newFixture(t)
	idir := &instantDirectory{fakeDirectory: f.dir, senders: map[string][]string{},
		leads: map[string]string{"alice": "alice", "bob": "alice", "carol": "alice"}}
	f.m = mail.New(f.st, func() time.Time { *f.now = f.now.Add(time.Second); return *f.now }, idir)
	f.m.SetWakeHook(f.rec.wake)
	f.m.SetEventHook(f.rec.hint)
	f.m.SetAuditHook(f.rec.audit)
	rt := &fakeRuntime{iterations: map[string]string{"alice": "it-1"},
		iterates: map[string]bool{"alice": true, "bob": true}}
	f.m.SetInstantRuntime(rt)
	return &instantFixture{fixture: f, idir: idir, rt: rt}
}

func wantStatus(t *testing.T, err error, code string, status int) {
	t.Helper()
	wantCode(t, err, code)
	if got := mail.ErrorStatus(err); got != status {
		t.Fatalf("status = %d; want %d", got, status)
	}
}

func TestInstantAllowedSenders(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	f.idir.senders["carol"] = []string{"agent:bob"}
	for _, l := range []mail.Letter{
		{From: "user:customer", To: "agent:bob"},
		{From: "plugin:telegram", To: "agent:bob"},
		{From: "agent:bob", To: "agent:carol"},   // from the list
		{From: "agent:alice", To: "agent:bob"},   // the lead, by default
		{From: "user:customer", To: "group:dev"}, // a group of agents
	} {
		l.Body, l.Instant = "x", true
		for _, got := range f.send(t, l) {
			if !got.Instant {
				t.Fatalf("%s -> %s: instant not stored", l.From, got.To)
			}
		}
	}
}

func TestInstantRefusedSenders(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	f.idir.senders["carol"] = []string{}
	for _, l := range []mail.Letter{
		{From: "system:goal", To: "agent:bob"},
		{From: "plugin:metrics", To: "agent:bob"},
		{From: "agent:carol", To: "agent:bob"},   // not the lead
		{From: "agent:alice", To: "agent:carol"}, // the empty list refuses the lead
		{From: "agent:bob", To: "agent:alice"},   // the lead is the recipient itself
	} {
		l.Body, l.Instant = "x", true
		_, err := f.m.Send(l)
		wantStatus(t, err, "instant_not_allowed", http.StatusForbidden)
		if err.Error() != "instant delivery is not allowed from this sender" {
			t.Fatalf("message = %q", err)
		}
	}
	_, err := f.m.Send(mail.Letter{From: "user:customer", To: "agent:bob,user:customer", Body: "x", Instant: true})
	wantStatus(t, err, "instant_recipient", http.StatusBadRequest)
	_, err = f.m.Send(mail.Letter{From: "agent:alice", To: "queue:OPS", Body: "x", Instant: true})
	wantStatus(t, err, "instant_recipient", http.StatusBadRequest)
	var n int
	if err := f.st.DB.QueryRow(`SELECT COUNT(*) FROM mail`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a refused send wrote %d rows (%v)", n, err)
	}
}

func TestInstantWithoutDirectoryRefusesAgents(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.m.Send(mail.Letter{From: "agent:alice", To: "agent:bob", Body: "x", Instant: true})
	wantCode(t, err, "instant_not_allowed")
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:bob", Body: "x", Instant: true})
	if l.InstantState != mail.InstantQueued {
		t.Fatalf("state without a runtime = %q; want queued", l.InstantState)
	}
}

func TestInstantStateAtSend(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	want := map[string]string{"agent:alice": mail.InstantWaiting, "agent:bob": mail.InstantQueued, "agent:carol": mail.InstantStopped}
	out := f.send(t, mail.Letter{From: "user:customer", To: "group:dev", Body: "x", Instant: true})
	for _, l := range out {
		if l.InstantState != want[l.To] || l.InstantIteration != "" || l.InstantAt != "" {
			t.Fatalf("%s: state %q iteration %q at %q; want %q", l.To, l.InstantState, l.InstantIteration, l.InstantAt, want[l.To])
		}
		if got := f.show(t, l.To, l.ID); got.InstantState != want[l.To] || !got.Instant {
			t.Fatalf("stored %s = %+v", l.To, got)
		}
	}
	if len(f.rt.marks) != 1 || f.rt.marks[0] != "alice it-1" {
		t.Fatalf("marks = %v; want alice it-1 only", f.rt.marks)
	}
	if got := f.rec.woken(); len(got) != 3 {
		t.Fatalf("woken = %v; an instant letter wakes like any letter", got)
	}
	plain := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "plain"})
	if plain.Instant || plain.InstantState != "" || len(f.rt.marks) != 1 {
		t.Fatalf("plain letter = %+v, marks %v", plain, f.rt.marks)
	}
}

func TestInstantTakeReturnsOnceAndCountsAttempt(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	notice := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "n", Instant: true})
	request := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "r", Instant: true, NeedsReply: true})
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "plain"})
	hints := len(f.rec.hints)
	got, err := f.m.TakeInstant("alice", "it-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != request.ID || got[1].ID != notice.ID {
		t.Fatalf("taken = %v; want the request, then the notice", ids(got))
	}
	for _, l := range got {
		stored := f.show(t, "agent:alice", l.ID)
		if stored.Attempts != 1 || stored.InstantState != mail.InstantDelivered ||
			stored.InstantIteration != "it-1" || stored.InstantAt == "" || !stored.Open() {
			t.Fatalf("stored = %+v", stored)
		}
		if l.Attempts != 1 || l.InstantState != mail.InstantDelivered {
			t.Fatalf("returned = %+v", l)
		}
	}
	if n := f.rec.auditKinds("mail_instant_shown"); n != 2 {
		t.Fatalf("mail_instant_shown = %d; want 2", n)
	}
	if len(f.rec.hints) != hints+2 {
		t.Fatalf("hints = %d; want %d", len(f.rec.hints), hints+2)
	}
	if len(f.rec.wakes) != 3 {
		t.Fatalf("the take woke the agent: %v", f.rec.wakes)
	}
	again, err := f.m.TakeInstant("alice", "it-1")
	if err != nil || len(again) != 0 {
		t.Fatalf("second take = %v, %v; want none", ids(again), err)
	}
	delivered, err := f.m.InstantDeliveredIDs("alice", "it-1")
	if err != nil || len(delivered) != 2 {
		t.Fatalf("delivered ids = %v, %v", delivered, err)
	}
}

func TestInstantTakeDeadLettersAfterFiveAttempts(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "n", Instant: true})
	if _, err := f.st.DB.Exec(`UPDATE mail SET attempts = 5 WHERE id = ?`, l.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.m.TakeInstant("alice", "it-1")
	if err != nil || len(got) != 0 {
		t.Fatalf("take = %v, %v; want none", ids(got), err)
	}
	stored := f.show(t, "agent:alice", l.ID)
	if stored.DeadReason != mail.DeadAttempts || stored.InstantState != mail.InstantNotDelivered {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestInstantRenderMarksReadAndHookSkipsIt(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "n", Instant: true})
	hints := len(f.rec.hints)
	if _, err := f.m.RenderIteration("alice", "it-2", 10); err != nil {
		t.Fatal(err)
	}
	stored := f.show(t, "agent:alice", l.ID)
	if stored.InstantState != mail.InstantRead || stored.InstantIteration != "it-2" || stored.InstantAt == "" {
		t.Fatalf("stored = %+v", stored)
	}
	if len(f.rec.hints) != hints+1 {
		t.Fatalf("hints = %d; want %d", len(f.rec.hints), hints+1)
	}
	got, err := f.m.TakeInstant("alice", "it-2")
	if err != nil || len(got) != 0 {
		t.Fatalf("take after render = %v, %v; want none", ids(got), err)
	}
}

func TestInstantRenderKeepsDelivered(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "r", Instant: true, NeedsReply: true})
	if _, err := f.m.TakeInstant("alice", "it-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.RenderIteration("alice", "it-2", 10); err != nil {
		t.Fatal(err)
	}
	stored := f.show(t, "agent:alice", l.ID)
	if stored.InstantState != mail.InstantDelivered || stored.InstantIteration != "it-1" || stored.Attempts != 2 {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestInstantDeadIsNotDelivered(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:bob", Body: "n", Instant: true})
	if _, err := f.st.DB.Exec(`UPDATE mail SET attempts = 5 WHERE id = ?`, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.RenderIteration("bob", "it-9", 10); err != nil {
		t.Fatal(err)
	}
	stored := f.show(t, "agent:bob", l.ID)
	if stored.DeadReason != mail.DeadAttempts || stored.InstantState != mail.InstantNotDelivered {
		t.Fatalf("stored = %+v", stored)
	}
	requeued, err := f.m.Requeue("agent:bob", l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if requeued.InstantState != mail.InstantQueued || f.show(t, "agent:bob", l.ID).InstantState != mail.InstantQueued {
		t.Fatalf("requeued = %+v", requeued)
	}
	// A letter over the inbox cap is dead on arrival.
	f.dir.agents["carol"] = 1
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:carol", Body: "a"})
	over := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:carol", Body: "b", Instant: true})
	if over.DeadReason != mail.DeadQueueLimit || over.InstantState != mail.InstantNotDelivered {
		t.Fatalf("over the cap = %+v", over)
	}
}

func TestInstantReply(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	q := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Body: "q", NeedsReply: true})
	reply, _, err := f.m.Reply(mail.ReplyRequest{As: "user:customer", ID: q.ID, Text: "a", Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Instant || reply.InstantState != mail.InstantWaiting {
		t.Fatalf("reply = %+v", reply)
	}
	back := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "q2", NeedsReply: true})
	_, _, err = f.m.Reply(mail.ReplyRequest{As: "agent:alice", ID: back.ID, Text: "a", Instant: true})
	wantStatus(t, err, "instant_recipient", http.StatusBadRequest)
	toBob := f.sendOne(t, mail.Letter{From: "agent:bob", To: "agent:carol", Body: "q3", NeedsReply: true})
	_, _, err = f.m.Reply(mail.ReplyRequest{As: "agent:carol", ID: toBob.ID, Text: "a", Instant: true})
	wantStatus(t, err, "instant_not_allowed", http.StatusForbidden)
}

func TestAgentActionPassesInstant(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	res, err := f.m.AgentAction("alice", "it-1", "send", map[string]any{"to": "agent:bob", "text": "x", "instant": true})
	if err != nil {
		t.Fatal(err)
	}
	if l := res.(mail.MailSendResult).Letters[0]; !l.Instant {
		t.Fatalf("send = %+v; want instant", l)
	}
	q := f.sendOne(t, mail.Letter{From: "agent:bob", To: "agent:carol", Body: "q", NeedsReply: true})
	_, err = f.m.AgentAction("carol", "", "reply", map[string]any{"id": q.ID, "text": "a", "instant": true})
	wantCode(t, err, "instant_not_allowed")
}
