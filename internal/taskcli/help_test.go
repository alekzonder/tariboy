package taskcli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// Losing help dispatch must fail before any transport can be constructed.
func TestDetailedHelpIsLocalForEveryCommand(t *testing.T) {
	old := newCaller
	t.Cleanup(func() { newCaller = old })
	newCaller = func(string) Caller { t.Fatal("help constructed a transport"); return nil }
	paths := []string{""}
	for _, spec := range cliSpecs() {
		paths = append(paths, strings.Join(spec.Words, " "))
	}
	reg, err := taskOperatorRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range reg.Commands() {
		paths = append(paths, strings.ReplaceAll(c.Path, ".", " "))
	}
	for _, g := range reg.Groups() {
		paths = append(paths, strings.ReplaceAll(g.Path, ".", " "))
	}
	for _, socket := range []string{"", "/missing/agent.sock"} {
		for _, path := range paths {
			for _, flag := range []string{"--help", "-h"} {
				t.Run(socket+"/"+path+"/"+flag, func(t *testing.T) {
					var out, errOut strings.Builder
					args := append(strings.Fields(path), flag)
					code := Run(context.Background(), args, mapEnv("TARIBOY_TOOLS_SOCKET", socket), &out, &errOut)
					if code != 0 || errOut.Len() != 0 {
						t.Fatalf("code %d: %s", code, &errOut)
					}
					for _, want := range []string{"Usage: ttasks", "Examples:", "--json"} {
						if !strings.Contains(out.String(), want) {
							t.Errorf("missing %q in %s", want, &out)
						}
					}
					if strings.Contains(out.String(), "Shared task command") {
						t.Error("placeholder help")
					}
				})
			}
		}
	}
}

func TestHelpExplainsTaskFormsAndFlags(t *testing.T) {
	cases := []struct {
		path  string
		wants []string
	}{
		{"", []string{"tariboy-tasks", "TARIBOY_TOOLS_SOCKET", "operator", "agent", "queue"}},
		{"ask", []string{"KEY", "PRINCIPAL", "TEXT", "--idempotency-key", "--file PATH", "stdin"}},
		{"comment", []string{"--body", "--file PATH", "stdin"}},
		{"create", []string{"--title", "--queue", "--parent", "P0", "P3", "filed"}},
		{"move", []string{"--to-root", "--before", "--parent"}},
		{"list", []string{"--subtree", "--sort", "--after", "next:", "workflow status"}},
		{"mine", []string{"alias of list"}},
		{"queue pool set", []string{"operator-only", "--agents", "--revision", "--idempotency-key"}},
		{"queue create", []string{"operator-only", "--prefix", "--name", "--owners"}},
	}
	for _, tc := range cases {
		var out strings.Builder
		if code := Run(context.Background(), append(strings.Fields(tc.path), "--help"), mapEnv(), &out, io.Discard); code != 0 {
			t.Errorf("%s: code %d", tc.path, code)
			continue
		}
		for _, want := range tc.wants {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: missing %q", tc.path, want)
			}
		}
	}
	var out strings.Builder
	Run(context.Background(), []string{"--help-json"}, mapEnv(), &out, io.Discard)
	var tree map[string]any
	if err := json.Unmarshal([]byte(out.String()), &tree); err != nil {
		t.Fatal(err)
	}
	for _, spec := range cliSpecs() {
		action, flags := spec.Action, commandFlags(spec)
		node := tree
		for _, part := range spec.Words {
			node = node[part].(map[string]any)
		}
		if node["summary"] == "Shared task command" || node["help"] == nil || node["examples"] == nil {
			t.Errorf("%s lacks detailed JSON help", action)
		}
		// Keep the pre-existing flags array while adding descriptions separately.
		if _, ok := node["flags"].([]any); !ok {
			t.Errorf("%s changed flags JSON type", action)
		}
		details, _ := node["flag_help"].(map[string]any)
		for flag := range flags {
			if details[flag] == nil || details[flag] == "" {
				t.Errorf("%s: no description for --%s", action, flag)
			}
		}
	}
}

// cliSpecs are the task commands ttasks parses.
func cliSpecs() []tasks.Spec {
	var out []tasks.Spec
	for _, spec := range tasks.Specs() {
		if len(spec.Words) > 0 {
			out = append(out, spec)
		}
	}
	return out
}
