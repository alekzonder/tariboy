package taskcli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// ttasks activity is the operator command tariboy tasks activity used to be.
func TestActivityIsOperatorOnly(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	recorded := &recorder{result: json.RawMessage(`{"totals":{},"tasks":[]}`)}
	newCaller = func(string) Caller { return recorded }
	argv := []string{"activity", "--since", "2026-10-01T00:00:00Z", "-a", "worker", "--json"}
	if code := Run(context.Background(), argv, operatorEnv(t), io.Discard, io.Discard); code != 0 {
		t.Fatalf("operator code = %d", code)
	}
	if len(recorded.calls) != 1 || recorded.calls[0].method != "GET" || recorded.calls[0].route != "/api/task-activity" {
		t.Fatalf("calls = %#v", recorded.calls)
	}
	if got, _ := json.Marshal(recorded.calls[0].body); !strings.Contains(string(got), `"agent":"worker"`) || !strings.Contains(string(got), `"since":"2026-10-01T00:00:00Z"`) {
		t.Fatalf("body = %s", got)
	}
	recorded.calls = nil
	if code := Run(context.Background(), argv, mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), io.Discard, io.Discard); code != 2 || len(recorded.calls) != 0 {
		t.Fatalf("agent code = %d calls = %#v", code, recorded.calls)
	}
}

// RunOperator never selects agent mode: tariboy tasks sees every task even
// when an agent calls it.
func TestRunOperatorIgnoresToolsSocket(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	t.Setenv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	var sockets []string
	recorded := &recorder{result: json.RawMessage(`{"tasks":[]}`)}
	newCaller = func(socket string) Caller {
		sockets = append(sockets, socket)
		return recorded
	}
	if code := RunOperator(context.Background(), []string{"list", "--json"}, "/daemon.sock", io.Discard, io.Discard); code != 0 {
		t.Fatalf("list code = %d", code)
	}
	if code := RunOperator(context.Background(), []string{"queue", "list", "--json"}, "/daemon.sock", io.Discard, io.Discard); code != 0 {
		t.Fatalf("queue list code = %d", code)
	}
	if strings.Join(sockets, ",") != "/daemon.sock,/daemon.sock" {
		t.Fatalf("sockets = %q", sockets)
	}
	for _, c := range recorded.calls {
		if strings.HasPrefix(c.route, "/tools/") {
			t.Fatalf("agent route %s", c.route)
		}
	}
}

func TestRunOperatorHelpNamesTariboyTasks(t *testing.T) {
	for _, path := range []string{"", "list", "queue", "queue create", "activity"} {
		var out strings.Builder
		if code := RunOperator(context.Background(), append(strings.Fields(path), "--help"), "/daemon.sock", &out, io.Discard); code != 0 {
			t.Fatalf("%q: code %d", path, code)
		}
		if !strings.Contains(out.String(), "Usage: tariboy tasks") || strings.Contains(out.String(), "ttasks ") {
			t.Errorf("%q help:\n%s", path, &out)
		}
	}
	var out strings.Builder
	RunOperator(context.Background(), []string{"--help"}, "/daemon.sock", &out, io.Discard)
	if strings.Contains(out.String(), "TARIBOY_TOOLS_SOCKET:") || !strings.Contains(out.String(), "Always operator mode") {
		t.Errorf("root help:\n%s", &out)
	}
}

// tariboy tasks queue create keeps the flags it had as a tariboy registry command.
func TestRunOperatorCreatesTaskQueue(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	recorded := &recorder{result: json.RawMessage(`{"prefix":"TEST"}`)}
	newCaller = func(string) Caller { return recorded }
	argv := []string{"queue", "create", "--prefix", "TEST", "--name", "Test queue",
		"--description", "CLI-created", "--owners", "alice,bob", "--responsible-agent", "worker"}
	var errOut strings.Builder
	if code := RunOperator(context.Background(), argv, "/daemon.sock", io.Discard, &errOut); code != 0 {
		t.Fatalf("code = %d: %s", code, &errOut)
	}
	if len(recorded.calls) != 1 || recorded.calls[0].method != "POST" || recorded.calls[0].route != "/api/task-queues" {
		t.Fatalf("calls = %#v", recorded.calls)
	}
	got, _ := json.Marshal(recorded.calls[0].body)
	if string(got) != `{"description":"CLI-created","name":"Test queue","owners":"alice,bob","prefix":"TEST","responsible_agent":"worker"}` {
		t.Fatalf("body = %s", got)
	}
}
