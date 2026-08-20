package bus

import "testing"

func TestGroupInboxNameAndKind(t *testing.T) {
	if got := GroupInbox("research"); got != "group:research:inbox" {
		t.Fatalf("GroupInbox = %q", got)
	}
	if k := ChannelKind("group:research:inbox"); k != "group_inbox" {
		t.Fatalf("kind = %q", k)
	}
	if k := ChannelKind("group:research:broadcast"); k != "group_broadcast" {
		t.Fatalf("broadcast kind = %q", k)
	}
}

func TestDeleteChannel(t *testing.T) {
	b := newBus(t)
	if _, err := b.Subscribe("scout", "group:research:broadcast", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Publish(Message{Channel: "group:research:broadcast", Type: "t", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteChannel("group:research:broadcast"); err != nil {
		t.Fatal(err)
	}
	chans, _ := b.Channels()
	for _, c := range chans {
		if c.Name == "group:research:broadcast" {
			t.Fatal("channel row survived DeleteChannel")
		}
	}
	// Subscriptions for the channel are gone too.
	subs, _ := b.ListSubscriptions("scout")
	for _, s := range subs {
		if s.Channel == "group:research:broadcast" {
			t.Fatal("subscription survived DeleteChannel")
		}
	}
}
