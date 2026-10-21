package bus

import (
	"fmt"
	"testing"
)

func deliveryState(t *testing.T, b *Bus, msgID string) (attempts int, dlq int, acked bool) {
	t.Helper()
	var ackedAt *string
	if err := b.db.QueryRow(`SELECT attempts, dlq, acked_at FROM deliveries WHERE message_id=?`, msgID).
		Scan(&attempts, &dlq, &ackedAt); err != nil {
		t.Fatal(err)
	}
	return attempts, dlq, ackedAt != nil
}

// A failed accounting UPDATE must reach the caller and leave every delivery
// untouched, so the DLQ counter never drifts from what was actually handed out.
func TestPendingAccountingFailureIsReturnedAndAtomic(t *testing.T) {
	t.Parallel()
	b := newBusSeconds(t)
	ch := ChatChannel("acct")
	if _, err := b.Subscribe("bob", ch, Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	first := pub(t, b, ch, "note", "one", nil)
	second := pub(t, b, ch, "note", "two", nil)
	if _, err := b.db.Exec(fmt.Sprintf(`CREATE TRIGGER fail_acct BEFORE UPDATE ON deliveries
		WHEN NEW.message_id = '%s' BEGIN SELECT RAISE(FAIL, 'boom'); END`, second.ID)); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Pending("bob", 10); err == nil {
		t.Fatalf("Pending returned %d messages and no error; want the accounting error", len(got))
	}
	for _, id := range []string{first.ID, second.ID} {
		if attempts, dlq, _ := deliveryState(t, b, id); attempts != 0 || dlq != 0 {
			t.Fatalf("%s: attempts=%d dlq=%d after failed accounting, want 0/0", id, attempts, dlq)
		}
	}

	if _, err := b.db.Exec(`DROP TRIGGER fail_acct`); err != nil {
		t.Fatal(err)
	}
	for call := 1; call <= maxAttempts+1; call++ {
		got, err := b.Pending("bob", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != first.ID {
			t.Fatalf("call %d: pending=%v, want [%s]", call, ids(got), first.ID)
		}
	}
	if attempts, dlq, _ := deliveryState(t, b, first.ID); attempts != maxAttempts || dlq != 1 {
		t.Fatalf("attempts=%d dlq=%d, want %d/1", attempts, dlq, maxAttempts)
	}
}

// Ack is all-or-nothing across the ids it is given.
func TestAckIsAtomic(t *testing.T) {
	t.Parallel()
	b := newBusSeconds(t)
	ch := ChatChannel("ack")
	if _, err := b.Subscribe("bob", ch, Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	first := pub(t, b, ch, "note", "one", nil)
	second := pub(t, b, ch, "note", "two", nil)
	if _, err := b.db.Exec(fmt.Sprintf(`CREATE TRIGGER fail_ack BEFORE UPDATE ON deliveries
		WHEN NEW.message_id = '%s' BEGIN SELECT RAISE(FAIL, 'boom'); END`, second.ID)); err != nil {
		t.Fatal(err)
	}
	if err := b.Ack("bob", []string{first.ID, second.ID}); err == nil {
		t.Fatal("Ack succeeded, want the trigger error")
	}
	if _, _, acked := deliveryState(t, b, first.ID); acked {
		t.Fatal("first message acked although the batch failed")
	}
	if _, err := b.db.Exec(`DROP TRIGGER fail_ack`); err != nil {
		t.Fatal(err)
	}
	if err := b.Ack("bob", []string{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.ID, second.ID} {
		if _, _, acked := deliveryState(t, b, id); !acked {
			t.Fatalf("%s not acked", id)
		}
	}
}
