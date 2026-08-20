package bus

import (
	"fmt"
	"strings"
)

// Matcher is a content matcher over a message's source/type/subject.*/data.*
// (spec §6). Keys are dotted paths; values are `*`-globs. An empty matcher
// matches all; every entry must match (AND).
type Matcher map[string]string

func (m Matcher) Match(msg Message) bool {
	for path, pattern := range m {
		val, ok := resolvePath(msg, path)
		if !ok || !globMatch(pattern, val) {
			return false
		}
	}
	return true
}

// MatchType reports whether typ matches any glob in filter; an empty filter
// matches all.
func MatchType(filter []string, typ string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, pat := range filter {
		if globMatch(pat, typ) {
			return true
		}
	}
	return false
}

// resolvePath returns the string value at a dotted path, or ok=false if absent.
func resolvePath(msg Message, path string) (string, bool) {
	switch {
	case path == "source":
		return msg.Source, true
	case path == "type":
		return msg.Type, true
	case strings.HasPrefix(path, "subject."):
		return walk(msg.Subject, strings.Split(path[len("subject."):], "."))
	case strings.HasPrefix(path, "data."):
		return walk(msg.Data, strings.Split(path[len("data."):], "."))
	default:
		return "", false
	}
}

func walk(root map[string]any, keys []string) (string, bool) {
	var cur any = root
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = m[k]
		if !ok {
			return "", false
		}
	}
	if cur == nil {
		return "", false
	}
	if s, ok := cur.(string); ok {
		return s, true
	}
	return fmt.Sprint(cur), true
}

// globMatch matches value against a pattern whose only metacharacter is `*`
// (zero or more of any character). Splitting on `*` and anchoring the ends is
// the minimal deterministic glob the bus needs.
func globMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	// Anchor the first part.
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = value[len(parts[0]):]
	// Anchor the last part.
	last := parts[len(parts)-1]
	if !strings.HasSuffix(value, last) {
		return false
	}
	value = value[:len(value)-len(last)]
	// Middle parts must appear in order.
	for _, p := range parts[1 : len(parts)-1] {
		if p == "" {
			continue
		}
		i := strings.Index(value, p)
		if i < 0 {
			return false
		}
		value = value[i+len(p):]
	}
	return true
}
