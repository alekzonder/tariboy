package taskcli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestQueueEnvSetTakesTheValueAsArgumentFlagOrStdin(t *testing.T) {
	r := &recorder{result: json.RawMessage(`{"queue":"DEV","key":"LOG_LEVEL","value":"debug","updated_at":"now"}`)}
	withCaller(t, r)
	old := isTerminal
	t.Cleanup(func() { isTerminal = old })
	isTerminal = func(io.Reader) bool { return false }
	for _, tt := range []struct {
		argv  []string
		stdin string
		want  string
	}{
		{[]string{"queue", "env", "set", "DEV", "LOG_LEVEL", "debug"}, "ignored", "debug"},
		{[]string{"queue", "env", "set", "DEV", "LOG_LEVEL", "--value", "--flag"}, "ignored", "--flag"},
		{[]string{"queue", "env", "set", "DEV", "LOG_LEVEL"}, "piped\n", "piped"},
	} {
		r.calls = nil
		withStdin(t, tt.stdin)
		var errOut strings.Builder
		if code := Run(context.Background(), tt.argv, operatorEnv(t), io.Discard, &errOut); code != 0 {
			t.Fatalf("%v: code %d: %s", tt.argv, code, &errOut)
		}
		if len(r.calls) != 1 || r.calls[0].method != "PUT" || r.calls[0].route != "/api/task-queues/DEV/env/LOG_LEVEL" ||
			!sameJSON(r.calls[0].body, map[string]any{"value": tt.want}) {
			t.Fatalf("%v: calls = %#v; want PUT with value %q", tt.argv, r.calls, tt.want)
		}
	}
	var errOut strings.Builder
	if code := Run(context.Background(), []string{"queue", "env", "set", "DEV", "LOG_LEVEL", "a", "b"}, operatorEnv(t), io.Discard, &errOut); code != 2 {
		t.Fatalf("two values = %d %q; want 2", code, &errOut)
	}
}

func TestQueueEnvCommandsUseRestRoutes(t *testing.T) {
	for _, tt := range []struct {
		argv          []string
		method, route string
	}{
		{[]string{"queue", "env", "ls", "DEV"}, "GET", "/api/task-queues/DEV/env"},
		{[]string{"queue", "env", "rm", "DEV", "LOG_LEVEL"}, "DELETE", "/api/task-queues/DEV/env/LOG_LEVEL"},
	} {
		r := &recorder{result: json.RawMessage(`{"env":[{"key":"LOG_LEVEL","value":"debug","updated_at":"now"}],"count":1}`)}
		withCaller(t, r)
		if code := Run(context.Background(), tt.argv, operatorEnv(t), io.Discard, io.Discard); code != 0 {
			t.Fatalf("%v: code %d", tt.argv, code)
		}
		if len(r.calls) != 1 || r.calls[0].method != tt.method || r.calls[0].route != tt.route {
			t.Fatalf("%v: calls = %#v", tt.argv, r.calls)
		}
	}
}

func TestQueueEnvCommandsAreRefusedInAgentMode(t *testing.T) {
	r := &recorder{result: json.RawMessage(`{}`)}
	withCaller(t, r)
	for _, argv := range [][]string{
		{"queue", "env", "set", "DEV", "LOG_LEVEL", "debug"},
		{"queue", "env", "ls", "DEV"},
		{"queue", "env", "rm", "DEV", "LOG_LEVEL"},
	} {
		if code := Run(context.Background(), argv, agentEnv(), io.Discard, io.Discard); code != 2 {
			t.Errorf("Run(%q) = %d; want 2", argv, code)
		}
	}
	if len(r.calls) != 0 {
		t.Fatalf("calls = %#v; want none", r.calls)
	}
}
