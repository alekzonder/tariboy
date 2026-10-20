package bus

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Every time the bus writes is in the canonical stored format.
func TestBusWritesCanonicalTimes(t *testing.T) {
	t.Parallel()
	b, chat := chatFixture(t)
	msg := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "hi"})
	if _, err := store.ParseTime(msg.TS); err != nil {
		t.Fatalf("message ts: %v", err)
	}
	for _, q := range []string{
		`SELECT created_at FROM channels`,
		`SELECT joined_at FROM chat_participants`,
	} {
		rows, err := b.db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ParseTime(v); err != nil {
				t.Errorf("%s: %v", q, err)
			}
		}
		rows.Close()
	}
}

// A read mark given with a different width or offset is stored canonically,
// so the forward-only comparison still follows time order: 10ns is earlier
// than 11ns although ".00000001Z" sorts after ".000000011Z" as text.
func TestSetReadTSNormalizesBeforeComparing(t *testing.T) {
	t.Parallel()
	b, chat := chatFixture(t)
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	if err := b.SetReadTS(chat.ID, "user:customer", base.Add(11).Format(time.RFC3339Nano), false); err != nil {
		t.Fatal(err)
	}
	if err := b.SetReadTS(chat.ID, "user:customer", base.Add(10).Format(time.RFC3339Nano), false); err != nil {
		t.Fatal(err)
	}
	if got, want := readTS(t, b, chat.ID, "user:customer"), store.FormatTime(base.Add(11)); got != want {
		t.Fatalf("read mark = %q, want %q", got, want)
	}
	plusOne := base.Add(time.Second).In(time.FixedZone("x", 3600)).Format(time.RFC3339Nano)
	if err := b.SetReadTS(chat.ID, "user:customer", plusOne, false); err != nil {
		t.Fatal(err)
	}
	if got, want := readTS(t, b, chat.ID, "user:customer"), store.FormatTime(base.Add(time.Second)); got != want {
		t.Fatalf("read mark = %q, want %q", got, want)
	}
}

// A paging cursor with an offset is the same instant as the canonical ts.
func TestChatFeedNormalizesBeforeCursor(t *testing.T) {
	t.Parallel()
	b, chat := chatFixture(t)
	publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "first"})
	second := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "second"})
	at, err := store.ParseTime(second.TS)
	if err != nil {
		t.Fatal(err)
	}
	cursor := at.In(time.FixedZone("x", 3600)).Format(time.RFC3339Nano)
	older, err := b.ChatFeed(chat, "user:customer", DefaultChatTypes, 0, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 1 || older[0].Text != "first" {
		t.Fatalf("before cursor with an offset must page strictly older messages: %+v", older)
	}
}
