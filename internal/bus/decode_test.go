package bus

import (
	"strings"
	"testing"
)

// breakMessage stores JSON that does not decode in a message row.
func breakMessage(t *testing.T, b *Bus, id string) {
	t.Helper()
	if _, err := b.db.Exec(`UPDATE messages SET data='{broken' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
}

// A message row that does not decode is an error naming the message, not a
// message with empty data.
func TestBrokenMessageRowIsAnError(t *testing.T) {
	t.Parallel()
	b, chat := chatFixture(t)
	m := publish(t, b, Message{Channel: chat.Channel, Source: "agent:worker", Type: "message", Text: "x"})
	breakMessage(t, b, m.ID)
	if _, err := b.Tail(chat.Channel, 10); err == nil || !strings.Contains(err.Error(), m.ID) {
		t.Fatalf("Tail err=%v, want a decode error naming %s", err, m.ID)
	}
	if _, err := b.ChatFeed(chat, "user:customer", DefaultChatTypes, 0, ""); err == nil || !strings.Contains(err.Error(), m.ID) {
		t.Fatalf("ChatFeed err=%v, want a decode error naming %s", err, m.ID)
	}
}

// Pending moves a delivery whose message does not decode to the DLQ with an
// audit event and still hands out the other messages, so one broken row does
// not stop the queue.
func TestPendingDeadLettersBrokenMessage(t *testing.T) {
	t.Parallel()
	b := newBusSeconds(t)
	var audits []string
	b.SetAuditHook(func(agent, kind, data string) { audits = append(audits, agent+" "+kind+" "+data) })
	ch := ChatChannel("broken")
	sub(t, b, "bob", ch)
	broken := pub(t, b, ch, "note", "broken", nil)
	good := pub(t, b, ch, "note", "good", nil)
	breakMessage(t, b, broken.ID)

	got, err := b.Pending("bob", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != good.ID {
		t.Fatalf("pending=%v, want only %s", ids(got), good.ID)
	}
	var dlq int
	var result string
	if err := b.db.QueryRow(`SELECT dlq, result FROM deliveries WHERE message_id=?`, broken.ID).Scan(&dlq, &result); err != nil {
		t.Fatal(err)
	}
	if dlq != 1 || result != "decode_error" {
		t.Fatalf("broken delivery dlq=%d result=%q, want 1/decode_error", dlq, result)
	}
	if len(audits) != 1 || !strings.Contains(audits[0], "bob message_dead_lettered") || !strings.Contains(audits[0], broken.ID) {
		t.Fatalf("audits=%v", audits)
	}
}

func TestBrokenSubscriptionMatcherIsAnError(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	s := sub(t, b, "bob", ChatChannel("m"))
	if _, err := b.db.Exec(`UPDATE subscriptions SET matcher='{broken' WHERE id=?`, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ListSubscriptions("bob"); err == nil || !strings.Contains(err.Error(), s.ID) {
		t.Fatalf("ListSubscriptions err=%v, want a decode error naming %s", err, s.ID)
	}
}
