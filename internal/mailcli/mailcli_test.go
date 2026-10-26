package mailcli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/client"
)

type call struct {
	method, route string
	body          any
}

type recorder struct {
	calls  []call
	result json.RawMessage
	err    error
}

func (r *recorder) Call(method, route string, body any) (json.RawMessage, error) {
	r.calls = append(r.calls, call{method, route, body})
	return r.result, r.err
}

func mapEnv(values ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i < len(values); i += 2 {
		m[values[i]] = values[i+1]
	}
	return func(key string) string { return m[key] }
}

func operatorEnv(t *testing.T) func(string) string {
	t.Helper()
	return mapEnv("TARIBOY_BASE_DIR", t.TempDir(), "TARIBOY_RUNTIME_DIR", t.TempDir())
}

// useRecorder replaces newCaller for one test and records the sockets used.
func useRecorder(t *testing.T, result string) (*recorder, *[]string) {
	t.Helper()
	old := newCaller
	t.Cleanup(func() { newCaller = old })
	rec := &recorder{result: json.RawMessage(result)}
	var sockets []string
	newCaller = func(socket string) Caller {
		sockets = append(sockets, socket)
		return rec
	}
	return rec, &sockets
}

func useStdin(t *testing.T, text string) {
	t.Helper()
	old := stdin
	t.Cleanup(func() { stdin = old })
	stdin = strings.NewReader(text)
}

func asJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("body %s: %v", raw, err)
	}
	return out
}

func TestMailCLIAgentModeUsesToolsSocket(t *testing.T) {
	rec, sockets := useRecorder(t, `{"letters":[]}`)
	env := mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	tests := []struct {
		argv   []string
		action string
		body   map[string]any
	}{
		{[]string{"inbox"}, "inbox", map[string]any{}},
		{[]string{"inbox", "--all", "--limit", "5", "--before", "2026-10-26T10:00:00Z"}, "inbox", map[string]any{"all": true, "limit": float64(5), "before": "2026-10-26T10:00:00Z"}},
		{[]string{"sent"}, "sent", map[string]any{}},
		{[]string{"archive", "--limit=3"}, "archive", map[string]any{"limit": float64(3)}},
		{[]string{"show", "m-1"}, "show", map[string]any{"id": "m-1"}},
		{[]string{"send", "--to", "user:customer,agent:dev", "--subject", "Hi", "--text", "Body", "--kind", "report", "--deadline", "5m", "--notice", "--data", `{"task_key":"DEV-1"}`}, "send",
			map[string]any{"to": "user:customer,agent:dev", "subject": "Hi", "text": "Body", "kind": "report", "deadline": "5m", "notice": true, "data": map[string]any{"task_key": "DEV-1"}}},
		{[]string{"reply", "m-1", "done", "--ask"}, "reply", map[string]any{"id": "m-1", "text": "done", "ask": true}},
		{[]string{"requeue", "m-1"}, "requeue", map[string]any{"id": "m-1"}},
	}
	for _, tt := range tests {
		rec.calls = nil
		if code := Run(context.Background(), append(tt.argv, "--json"), env, io.Discard, io.Discard); code != 0 {
			t.Fatalf("%q: code %d", tt.argv, code)
		}
		if len(rec.calls) != 1 || rec.calls[0].method != "POST" || rec.calls[0].route != "/tools/mail/"+tt.action {
			t.Fatalf("%q: calls = %#v", tt.argv, rec.calls)
		}
		body := asJSON(t, rec.calls[0].body)
		if _, ok := body["as"]; ok {
			t.Fatalf("%q: agent mode sent an identity: %v", tt.argv, body)
		}
		if !reflect.DeepEqual(body, tt.body) {
			t.Fatalf("%q: body = %#v, want %#v", tt.argv, body, tt.body)
		}
	}
	for _, socket := range *sockets {
		if socket != "/agent.sock" {
			t.Fatalf("socket %q, want the tools socket", socket)
		}
	}
}

func TestMailCLIOperatorModeDefaultsToCustomer(t *testing.T) {
	rec, _ := useRecorder(t, `{"letters":[]}`)
	tests := []struct {
		argv          []string
		method, route string
		body          any
	}{
		{[]string{"inbox"}, "GET", "/api/mail", map[string]string{"box": "inbox"}},
		{[]string{"inbox", "--all"}, "GET", "/api/mail", map[string]string{"box": "all"}},
		{[]string{"sent", "--limit", "2"}, "GET", "/api/mail", map[string]string{"box": "sent", "limit": "2"}},
		{[]string{"archive", "--as", "agent:dev", "--before", "x"}, "GET", "/api/mail", map[string]string{"box": "archive", "as": "agent:dev", "before": "x"}},
		{[]string{"show", "m/1"}, "GET", "/api/mail/m%2F1", map[string]string{}},
		{[]string{"send", "--to", "agent:dev", "--subject", "S", "--text", "T"}, "POST", "/api/mail", map[string]any{"to": "agent:dev", "subject": "S", "text": "T"}},
		{[]string{"reply", "m-1", "ok", "--as", "agent:dev"}, "POST", "/api/mail/m-1/reply", map[string]any{"text": "ok", "as": "agent:dev"}},
		{[]string{"requeue", "m-1"}, "POST", "/api/mail/m-1/requeue", map[string]any{}},
		{[]string{"close", "m-1", "--note", "dup"}, "POST", "/api/mail/m-1/close", map[string]any{"note": "dup"}},
		{[]string{"read", "m-1", "m-2"}, "POST", "/api/mail/read", map[string]any{"ids": []any{"m-1", "m-2"}}},
		{[]string{"unread"}, "GET", "/api/mail/unread", map[string]string{}},
		{[]string{"clear", "--as", "agent:dev", "--yes"}, "POST", "/api/agents/dev/mail/clear", map[string]any{}},
	}
	for _, tt := range tests {
		rec.calls = nil
		if code := Run(context.Background(), append(tt.argv, "--json"), operatorEnv(t), io.Discard, io.Discard); code != 0 {
			t.Fatalf("%q: code %d", tt.argv, code)
		}
		if len(rec.calls) != 1 || rec.calls[0].method != tt.method || rec.calls[0].route != tt.route {
			t.Fatalf("%q: calls = %#v", tt.argv, rec.calls)
		}
		got := rec.calls[0].body
		if query, ok := got.(map[string]string); ok {
			if !reflect.DeepEqual(query, tt.body) {
				t.Fatalf("%q: query = %#v, want %#v", tt.argv, query, tt.body)
			}
			continue
		}
		if body := asJSON(t, got); !reflect.DeepEqual(body, tt.body) {
			t.Fatalf("%q: body = %#v, want %#v", tt.argv, body, tt.body)
		}
	}
}

func TestRunOperatorIgnoresToolsSocket(t *testing.T) {
	rec, sockets := useRecorder(t, `{"letters":[]}`)
	t.Setenv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	if code := RunOperator(context.Background(), []string{"inbox", "--json"}, "/daemon.sock", io.Discard, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	if strings.Join(*sockets, ",") != "/daemon.sock" || rec.calls[0].route != "/api/mail" {
		t.Fatalf("sockets = %q calls = %#v", *sockets, rec.calls)
	}
}

func TestMailCLIRefusesOperatorOnlyInAgentMode(t *testing.T) {
	rec, _ := useRecorder(t, `{}`)
	env := mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	for _, argv := range [][]string{
		{"close", "m-1"},
		{"read", "m-1"},
		{"clear", "--as", "agent:dev", "--yes"},
		{"unread"},
		{"inbox", "--as", "agent:dev"},
		{"reply", "m-1", "ok", "--as", "agent:dev"},
	} {
		var errOut strings.Builder
		if code := Run(context.Background(), argv, env, io.Discard, &errOut); code != 2 {
			t.Fatalf("%q: code %d", argv, code)
		}
		if !strings.Contains(errOut.String(), "operator") {
			t.Fatalf("%q: stderr %q does not explain the refusal", argv, errOut.String())
		}
	}
	if len(rec.calls) != 0 {
		t.Fatalf("calls = %#v", rec.calls)
	}
}

func TestMailCLIParseErrors(t *testing.T) {
	useRecorder(t, `{}`)
	for _, argv := range [][]string{
		{"bogus"},
		{"show"},
		{"show", "a", "b"},
		{"send", "--subject", "s", "--text", "t"},
		{"send", "--to"},
		{"send", "--to", "agent:a", "--data", "[1]"},
		{"send", "--to", "agent:a", "--data", "{"},
		{"inbox", "--limit", "x"},
		{"inbox", "--nope"},
		{"inbox", "--all=maybe"},
		{"reply", "m-1"},
		{"reply", "m-1", "a", "--text", "b"},
		{"read"},
		{"clear", "--yes"},
		{"clear", "--as", "user:x", "--yes"},
	} {
		var errOut strings.Builder
		if code := Run(context.Background(), argv, operatorEnv(t), io.Discard, &errOut); code != 2 {
			t.Fatalf("%q: code %d stderr %q", argv, code, errOut.String())
		}
		if errOut.Len() == 0 {
			t.Fatalf("%q: no message", argv)
		}
	}
}

func TestMailCLIParsesFlagForms(t *testing.T) {
	rec, _ := useRecorder(t, `{"letters":[]}`)
	env := mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	// A text that starts with a dash is still text; -- ends the flags.
	if code := Run(context.Background(), []string{"reply", "m-1", "--", "--ask is literal"}, env, io.Discard, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	if body := asJSON(t, rec.calls[0].body); body["text"] != "--ask is literal" || body["ask"] != nil {
		t.Fatalf("body = %#v", body)
	}
	rec.calls = nil
	if code := Run(context.Background(), []string{"send", "--to=agent:a", "--text=-42", "--notice=false"}, env, io.Discard, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	if body := asJSON(t, rec.calls[0].body); body["text"] != "-42" || body["notice"] != nil {
		t.Fatalf("body = %#v", body)
	}
	rec.calls = nil
	if code := Run(context.Background(), []string{"reply", "m-1", "--text", "flag text"}, env, io.Discard, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	if body := asJSON(t, rec.calls[0].body); body["text"] != "flag text" {
		t.Fatalf("body = %#v", body)
	}
}

func TestMailCLITextFromStdin(t *testing.T) {
	rec, _ := useRecorder(t, `{"letters":[]}`)
	env := mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	useStdin(t, "line one\nline two\n")
	if code := Run(context.Background(), []string{"send", "--to", "agent:a", "--subject", "s", "--text", "-"}, env, io.Discard, io.Discard); code != 0 {
		t.Fatalf("send code %d", code)
	}
	if body := asJSON(t, rec.calls[0].body); body["text"] != "line one\nline two\n" {
		t.Fatalf("send body = %#v", body)
	}
	rec.calls = nil
	useStdin(t, "the answer")
	if code := Run(context.Background(), []string{"reply", "m-1", "-"}, env, io.Discard, io.Discard); code != 0 {
		t.Fatalf("reply code %d", code)
	}
	if body := asJSON(t, rec.calls[0].body); body["text"] != "the answer" {
		t.Fatalf("reply body = %#v", body)
	}
	rec.calls = nil
	useStdin(t, "flag answer")
	if code := Run(context.Background(), []string{"reply", "m-1", "--text", "-"}, env, io.Discard, io.Discard); code != 0 {
		t.Fatalf("reply --text code %d", code)
	}
	if body := asJSON(t, rec.calls[0].body); body["text"] != "flag answer" {
		t.Fatalf("reply --text body = %#v", body)
	}
}

func TestMailCLIClearConfirms(t *testing.T) {
	rec, _ := useRecorder(t, `{"deleted":3}`)
	useStdin(t, "n\n")
	var out, errOut strings.Builder
	if code := Run(context.Background(), []string{"clear", "--as", "agent:dev"}, operatorEnv(t), &out, &errOut); code != 0 {
		t.Fatalf("declined code %d", code)
	}
	if len(rec.calls) != 0 || !strings.Contains(errOut.String(), "[y/N]") {
		t.Fatalf("declined: calls %#v stderr %q", rec.calls, errOut.String())
	}
	useStdin(t, "y\n")
	out.Reset()
	if code := Run(context.Background(), []string{"clear", "--as", "agent:dev"}, operatorEnv(t), &out, io.Discard); code != 0 {
		t.Fatalf("confirmed code %d", code)
	}
	if len(rec.calls) != 1 || rec.calls[0].route != "/api/agents/dev/mail/clear" || !strings.Contains(out.String(), "3") {
		t.Fatalf("confirmed: calls %#v stdout %q", rec.calls, out.String())
	}
}

func TestMailCLIErrors(t *testing.T) {
	rec, _ := useRecorder(t, `{}`)
	rec.err = &client.APIError{Code: "not_found", Msg: "no letter m-9"}
	var errOut strings.Builder
	if code := Run(context.Background(), []string{"show", "m-9"}, mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), io.Discard, &errOut); code != 1 {
		t.Fatalf("api error code %d", code)
	}
	if !strings.Contains(errOut.String(), "not_found") || !strings.Contains(errOut.String(), "no letter m-9") {
		t.Fatalf("stderr %q", errOut.String())
	}
	rec.err = errors.New("dial unix /agent.sock: connect: no such file")
	errOut.Reset()
	if code := Run(context.Background(), []string{"inbox"}, mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), io.Discard, &errOut); code != 2 {
		t.Fatalf("socket error code %d (%q)", code, errOut.String())
	}
}

const sampleList = `{"box":"inbox","as":"agent:dev","count":3,"next_before":"2026-10-26T10:00:00Z","letters":[
{"id":"m-1","from":"user:customer","to":"agent:dev","subject":"Question","kind":"message","needs_reply":true},
{"id":"m-2","from":"system:loop","to":"agent:dev","subject":"Done","kind":"task.done","needs_reply":false,"answered_at":"2026-10-26T10:01:00Z"},
{"id":"m-3","from":"agent:qa","to":"agent:dev","subject":"Old","kind":"message","needs_reply":true,"dead_at":"x","dead_reason":"attempts"}]}`

func TestMailCLIPrintsListsAndLetters(t *testing.T) {
	rec, _ := useRecorder(t, sampleList)
	env := mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	var out strings.Builder
	if code := Run(context.Background(), []string{"inbox", "--all"}, env, &out, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	text := out.String()
	for _, want := range []string{"m-1", "user:customer -> agent:dev", "open", "Question", "closed", "dead:attempts", "next: tmail inbox --all --before 2026-10-26T10:00:00Z"} {
		if !strings.Contains(text, want) {
			t.Fatalf("list output lacks %q:\n%s", want, text)
		}
	}
	if lines := strings.Count(text, "\n"); lines != 4 {
		t.Fatalf("list output has %d lines:\n%s", lines, text)
	}
	rec.result = json.RawMessage(`{"letter":{"id":"m-1","from":"user:customer","to":"agent:dev","subject":"Q","kind":"message","body":"What now?","needs_reply":true,"answered_at":"t","answer_id":"m-5"},
"reply":{"id":"m-5","from":"agent:dev","to":"user:customer","subject":"Re: Q","kind":"message","body":"This.","created_at":"t2"}}`)
	out.Reset()
	if code := Run(context.Background(), []string{"show", "m-1"}, env, &out, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	text = out.String()
	for _, want := range []string{"id: m-1", "from: user:customer", "state: answered", "What now?", "reply m-5", "This."} {
		if !strings.Contains(text, want) {
			t.Fatalf("show output lacks %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "What now?") > strings.Index(text, "This.") {
		t.Fatalf("reply printed before body:\n%s", text)
	}
	rec.result = json.RawMessage(sampleList)
	out.Reset()
	if code := Run(context.Background(), []string{"inbox", "--json"}, env, &out, io.Discard); code != 0 || strings.TrimSpace(out.String()) != strings.TrimSpace(sampleList) {
		t.Fatalf("--json code %d output %q", code, out.String())
	}
}

func TestMailCLIHelp(t *testing.T) {
	for _, argv := range [][]string{{}, {"--help"}, {"help"}, {"-h"}, {"send", "--help"}, {"help", "reply"}, {"clear", "-h"}} {
		var out strings.Builder
		code := Run(context.Background(), argv, mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), &out, io.Discard)
		if code != 0 || !strings.Contains(out.String(), "Usage: tmail") {
			t.Fatalf("%q: code %d\n%s", argv, code, &out)
		}
	}
	var out strings.Builder
	Run(context.Background(), []string{"--help"}, mapEnv(), &out, io.Discard)
	for _, name := range []string{"inbox", "sent", "archive", "show", "send", "reply", "requeue", "close", "read", "clear"} {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Fatalf("root help lacks %s:\n%s", name, &out)
		}
	}
	out.Reset()
	if code := RunOperator(context.Background(), []string{"reply", "--help"}, "/daemon.sock", &out, io.Discard); code != 0 || !strings.Contains(out.String(), "Usage: tariboy mail reply") || strings.Contains(out.String(), "tmail ") {
		t.Fatalf("operator help code %d:\n%s", code, &out)
	}
	if code := Run(context.Background(), []string{"help", "bogus"}, mapEnv(), io.Discard, io.Discard); code != 2 {
		t.Fatalf("unknown help code %d", code)
	}
}

func TestMailCLIHelpJSON(t *testing.T) {
	var out strings.Builder
	if code := Run(context.Background(), []string{"--help-json"}, mapEnv(), &out, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	var tree map[string]struct {
		Summary      string            `json:"summary"`
		Usage        string            `json:"usage"`
		Flags        []string          `json:"flags"`
		FlagHelp     map[string]string `json:"flag_help"`
		Examples     []string          `json:"examples"`
		OperatorOnly bool              `json:"operator_only"`
	}
	if err := json.Unmarshal([]byte(out.String()), &tree); err != nil {
		t.Fatalf("%v\n%s", err, &out)
	}
	if len(tree) != len(commands) {
		t.Fatalf("tree has %d commands, want %d", len(tree), len(commands))
	}
	send := tree["send"]
	if send.Summary == "" || !strings.HasPrefix(send.Usage, "send ") || len(send.Examples) == 0 || send.FlagHelp["to"] == "" {
		t.Fatalf("send = %#v", send)
	}
	if !reflect.DeepEqual(send.Flags, []string{"as", "data", "deadline", "kind", "notice", "subject", "text", "to"}) {
		t.Fatalf("send flags = %q", send.Flags)
	}
	if !tree["close"].OperatorOnly || tree["reply"].OperatorOnly {
		t.Fatalf("operator_only: close %v reply %v", tree["close"].OperatorOnly, tree["reply"].OperatorOnly)
	}
}

func TestMailCLIVersion(t *testing.T) {
	var out strings.Builder
	if code := Run(context.Background(), []string{"--version"}, mapEnv(), &out, io.Discard); code != 0 || strings.TrimSpace(out.String()) == "" {
		t.Fatalf("code %d out %q", code, &out)
	}
}
