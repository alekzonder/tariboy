package mail_test

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/mail"
)

func TestSendWritesOneRowPerRecipientAndWakesAgents(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	out := f.send(t, mail.Letter{
		From: "user:customer", To: "agent:alice, agent:bob,user:customer",
		Subject: "hello", Body: "do it", NeedsReply: true,
		Data: map[string]any{"task_key": "OPS-1"},
	})
	if len(out) != 3 {
		t.Fatalf("letters = %d; want 3", len(out))
	}
	idRE := regexp.MustCompile(`^m-\d{8}T\d{6}Z-\d{6}$`)
	seen := map[string]bool{}
	for i, l := range out {
		if !idRE.MatchString(l.ID) || seen[l.ID] {
			t.Fatalf("letter %d id %q is malformed or repeated", i, l.ID)
		}
		seen[l.ID] = true
		if l.Seq == 0 || !strings.HasSuffix(l.ID, "-"+padSeq(l.Seq)) {
			t.Fatalf("letter id %q does not end with seq %d", l.ID, l.Seq)
		}
		if l.From != "user:customer" || l.Kind != "message" || !l.NeedsReply || l.AnsweredAt != "" {
			t.Fatalf("letter %d = %+v", i, l)
		}
	}
	if got := []string{out[0].To, out[1].To, out[2].To}; !reflect.DeepEqual(got, []string{"agent:alice", "agent:bob", "user:customer"}) {
		t.Fatalf("recipients = %v", got)
	}
	if got := f.rec.woken(); !reflect.DeepEqual(got, []string{"alice", "bob"}) {
		t.Fatalf("woken = %v; want alice, bob", got)
	}
	if len(f.rec.hints) != 3 || f.rec.hints[0].ID != out[0].ID || f.rec.hints[0].From != "user:customer" ||
		f.rec.hints[0].To != "agent:alice" || f.rec.hints[0].Kind != "message" || f.rec.hints[0].TS != out[0].CreatedAt {
		t.Fatalf("hints = %+v", f.rec.hints)
	}
	if n := f.rec.auditKinds("mail_sent"); n != 3 {
		t.Fatalf("mail_sent audits = %d; want 3", n)
	}
	for _, agent := range []string{"alice", "bob"} {
		if n, err := f.m.OpenCount(agent); err != nil || n != 1 {
			t.Fatalf("open count of %s = %d, %v; want 1", agent, n, err)
		}
	}
	got := f.show(t, "agent:alice", out[0].ID)
	if got.Data["task_key"] != "OPS-1" || got.Body != "do it" || got.Subject != "hello" {
		t.Fatalf("stored letter = %+v", got)
	}
}

func padSeq(seq int64) string {
	s := strings.Repeat("0", 6) + itoa(seq)
	return s[len(s)-6:]
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestLetterJSONUsesTheFixedFieldNames(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	l := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Subject: "s", Body: "b", NeedsReply: true})
	raw, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	want := []string{"id", "seq", "from", "to", "subject", "body", "kind", "data", "needs_reply", "in_reply_to",
		"deadline", "created_at", "read_at", "answered_at", "answer_id", "close_note", "attempts", "dead_at",
		"dead_reason", "produced_in_iteration"}
	if len(fields) != len(want) {
		t.Fatalf("letter JSON has %d fields: %s", len(fields), raw)
	}
	for _, k := range want {
		if _, ok := fields[k]; !ok {
			t.Fatalf("letter JSON lacks %q: %s", k, raw)
		}
	}
	if string(mustJSON(t, fields["data"])) != "{}" {
		t.Fatalf("empty data = %s; want {}", mustJSON(t, fields["data"]))
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSendGroupExpandsWithoutSender(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	out := f.send(t, mail.Letter{From: "agent:alice", To: "group:dev", Body: "standup", NeedsReply: true})
	var to []string
	for _, l := range out {
		to = append(to, l.To)
	}
	if !reflect.DeepEqual(to, []string{"agent:bob", "agent:carol"}) {
		t.Fatalf("group expanded to %v; want bob and carol", to)
	}
	if got := f.rec.woken(); !reflect.DeepEqual(got, []string{"bob", "carol"}) {
		t.Fatalf("woken = %v", got)
	}
	// A group listed beside one of its members writes one letter per member.
	out = f.send(t, mail.Letter{From: "user:customer", To: "agent:bob,group:dev", Body: "all"})
	if len(out) != 3 {
		t.Fatalf("group plus member wrote %d letters; want 3", len(out))
	}
	_, err := f.m.Send(mail.Letter{From: "agent:alice", To: "group:empty", Body: "nobody"})
	wantCode(t, err, "empty_group")
	_, err = f.m.Send(mail.Letter{From: "agent:alice", To: "group:nope", Body: "x"})
	wantCode(t, err, "unknown_recipient")
}

func TestSendToSystemPrincipalIsStoredClosed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	out := f.send(t, mail.Letter{From: "agent:alice", To: "system:goal,queue:OPS", Body: "report", NeedsReply: true})
	if len(out) != 2 {
		t.Fatalf("letters = %d; want 2", len(out))
	}
	for _, l := range out {
		if l.AnsweredAt == "" || l.AnsweredAt != l.CreatedAt {
			t.Fatalf("letter to %s answered_at %q; want created_at %q", l.To, l.AnsweredAt, l.CreatedAt)
		}
	}
	if len(f.rec.woken()) != 0 {
		t.Fatalf("a closed letter woke %v", f.rec.woken())
	}
	sent, _, err := f.m.List(mail.BoxSent, "agent:alice", 10, "")
	if err != nil || len(sent) != 2 {
		t.Fatalf("sent = %d, %v; want 2", len(sent), err)
	}
	inbox, _, err := f.m.List(mail.BoxArchive, "system:goal", 10, "")
	if err != nil || len(inbox) != 1 {
		t.Fatalf("system:goal archive = %d, %v; want 1", len(inbox), err)
	}
	_, err = f.m.Send(mail.Letter{From: "agent:alice", To: "queue:NOPE", Body: "x"})
	wantCode(t, err, "unknown_recipient")
}

func TestSendRejectsUnknownRecipient(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, to := range []string{"agent:ghost", "user:someone", "plugin:slack", "chat:service:alice", "alice", "system:", "agent:alice,agent:ghost"} {
		_, err := f.m.Send(mail.Letter{From: "user:customer", To: to, Body: "x"})
		wantCode(t, err, "unknown_recipient")
		if !strings.Contains(err.Error(), "ghost") && !strings.Contains(err.Error(), to) {
			t.Fatalf("error %q does not name the recipient %q", err, to)
		}
	}
	var n int
	if err := f.st.DB.QueryRow(`SELECT COUNT(*) FROM mail`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a rejected send wrote %d rows (%v)", n, err)
	}
	// The plugin and customer principals are valid recipients.
	f.send(t, mail.Letter{From: "agent:alice", To: "plugin:telegram,user:customer", Body: "x"})
	if len(f.rec.plugins) != 1 || !reflect.DeepEqual(f.rec.plugins[0], []string{"telegram"}) {
		t.Fatalf("plugin wakes = %v", f.rec.plugins)
	}
}

func TestSendIdempotencyKeyPerRecipient(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first := f.sendOne(t, mail.Letter{From: "system:tasks", To: "agent:alice", Body: "x", IdempotencyKey: "n-1"})
	again := f.sendOne(t, mail.Letter{From: "system:tasks", To: "agent:alice", Body: "changed", IdempotencyKey: "n-1"})
	if again.ID != first.ID || again.Body != "x" {
		t.Fatalf("retry returned %+v; want the existing %s", again, first.ID)
	}
	if n, _ := f.m.OpenCount("alice"); n != 1 {
		t.Fatalf("open = %d; want 1", n)
	}
	if len(f.rec.wakes) != 1 || len(f.rec.hints) != 1 {
		t.Fatalf("the retry fired hooks: wakes %v hints %v", f.rec.wakes, f.rec.hints)
	}
	// The same key to another recipient is a different letter.
	other := f.sendOne(t, mail.Letter{From: "system:tasks", To: "agent:bob", Body: "x", IdempotencyKey: "n-1"})
	if other.ID == first.ID {
		t.Fatal("the key collided across recipients")
	}
}

func TestSendAtCapStoresDeadWithoutWake(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.dir.agents["alice"] = 2
	f.send(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "1"})
	f.send(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "2"})
	f.rec.wakes = nil
	out := f.send(t, mail.Letter{From: "user:customer", To: "agent:alice,agent:bob", Body: "3"})
	if out[0].DeadAt == "" || out[0].DeadReason != "queue_limit" || out[0].AnsweredAt == "" {
		t.Fatalf("letter over the cap = %+v; want dead queue_limit", out[0])
	}
	if out[1].DeadAt != "" {
		t.Fatalf("letter to bob = %+v; want open", out[1])
	}
	if got := f.rec.woken(); !reflect.DeepEqual(got, []string{"bob"}) {
		t.Fatalf("woken = %v; want only bob", got)
	}
	if n, _ := f.m.OpenCount("alice"); n != 2 {
		t.Fatalf("alice open = %d; want 2", n)
	}
	if f.rec.auditKinds("mail_dead") != 1 {
		t.Fatalf("audits = %v; want one mail_dead", f.rec.audits)
	}
	archive, _, err := f.m.List(mail.BoxArchive, "agent:alice", 10, "")
	if err != nil || len(archive) != 1 || archive[0].ID != out[0].ID {
		t.Fatalf("archive = %v, %v; want the dead letter", ids(archive), err)
	}
}

func TestSendToSelfWakesTheAgent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.send(t, mail.Letter{From: "agent:alice", To: "agent:alice", Body: "note to self", NeedsReply: true})
	if got := f.rec.woken(); !reflect.DeepEqual(got, []string{"alice"}) {
		t.Fatalf("woken = %v; want alice", got)
	}
}

func TestSendValidatesSenderAndDeadline(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.m.Send(mail.Letter{From: "", To: "agent:alice", Body: "x"})
	wantCode(t, err, "invalid_argument")
	for _, from := range []string{"agent:ghost", "group:dev", "queue:OPS", "user:other"} {
		_, err := f.m.Send(mail.Letter{From: from, To: "agent:alice", Body: "x"})
		wantCode(t, err, "unknown_sender")
	}
	_, err = f.m.Send(mail.Letter{From: "agent:alice", To: "", Body: "x"})
	wantCode(t, err, "invalid_argument")
	_, err = f.m.Send(mail.Letter{From: "agent:alice", To: "agent:bob", Body: "x", NeedsReply: true, Deadline: "soon"})
	wantCode(t, err, "invalid_argument")
	// A duration deadline is stored as the absolute time it names.
	l := f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "x", NeedsReply: true, Deadline: "5m"})
	if l.Deadline == "" || l.Deadline <= l.CreatedAt {
		t.Fatalf("deadline = %q, created %q", l.Deadline, l.CreatedAt)
	}
}

func TestSendNormalisesSubjectLineBreaks(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	l := f.sendOne(t, mail.Letter{From: "agent:alice", To: "user:customer", Subject: "  one\r\ntwo\n\n# three\u0000 ", Body: "a\nb"})
	if l.Subject != "one two # three" || l.Body != "a\nb" {
		t.Fatalf("subject %q body %q; want one line and the body untouched", l.Subject, l.Body)
	}
	if got := f.show(t, "user:customer", l.ID); got.Subject != "one two # three" {
		t.Fatalf("stored subject %q", got.Subject)
	}
}

// A plugin recipient must be an enabled mail sink: a disabled plugin or one
// without mail.receive has no drainer, so its letters would sit open forever.
func TestSendToDisabledPluginRejected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// metrics is installed (it may send) but does not receive mail.
	_, err := f.m.Send(mail.Letter{From: "agent:alice", To: "plugin:metrics", Body: "x"})
	wantCode(t, err, "unknown_recipient")
	if _, err := f.m.Send(mail.Letter{From: "plugin:metrics", To: "agent:alice", Body: "x"}); err != nil {
		t.Fatalf("an installed plugin without mail.receive cannot send: %v", err)
	}
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "plugin:telegram", Body: "x"})
	f.dir.sinks["telegram"] = false // disabled
	_, err = f.m.Send(mail.Letter{From: "agent:alice", To: "plugin:telegram", Body: "x"})
	wantCode(t, err, "unknown_recipient")
}
