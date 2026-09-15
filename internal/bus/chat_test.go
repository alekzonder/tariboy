package bus

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func chatBus(t *testing.T) *Bus {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	tick := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	return New(st, func() time.Time {
		tick = tick.Add(time.Second)
		return tick
	})
}

func publish(t *testing.T, b *Bus, msg Message) Message {
	t.Helper()
	out, err := b.Publish(msg)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMessageFromPrefersExplicitAttribution(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  Message
		want string
	}{
		{"data.from wins over a system source", Message{Source: "system:tasks", Data: map[string]any{"from": "agent:worker"}}, "agent:worker"},
		{"agent source", Message{Source: "agent:worker"}, "agent:worker"},
		{"user source", Message{Source: "user:customer"}, "user:customer"},
		{"produced_by_agent fallback", Message{Source: "operator", ProducedByAgent: "worker"}, "agent:worker"},
		{"unattributed", Message{Source: "operator"}, "system"},
	} {
		if got := MessageFrom(tc.msg); got != tc.want {
			t.Errorf("%s: MessageFrom = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The chat with one agent is the two existing inboxes merged: what the customer
// sent into the agent's inbox and what that agent sent into the customer's.
func TestChatMessagesMergeBothInboxesAndFilterByType(t *testing.T) {
	b := chatBus(t)
	publish(t, b, Message{Channel: "agent:worker:inbox", Source: "user:customer", Type: "message", Text: "from the customer"})
	publish(t, b, Message{Channel: "user:customer", Source: "system:tasks", Type: "task.question",
		Data: map[string]any{"from": "agent:worker"}, Text: "worker asks"})
	publish(t, b, Message{Channel: "agent:worker:inbox", Source: "system:tasks", Type: "task.goal", Text: "own wake"})
	publish(t, b, Message{Channel: "agent:worker:inbox", Source: "agent:other", Type: "message", Text: "peer traffic"})
	publish(t, b, Message{Channel: "user:customer", Source: "agent:other", Type: "message", Text: "another chat"})
	publish(t, b, Message{Channel: "user:customer", Source: "agent:worker", Type: "script.result", Text: "worker script"})

	msgs, err := b.ChatMessages("user:customer", "worker", DefaultChatTypes, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, m := range msgs {
		texts = append(texts, m.Text+"/"+MessageFrom(m))
	}
	want := []string{"from the customer/user:customer", "worker asks/agent:worker"}
	if len(texts) != len(want) {
		t.Fatalf("chat = %v, want %v", texts, want)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Fatalf("chat = %v, want %v", texts, want)
		}
	}
}

// Message ids embed their channel, so the merged feed can only page by time:
// an id cursor would sort every "agent:*" id below every "user:*" one.
func TestChatMessagesPagesBackwardsByTimeAcrossBothChannels(t *testing.T) {
	b := chatBus(t)
	publish(t, b, Message{Channel: "user:customer", Source: "agent:worker", Type: "message", Text: "first"})
	second := publish(t, b, Message{Channel: "agent:worker:inbox", Source: "user:customer", Type: "message", Text: "second"})
	publish(t, b, Message{Channel: "user:customer", Source: "agent:worker", Type: "message", Text: "third"})

	older, err := b.ChatMessages("user:customer", "worker", DefaultChatTypes, 50, second.TS)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 1 || older[0].Text != "first" {
		t.Fatalf("page before the second message = %#v", older)
	}
}

func TestChatSummariesRankByLastMessageAndCountUnread(t *testing.T) {
	b := chatBus(t)
	publish(t, b, Message{Channel: "user:customer", Source: "agent:alpha", Type: "message", Text: "alpha first"})
	publish(t, b, Message{Channel: "agent:beta:inbox", Source: "user:customer", Type: "message", Text: "to beta"})
	read := publish(t, b, Message{Channel: "user:customer", Source: "agent:beta", Type: "message", Text: "beta read"})
	publish(t, b, Message{Channel: "user:customer", Source: "agent:beta", Type: "message", Text: "beta unread"})
	publish(t, b, Message{Channel: "agent:beta:inbox", Source: "system:tasks", Type: "script.result", Text: "own alarm"})

	rows, err := b.ChatSummaries("user:customer", DefaultChatTypes, map[string]string{"beta": read.TS})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Agent != "beta" || rows[1].Agent != "alpha" {
		t.Fatalf("summaries = %#v, want beta before alpha", rows)
	}
	if rows[0].LastText != "beta unread" || rows[0].LastFrom != "agent:beta" || rows[0].Unread != 1 {
		t.Fatalf("beta summary = %#v", rows[0])
	}
	// A message the customer sent is never unread, and a filtered-out type
	// neither counts nor moves the agent up the list.
	if rows[1].Unread != 1 || rows[1].LastText != "alpha first" {
		t.Fatalf("alpha summary = %#v", rows[1])
	}
}
