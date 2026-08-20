package bus

import "testing"

func msg() Message {
	return Message{
		Source: "agent:alice", Type: "deploy.requested",
		Subject: map[string]any{"env": "prod", "svc": "api"},
		Data:    map[string]any{"priority": "high", "meta": map[string]any{"region": "eu"}},
	}
}

func TestMatcherMatch(t *testing.T) {
	cases := []struct {
		name string
		m    Matcher
		want bool
	}{
		{"empty matches all", Matcher{}, true},
		{"exact type", Matcher{"type": "deploy.requested"}, true},
		{"glob type", Matcher{"type": "deploy.*"}, true},
		{"glob type miss", Matcher{"type": "build.*"}, false},
		{"source glob", Matcher{"source": "agent:*"}, true},
		{"subject equality", Matcher{"subject.env": "prod"}, true},
		{"subject miss", Matcher{"subject.env": "staging"}, false},
		{"nested data path", Matcher{"data.meta.region": "eu"}, true},
		{"nested data miss", Matcher{"data.meta.region": "us"}, false},
		{"missing path is non-match", Matcher{"data.nope": "x"}, false},
		{"multiple all-match (AND)", Matcher{"type": "deploy.*", "subject.env": "prod"}, true},
		{"multiple one-miss (AND)", Matcher{"type": "deploy.*", "subject.env": "staging"}, false},
		{"star only", Matcher{"data.priority": "*"}, true},
	}
	for _, c := range cases {
		if got := c.m.Match(msg()); got != c.want {
			t.Fatalf("%s: Match = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMatchType(t *testing.T) {
	if !MatchType(nil, "anything") {
		t.Fatal("empty filter must match all")
	}
	if !MatchType([]string{"build.*", "deploy.*"}, "deploy.done") {
		t.Fatal("OR of type globs failed")
	}
	if MatchType([]string{"build.*"}, "deploy.done") {
		t.Fatal("non-matching type accepted")
	}
}
