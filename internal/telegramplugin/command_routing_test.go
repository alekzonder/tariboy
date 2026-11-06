package telegramplugin

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestRunCommandRouting(t *testing.T) {
	const invalid = "Unknown or invalid command. Use /help."
	post := func(path string, body any) *daemonCall {
		return &daemonCall{method: http.MethodPost, path: path, body: body}
	}
	get := func(path string) *daemonCall {
		return &daemonCall{method: http.MethodGet, path: path}
	}
	for name, tc := range map[string]struct {
		raw, agent string
		reply      string
		errText    string
		call       *daemonCall
	}{
		"agents":               {raw: "/agents", reply: "OK", call: get("/api/agents")},
		"agents in topic":      {raw: "/agents", agent: "worker", reply: invalid},
		"agent no args":        {raw: "/agent", reply: invalid},
		"agent one arg":        {raw: "/agent show", reply: invalid},
		"agent in topic":       {raw: "/agent show worker", agent: "worker", reply: invalid},
		"agent unknown verb":   {raw: "/agent drop worker", reply: invalid},
		"agent create":         {raw: "/agent create w basic:latest", reply: "OK", call: post("/api/agents", map[string]any{"name": "w", "image": "basic:latest"})},
		"agent create short":   {raw: "/agent create w", reply: invalid},
		"agent create long":    {raw: "/agent create w i x", reply: invalid},
		"agent show":           {raw: "/agent show a/b", reply: "OK", call: get("/api/agents/a%2Fb")},
		"agent show long":      {raw: "/agent show w x", reply: invalid},
		"agent set":            {raw: "/agent set w alias The Worker", reply: "OK", call: post("/api/agents/w/alias", map[string]any{"name": "w", "value": "The Worker"})},
		"agent set short":      {raw: "/agent set w alias", reply: invalid},
		"agent set bad field":  {raw: "/agent set w loop false", errText: "field is not editable from Telegram"},
		"agent set double gap": {raw: "/agent  set w alias x", errText: "field is not editable from Telegram"},
		"start by name":        {raw: "/start w", reply: "OK", call: post("/api/agents/w/start", map[string]any{"name": "w"})},
		"stop by name":         {raw: "/stop w", reply: "OK", call: post("/api/agents/w/stop", map[string]any{"name": "w"})},
		"kill escapes name":    {raw: "/kill a/b", reply: "OK", call: post("/api/agents/a%2Fb/kill", map[string]any{"name": "a/b"})},
		"start no name":        {raw: "/start", reply: invalid},
		"start two names":      {raw: "/start a b", reply: invalid},
		"start in topic":       {raw: "/start", agent: "worker", reply: "OK", call: post("/api/agents/worker/start", map[string]any{"name": "worker"})},
		"start other in topic": {raw: "/start other", agent: "worker", reply: invalid},
		"tasks all":            {raw: "/tasks", reply: "OK", call: get("/api/tasks")},
		"tasks two names":      {raw: "/tasks a b", reply: invalid},
		"tasks one name":       {raw: "/tasks a&b", reply: "OK", call: get("/api/tasks?scope_agent=a%26b")},
		"tasks in topic":       {raw: "/tasks", agent: "worker", reply: "OK", call: get("/api/tasks?scope_agent=worker")},
		"tasks other in topic": {raw: "/tasks other", agent: "worker", reply: invalid},
		"task dispatches":      {raw: "/task show K-1", reply: "OK", call: get("/api/tasks/K-1")},
		"task incomplete":      {raw: "/task show", reply: invalid},
		"unknown":              {raw: "/nope", reply: invalid},
		"unknown in topic":     {raw: "/nope", agent: "worker", reply: invalid},
	} {
		t.Run(name, func(t *testing.T) {
			daemon := &fakeDaemon{}
			server := NewServer(nil, nil, daemon)
			fields := strings.Fields(tc.raw)
			reply, err := server.runCommand(context.Background(), strings.TrimPrefix(fields[0], "/"), fields[1:], tc.raw, tc.agent, 1)
			if tc.errText != "" {
				if err == nil || err.Error() != tc.errText {
					t.Fatalf("err = %v, want %q", err, tc.errText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if reply != tc.reply {
				t.Fatalf("reply = %q, want %q", reply, tc.reply)
			}
			if tc.call == nil {
				if len(daemon.calls) != 0 {
					t.Fatalf("calls = %#v", daemon.calls)
				}
				return
			}
			if len(daemon.calls) != 1 || !reflect.DeepEqual(daemon.calls[0], *tc.call) {
				t.Fatalf("calls = %#v, want %#v", daemon.calls, *tc.call)
			}
		})
	}
}

func TestRunCommandHelpAndUnavailableDaemon(t *testing.T) {
	server := NewServer(nil, nil)
	for agent, want := range map[string][]string{"": managementHelp, "worker": agentHelp} {
		got, err := server.runCommand(context.Background(), "help", nil, "/help", agent, 1)
		if err != nil || got != strings.Join(want, "\n") {
			t.Fatalf("help for %q = %q, %v", agent, got, err)
		}
	}
	for _, command := range []string{"agents", "nope"} {
		if _, err := server.runCommand(context.Background(), command, nil, "/"+command, "", 1); err == nil || err.Error() != "daemon API unavailable" {
			t.Fatalf("%s err = %v", command, err)
		}
	}
}

func TestRunCommandReturnsDaemonError(t *testing.T) {
	callErr := errors.New("daemon down")
	server := NewServer(nil, nil, &scriptedDaemon{fakeDaemon: &fakeDaemon{}, callErr: callErr})
	for _, raw := range []string{"/agents", "/agent show w", "/start w", "/tasks"} {
		fields := strings.Fields(raw)
		reply, err := server.runCommand(context.Background(), strings.TrimPrefix(fields[0], "/"), fields[1:], raw, "", 1)
		if !errors.Is(err, callErr) || reply != "" {
			t.Fatalf("%s: reply=%q err=%v", raw, reply, err)
		}
	}
}
