package bus

import "testing"

func TestValidChannel(t *testing.T) {
	ok := []string{"chat:room", "chat:messenger:0-0-abc", "group:dev-team:inbox", "agent:worker:inbox", "user:alice", "plugin:issue-provider:query", "system:x"}
	bad := []string{"", "room", "issue-provider:query", "chat:", "chat:Room", "chat:a b", "chat:*", "group:dev/team:inbox", "chat::x"}
	for _, s := range ok {
		if !ValidChannel(s) {
			t.Errorf("ValidChannel(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if ValidChannel(s) {
			t.Errorf("ValidChannel(%q) = true, want false", s)
		}
	}
}

func TestIsProtectedSubscription(t *testing.T) {
	cases := []struct {
		agent, channel string
		want           bool
	}{
		{"worker", "agent:worker:inbox", true}, // own inbox
		{"worker", "group:dev:inbox", true},    // any group channel
		{"worker", "group:dev:broadcast", true},
		{"worker", "chat:messenger:x", false},  // ad-hoc chat
		{"worker", "agent:other:inbox", false}, // another agent's inbox is not protected for worker
	}
	for _, c := range cases {
		if got := IsProtectedSubscription(c.agent, c.channel); got != c.want {
			t.Errorf("IsProtectedSubscription(%q,%q) = %v, want %v", c.agent, c.channel, got, c.want)
		}
	}
}
