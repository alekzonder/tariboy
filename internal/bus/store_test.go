package bus

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func TestPublishWithGuardSerializesGuardAndMessageInsert(t *testing.T) {
	b := newBus(t)
	if _, err := b.db.Exec(`CREATE TABLE publish_guard_state(active INTEGER NOT NULL); INSERT INTO publish_guard_state VALUES (0)`); err != nil {
		t.Fatal(err)
	}
	checked, release := make(chan struct{}), make(chan struct{})
	published := make(chan error, 1)
	go func() {
		_, err := b.PublishWithGuard(Message{Channel: "chat:guard", Type: "test"}, func(tx *sql.Tx, _ time.Time) error {
			var active int
			if err := tx.QueryRow(`SELECT active FROM publish_guard_state`).Scan(&active); err != nil {
				return err
			}
			if active != 0 {
				return ErrPublishGuardDenied
			}
			close(checked)
			<-release
			return nil
		})
		published <- err
	}()
	<-checked
	attempted := make(chan struct{})
	claimed := make(chan error, 1)
	go func() {
		close(attempted)
		_, err := b.db.Exec(`UPDATE publish_guard_state SET active=1`)
		claimed <- err
	}()
	<-attempted
	select {
	case err := <-claimed:
		t.Fatalf("state mutation landed between guard and publish: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-published; err != nil {
		t.Fatal(err)
	}
	if err := <-claimed; err != nil {
		t.Fatal(err)
	}
	var messages int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE channel='chat:guard'`).Scan(&messages); err != nil || messages != 1 {
		t.Fatalf("messages=%d err=%v", messages, err)
	}
}

func TestPublishWithDeniedGuardWritesNothing(t *testing.T) {
	b := newBus(t)
	if _, err := b.Subscribe("worker", "chat:denied", nil, nil); err != nil {
		t.Fatal(err)
	}
	_, err := b.PublishWithGuard(Message{Channel: "chat:denied", Type: "test"}, func(tx *sql.Tx, _ time.Time) error {
		var one int
		if err := tx.QueryRow(`SELECT 1`).Scan(&one); err != nil {
			return err
		}
		return ErrPublishGuardDenied
	})
	if !errors.Is(err, ErrPublishGuardDenied) {
		t.Fatalf("publish err=%v", err)
	}
	for table, query := range map[string]string{
		"messages":   `SELECT COUNT(*) FROM messages WHERE channel='chat:denied'`,
		"sequence":   `SELECT COUNT(*) FROM task_workflow_message_sequence`,
		"deliveries": `SELECT COUNT(*) FROM deliveries`,
	} {
		var count int
		if err := b.db.QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func newBus(t *testing.T) *Bus {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	tick := int64(0)
	clk := func() time.Time {
		tick++
		return time.Date(2026, 7, 6, 10, 0, 0, int(tick), time.UTC)
	}
	return New(s, clk)
}

// newBusSeconds is like newBus but ticks one whole second per call, so message
// timestamps stay strictly monotonic as RFC3339Nano strings (that format strips
// trailing zeros, so sub-second-only ticks do not string-sort monotonically).
func newBusSeconds(t *testing.T) *Bus {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	tick := int64(0)
	clk := func() time.Time {
		tick++
		return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC).Add(time.Duration(tick) * time.Second)
	}
	return New(s, clk)
}

func pub(t *testing.T, b *Bus, channel, typ, text string, subject map[string]any) Message {
	t.Helper()
	m, err := b.Publish(Message{Channel: channel, Type: typ, Text: text, Subject: subject,
		Source: "test", ProducedByAgent: "producer", ProducedInIteration: "producer-1"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPublishIdempotencyKeyReturnsOriginalWithoutDuplicateDelivery(t *testing.T) {
	b := newBusSeconds(t)
	channel := InboxChannel("alice")
	sub(t, b, "alice", channel)
	hookCalls := 0
	b.SetPublishHook(func(Message, []string) { hookCalls++ })

	first, err := b.Publish(Message{
		Channel: channel, Type: "task.question", Text: "first",
		IdempotencyKey: "task-notification-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Publish(Message{
		Channel: channel, Type: "task.question", Text: "retry payload is ignored",
		IdempotencyKey: "task-notification-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.Text != "first" {
		t.Fatalf("retry returned %#v; want original %#v", second, first)
	}
	if hookCalls != 1 {
		t.Fatalf("publish hook calls = %d; want 1", hookCalls)
	}
	pending, err := b.Pending("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != first.ID {
		t.Fatalf("pending = %#v; want one original delivery", pending)
	}
}

func TestPublishDeadLettersMessagesBeyondAgentQueueLimit(t *testing.T) {
	b := newBusSeconds(t)
	if _, err := b.db.Exec(`INSERT INTO agents(name, image_ref, messages_max_queue) VALUES ('alice', 'basic:latest', 2)`); err != nil {
		t.Fatal(err)
	}
	channel := InboxChannel("alice")
	sub(t, b, "alice", channel)
	if _, err := b.Subscribe("alice", channel, nil, []string{"*"}); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err := b.Publish(Message{Channel: channel, Type: "note", Text: fmt.Sprintf("%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := b.Inbox("alice", "pending", 10, "")
	if err != nil || len(pending) != 2 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	dlq, err := b.Inbox("alice", "dlq", 10, "")
	if err != nil || len(dlq) != 1 || dlq[0].Result != "queue_limit" {
		t.Fatalf("dlq=%+v err=%v", dlq, err)
	}
}

// hasMsg reports whether any message in ms has id.
func hasMsg(ms []Message, id string) bool {
	for _, m := range ms {
		if m.ID == id {
			return true
		}
	}
	return false
}

// TestPublishExcludesAuthor covers the self-delivery suppression rule: an agent
// never receives a delivery of a message it authored, while every OTHER matching
// subscriber (another agent, a plugin sink) still does. Both the plain Publish
// path and the Reply path (a reply routed back to a chat channel the replying
// agent is itself subscribed to) are exercised.
func TestPublishExcludesAuthor(t *testing.T) {
	t.Run("plain publish", func(t *testing.T) {
		b := newBusSeconds(t)
		ch := ChatChannel("room")
		// alice authors; bob is another agent; sink stands in for a plugin sink
		// that forwards the message out. All three subscribe to the same channel.
		for _, a := range []string{"alice", "bob", "sink"} {
			sub(t, b, a, ch)
		}
		m, err := b.Publish(Message{Channel: ch, Type: "note", Text: "hi",
			Source: "agent:alice", ProducedByAgent: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := b.Pending("alice", 10); hasMsg(got, m.ID) {
			t.Fatalf("author alice received her own message: %+v", got)
		}
		if got, _ := b.Pending("bob", 10); !hasMsg(got, m.ID) {
			t.Fatalf("other subscriber bob missed the message: %+v", got)
		}
		if got, _ := b.Pending("sink", 10); !hasMsg(got, m.ID) {
			t.Fatalf("plugin sink missed the message: %+v", got)
		}
	})

	t.Run("reply to chat channel", func(t *testing.T) {
		b := newBusSeconds(t)
		ch := ChatChannel("room")
		// alice (the replying agent) and sink (forwards the reply out) both watch
		// the chat channel.
		sub(t, b, "alice", ch)
		sub(t, b, "sink", ch)
		// A plugin-sourced inbound chat message (no agent author) reaches both.
		orig, err := b.Publish(Message{Channel: ch, Type: "chat.message", Text: "hello",
			Source: "plugin:messenger"})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := b.Pending("alice", 10); !hasMsg(got, orig.ID) {
			t.Fatalf("alice should receive the inbound chat message: %+v", got)
		}
		// alice replies; replyTarget routes it back to the chat channel (source is
		// a plugin, no explicit reply_to), so the sink forwards it out.
		reply, err := b.Reply("alice", orig.ID, "answer", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if reply.Channel != ch {
			t.Fatalf("reply should land on the chat channel, got %q", reply.Channel)
		}
		// The reply is delivered to the sink but NOT back into alice's own queue.
		if got, _ := b.Pending("sink", 10); !hasMsg(got, reply.ID) {
			t.Fatalf("sink missed the reply it must forward out: %+v", got)
		}
		if got, _ := b.Pending("alice", 10); hasMsg(got, reply.ID) {
			t.Fatalf("reply echoed back into author alice's queue: %+v", got)
		}
	})
}

// TestPublishSelfInboxDeliveredToAuthor is the regression guard for the
// self-delivery fix's overreach (dev-t-gmb.5). The self-exclusion must be
// OUTBOUND-ONLY: a system message produced FOR an agent and delivered TO its
// own inbox carries ProducedByAgent == that agent (provenance), yet the agent
// must still receive it. These are the concrete live paths that regressed when
// the exclusion suppressed by author alone: request-reply timeout events, proxy
// budget.warn, loop script.result, and self-targeted schedules — all published
// to InboxChannel(agent) with ProducedByAgent == agent. This test FAILS on the
// author-only exclusion (message silently dropped) and PASSES once the
// exclusion is scoped to non-inbox channels.
func TestPublishSelfInboxDeliveredToAuthor(t *testing.T) {
	for _, typ := range []string{"budget.warn", "script.result", "request.timeout"} {
		t.Run(typ, func(t *testing.T) {
			b := newBusSeconds(t)
			ch := InboxChannel("alice")
			// alice is subscribed to her own inbox (as group members are:
			// provisioner subscribes m.Name -> InboxChannel(m.Name)).
			sub(t, b, "alice", ch)
			// A system message generated for alice and delivered to her inbox:
			// author == recipient == alice.
			m, err := b.Publish(Message{Channel: ch, Type: typ, Text: "self note",
				Source: "agent:alice", ProducedByAgent: "alice"})
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := b.Pending("alice", 10); !hasMsg(got, m.ID) {
				t.Fatalf("self-authored %s on own inbox dropped from author's queue: %+v", typ, got)
			}
		})
	}
}

func TestPublishFanoutAndPending(t *testing.T) {
	b := newBus(t)
	ch := InboxChannel("bob")
	if _, err := b.Subscribe("bob", ch, Matcher{"type": "deploy.*"}, nil); err != nil {
		t.Fatal(err)
	}
	// A matching message reaches bob; a non-matching one does not.
	m1 := pub(t, b, ch, "deploy.requested", "go", map[string]any{"env": "prod"})
	pub(t, b, ch, "build.done", "nope", nil)

	pend, err := b.Pending("bob", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || pend[0].ID != m1.ID || pend[0].Text != "go" {
		t.Fatalf("pending = %+v", pend)
	}
	if pend[0].ProducedByAgent != "producer" {
		t.Fatalf("attribution lost: %+v", pend[0])
	}
	// Ack clears it.
	if err := b.Ack("bob", []string{m1.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Pending("bob", 10); len(got) != 0 {
		t.Fatalf("acked message still pending: %+v", got)
	}
	// Announce redelivers it.
	if err := b.Announce("bob", ch); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Pending("bob", 10); len(got) != 1 {
		t.Fatalf("announce did not redeliver: %+v", got)
	}
}

func TestPendingOrderAndTypeFilter(t *testing.T) {
	b := newBus(t)
	ch := ChatChannel("room")
	// type_filter narrows delivery even when matcher is empty.
	if _, err := b.Subscribe("bob", ch, Matcher{}, []string{"note.*"}); err != nil {
		t.Fatal(err)
	}
	a := pub(t, b, ch, "note.a", "first", nil)
	pub(t, b, ch, "other", "skip", nil)
	c := pub(t, b, ch, "note.c", "second", nil)
	pend, _ := b.Pending("bob", 10)
	if len(pend) != 2 || pend[0].ID != a.ID || pend[1].ID != c.ID {
		t.Fatalf("pending order/filter = %+v", pend)
	}
}

// TestSubscribeDistinctTypeFiltersBothTakeEffect covers review finding F2: two
// subscribes to the same channel+matcher differing only by their type globs must
// each get their own row and both take effect. Before the fix the dedup key was
// (agent,channel,matcher) and the second subscribe collapsed onto the first row,
// silently swallowing its filter.
func TestSubscribeDistinctTypeFiltersBothTakeEffect(t *testing.T) {
	b := newBus(t)
	ch := ChatChannel("ci")
	s1, err := b.Subscribe("bob", ch, Matcher{}, []string{"run.finished"})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := b.Subscribe("bob", ch, Matcher{}, []string{"run.started"})
	if err != nil {
		t.Fatal(err)
	}
	if s1.ID == s2.ID {
		t.Fatalf("distinct type filters collapsed onto one row: %s", s1.ID)
	}
	// Idempotency preserved: re-subscribing with the same filter returns the same
	// row (guards the NULL-safe IS comparison against a non-null filter).
	s1again, err := b.Subscribe("bob", ch, Matcher{}, []string{"run.finished"})
	if err != nil {
		t.Fatal(err)
	}
	if s1again.ID != s1.ID {
		t.Fatalf("same-filter re-subscribe made a new row: %s vs %s", s1again.ID, s1.ID)
	}
	fin := pub(t, b, ch, "run.finished", "done", nil)
	sta := pub(t, b, ch, "run.started", "go", nil)
	pub(t, b, ch, "run.other", "skip", nil)
	pend, _ := b.Pending("bob", 10)
	if len(pend) != 2 {
		t.Fatalf("want both run.finished and run.started (not run.other), got %+v", pend)
	}
	got := map[string]bool{pend[0].ID: true, pend[1].ID: true}
	if !got[fin.ID] || !got[sta.ID] {
		t.Fatalf("missing one of the typed messages: %+v", pend)
	}
}

// TestSubscribeBareThenTypedKeepsBoth covers the string-then-typed case of F2: a
// bare (no type_filter) subscribe followed by a typed one to the same
// channel+matcher must keep both rows. Before the fix the typed subscribe deduped
// onto the bare (NULL type_filter) row and its filter was dropped. Also guards the
// NULL-safe IS comparison against a NULL filter.
func TestSubscribeBareThenTypedKeepsBoth(t *testing.T) {
	b := newBus(t)
	ch := ChatChannel("ci2")
	bare, err := b.Subscribe("bob", ch, Matcher{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := b.Subscribe("bob", ch, Matcher{}, []string{"run.started"})
	if err != nil {
		t.Fatal(err)
	}
	if bare.ID == typed.ID {
		t.Fatalf("typed subscribe collapsed onto bare row, filter swallowed: %s", bare.ID)
	}
	// Bare idempotency still holds: re-subscribing bare returns the same NULL row.
	bareAgain, err := b.Subscribe("bob", ch, Matcher{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bareAgain.ID != bare.ID {
		t.Fatalf("bare re-subscribe made a new row: %s vs %s", bareAgain.ID, bare.ID)
	}
	subs, err := b.ListSubscriptions("bob")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("want 2 distinct rows, got %d: %+v", len(subs), subs)
	}
	var typedRow *Subscription
	for i := range subs {
		if subs[i].ID == typed.ID {
			typedRow = &subs[i]
		}
	}
	if typedRow == nil || len(typedRow.TypeFilter) != 1 || typedRow.TypeFilter[0] != "run.started" {
		t.Fatalf("typed filter not stored: %+v", typedRow)
	}
}

func TestPendingDedupesAcrossSubs(t *testing.T) {
	b := newBus(t)
	ch := ChatChannel("multi")
	b.Subscribe("bob", ch, Matcher{"type": "x.*"}, nil)
	b.Subscribe("bob", ch, Matcher{"source": "test"}, nil) // also matches
	m := pub(t, b, ch, "x.1", "hi", nil)
	pend, _ := b.Pending("bob", 10)
	if len(pend) != 1 {
		t.Fatalf("multiply-subscribed message not deduped: %+v", pend)
	}
	if err := b.Ack("bob", []string{m.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Pending("bob", 10); len(got) != 0 {
		t.Fatalf("ack did not clear all delivery rows: %+v", got)
	}
}

func TestHasPendingAndHook(t *testing.T) {
	b := newBus(t)
	var hookMsg Message
	var hookAgents []string
	b.SetPublishHook(func(m Message, agents []string) { hookMsg = m; hookAgents = agents })
	ch := InboxChannel("bob")
	b.Subscribe("bob", ch, Matcher{}, nil)
	m := pub(t, b, ch, "any", "x", nil)
	if hookMsg.ID != m.ID || len(hookAgents) != 1 || hookAgents[0] != "bob" {
		t.Fatalf("hook = %q %v", hookMsg.ID, hookAgents)
	}
	if ok, _ := b.HasPending("bob"); !ok {
		t.Fatal("HasPending should be true")
	}
	// HasPending must not consume: Pending still sees it.
	if got, _ := b.Pending("bob", 10); len(got) != 1 {
		t.Fatalf("HasPending consumed the message: %+v", got)
	}
}

func TestPendingTotal(t *testing.T) {
	b := newBus(t)
	ch := InboxChannel("bob")
	b.Subscribe("bob", ch, Matcher{}, nil)
	if n, err := b.PendingTotal(); err != nil || n != 0 {
		t.Fatalf("empty pending total = %d, %v", n, err)
	}
	m := pub(t, b, ch, "x", "go", nil)
	if n, err := b.PendingTotal(); err != nil || n != 1 {
		t.Fatalf("pending total = %d, %v, want 1", n, err)
	}
	if err := b.Ack("bob", []string{m.ID}); err != nil {
		t.Fatal(err)
	}
	if n, err := b.PendingTotal(); err != nil || n != 0 {
		t.Fatalf("after ack pending total = %d, %v, want 0", n, err)
	}
}

// dlqCount reports how many delivery rows have been flagged dlq.
func dlqCount(t *testing.T, b *Bus) int {
	t.Helper()
	var n int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM deliveries WHERE dlq=1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ids(msgs []Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}

// A backlog larger than `limit` must page cleanly: Pending returns the oldest
// page, is idempotent without ack, advances after ack, and NEVER pushes the
// unread tail into the DLQ (the accounting-under-backlog bug).
func TestPendingBacklogPagingAndNoDLQ(t *testing.T) {
	b := newBusSeconds(t)
	ch := ChatChannel("backlog")
	if _, err := b.Subscribe("bob", ch, Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	const n = 25
	var published []string
	for i := 0; i < n; i++ {
		m := pub(t, b, ch, "note", "n", nil)
		published = append(published, m.ID)
	}

	first := published[:10]

	// Idempotent under repeated reads with no ack: same 10 oldest each time, and
	// nothing is ever DLQ'd from merely being (or not being) read.
	for call := 0; call < 3; call++ {
		pend, err := b.Pending("bob", 10)
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(pend); len(got) != 10 || !equalStrs(got, first) {
			t.Fatalf("call %d: pending = %v, want oldest 10 %v", call, got, first)
		}
		if dq := dlqCount(t, b); dq != 0 {
			t.Fatalf("call %d: dlq count = %d, want 0", call, dq)
		}
	}

	// After acking the first page, the next page is the following 10 oldest.
	if err := b.Ack("bob", first); err != nil {
		t.Fatal(err)
	}
	pend, err := b.Pending("bob", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(pend); len(got) != 10 || !equalStrs(got, published[10:20]) {
		t.Fatalf("second page = %v, want %v", got, published[10:20])
	}
	if dq := dlqCount(t, b); dq != 0 {
		t.Fatalf("dlq count after paging = %d, want 0", dq)
	}
}

// Message ids are collision-safe and (ts, seq)-ordered even within one clock
// second and across a prune of the oldest rows.
func TestMessageIDsCollisionSafeAcrossPrune(t *testing.T) {
	b := newBus(t)
	ch := ChatChannel("ids")

	// Two publishes in the same clock-second get distinct, ordered ids.
	m1 := pub(t, b, ch, "a", "1", nil)
	m2 := pub(t, b, ch, "b", "2", nil)
	if m1.ID == m2.ID {
		t.Fatalf("same-second publishes collided: %s", m1.ID)
	}
	if !(m1.ID < m2.ID) {
		t.Fatalf("ids not ordered: %s !< %s", m1.ID, m2.ID)
	}

	m3 := pub(t, b, ch, "c", "3", nil)

	// Simulate a prune: delete the oldest surviving rows.
	if _, err := b.db.Exec(`DELETE FROM messages WHERE id IN (?,?)`, m1.ID, m2.ID); err != nil {
		t.Fatal(err)
	}

	// A publish in the same second must NOT reuse an id of any survivor (m3).
	m4 := pub(t, b, ch, "d", "4", nil)
	if m4.ID == m3.ID {
		t.Fatalf("post-prune publish collided with survivor: %s", m4.ID)
	}
	if !(m3.ID < m4.ID) {
		t.Fatalf("post-prune id not ordered after survivor: %s !< %s", m3.ID, m4.ID)
	}

	// Channel names containing '_' must not over-match via the prefix scan.
	chUnder := ChatChannel("a_b")
	chWild := ChatChannel("axb")
	u1 := pub(t, b, chUnder, "x", "u", nil)
	w1 := pub(t, b, chWild, "x", "w", nil)
	u2 := pub(t, b, chUnder, "y", "u", nil)
	// If '_' were treated as a LIKE wildcard, w1 would inflate a_b's seq.
	if !(u1.ID < u2.ID) || u1.ID == u2.ID {
		t.Fatalf("underscore channel ids not sequential: %s %s", u1.ID, u2.ID)
	}
	if got := seqOf(t, u2.ID); got != 2 {
		t.Fatalf("underscore channel over-matched wildcard neighbor: seq=%d (w1=%s)", got, w1.ID)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// seqOf parses the numeric suffix of a message id.
func seqOf(t *testing.T, id string) int {
	t.Helper()
	i := len(id) - 1
	for i >= 0 && id[i] != '-' {
		i--
	}
	n, err := strconv.Atoi(id[i+1:])
	if err != nil {
		t.Fatalf("bad id suffix %q: %v", id, err)
	}
	return n
}

// TestPendingOrdersByTrueChronologyNotLexicalTS publishes four messages within
// the SAME wall-clock second whose nanosecond offsets (10, 11, 2, 100) are
// chosen so RFC3339Nano's trailing-zero stripping does not sort lexically the
// same as chronologically: e.g. 11ns strips to "...011" while 100ns strips to
// "...1", and once the trailing zone byte ('Z') is factored in, the shorter
// stripped string sorts AFTER the longer one even though 11ns < 100ns. Publish
// is called in a deliberately non-monotonic order (+10, +11, +2, +100) so
// publish order, message-id order, and lexical ts order all disagree with true
// chronological order; only a fixed-width ts format sorts Pending oldest-first
// (spec §6).
func TestPendingOrdersByTrueChronologyNotLexicalTS(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	// One clock tick is consumed by Subscribe, one per Publish call (in this
	// exact non-monotonic order), and one more by Pending's delivered_at stamp.
	offsets := []time.Duration{10 * time.Nanosecond, 11 * time.Nanosecond, 2 * time.Nanosecond, 100 * time.Nanosecond}
	calls := []time.Time{base.Add(-time.Second)}
	for _, o := range offsets {
		calls = append(calls, base.Add(o))
	}
	calls = append(calls, base.Add(time.Second))
	i := 0
	clk := func() time.Time {
		tm := calls[i]
		i++
		return tm
	}
	b := New(s, clk)

	ch := ChatChannel("ord")
	if _, err := b.Subscribe("bob", ch, Matcher{}, nil); err != nil {
		t.Fatal(err)
	}

	texts := []string{"n10", "n11", "n2", "n100"} // published in this order
	for _, txt := range texts {
		if _, err := b.Publish(Message{Channel: ch, Type: "note", Text: txt, Source: "test"}); err != nil {
			t.Fatal(err)
		}
	}

	pend, err := b.Pending("bob", 10)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(pend))
	for i, m := range pend {
		got[i] = m.Text
	}
	want := []string{"n2", "n10", "n11", "n100"} // true chronological order
	if !equalStrs(got, want) {
		t.Fatalf("pending order = %v, want true chronological %v", got, want)
	}
}

// TestUnsubscribeScopedByAgent guards against a cross-agent authorization bug:
// subscription ids embed the owning agent's name (sub-<agent>-<ts>) and are
// therefore guessable, so Unsubscribe must scope its DELETE to the calling
// agent rather than trusting the id alone.
func TestUnsubscribeScopedByAgent(t *testing.T) {
	b := newBus(t)
	ch := InboxChannel("shared")
	subA, err := b.Subscribe("alice", ch, Matcher{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// bob (agent B) must not be able to remove alice's (agent A) subscription,
	// even though he knows/guesses her subscription id.
	if err := b.Unsubscribe("bob", subA.ID); err != ErrNotFound {
		t.Fatalf("cross-agent unsubscribe: err = %v, want ErrNotFound", err)
	}

	// alice's subscription must still exist and still receive deliveries.
	subs, err := b.ListSubscriptions("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].ID != subA.ID {
		t.Fatalf("alice's subscription was removed by bob's call: %+v", subs)
	}
	m := pub(t, b, ch, "any", "hello", nil)
	pend, err := b.Pending("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || pend[0].ID != m.ID {
		t.Fatalf("alice did not receive delivery after bob's failed unsubscribe: %+v", pend)
	}

	// alice unsubscribing her own subscription succeeds and removes it.
	if err := b.Unsubscribe("alice", subA.ID); err != nil {
		t.Fatalf("owner unsubscribe failed: %v", err)
	}
	subs, err = b.ListSubscriptions("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 0 {
		t.Fatalf("alice's subscription survived her own unsubscribe: %+v", subs)
	}
}

func TestChannelsAndTail(t *testing.T) {
	b := newBus(t)
	ch := ChatChannel("room")
	pub(t, b, ch, "a", "1", nil)
	pub(t, b, ch, "b", "2", nil)
	chans, _ := b.Channels()
	if len(chans) != 1 || chans[0].Name != ch || chans[0].Kind != "chat" {
		t.Fatalf("channels = %+v", chans)
	}
	tail, _ := b.Tail(ch, 10)
	if len(tail) != 2 || tail[0].Text != "1" || tail[1].Text != "2" {
		t.Fatalf("tail = %+v", tail)
	}
	_, count, err := b.InspectChannel(ch)
	if err != nil || count != 2 {
		t.Fatalf("inspect: count=%d err=%v", count, err)
	}
}

func TestPublishAssignsMonotonicWorkflowIngressSequence(t *testing.T) {
	b := newBus(t)
	for _, channel := range []string{"chat:z", "chat:a"} {
		if _, err := b.Publish(Message{Channel: channel, Source: "plugin:test", Type: "event"}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := b.db.Query(`SELECT s.sequence,m.channel FROM task_workflow_message_sequence s JOIN messages m ON m.id=s.message_id ORDER BY s.sequence`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var channels []string
	for rows.Next() {
		var sequence int64
		var channel string
		if err := rows.Scan(&sequence, &channel); err != nil {
			t.Fatal(err)
		}
		channels = append(channels, channel)
	}
	if len(channels) != 2 || channels[0] != "chat:z" || channels[1] != "chat:a" {
		t.Fatalf("sequenced channels = %#v", channels)
	}
}

func TestUnsubscribeChannel(t *testing.T) {
	b := newBus(t)
	if _, err := b.Subscribe("worker", "chat:room", nil, nil); err != nil {
		t.Fatal(err)
	}
	n, err := b.UnsubscribeChannel("worker", "chat:room")
	if err != nil || n != 1 {
		t.Fatalf("UnsubscribeChannel = (%d, %v), want (1, nil)", n, err)
	}
	// Idempotent: a second removal reports ErrNotFound, not success.
	if _, err := b.UnsubscribeChannel("worker", "chat:room"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second UnsubscribeChannel err = %v, want ErrNotFound", err)
	}
	// It must not touch another agent's identically-named subscription.
	if _, err := b.Subscribe("other", "chat:room", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UnsubscribeChannel("worker", "chat:room"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-agent leak: worker removal affected other; err = %v", err)
	}
	subs, _ := b.ListSubscriptions("other")
	if len(subs) != 1 {
		t.Fatalf("other lost its subscription: %+v", subs)
	}
}

// TestEnsureChannelMaterializesAndValidates proves EnsureChannel lists a channel
// with no message or subscription (dev-t-dbu.1 materialize-on-bind), is
// idempotent, and rejects a malformed name rather than seeding a junk row.
func TestEnsureChannelMaterializesAndValidates(t *testing.T) {
	b := newBus(t)
	if chans, _ := b.Channels(); len(chans) != 0 {
		t.Fatalf("fresh bus has channels: %+v", chans)
	}
	if err := b.EnsureChannel("chat:idle"); err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	chans, err := b.Channels()
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 1 || chans[0].Name != "chat:idle" || chans[0].Kind != "chat" {
		t.Fatalf("materialized channel = %+v, want one chat:idle of kind chat", chans)
	}
	// Idempotent: a second call neither errors nor duplicates the row.
	if err := b.EnsureChannel("chat:idle"); err != nil {
		t.Fatalf("EnsureChannel (repeat): %v", err)
	}
	if chans, _ := b.Channels(); len(chans) != 1 {
		t.Fatalf("EnsureChannel not idempotent: %+v", chans)
	}
	// A malformed name is rejected and seeds nothing.
	if err := b.EnsureChannel("Bad Channel"); err == nil {
		t.Fatal("EnsureChannel accepted a malformed name")
	}
	if chans, _ := b.Channels(); len(chans) != 1 {
		t.Fatalf("malformed name seeded a row: %+v", chans)
	}
}
