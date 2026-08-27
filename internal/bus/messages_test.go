package bus

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

// sub subscribes agent to channel with an all-match matcher (helper).
func sub(t *testing.T, b *Bus, agent, channel string) Subscription {
	t.Helper()
	s, err := b.Subscribe(agent, channel, Matcher{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// deliver publishes msg and drains agent's pending so its deliveries are marked
// delivered (attempts>0), mirroring a render into the prompt.
func deliverOne(t *testing.T, b *Bus, agent, channel, text string) Message {
	t.Helper()
	m := pub(t, b, channel, "note", text, nil)
	if _, err := b.Pending(agent, 10); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInboxStatusFiltering(t *testing.T) {
	b := newBusSeconds(t)
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)

	m1 := deliverOne(t, b, "bob", ch, "one")
	m2 := deliverOne(t, b, "bob", ch, "two")

	// Both pending initially.
	pend, err := b.Inbox("bob", "pending", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 2 {
		t.Fatalf("pending=%d want 2", len(pend))
	}
	// Newest-first: m2 before m1.
	if pend[0].ID != m2.ID || pend[1].ID != m1.ID {
		t.Fatalf("order: got %s,%s want %s,%s", pend[0].ID, pend[1].ID, m2.ID, m1.ID)
	}

	// Process m1 → moves to archive.
	if _, err := b.MarkProcessed("bob", m1.ID, "handled"); err != nil {
		t.Fatal(err)
	}
	pend, _ = b.Inbox("bob", "pending", 100, "")
	if len(pend) != 1 || pend[0].ID != m2.ID {
		t.Fatalf("pending after process: %+v", pend)
	}
	arch, _ := b.Inbox("bob", "processed", 100, "")
	if len(arch) != 1 || arch[0].ID != m1.ID || arch[0].Result != "handled" || arch[0].ProcessedAt == "" {
		t.Fatalf("archive: %+v", arch)
	}
	all, _ := b.Inbox("bob", "all", 100, "")
	if len(all) != 2 {
		t.Fatalf("all=%d want 2", len(all))
	}
}

func TestInboxBeforeCursor(t *testing.T) {
	b := newBusSeconds(t)
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	m1 := deliverOne(t, b, "bob", ch, "one")
	m2 := deliverOne(t, b, "bob", ch, "two")
	m3 := deliverOne(t, b, "bob", ch, "three")

	// Page 1: newest, limit 2 → m3, m2.
	p1, _ := b.Inbox("bob", "all", 2, "")
	if len(p1) != 2 || p1[0].ID != m3.ID || p1[1].ID != m2.ID {
		t.Fatalf("page1: %+v", p1)
	}
	// Page 2: before the last returned id → m1.
	p2, _ := b.Inbox("bob", "all", 2, p1[1].ID)
	if len(p2) != 1 || p2[0].ID != m1.ID {
		t.Fatalf("page2: %+v", p2)
	}
}

// TestInboxBeforeCursorCrossChannel is the regression for P8-F1: message ids are
// channel-prefixed, so a plain `id < before` cursor drops older rows that belong
// to a lexically-later channel. Here the OLDER message lives in "zebra" (whose id
// sorts AFTER "alpha"); paging before the newer alpha message must still surface
// it, which the composite (ts, id) cursor guarantees.
func TestInboxBeforeCursorCrossChannel(t *testing.T) {
	b := newBusSeconds(t)
	sub(t, b, "bob", "zebra")
	sub(t, b, "bob", "alpha")
	// zebra published first → strictly older ts; alpha published second → newer.
	older := deliverOne(t, b, "bob", "zebra", "old")
	newer := deliverOne(t, b, "bob", "alpha", "new")
	if !(older.TS < newer.TS) {
		t.Fatalf("setup: older.TS %q should be < newer.TS %q", older.TS, newer.TS)
	}
	if !(older.ID > newer.ID) {
		t.Fatalf("setup: expected zebra id %q to sort lexically AFTER alpha id %q", older.ID, newer.ID)
	}
	// Page before the newer (alpha) message: the older zebra row must appear.
	page, err := b.Inbox("bob", "all", 10, newer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].ID != older.ID {
		t.Fatalf("before page dropped cross-channel older row: got %+v want [%s]", page, older.ID)
	}
}

func TestMarkProcessedRejectsEmptyResult(t *testing.T) {
	b := newBusSeconds(t)
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	m := deliverOne(t, b, "bob", ch, "x")
	if _, err := b.MarkProcessed("bob", m.ID, ""); err == nil {
		t.Fatal("empty result must be rejected")
	}
	if _, err := b.MarkProcessed("bob", m.ID, "   "); err == nil {
		t.Fatal("whitespace-only result must be rejected")
	}
	// Still pending.
	pend, _ := b.Inbox("bob", "pending", 10, "")
	if len(pend) != 1 {
		t.Fatalf("should stay pending: %+v", pend)
	}
}

func TestMarkProcessedIdempotentAndAudit(t *testing.T) {
	b := newBusSeconds(t)
	var audits []string
	b.SetAuditHook(func(agent, kind, data string) { audits = append(audits, kind) })
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	m := deliverOne(t, b, "bob", ch, "x")

	it1, err := b.MarkProcessed("bob", m.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	// Re-process: no change, returns the existing record (result unchanged).
	it2, err := b.MarkProcessed("bob", m.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if it2.Result != "first" || it2.ProcessedAt != it1.ProcessedAt {
		t.Fatalf("idempotency broken: %+v vs %+v", it1, it2)
	}
	// Exactly one audit event (only the transition).
	if len(audits) != 1 || audits[0] != "message_processed" {
		t.Fatalf("audits=%v want one message_processed", audits)
	}
	// Unknown message for this agent → ErrNotFound.
	if _, err := b.MarkProcessed("bob", "nope-000", "r"); err != ErrNotFound {
		t.Fatalf("unknown msg: err=%v want ErrNotFound", err)
	}
}

func TestMarkProcessedAllAgentDeliveries(t *testing.T) {
	// bob subscribes twice (two matchers) → two deliveries of one message;
	// processing marks both, and the inbox collapses to a single archived item.
	b := newBusSeconds(t)
	ch := ChatChannel("room")
	if _, err := b.Subscribe("bob", ch, Matcher{"type": "note"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Subscribe("bob", ch, Matcher{"source": "test"}, nil); err != nil {
		t.Fatal(err)
	}
	m := pub(t, b, ch, "note", "hi", nil)
	if _, err := b.Pending("bob", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := b.MarkProcessed("bob", m.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if pend, _ := b.Inbox("bob", "pending", 10, ""); len(pend) != 0 {
		t.Fatalf("both deliveries should be processed: %+v", pend)
	}
	arch, _ := b.Inbox("bob", "processed", 10, "")
	if len(arch) != 1 {
		t.Fatalf("archive should collapse to one item: %+v", arch)
	}
}

func TestMarkProcessedAllowedOnDLQ(t *testing.T) {
	b := newBusSeconds(t)
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	m := pub(t, b, ch, "note", "x", nil)
	// Exhaust attempts → DLQ (maxAttempts renders).
	for range maxAttempts + 1 {
		if _, err := b.Pending("bob", 10); err != nil {
			t.Fatal(err)
		}
	}
	dlq, _ := b.Inbox("bob", "dlq", 10, "")
	if len(dlq) != 1 || dlq[0].ID != m.ID {
		t.Fatalf("expected DLQ item: %+v", dlq)
	}
	// Operator can still process a DLQ'd delivery.
	if _, err := b.MarkProcessed("bob", m.ID, "operator: cleaned"); err != nil {
		t.Fatal(err)
	}
	arch, _ := b.Inbox("bob", "processed", 10, "")
	if len(arch) != 1 || arch[0].Result != "operator: cleaned" {
		t.Fatalf("archive: %+v", arch)
	}
}

func TestRequeue(t *testing.T) {
	b := newBusSeconds(t)
	var audits []string
	b.SetAuditHook(func(a, k, d string) { audits = append(audits, k) })
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	m := pub(t, b, ch, "note", "x", nil)
	for range maxAttempts + 1 {
		b.Pending("bob", 10)
	}
	if dlq, _ := b.Inbox("bob", "dlq", 10, ""); len(dlq) != 1 {
		t.Fatalf("precondition DLQ: %+v", dlq)
	}
	if err := b.Requeue("bob", m.ID); err != nil {
		t.Fatal(err)
	}
	// Back to pending with attempts reset.
	pend, _ := b.Inbox("bob", "pending", 10, "")
	if len(pend) != 1 || pend[0].ID != m.ID || pend[0].Attempts != 0 || pend[0].DLQ {
		t.Fatalf("after requeue: %+v", pend)
	}
	if len(audits) != 1 || audits[0] != "message_requeued" {
		t.Fatalf("audits=%v", audits)
	}
	// Unknown → ErrNotFound.
	if err := b.Requeue("bob", "nope-000"); err != ErrNotFound {
		t.Fatalf("requeue unknown: %v", err)
	}
}

// TestProcessThenRequeueRedrains is the regression for P8-F3: a process-then-
// requeue must return the message to the drainable queue. MarkProcessed sets
// acked_at/processed_at/result; Requeue only cleared dlq/attempts, leaving
// acked_at non-NULL so Pending() (WHERE acked_at IS NULL) never re-drained it —
// the inbox showed it pending forever while the agent never received it again.
func TestProcessThenRequeueRedrains(t *testing.T) {
	b := newBusSeconds(t)
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	m := deliverOne(t, b, "bob", ch, "x")
	if _, err := b.MarkProcessed("bob", m.ID, "handled"); err != nil {
		t.Fatal(err)
	}
	if err := b.Requeue("bob", m.ID); err != nil {
		t.Fatal(err)
	}
	// Inbox must show it pending again (not processed, not dlq).
	pend, _ := b.Inbox("bob", "pending", 10, "")
	if len(pend) != 1 || pend[0].ID != m.ID || pend[0].ProcessedAt != "" || pend[0].Result != "" || pend[0].DLQ {
		t.Fatalf("requeued item must be clean pending: %+v", pend)
	}
	if arch, _ := b.Inbox("bob", "processed", 10, ""); len(arch) != 0 {
		t.Fatalf("requeue must clear processed state, archive=%+v", arch)
	}
	// And Pending() must actually re-drain it — the whole point of requeue.
	drained, err := b.Pending("bob", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(drained) != 1 || drained[0].ID != m.ID {
		t.Fatalf("Pending must re-drain the requeued message: %+v", drained)
	}
}

// TestDLQProcessedInExactlyOneTab is the regression for P8-F6: processing a
// DLQ'd delivery (operator archive cleanup) must move it to exactly one operator
// tab. Before the fix MarkProcessed set processed_at without clearing dlq, so the
// row matched BOTH the 'processed' and 'dlq' HAVING filters and appeared twice.
func TestDLQProcessedInExactlyOneTab(t *testing.T) {
	b := newBusSeconds(t)
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	m := pub(t, b, ch, "note", "x", nil)
	for range maxAttempts + 1 {
		if _, err := b.Pending("bob", 10); err != nil {
			t.Fatal(err)
		}
	}
	if dlq, _ := b.Inbox("bob", "dlq", 10, ""); len(dlq) != 1 {
		t.Fatalf("precondition DLQ: %+v", dlq)
	}
	if _, err := b.MarkProcessed("bob", m.ID, "operator: cleaned"); err != nil {
		t.Fatal(err)
	}
	// Now processed, and NO LONGER in the DLQ tab — exactly one terminal tab.
	if dlq, _ := b.Inbox("bob", "dlq", 10, ""); len(dlq) != 0 {
		t.Fatalf("processed DLQ row must leave the dlq tab: %+v", dlq)
	}
	arch, _ := b.Inbox("bob", "processed", 10, "")
	if len(arch) != 1 || arch[0].ID != m.ID || arch[0].DLQ {
		t.Fatalf("processed tab: %+v", arch)
	}
}

// TestMultiSubPartialDLQConsistent is the regression for P8-F5: when one of an
// agent's several deliveries of a single message dead-letters but another is
// still live, Inbox and Pending must agree. The message-level DLQ rule is
// per-delivery-all-dead (MIN(dlq)=1): a message counts as DLQ'd only when EVERY
// delivery is dead, matching Pending() which keeps draining any live delivery.
// (Divergent per-delivery attempts only arise from an internal split, so this
// white-box test sets the delivery dlq flags directly.)
func TestMultiSubPartialDLQConsistent(t *testing.T) {
	b := newBusSeconds(t)
	ch := ChatChannel("room")
	s1, err := b.Subscribe("bob", ch, Matcher{"type": "note"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Subscribe("bob", ch, Matcher{"source": "test"}, nil); err != nil {
		t.Fatal(err)
	}
	m := pub(t, b, ch, "note", "hi", nil)
	if _, err := b.Pending("bob", 10); err != nil {
		t.Fatal(err)
	}
	// Dead-letter ONLY the first subscription's delivery; the other stays live.
	if _, err := b.db.Exec(`UPDATE deliveries SET dlq=1 WHERE subscription_id=? AND message_id=?`, s1.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	// Partial DLQ: not dead yet. Inbox must NOT show it in dlq, and must show it
	// pending — consistent with Pending() still draining the live delivery.
	if dlq, _ := b.Inbox("bob", "dlq", 10, ""); len(dlq) != 0 {
		t.Fatalf("partial DLQ must not flag the message dead: %+v", dlq)
	}
	if pend, _ := b.Inbox("bob", "pending", 10, ""); len(pend) != 1 || pend[0].ID != m.ID {
		t.Fatalf("partial DLQ must stay pending: %+v", pend)
	}
	if drained, _ := b.Pending("bob", 10); len(drained) != 1 || drained[0].ID != m.ID {
		t.Fatalf("Pending must still drain the live delivery: %+v", drained)
	}
	// Now dead-letter the second delivery too: every delivery dead → dlq tab, and
	// Pending() stops draining. Inbox and Pending agree the message is dead.
	if _, err := b.db.Exec(`UPDATE deliveries SET dlq=1 WHERE message_id=?`, m.ID); err != nil {
		t.Fatal(err)
	}
	if dlq, _ := b.Inbox("bob", "dlq", 10, ""); len(dlq) != 1 || dlq[0].ID != m.ID {
		t.Fatalf("all deliveries dead → dlq tab: %+v", dlq)
	}
	if pend, _ := b.Inbox("bob", "pending", 10, ""); len(pend) != 0 {
		t.Fatalf("fully DLQ'd message must leave pending: %+v", pend)
	}
	if drained, _ := b.Pending("bob", 10); len(drained) != 0 {
		t.Fatalf("Pending must not drain a fully DLQ'd message: %+v", drained)
	}
}

func TestReplyRoutingReplyTo(t *testing.T) {
	// Rule 1: explicit reply_to wins.
	b := newBusSeconds(t)
	target := ChatChannel("back")
	orig, err := b.Publish(Message{Channel: ChatChannel("room"), Type: "q", Text: "?",
		Source: "plugin:issue-provider", ReplyTo: target})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := b.Reply("alice", orig.ID, "answer", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Channel != target {
		t.Fatalf("reply channel=%s want %s", reply.Channel, target)
	}
	if reply.Kind != "reply" || reply.InReplyTo != orig.ID || reply.Type != "q" {
		t.Fatalf("reply fields: %+v", reply)
	}
	// correlation_id defaults to orig id when the original had none.
	if reply.CorrelationID != orig.ID {
		t.Fatalf("correlation=%s want %s", reply.CorrelationID, orig.ID)
	}
}

func TestReplyRoutingSourceAgent(t *testing.T) {
	// Rule 2: no reply_to, source is agent:<n> → that agent's inbox.
	b := newBusSeconds(t)
	orig, err := b.Publish(Message{Channel: ChatChannel("room"), Type: "q", Text: "?",
		Source: "agent:carol"})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := b.Reply("alice", orig.ID, "answer", nil, "over")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Channel != InboxChannel("carol") {
		t.Fatalf("reply channel=%s want %s", reply.Channel, InboxChannel("carol"))
	}
	if reply.Type != "over" {
		t.Fatalf("typeOverride ignored: %s", reply.Type)
	}
}

func TestReplyRoutingOriginChannel(t *testing.T) {
	// Rule 3: no reply_to, non-agent source → originating channel.
	b := newBusSeconds(t)
	room := ChatChannel("room")
	orig, err := b.Publish(Message{Channel: room, Type: "q", Text: "?", Source: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := b.Reply("alice", orig.ID, "answer", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Channel != room {
		t.Fatalf("reply channel=%s want %s", reply.Channel, room)
	}
}

func TestReplyInheritsSubjectMinusWatch(t *testing.T) {
	// A reply inherits every subject key of the original except 'watch', so sink
	// plugins can route it back via subject.chat_id/message_id (§4.1).
	b := newBusSeconds(t)
	orig, err := b.Publish(Message{Channel: ChatChannel("room"), Type: "q", Text: "?",
		Source:  "operator",
		Subject: map[string]any{"chat_id": "c1", "message_id": "m1", "watch": "w1"}})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := b.Reply("alice", orig.ID, "answer", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := reply.Subject["chat_id"]; got != "c1" {
		t.Fatalf("reply chat_id=%v want c1", got)
	}
	if got := reply.Subject["message_id"]; got != "m1" {
		t.Fatalf("reply message_id=%v want m1", got)
	}
	if _, ok := reply.Subject["watch"]; ok {
		t.Fatalf("reply subject retained watch: %+v", reply.Subject)
	}
}

func TestReplyWatchOnlySubjectStaysEmpty(t *testing.T) {
	// An original whose only subject key is 'watch' yields a subject-less reply.
	b := newBusSeconds(t)
	orig, err := b.Publish(Message{Channel: ChatChannel("room"), Type: "q", Text: "?",
		Source: "operator", Subject: map[string]any{"watch": "w1"}})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := b.Reply("alice", orig.ID, "answer", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Subject) != 0 {
		t.Fatalf("reply subject=%+v want empty", reply.Subject)
	}
}

func TestReplyAutoProcesses(t *testing.T) {
	b := newBusSeconds(t)
	var audits []string
	b.SetAuditHook(func(a, k, d string) { audits = append(audits, k) })
	ch := InboxChannel("bob")
	sub(t, b, "bob", ch)
	// bob receives a message and replies to it → his delivery auto-processes.
	m := deliverOne(t, b, "bob", ch, "please answer")
	reply, err := b.Reply("bob", m.ID, "here you go", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := b.Inbox("bob", "processed", 10, "")
	if len(arch) != 1 || arch[0].ID != m.ID || arch[0].Result != "replied: "+reply.ID {
		t.Fatalf("auto-process: %+v", arch)
	}
	// A later explicit processed is a no-op (already processed).
	it, err := b.MarkProcessed("bob", m.ID, "again")
	if err != nil {
		t.Fatal(err)
	}
	if it.Result != "replied: "+reply.ID {
		t.Fatalf("explicit processed after reply should be a no-op: %s", it.Result)
	}
	// Audit: message_processed (from auto-process) then message_replied.
	if len(audits) != 2 || audits[0] != "message_processed" || audits[1] != "message_replied" {
		t.Fatalf("audits=%v", audits)
	}
}

func TestReplyByNonSubscriberDoesNotError(t *testing.T) {
	// An operator (no delivery of the original) can reply; the missing
	// auto-process is silent, not an error.
	b := newBusSeconds(t)
	orig, err := b.Publish(Message{Channel: ChatChannel("room"), Type: "q", Text: "?", Source: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Reply("operator", orig.ID, "answer", nil, ""); err != nil {
		t.Fatalf("reply by non-subscriber: %v", err)
	}
}

func TestReplyUnknownMessage(t *testing.T) {
	b := newBusSeconds(t)
	if _, err := b.Reply("alice", "nope-000", "x", nil, ""); err != ErrNotFound {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestRequestAndPending(t *testing.T) {
	b := newBusSeconds(t)
	target := GroupDirect("dev", "carol")
	req, err := b.Request("alice", target, "can you help?", "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != "request" || req.ReplyTo != InboxChannel("alice") || req.CorrelationID != req.ID {
		t.Fatalf("request fields: %+v", req)
	}
	// alice has one pending request.
	pend, err := b.PendingRequests("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || pend[0].ID != req.ID {
		t.Fatalf("pending requests: %+v", pend)
	}
	// A reply on the request retires it from pending.
	if _, err := b.Reply("carol", req.ID, "sure", nil, ""); err != nil {
		t.Fatal(err)
	}
	pend, _ = b.PendingRequests("alice")
	if len(pend) != 0 {
		t.Fatalf("request should be answered: %+v", pend)
	}
}

func TestRequestDeadlineArmsAndReplyCancels(t *testing.T) {
	b := newBusSeconds(t)
	var armed [][3]string
	var cancelled []string
	b.SetDeadlineHooks(
		func(_ *sql.Tx, agent, corr, dl string) error {
			armed = append(armed, [3]string{agent, corr, dl})
			return nil
		},
		func(_ *sql.Tx, corr string) error { cancelled = append(cancelled, corr); return nil },
	)
	req, err := b.Request("alice", GroupDirect("dev", "carol"), "help", "2026-07-06T10:05:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(armed) != 1 || armed[0][0] != "alice" || armed[0][1] != req.CorrelationID || armed[0][2] != "2026-07-06T10:05:00Z" {
		t.Fatalf("armDeadline args: %+v", armed)
	}
	// Reply cancels the deadline for the correlation id.
	if _, err := b.Reply("carol", req.ID, "ok", nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 1 || cancelled[0] != req.CorrelationID {
		t.Fatalf("cancelDeadline args: %+v", cancelled)
	}
}

func TestRequestNoDeadlineDoesNotArm(t *testing.T) {
	b := newBusSeconds(t)
	armedCount := 0
	b.SetDeadlineHooks(func(*sql.Tx, string, string, string) error { armedCount++; return nil }, nil)
	if _, err := b.Request("alice", ChatChannel("x"), "hi", ""); err != nil {
		t.Fatal(err)
	}
	if armedCount != 0 {
		t.Fatalf("no deadline should not arm: %d", armedCount)
	}
}

func TestRequestDeadlineWithoutHookIsRejected(t *testing.T) {
	// No SetDeadlineHooks wired: a --deadline request must fail honestly (P8-F2)
	// rather than return success with no timeout ever armed. And it must not
	// publish a request message when it can't honour the deadline.
	b := newBusSeconds(t)
	_, err := b.Request("alice", ChatChannel("x"), "hi", "2026-07-06T10:05:00Z")
	if !errors.Is(err, ErrDeadlineUnsupported) {
		t.Fatalf("want ErrDeadlineUnsupported, got %v", err)
	}
	pend, _ := b.PendingRequests("alice")
	if len(pend) != 0 {
		t.Fatalf("rejected request must not be published: %+v", pend)
	}
}

func TestRequestDeadlineValidatedBeforePublish(t *testing.T) {
	// F1 regression: a malformed deadline must be rejected BEFORE the request is
	// published, so no live, timeout-less request is orphaned in the member inbox
	// (a retry would then duplicate it). A well-formed deadline still publishes
	// and arms exactly once.
	b := newBusSeconds(t)
	armed := 0
	b.SetDeadlineHooks(func(*sql.Tx, string, string, string) error { armed++; return nil }, nil)
	b.SetDeadlineValidator(func(d string) error { _, err := time.ParseDuration(d); return err })

	// "5min" is not a Go duration (5m is): reject, arm nothing, and leave no
	// request behind in the inbox.
	if _, err := b.Request("alice", GroupDirect("dev", "carol"), "help", "5min"); err == nil {
		t.Fatal("want error for unparseable deadline, got nil")
	}
	if armed != 0 {
		t.Fatalf("malformed deadline must not arm: %d", armed)
	}
	if pend, _ := b.PendingRequests("alice"); len(pend) != 0 {
		t.Fatalf("rejected request must not be published: %+v", pend)
	}

	// A valid duration still goes through: published once and armed once.
	req, err := b.Request("alice", GroupDirect("dev", "carol"), "help", "5m")
	if err != nil {
		t.Fatal(err)
	}
	if armed != 1 {
		t.Fatalf("valid deadline should arm once: %d", armed)
	}
	if pend, _ := b.PendingRequests("alice"); len(pend) != 1 || pend[0].ID != req.ID {
		t.Fatalf("valid request should be pending: %+v", pend)
	}
}

func TestRequestRollsBackWhenDeadlineArmFails(t *testing.T) {
	b := newBusSeconds(t)
	b.SetDeadlineHooks(func(*sql.Tx, string, string, string) error { return errors.New("arm failed") }, nil)
	if _, err := b.Request("alice", GroupDirect("dev", "carol"), "help", "5m"); err == nil {
		t.Fatal("request succeeded despite deadline arm failure")
	}
	if pending, _ := b.PendingRequests("alice"); len(pending) != 0 {
		t.Fatalf("failed request was committed: %+v", pending)
	}
}

func TestReplyRollsBackWhenDeadlineCancelFails(t *testing.T) {
	b := newBusSeconds(t)
	target := GroupDirect("dev", "carol")
	sub(t, b, "carol", target)
	req, err := b.Request("alice", target, "help", "")
	if err != nil {
		t.Fatal(err)
	}
	b.SetDeadlineHooks(nil, func(*sql.Tx, string) error { return errors.New("cancel failed") })
	if _, err := b.Reply("carol", req.ID, "ok", nil, ""); err == nil {
		t.Fatal("reply succeeded despite deadline cancel failure")
	}
	if pending, _ := b.PendingRequests("alice"); len(pending) != 1 || pending[0].ID != req.ID {
		t.Fatalf("failed reply changed request state: %+v", pending)
	}
	items, err := b.Inbox("carol", "pending", 10, "")
	if err != nil || len(items) != 1 || items[0].ID != req.ID {
		t.Fatalf("failed reply processed original: %+v err=%v", items, err)
	}
}

func TestComputeWatchDeterministicAndKeyed(t *testing.T) {
	// Same params (regardless of key order) → same watch; different channel or
	// params → different watch; empty params → empty watch.
	p1 := map[string]any{"query": "PROJ", "limit": float64(5)}
	p2 := map[string]any{"limit": float64(5), "query": "PROJ"}
	w1, _ := ComputeWatch("issue-provider:query", p1)
	w2, _ := ComputeWatch("issue-provider:query", p2)
	if w1 == "" || w1 != w2 {
		t.Fatalf("watch should be order-independent: %q vs %q", w1, w2)
	}
	if len(w1) != 16 {
		t.Fatalf("watch len=%d want 16", len(w1))
	}
	w3, _ := ComputeWatch("issue-provider:query", map[string]any{"query": "OTHER"})
	if w3 == w1 {
		t.Fatal("different params must differ")
	}
	w4, _ := ComputeWatch("other:query", p1)
	if w4 == w1 {
		t.Fatal("different channel must differ")
	}
	if w, _ := ComputeWatch("issue-provider:query", nil); w != "" {
		t.Fatalf("empty params → empty watch, got %q", w)
	}
}

func TestSubscribeParamsWatchDedupAndIsolation(t *testing.T) {
	b := newBusSeconds(t)
	ch := "issue-provider:query"
	pA := map[string]any{"query": "PROJ-A"}
	pB := map[string]any{"query": "PROJ-B"}

	// Two agents, same params → same watch → one unit of demand.
	s1, err := b.SubscribeParams("alice", ch, Matcher{}, nil, pA)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := b.SubscribeParams("bob", ch, Matcher{}, nil, pA)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Watch == "" || s1.Watch != s2.Watch {
		t.Fatalf("same params → same watch: %q vs %q", s1.Watch, s2.Watch)
	}
	if s1.Matcher["subject.watch"] != s1.Watch {
		t.Fatalf("watch not auto-injected into matcher: %+v", s1.Matcher)
	}
	// A third agent with different params → different watch.
	s3, err := b.SubscribeParams("carol", ch, Matcher{}, nil, pB)
	if err != nil {
		t.Fatal(err)
	}
	if s3.Watch == s1.Watch {
		t.Fatal("different params must yield a different watch")
	}

	// WatchList: two distinct watches; the shared one has two subscribers.
	wl, err := b.WatchList(ch)
	if err != nil {
		t.Fatal(err)
	}
	if len(wl) != 2 {
		t.Fatalf("watch list=%d want 2", len(wl))
	}
	subsByWatch := map[string]int{}
	for _, w := range wl {
		subsByWatch[w.Watch] = len(w.Subscribers)
	}
	if subsByWatch[s1.Watch] != 2 || subsByWatch[s3.Watch] != 1 {
		t.Fatalf("subscriber counts: %+v", subsByWatch)
	}

	// Delivery isolation: a provider message stamped for watch A reaches only
	// the two watch-A subscribers, never the watch-B one.
	if _, err := b.Publish(Message{Channel: ch, Type: "ticket.updated", Source: "plugin:issue-provider",
		Subject: map[string]any{"watch": s1.Watch}, Text: "A moved"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := b.Pending("alice", 10); len(p) != 1 {
		t.Fatalf("alice (watch A) should get the message: %+v", p)
	}
	if p, _ := b.Pending("bob", 10); len(p) != 1 {
		t.Fatalf("bob (watch A) should get the message: %+v", p)
	}
	if p, _ := b.Pending("carol", 10); len(p) != 0 {
		t.Fatalf("carol (watch B) must NOT get the watch-A message: %+v", p)
	}
}

func TestSubscribeParamsEmptyIsPlainSubscribe(t *testing.T) {
	b := newBusSeconds(t)
	s, err := b.SubscribeParams("alice", ChatChannel("x"), Matcher{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Watch != "" || s.Params != nil {
		t.Fatalf("empty params → plain subscription: %+v", s)
	}
}

func TestSubscribeParamsRunsValidator(t *testing.T) {
	b := newBusSeconds(t)
	var gotChannel string
	var gotParams map[string]any
	b.SetParamsValidator(func(channel string, params map[string]any) error {
		gotChannel, gotParams = channel, params
		if params["bad"] == true {
			return errors.New("bad params")
		}
		return nil
	})

	// A validator error fails the subscribe and persists no row.
	if _, err := b.SubscribeParams("alice", "plugin:issue-provider:query", Matcher{}, nil, map[string]any{"bad": true}); err == nil {
		t.Fatal("expected validator to fail subscribe")
	}
	if gotChannel != "plugin:issue-provider:query" || gotParams["bad"] != true {
		t.Fatalf("validator saw channel=%q params=%v", gotChannel, gotParams)
	}
	subs, err := b.ListSubscriptions("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 0 {
		t.Fatalf("failed subscribe must not persist a row, got %d", len(subs))
	}

	// A passing validator lets the subscribe through normally.
	if _, err := b.SubscribeParams("alice", "plugin:issue-provider:query", Matcher{}, nil, map[string]any{"ok": true}); err != nil {
		t.Fatalf("valid params should subscribe: %v", err)
	}

	// The empty-params path never invokes the validator.
	gotChannel = ""
	if _, err := b.SubscribeParams("bob", ChatChannel("y"), Matcher{}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotChannel != "" {
		t.Fatalf("empty params must not call the validator, saw %q", gotChannel)
	}
}

// TestSubscriptionHookFires verifies the §6.2 seam after review findings #3/#6:
// the hook fires only for changes that can actually alter a channel's provider
// watch list — a watch-bearing (parameterized) subscribe, or an unsubscribe that
// removed a watch-bearing row. Plain (watch-less) subscribes/unsubscribes,
// idempotent no-ops, and failed ops never fire it, so the core subscribe path
// never touches the plugin store.
func TestSubscriptionHookFires(t *testing.T) {
	b := newBus(t)
	var fired []string
	b.SetSubscriptionHook(func(channel string) { fired = append(fired, channel) })

	// Plain Subscribe (no params → no watch) → NO fire: cannot change any watch list.
	sPlain, err := b.Subscribe("alice", "issue-provider:query", Matcher{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Idempotent re-subscribe (same agent/channel/matcher) → no new row, no fire.
	if _, err := b.Subscribe("alice", "issue-provider:query", Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	// Empty-params SubscribeParams is a plain subscribe → still NO fire.
	if _, err := b.SubscribeParams("carol", "issue-provider:query", Matcher{}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 {
		t.Fatalf("watch-less subscribes must not fire, got %v", fired)
	}

	// Parameterized subscribe (creates a watch) → fires once with the channel.
	if _, err := b.SubscribeParams("bob", "issue-provider:query", Matcher{}, nil, map[string]any{"q": "X"}); err != nil {
		t.Fatal(err)
	}
	if !equalStrs(fired, []string{"issue-provider:query"}) {
		t.Fatalf("param subscribe fired=%v", fired)
	}
	// Idempotent param re-subscribe (same agent/params → existing row) → no fire.
	fired = nil
	if _, err := b.SubscribeParams("bob", "issue-provider:query", Matcher{}, nil, map[string]any{"q": "X"}); err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 {
		t.Fatalf("duplicate param subscribe must not fire, got %v", fired)
	}

	// Unsubscribe the PLAIN sub by id → watch-less removal → NO fire.
	fired = nil
	if err := b.Unsubscribe("alice", sPlain.ID); err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 {
		t.Fatalf("watch-less unsubscribe must not fire, got %v", fired)
	}
	// Unsubscribing a missing id → ErrNotFound, no fire.
	fired = nil
	if err := b.Unsubscribe("alice", "sub-nope"); err != ErrNotFound {
		t.Fatalf("missing unsubscribe err=%v", err)
	}
	if len(fired) != 0 {
		t.Fatalf("failed unsubscribe must not fire, got %v", fired)
	}

	// UnsubscribeChannel removing bob's watch-bearing sub → fires with the channel.
	fired = nil
	if _, err := b.UnsubscribeChannel("bob", "issue-provider:query"); err != nil {
		t.Fatal(err)
	}
	if !equalStrs(fired, []string{"issue-provider:query"}) {
		t.Fatalf("param UnsubscribeChannel fired=%v", fired)
	}

	// UnsubscribeChannel over a channel of only plain subs → rows removed, NO fire.
	fired = nil
	if _, err := b.Subscribe("dave", ChatChannel("x"), Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	if n, err := b.UnsubscribeChannel("dave", ChatChannel("x")); err != nil || n != 1 {
		t.Fatalf("UnsubscribeChannel n=%d err=%v", n, err)
	}
	if len(fired) != 0 {
		t.Fatalf("watch-less UnsubscribeChannel must not fire, got %v", fired)
	}
	// No-op UnsubscribeChannel (nothing to remove) → ErrNotFound, no fire.
	fired = nil
	if _, err := b.UnsubscribeChannel("nobody", ChatChannel("x")); err != ErrNotFound {
		t.Fatalf("empty UnsubscribeChannel err=%v", err)
	}
	if len(fired) != 0 {
		t.Fatalf("no-op UnsubscribeChannel must not fire, got %v", fired)
	}
}
