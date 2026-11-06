package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

// noHandler satisfies the registry for a command served remotely in a test.
func noHandler(*registry.Ctx, registry.Request) (any, error) { return nil, nil }

type dispatchOutcome struct {
	code        int
	out, errOut string
}

func runDispatch(t *testing.T, r *registry.Registry, argv []string, call Caller, local *registry.Ctx) dispatchOutcome {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), r, argv, call, local, &out, &errOut)
	return dispatchOutcome{code: code, out: out.String(), errOut: errOut.String()}
}

func assertDispatch(t *testing.T, got, want dispatchOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("outcome:\n got %#v\nwant %#v", got, want)
	}
}

func followRegistry(t *testing.T, follow registry.FollowFunc) *registry.Registry {
	t.Helper()
	r := registry.New()
	if err := r.Register(registry.Command{
		Path: "logs.show", Summary: "logs",
		Args:       []registry.Arg{{Name: "follow", Flag: "follow", Short: "f", Type: registry.Bool}},
		HTTP:       &registry.HTTPRoute{Method: "GET", Path: "/api/logs"},
		Follow:     follow,
		FollowFlag: "follow",
		Local: func(c *registry.Ctx, p registry.Request) (any, error) {
			return "local", nil
		},
		Handler: noHandler,
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

// Catches follow mode losing its place ahead of a Local command, or running
// without a socket.
func TestRunFollowMode(t *testing.T) {
	t.Parallel()
	gotSock := ""
	r := followRegistry(t, func(_ context.Context, sock string, p registry.Params, out io.Writer) error {
		gotSock = sock
		if p["follow"] != true {
			t.Errorf("params = %#v", p)
		}
		_, err := io.WriteString(out, "tail\n")
		return err
	})
	assertDispatch(t, runDispatch(t, r, []string{"logs", "show", "--follow"}, &fakeCaller{}, &registry.Ctx{Socket: "/s"}), dispatchOutcome{out: "tail\n"})
	if gotSock != "/s" {
		t.Fatalf("socket = %q", gotSock)
	}
	want := dispatchOutcome{code: 2, errOut: "logs show: follow mode needs a daemon socket\n"}
	assertDispatch(t, runDispatch(t, r, []string{"logs", "show", "--follow"}, &fakeCaller{}, &registry.Ctx{}), want)
	assertDispatch(t, runDispatch(t, r, []string{"logs", "show", "--follow"}, &fakeCaller{}, nil), want)
}

func TestRunFollowFailure(t *testing.T) {
	t.Parallel()
	r := followRegistry(t, func(context.Context, string, registry.Params, io.Writer) error {
		return api.UserError{Code: "gone", Msg: "stream ended"}
	})
	assertDispatch(t, runDispatch(t, r, []string{"logs", "show", "-f"}, &fakeCaller{}, &registry.Ctx{Socket: "/s"}),
		dispatchOutcome{code: 1, errOut: "error (gone): stream ended\n"})
}

// Catches a false follow flag or a missing local context stopping the fall
// through to the Local command and then to its HTTP route.
func TestRunFallsThroughFollowToLocalThenHTTP(t *testing.T) {
	t.Parallel()
	r := followRegistry(t, func(context.Context, string, registry.Params, io.Writer) error {
		t.Error("follow ran with the flag off")
		return nil
	})
	assertDispatch(t, runDispatch(t, r, []string{"logs", "show", "--follow=false"}, &fakeCaller{}, &registry.Ctx{}), dispatchOutcome{})
	call := &fakeCaller{result: json.RawMessage(`{"via":"http"}`)}
	assertDispatch(t, runDispatch(t, r, []string{"logs", "show"}, call, nil), dispatchOutcome{out: "via: http\n"})
	if call.route != "/api/logs" {
		t.Fatalf("route = %q", call.route)
	}
}

// Catches a Local command printing its result or its exit code ignoring it.
func TestRunLocalPrintsNothingAndUsesExitCode(t *testing.T) {
	t.Parallel()
	r := registry.New()
	if err := r.Register(registry.Command{
		Path: "self.check", Summary: "check",
		Local: func(c *registry.Ctx, p registry.Request) (any, error) {
			if !c.JSON {
				t.Errorf("JSON flag did not reach the local context")
			}
			_, _ = io.WriteString(c.Out, "printed\n")
			_, _ = io.WriteString(c.ErrOut, "warned\n")
			return map[string]any{"failures": 3}, nil
		},
		ExitCodeFrom: func(raw json.RawMessage) int {
			if string(raw) != `{"failures":3}` {
				t.Errorf("raw = %s", raw)
			}
			return 7
		},
	}); err != nil {
		t.Fatal(err)
	}
	assertDispatch(t, runDispatch(t, r, []string{"self", "check", "--json"}, &fakeCaller{}, &registry.Ctx{}),
		dispatchOutcome{code: 7, out: "printed\n", errOut: "warned\n"})
}

func TestRunLocalFailure(t *testing.T) {
	t.Parallel()
	r := registry.New()
	if err := r.Register(registry.Command{
		Path: "self.check", Summary: "check",
		Local: func(*registry.Ctx, registry.Request) (any, error) { return nil, errors.New("boom") },
	}); err != nil {
		t.Fatal(err)
	}
	assertDispatch(t, runDispatch(t, r, []string{"self", "check"}, &fakeCaller{}, &registry.Ctx{}),
		dispatchOutcome{code: 1, errOut: "error: boom\n"})
}

func handlerRegistry(t *testing.T, handler func(*registry.Ctx, registry.Request) (any, error)) *registry.Registry {
	t.Helper()
	r := registry.New()
	if err := r.Register(registry.Command{Path: "img.ls", Summary: "list", Handler: handler,
		ExitCodeFrom: func(raw json.RawMessage) int { return len(raw) }}); err != nil {
		t.Fatal(err)
	}
	return r
}

// Catches an in-process command skipping --json or its ExitCodeFrom.
func TestRunInProcessOutput(t *testing.T) {
	t.Parallel()
	r := handlerRegistry(t, func(*registry.Ctx, registry.Request) (any, error) { return map[string]any{"a": 1}, nil })
	assertDispatch(t, runDispatch(t, r, []string{"img", "ls", "--json"}, &fakeCaller{}, &registry.Ctx{}), dispatchOutcome{code: 7, out: `{"a":1}` + "\n"})
	assertDispatch(t, runDispatch(t, r, []string{"img", "ls"}, &fakeCaller{}, &registry.Ctx{}), dispatchOutcome{code: 7, out: "a: 1\n"})
}

func TestRunInProcessFailures(t *testing.T) {
	t.Parallel()
	r := handlerRegistry(t, func(*registry.Ctx, registry.Request) (any, error) { return nil, errors.New("boom") })
	assertDispatch(t, runDispatch(t, r, []string{"img", "ls"}, &fakeCaller{}, &registry.Ctx{}), dispatchOutcome{code: 1, errOut: "error: boom\n"})

	unencodable := handlerRegistry(t, func(*registry.Ctx, registry.Request) (any, error) { return make(chan int), nil })
	assertDispatch(t, runDispatch(t, unencodable, []string{"img", "ls"}, &fakeCaller{}, &registry.Ctx{}),
		dispatchOutcome{code: 1, errOut: "error: json: unsupported type: chan int\n"})
}

func routeRegistry(t *testing.T, method string) *registry.Registry {
	t.Helper()
	r := registry.New()
	if err := r.Register(registry.Command{
		Path: "img.get", Summary: "get",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String},
			{Name: "limit", Flag: "limit", Type: registry.Int},
		},
		HTTP:         &registry.HTTPRoute{Method: method, Path: "/api/images/{name}"},
		ExitCodeFrom: func(json.RawMessage) int { return 4 },
		Handler:      noHandler,
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

// Catches a missing path value reaching the daemon, a value left in the body
// after the path took it, and GET or DELETE sending a typed body.
func TestRunHTTPRouteAndBody(t *testing.T) {
	t.Parallel()
	assertDispatch(t, runDispatch(t, routeRegistry(t, "GET"), []string{"img", "get"}, &fakeCaller{}, nil),
		dispatchOutcome{code: 2, errOut: "img get: missing name\n"})

	for _, method := range []string{"GET", "DELETE"} {
		call := &fakeCaller{result: json.RawMessage(`{"ok":true}`)}
		assertDispatch(t, runDispatch(t, routeRegistry(t, method), []string{"img", "get", "a b", "--limit", "3"}, call, nil), dispatchOutcome{code: 4, out: "ok: true\n"})
		body, ok := call.body.(map[string]string)
		if call.route != "/api/images/a%20b" || call.method != method || !ok || len(body) != 1 || body["limit"] != "3" {
			t.Fatalf("%s route = %q body = %#v", method, call.route, call.body)
		}
	}

	call := &fakeCaller{result: json.RawMessage(`{"ok":true}`)}
	assertDispatch(t, runDispatch(t, routeRegistry(t, "POST"), []string{"img", "get", "a", "--limit", "3", "--json"}, call, nil), dispatchOutcome{code: 4, out: `{"ok":true}` + "\n"})
	body, ok := call.body.(registry.Params)
	if !ok || len(body) != 1 || body["limit"] != 3 {
		t.Fatalf("POST body = %#v", call.body)
	}
}

func TestRunHTTPFailure(t *testing.T) {
	t.Parallel()
	assertDispatch(t, runDispatch(t, routeRegistry(t, "GET"), []string{"img", "get", "a"}, &fakeCaller{err: errors.New("boom")}, nil),
		dispatchOutcome{code: 1, errOut: "error: boom\n"})
}

// Catches a parse failure losing its prefix or the usage text that follows.
func TestRunParseFailurePrintsUsage(t *testing.T) {
	t.Parallel()
	got := runDispatch(t, routeRegistry(t, "GET"), []string{"img", "get", "--nope"}, &fakeCaller{}, nil)
	if got.code != 2 || got.out != "" || !strings.HasPrefix(got.errOut, "img get: unknown flag --nope\nUsage: tariboy img get") {
		t.Fatalf("outcome = %#v", got)
	}
}

// Catches a value-taking path argument not being made absolute before the
// dispatch that follows.
func TestRunMakesPathArgsAbsoluteBeforeDispatch(t *testing.T) {
	t.Parallel()
	r := registry.New()
	if err := r.Register(registry.Command{
		Path: "img.build", Summary: "build", AbsPathArgs: []string{"path", "empty", "absent"},
		Args: []registry.Arg{
			{Name: "path", Flag: "path", Type: registry.String},
			{Name: "empty", Flag: "empty", Type: registry.String},
		},
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/images"},
		Handler: noHandler,
	}); err != nil {
		t.Fatal(err)
	}
	call := &fakeCaller{result: json.RawMessage(`{}`)}
	runDispatch(t, r, []string{"img", "build", "--path", "src", "--empty="}, call, nil)
	body := call.body.(registry.Params)
	path, _ := body["path"].(string)
	if !strings.HasPrefix(path, "/") || !strings.HasSuffix(path, "/src") || body["empty"] != "" {
		t.Fatalf("body = %#v", body)
	}
}

// Each case pins one parse failure with its exact text.
func TestParseArgsFailures(t *testing.T) {
	t.Parallel()
	cmd := cmdWith(
		registry.Arg{Name: "name", Type: registry.String, Required: true},
		registry.Arg{Name: "count", Flag: "count", Type: registry.Int},
		registry.Arg{Name: "tag", Flag: "tag", Type: registry.Int, Repeatable: true},
		registry.Arg{Name: "on", Flag: "on", Type: registry.Bool},
	)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown long", []string{"--nope"}, "unknown flag --nope"},
		{"unknown with value", []string{"--nope=1"}, "unknown flag --nope=1"},
		{"unknown short", []string{"-z"}, "unknown flag -z"},
		{"missing value", []string{"a", "--count"}, "flag --count needs a value"},
		{"bad flag value", []string{"a", "--count", "x"}, `argument count: "x" is not an integer`},
		{"bad bool value", []string{"a", "--on=maybe"}, `argument on: "maybe" is not a boolean (use true/false)`},
		{"repeatable not string", []string{"a", "--tag", "1"}, "repeatable argument tag must be a string"},
		{"bad positional value", []string{"x", "y"}, `argument count: "y" is not an integer`},
		{"unexpected positional", []string{"a", "1", "2", "true", "extra"}, "unexpected argument: extra"},
		{"missing required", []string{"--count", "1"}, "missing required argument: name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseArgs(cmd, tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	simple := cmdWith(registry.Arg{Name: "name", Type: registry.String})
	if _, err := parseArgs(simple, []string{"a", "b"}); err == nil || err.Error() != "unexpected argument: b" {
		t.Fatalf("err = %v", err)
	}
}

// Catches a flag parse change: the value forms, repeats and defaults.
func TestParseArgsFlagForms(t *testing.T) {
	t.Parallel()
	cmd := cmdWith(
		registry.Arg{Name: "name", Type: registry.String},
		registry.Arg{Name: "on", Flag: "on", Type: registry.Bool},
		registry.Arg{Name: "off", Flag: "off", Type: registry.Bool},
		registry.Arg{Name: "tag", Flag: "tag", Type: registry.String, Repeatable: true},
		registry.Arg{Name: "dash", Flag: "dash", Type: registry.String},
		registry.Arg{Name: "size", Flag: "size", Type: registry.Int, Default: 9},
	)
	got, err := parseArgs(cmd, []string{"--on", "next", "--off", "false", "--tag", "a", "--tag=b", "--dash", "--on"})
	if err != nil {
		t.Fatal(err)
	}
	tags, _ := got["tag"].([]string)
	if got["name"] != "next" || got["on"] != true || got["off"] != false || len(tags) != 2 || tags[1] != "b" || got["dash"] != "--on" || got["size"] != 9 {
		t.Fatalf("params = %#v", got)
	}
}
