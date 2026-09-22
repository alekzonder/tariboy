package bus

import "testing"

// A reply with no explicit reply_to must land in the chat that owns the
// original, not in the replying agent's own inbox. Before chats it landed on
// agent:worker:inbox and came back to its author as a fresh unprocessed
// message, so the conversation never saw the answer at all.
func TestReplyWithoutReplyToLandsInTheChat(t *testing.T) {
	b, chat := chatFixture(t)
	orig := publish(t, b, Message{
		Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "ping",
	})
	reply, err := b.Reply("worker", orig.ID, "pong", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Channel != chat.Channel {
		t.Fatalf("reply must land in the chat, got %q", reply.Channel)
	}
	pending, err := b.Pending("worker", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range pending {
		if m.ID == reply.ID {
			t.Fatal("an agent must not receive its own chat reply")
		}
	}
}

// Review Focus item 3: an original that lives on a non-chat channel keeps the
// existing inbox and origin rules, which is what group requests and plugin
// sinks depend on.
func TestReplyTargetFallsThroughForNonChatChannels(t *testing.T) {
	b, _ := chatFixture(t)
	orig := publish(t, b, Message{
		Channel: "group:dev-team:direct:worker", Source: "agent:lead",
		Kind: "request", Type: "group.request", Text: "status?",
	})
	reply, err := b.Reply("worker", orig.ID, "green", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Channel != "agent:lead:inbox" {
		t.Fatalf("a non-chat original keeps the inbox rule, got %q", reply.Channel)
	}
}

// An explicit reply_to still wins: external sinks such as the Telegram bundle
// route their replies with it.
func TestExplicitReplyToStillOverridesTheChat(t *testing.T) {
	b, chat := chatFixture(t)
	orig := publish(t, b, Message{
		Channel: chat.Channel, Source: "user:customer", Type: "message",
		Text: "ping", ReplyTo: "chat:telegram:worker",
	})
	reply, err := b.Reply("worker", orig.ID, "pong", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Channel != "chat:telegram:worker" {
		t.Fatalf("explicit reply_to must win, got %q", reply.Channel)
	}
}

// An agent participant holds a real subscription to the chat channel, so a
// customer message in a chat becomes a delivery and wakes the agent exactly as
// an inbox message does.
func TestChatParticipationDeliversToTheAgent(t *testing.T) {
	b, chat := chatFixture(t)
	sent := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "ping"})
	pending, err := b.Pending("worker", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != sent.ID {
		t.Fatalf("a chat message must be delivered to its agent participant: %+v", pending)
	}
	has, err := b.HasPending("worker")
	if err != nil || !has {
		t.Fatalf("HasPending = %v err=%v, want the chat message to wake the agent", has, err)
	}
}

// Review Focus item 2: removing a participant unsubscribes it, but deliveries
// already created are outstanding work and must survive.
func TestRemovingAParticipantKeepsItsPendingDeliveries(t *testing.T) {
	b, chat := chatFixture(t)
	publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "still mine"})
	if err := b.RemoveParticipant(chat.ID, "agent:worker"); err != nil {
		t.Fatal(err)
	}
	pending, err := b.Pending("worker", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("already-created deliveries must survive an unsubscribe, got %d", len(pending))
	}
	// A message published after the removal is no longer that agent's work.
	publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "not mine"})
	pending, err = b.Pending("worker", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("a removed participant must receive nothing new, got %d", len(pending))
	}
}

// A per-agent chat is as protected as the agent's own inbox: an operator
// unsubscribe must not be able to cut an agent out of its own conversation.
func TestPerAgentChatSubscriptionsAreProtected(t *testing.T) {
	for _, channel := range []string{"chat:dm:worker", "chat:tasks:worker", "chat:service:worker"} {
		if !IsProtectedSubscription("worker", channel) {
			t.Errorf("IsProtectedSubscription(worker, %q) = false, want true", channel)
		}
	}
	for _, channel := range []string{"chat:dm:other", "chat:team-standup", "chat:messenger:x"} {
		if IsProtectedSubscription("worker", channel) {
			t.Errorf("IsProtectedSubscription(worker, %q) = true, want false", channel)
		}
	}
}
