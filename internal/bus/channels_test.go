package bus

import "testing"

func TestChannelHelpers(t *testing.T) {
	cases := []struct {
		got, want, kind string
	}{
		{InboxChannel("alice"), "agent:alice:inbox", "agent_inbox"},
		{StreamChannel("alice"), "agent:alice:stream", "agent_stream"},
		{GroupBroadcast("ops"), "group:ops:broadcast", "group_broadcast"},
		{GroupDirect("ops", "bob"), "group:ops:direct:bob", "group_direct"},
		{ChatChannel("room1"), "chat:room1", "chat"},
		{UserChannel("carol"), "user:carol", "user"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("helper = %q, want %q", c.got, c.want)
		}
		if k := ChannelKind(c.got); k != c.kind {
			t.Fatalf("ChannelKind(%q) = %q, want %q", c.got, k, c.kind)
		}
	}
	if ChannelKind("something-custom") != "chat" {
		t.Fatalf("unknown channel should default to chat kind, got %q", ChannelKind("something-custom"))
	}
}
