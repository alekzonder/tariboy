package registry

import (
	"reflect"
	"testing"
)

func TestParamsAccessors(t *testing.T) {
	t.Parallel()
	p := Params{
		"s": " a ", "n": 4, "f": 5.0, "ns": "6", "bad": "x", "b": true, "bs": "1",
		"csv": " a, ,b ", "list": []any{"x", "y"}, "strs": []string{"z"}, "nil": nil,
		"obj": map[string]any{"name": "n"},
	}
	if p.String("s") != " a " || p.Trimmed("s") != "a" || p.String("n") != "" {
		t.Fatalf("string accessors: %q %q %q", p.String("s"), p.Trimmed("s"), p.String("n"))
	}
	if p.Int("n") != 4 || p.Int("f") != 5 || p.Int("ns") != 6 || p.Int("bad") != 0 || p.Int("missing") != 0 {
		t.Fatalf("Int: %d %d %d %d", p.Int("n"), p.Int("f"), p.Int("ns"), p.Int("bad"))
	}
	if p.Int64("ns") != 6 || p.IntOr("missing", 9) != 9 || p.IntOr("bad", 9) != 9 || p.IntOr("n", 9) != 4 {
		t.Fatal("Int64/IntOr")
	}
	if !p.Bool("b") || !p.Bool("bs") || p.Bool("s") || p.Bool("missing") {
		t.Fatal("Bool")
	}
	if v, ok := p.OptBool("bs"); !ok || !v {
		t.Fatal("OptBool present")
	}
	if _, ok := p.OptBool("missing"); ok {
		t.Fatal("OptBool missing")
	}
	if got := p.OptString("s"); got == nil || *got != " a " {
		t.Fatalf("OptString = %v", got)
	}
	if p.OptString("missing") != nil || p.OptString("nil") != nil || p.OptString("n") != nil {
		t.Fatal("OptString of an absent or non-string value")
	}
	for key, want := range map[string][]string{"csv": {"a", "b"}, "list": {"x", "y"}, "strs": {"z"}, "missing": nil} {
		if got := p.Strings(key); !reflect.DeepEqual(got, want) {
			t.Fatalf("Strings(%s) = %#v, want %#v", key, got, want)
		}
	}
	if !p.Has("s") || p.Has("nil") || p.Has("missing") {
		t.Fatal("Has")
	}
	var out struct{ Name string }
	if err := p.Decode("obj", &out); err != nil || out.Name != "n" {
		t.Fatalf("Decode = %v %+v", err, out)
	}
	if err := p.Decode("s", &out); err == nil {
		t.Fatal("Decode of a string into a struct succeeded")
	}
}
