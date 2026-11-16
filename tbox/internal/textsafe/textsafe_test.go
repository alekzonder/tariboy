package textsafe

import "testing"

func TestLine(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"plain", 20, "plain"},
		{"a\x1b[31mred\x1b[0m", 40, "a [31mred [0m"},
		{"two\nlines\r\nhere", 40, "two lines here"},
		{"c1\u0085\u009bx", 40, "c1 x"},
		{"del\x7fx", 40, "del x"},
		{"bad\xffutf", 40, "bad utf"},
		{"  a   b  ", 40, "a b"},
		{"tab\tsep", 40, "tab sep"},
		{"abcdefghij", 5, "abcd…"},
		{"héllo wörld", 6, "héllo…"},
		{"abcde", 5, "abcde"},
		{"", 5, ""},
		{"bidi\u202eevil\u202c end", 40, "bidi evil end"},
		{"zero\u200bwidth\u200djoin\ufeff", 40, "zero width join"},
	}
	for _, c := range cases {
		if got := Line(c.in, c.max); got != c.want {
			t.Errorf("Line(%q,%d) = %q want %q", c.in, c.max, got, c.want)
		}
	}
}
