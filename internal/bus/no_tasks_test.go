package bus

import (
	"database/sql"
	"errors"
	"testing"
)

// The bus owns only its own tables: with every Native Tasks table gone it still
// publishes, drains, acknowledges and clears.
func TestBusWorksWithoutTaskTables(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	rows, err := b.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'task%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if _, err := b.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		if _, err := b.db.Exec(`DROP TABLE "` + name + `"`); err != nil {
			t.Fatalf("drop %s: %v", name, err)
		}
	}

	if _, err := b.db.Exec(`INSERT INTO agents(name, image_ref) VALUES ('bob', 'basic:latest')`); err != nil {
		t.Fatal(err)
	}
	ch := ChatChannel("plain")
	sub(t, b, "bob", ch)
	acked := pub(t, b, ch, "note", "one", nil)
	pub(t, b, ch, "note", "two", nil)
	if got, err := b.Pending("bob", 10); err != nil || len(got) != 2 {
		t.Fatalf("pending=%v err=%v", ids(got), err)
	}
	if err := b.Ack("bob", []string{acked.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := b.ClearPending("bob")
	if err != nil || got.DeletedDeliveries != 1 || got.DeletedMessages != 1 {
		t.Fatalf("clear=%+v err=%v", got, err)
	}
}

// A publish transaction hook writes with the message and its failure leaves
// nothing behind.
func TestPublishTxHookCommitsWithMessage(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	if _, err := b.db.Exec(`CREATE TABLE hook_log(message_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	fail := false
	b.SetPublishTxHook(func(tx *sql.Tx, msg Message) error {
		if fail {
			return boom
		}
		_, err := tx.Exec(`INSERT INTO hook_log VALUES (?)`, msg.ID)
		return err
	})
	ch := ChatChannel("hooked")
	m := pub(t, b, ch, "note", "one", nil)
	var logged string
	if err := b.db.QueryRow(`SELECT message_id FROM hook_log`).Scan(&logged); err != nil || logged != m.ID {
		t.Fatalf("hook_log=%q err=%v, want %s", logged, err, m.ID)
	}
	fail = true
	if _, err := b.Publish(Message{Channel: ch, Type: "note"}); !errors.Is(err, boom) {
		t.Fatalf("publish err=%v, want hook error", err)
	}
	var n int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE channel=?`, ch).Scan(&n); err != nil || n != 1 {
		t.Fatalf("messages=%d err=%v, want 1", n, err)
	}
}

// ClearPending keeps a message the retention condition holds.
func TestClearPendingKeepsRetainedMessages(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	b.SetMessageRetention(`messages.text = 'keep'`)
	if _, err := b.db.Exec(`INSERT INTO agents(name, image_ref) VALUES ('bob', 'basic:latest')`); err != nil {
		t.Fatal(err)
	}
	ch := ChatChannel("retained")
	sub(t, b, "bob", ch)
	pub(t, b, ch, "note", "keep", nil)
	pub(t, b, ch, "note", "drop", nil)
	got, err := b.ClearPending("bob")
	if err != nil || got.DeletedDeliveries != 2 || got.DeletedMessages != 1 {
		t.Fatalf("clear=%+v err=%v", got, err)
	}
}
