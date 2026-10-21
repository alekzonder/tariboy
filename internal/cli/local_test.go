package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

// TestRunJSONAsFlagValueIsNotEaten pins that --json is the global flag only in
// flag position: as the value of another flag it is passed through.
func TestRunJSONAsFlagValueIsNotEaten(t *testing.T) {
	t.Parallel()
	f := &fakeCaller{result: json.RawMessage(`{"a":1}`)}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), testReg(t), []string{"daemon", "config", "set", "--key", "k", "--value", "--json"}, f, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errOut.String())
	}
	if body := f.body.(registry.Params); body["value"] != "--json" {
		t.Fatalf("body=%#v, want value --json", body)
	}
	if strings.TrimSpace(out.String()) == `{"a":1}` {
		t.Fatalf("--json value switched on JSON output: %q", out.String())
	}

	for _, argv := range [][]string{
		{"daemon", "config", "set", "--key", "k", "--value", "v", "--json"},
		{"daemon", "config", "set", "--json", "--key", "k", "--value", "v"},
		{"daemon", "--json", "config", "set", "k", "v"},
	} {
		f := &fakeCaller{result: json.RawMessage(`{"a":1}`)}
		out.Reset()
		if code := Run(context.Background(), testReg(t), argv, f, nil, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != `{"a":1}` {
			t.Fatalf("%v: exit=%d out=%q", argv, code, out.String())
		}
		if body := f.body.(registry.Params); body["key"] != "k" || body["value"] != "v" {
			t.Fatalf("%v: body=%#v", argv, body)
		}
	}
}

func localReg(t *testing.T, c registry.Command) *registry.Registry {
	t.Helper()
	r := registry.New()
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	return r
}

// TestRunPrefersLocal pins that a command with Local runs in the CLI process,
// prints its own output and is not sent over HTTP; ExitCodeFrom reads its result.
func TestRunPrefersLocal(t *testing.T) {
	t.Parallel()
	r := localReg(t, registry.Command{
		Path: "status", Summary: "s",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/status"},
		Handler: func(*registry.Ctx, registry.Request) (any, error) { return nil, nil },
		Local: func(c *registry.Ctx, p registry.Request) (any, error) {
			if c.JSON {
				c.Out.Write([]byte("json\n"))
			} else {
				c.Out.Write([]byte("human\n"))
			}
			return map[string]any{"running": false}, nil
		},
		ExitCodeFrom: func(raw json.RawMessage) int {
			if strings.Contains(string(raw), `"running":false`) {
				return 3
			}
			return 0
		},
	})
	for argv, want := range map[string]string{"status": "human\n", "status --json": "json\n"} {
		f := &fakeCaller{}
		var out, errOut bytes.Buffer
		code := Run(context.Background(), r, strings.Fields(argv), f, &registry.Ctx{}, &out, &errOut)
		if code != 3 || out.String() != want || f.route != "" {
			t.Fatalf("%s: exit=%d out=%q route=%q err=%q", argv, code, out.String(), f.route, errOut.String())
		}
	}
	// Without a local context the HTTP route still serves the command.
	f := &fakeCaller{result: json.RawMessage(`{}`)}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), r, []string{"status"}, f, nil, &out, &errOut); code != 0 || f.route != "/api/status" {
		t.Fatalf("exit=%d route=%q", code, f.route)
	}
}

// TestRunLocalExitCodeIsSilent pins that a Local handler which reported its
// own failure sets the exit code without a second error line.
func TestRunLocalExitCodeIsSilent(t *testing.T) {
	t.Parallel()
	r := localReg(t, registry.Command{
		Path: "up", Summary: "s",
		Local: func(c *registry.Ctx, p registry.Request) (any, error) {
			c.ErrOut.Write([]byte("up: failed\n"))
			return nil, registry.ExitCode(2)
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), r, []string{"up"}, &fakeCaller{}, &registry.Ctx{}, &out, &errOut); code != 2 || errOut.String() != "up: failed\n" || out.Len() != 0 {
		t.Fatalf("exit=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunMakesAbsPathArgsAbsolute(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	r := localReg(t, registry.Command{
		Path: "build", Summary: "s",
		Args:        []registry.Arg{{Name: "dir", Type: registry.String}, {Name: "name", Type: registry.String}},
		AbsPathArgs: []string{"dir"},
		HTTP:        &registry.HTTPRoute{Method: "POST", Path: "/api/build"},
		Handler:     func(*registry.Ctx, registry.Request) (any, error) { return nil, nil },
	})
	f := &fakeCaller{result: json.RawMessage(`{}`)}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), r, []string{"build", "--dir", "../src", "--name", "rel"}, f, nil, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d err=%s", code, errOut.String())
	}
	body := f.body.(registry.Params)
	if body["dir"] != filepath.Join(root, "src") || body["name"] != "rel" {
		t.Fatalf("body=%#v", body)
	}
}

// TestRunRestArgTakesRemainingPositionals pins that a Rest arg takes every
// remaining positional, and that args after it are filled only by flag.
func TestRunRestArgTakesRemainingPositionals(t *testing.T) {
	t.Parallel()
	var got registry.Params
	r := localReg(t, registry.Command{
		Path: "exec", Summary: "s",
		Args: []registry.Arg{
			{Name: "member", Type: registry.String},
			{Name: "words", Type: registry.String, Repeatable: true, Rest: true},
			{Name: "file", Flag: "file", Short: "f", Type: registry.String, Default: "x.yaml"},
		},
		Local: func(c *registry.Ctx, p registry.Request) (any, error) {
			got = p.Params
			return nil, nil
		},
	})
	for argv, want := range map[string][2]string{
		"exec dev hello big -f c.yaml world": {"hello big world", "c.yaml"},
		"exec dev hello":                     {"hello", "x.yaml"},
		"exec dev":                           {"", "x.yaml"},
	} {
		got = nil
		var out, errOut bytes.Buffer
		if code := Run(context.Background(), r, strings.Fields(argv), &fakeCaller{}, &registry.Ctx{}, &out, &errOut); code != 0 {
			t.Fatalf("%s: exit=%d err=%s", argv, code, errOut.String())
		}
		words, _ := got["words"].([]string)
		if got["member"] != "dev" || strings.Join(words, " ") != want[0] || got["file"] != want[1] {
			t.Fatalf("%s: params=%#v", argv, got)
		}
	}
}
