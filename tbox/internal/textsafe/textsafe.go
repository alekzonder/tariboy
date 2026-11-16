// Package textsafe makes untrusted text safe to print on a terminal.
package textsafe

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Line returns s as one printable line: control characters (below 0x20,
// 0x7f and the C1 range 0x80-0x9f), Unicode format characters (bidi
// overrides, zero-width joiners) and invalid UTF-8 become spaces, runs of
// spaces collapse, the result is trimmed and cut to max runes with a final
// ellipsis. A max below 1 means no limit.
func Line(s string, max int) string {
	var b strings.Builder
	space := true
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		if r == utf8.RuneError || r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r) {
			r = ' '
		}
		if r == ' ' {
			if space {
				continue
			}
			space = true
		} else {
			space = false
		}
		b.WriteRune(r)
	}
	out := strings.TrimRight(b.String(), " ")
	if max > 0 && utf8.RuneCountInString(out) > max {
		r := []rune(out)
		out = strings.TrimRight(string(r[:max-1]), " ") + "…"
	}
	return out
}
