package registry

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func bindCommand(args ...Arg) Command {
	return Command{Path: "x", Summary: "x", Args: args,
		Handler: func(*Ctx, Request) (any, error) { return nil, nil }}
}

func paramErrorCode(t *testing.T, err error) string {
	t.Helper()
	var pe *ParamError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v (%T), want *ParamError", err, err)
	}
	return pe.Code
}

func TestBindRequiredMissing(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "name", Type: String, Required: true})
	for _, raw := range []map[string]any{nil, {}, {"name": nil}} {
		_, err := Bind(cmd, raw)
		if code := paramErrorCode(t, err); code != "missing_param" {
			t.Fatalf("raw %v: code = %q, want missing_param", raw, code)
		}
	}
	if _, err := Bind(cmd, map[string]any{"name": ""}); err != nil {
		t.Fatalf("an empty string is a present value: %v", err)
	}
	limit := bindCommand(Arg{Name: "limit", Type: Int, Required: true})
	_, err := Bind(limit, map[string]any{"limit": " "})
	if code := paramErrorCode(t, err); code != "missing_param" {
		t.Fatalf("empty number text: code = %q, want missing_param", code)
	}
	p, err := Bind(bindCommand(Arg{Name: "force", Type: Bool}), map[string]any{"force": ""})
	if err != nil || p.Has("force") {
		t.Fatalf("empty bool text: p=%v err=%v", p, err)
	}
}

func TestBindIntRejectsText(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "limit", Type: Int})
	for _, value := range []any{"abc", 1.5, true, []any{1}} {
		_, err := Bind(cmd, map[string]any{"limit": value})
		if code := paramErrorCode(t, err); code != "invalid_param" {
			t.Fatalf("value %#v: code = %q, want invalid_param", value, code)
		}
	}
}

func TestBindIntAcceptsWholeFloatAndString(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "limit", Type: Int})
	for _, value := range []any{7, int64(7), 7.0, "7"} {
		p, err := Bind(cmd, map[string]any{"limit": value})
		if err != nil {
			t.Fatalf("value %#v: %v", value, err)
		}
		if got, ok := p["limit"].(int); !ok || got != 7 {
			t.Fatalf("value %#v bound to %#v, want int 7", value, p["limit"])
		}
	}
}

func TestBindBoolForms(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "force", Type: Bool})
	for value, want := range map[any]bool{true: true, false: false, "true": true, "1": true, "false": false, "0": false} {
		p, err := Bind(cmd, map[string]any{"force": value})
		if err != nil {
			t.Fatalf("value %#v: %v", value, err)
		}
		if p["force"] != want {
			t.Fatalf("value %#v bound to %#v, want %v", value, p["force"], want)
		}
	}
	_, err := Bind(cmd, map[string]any{"force": "yes"})
	if code := paramErrorCode(t, err); code != "invalid_param" {
		t.Fatalf("code = %q, want invalid_param", code)
	}
}

func TestBindStringRejectsOtherTypes(t *testing.T) {
	t.Parallel()
	for _, typ := range []ArgType{String, ""} {
		cmd := bindCommand(Arg{Name: "name", Type: typ})
		_, err := Bind(cmd, map[string]any{"name": 3.0})
		if code := paramErrorCode(t, err); code != "invalid_param" {
			t.Fatalf("type %q: code = %q, want invalid_param", typ, code)
		}
	}
}

func TestBindRepeatableStringTakesAList(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "tag", Type: String, Repeatable: true})
	p, err := Bind(cmd, map[string]any{"tag": []any{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p["tag"], []string{"a", "b"}) {
		t.Fatalf("tag = %#v", p["tag"])
	}
	if _, err := Bind(cmd, map[string]any{"tag": []any{"a", 1.0}}); err == nil {
		t.Fatal("a list with a number was accepted")
	}
}

func TestBindIntegerList(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "ids", Type: IntegerList})
	for _, value := range []any{"1, 2", []any{1.0, 2.0}, []int64{1, 2}} {
		p, err := Bind(cmd, map[string]any{"ids": value})
		if err != nil {
			t.Fatalf("value %#v: %v", value, err)
		}
		if !reflect.DeepEqual(p["ids"], []int64{1, 2}) {
			t.Fatalf("value %#v bound to %#v", value, p["ids"])
		}
	}
	if _, err := Bind(cmd, map[string]any{"ids": "1,x"}); err == nil {
		t.Fatal("a list with text was accepted")
	}
}

func TestBindJSONObject(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "bundle", Type: JSONObject})
	if _, err := Bind(cmd, map[string]any{"bundle": map[string]any{"a": 1.0}}); err != nil {
		t.Fatal(err)
	}
	_, err := Bind(cmd, map[string]any{"bundle": "x"})
	if code := paramErrorCode(t, err); code != "invalid_param" {
		t.Fatalf("code = %q, want invalid_param", code)
	}
}

func TestBindSchemaArgKeepsItsShape(t *testing.T) {
	t.Parallel()
	cmd := bindCommand(Arg{Name: "owners", Type: String, Schema: map[string]any{"type": "array"}})
	p, err := Bind(cmd, map[string]any{"owners": []any{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p["owners"], []any{"a"}) {
		t.Fatalf("owners = %#v", p["owners"])
	}
}

func TestBindKeepsUndeclared(t *testing.T) {
	t.Parallel()
	raw := map[string]any{"nested": map[string]any{"k": "v"}, "n": 1.5}
	p, err := Bind(bindCommand(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(map[string]any(p), raw) {
		t.Fatalf("p = %#v", p)
	}
	p["n"] = 2
	if raw["n"] != 1.5 {
		t.Fatal("Bind changed the caller's map")
	}
}

func TestCallPassesContextAndBoundParams(t *testing.T) {
	t.Parallel()
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "v")
	var got Request
	cmd := bindCommand(Arg{Name: "limit", Type: Int})
	cmd.Handler = func(_ *Ctx, r Request) (any, error) { got = r; return nil, nil }
	if _, err := Call(&Ctx{}, ctx, cmd, map[string]any{"limit": "3"}); err != nil {
		t.Fatal(err)
	}
	if got.Ctx.Value(key{}) != "v" || got.Int("limit") != 3 {
		t.Fatalf("request = %#v", got)
	}
	if _, err := Call(&Ctx{}, nil, cmd, nil); err != nil || got.Ctx == nil {
		t.Fatalf("nil context: err=%v ctx=%v", err, got.Ctx)
	}
	called := false
	cmd.Handler = func(*Ctx, Request) (any, error) { called = true; return nil, nil }
	if _, err := Call(&Ctx{}, ctx, cmd, map[string]any{"limit": "x"}); err == nil || called {
		t.Fatalf("invalid param reached the handler: err=%v called=%v", err, called)
	}
}
