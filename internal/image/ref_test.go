package image

import "testing"

func TestParseRef(t *testing.T) {
	r, err := ParseRef("foo")
	if err != nil || r.Name != "foo" || r.Tag != "latest" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	r, err = ParseRef("foo-bar.1:v2_0")
	if err != nil || r.Name != "foo-bar.1" || r.Tag != "v2_0" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if got := r.String(); got != "foo-bar.1:v2_0" {
		t.Fatalf("String() = %q", got)
	}
	for _, bad := range []string{"", "Foo", "a:B", "a b", "a:", ":t", "a/b"} {
		if _, err := ParseRef(bad); err == nil {
			t.Fatalf("ParseRef(%q) accepted", bad)
		}
	}
}
