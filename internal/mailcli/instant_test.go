package mailcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func instantEnv(t *testing.T, flag bool) func(string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "instant-mail")
	if flag {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock", "TARIBOY_INSTANT_MAIL_FLAG", path)
}

func runInstantCLI(t *testing.T, env func(string) string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), append([]string{"instant"}, args...), env, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestInstantWithoutFlagDoesNotCallTheSocket(t *testing.T) {
	rec, sockets := useRecorder(t, `{"ids":["m-1"],"text":"x"}`)
	for _, env := range []func(string) string{
		instantEnv(t, false),
		mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), // no flag variable
	} {
		code, out, errOut := runInstantCLI(t, env)
		if code != 0 || out != "" || errOut != "" {
			t.Fatalf("code %d out %q err %q; want 0 and nothing", code, out, errOut)
		}
	}
	if len(rec.calls) != 0 || len(*sockets) != 0 {
		t.Fatalf("calls = %v sockets = %v; want none", rec.calls, *sockets)
	}
}

func TestInstantWithFlagPrintsTheBlock(t *testing.T) {
	rec, _ := useRecorder(t, `{"ids":["m-1"],"text":"# Instant messages\nhello\n"}`)
	code, out, _ := runInstantCLI(t, instantEnv(t, true))
	if code != 0 || out != "# Instant messages\nhello\n" {
		t.Fatalf("code %d out %q", code, out)
	}
	if len(rec.calls) != 1 || rec.calls[0].method != "POST" || rec.calls[0].route != "/tools/mail/instant" {
		t.Fatalf("calls = %+v", rec.calls)
	}
	code, out, _ = runInstantCLI(t, instantEnv(t, true), "--format", "json")
	var got struct {
		IDs  []string `json:"ids"`
		Text string   `json:"text"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got.IDs) != 1 || got.Text == "" {
		t.Fatalf("json: code %d out %q", code, out)
	}
}

func TestInstantEmptyResultPrintsNothing(t *testing.T) {
	useRecorder(t, `{"ids":[],"text":""}`)
	for _, format := range []string{"text", "json"} {
		if code, out, _ := runInstantCLI(t, instantEnv(t, true), "--format", format); code != 0 || out != "" {
			t.Fatalf("%s: code %d out %q", format, code, out)
		}
	}
}

func TestInstantSocketErrorIsSilent(t *testing.T) {
	rec, _ := useRecorder(t, "")
	rec.err = errors.New("connection refused")
	if code, out, errOut := runInstantCLI(t, instantEnv(t, true)); code != 0 || out != "" || errOut != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

// slowCaller answers after the instant timeout.
type slowCaller struct{}

func (slowCaller) Call(string, string, any) (json.RawMessage, error) {
	time.Sleep(3 * time.Second)
	return json.RawMessage(`{"ids":["m-1"],"text":"late"}`), nil
}

func TestInstantTimeoutIsSilent(t *testing.T) {
	old := newCaller
	t.Cleanup(func() { newCaller = old })
	newCaller = func(string) Caller { return slowCaller{} }
	start := time.Now()
	code, out, errOut := runInstantCLI(t, instantEnv(t, true))
	if code != 0 || out != "" || errOut != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("instant took %s; want the 1.5 s timeout", elapsed)
	}
}

func TestInstantInOperatorModeIsSilent(t *testing.T) {
	rec, _ := useRecorder(t, `{"ids":["m-1"],"text":"x"}`)
	var out bytes.Buffer
	if code := RunOperator(context.Background(), []string{"instant"}, "/daemon.sock", &out, &out); code != 0 || out.Len() != 0 {
		t.Fatalf("operator: code %d out %q", code, out.String())
	}
	if len(rec.calls) != 0 {
		t.Fatalf("operator mode called %v", rec.calls)
	}
}

func TestSendAndReplyPassInstant(t *testing.T) {
	rec, _ := useRecorder(t, `{"letters":[]}`)
	env := mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock")
	if code := Run(context.Background(), []string{"send", "--to", "agent:bob", "--text", "x", "--instant"}, env, io2(), io2()); code != 0 {
		t.Fatalf("send code %d", code)
	}
	if body := asJSON(t, rec.calls[0].body); body["instant"] != true {
		t.Fatalf("send body = %v", body)
	}
	rec.result = json.RawMessage(`{"letter":{},"closed":true}`)
	if code := Run(context.Background(), []string{"reply", "m-1", "ok", "--instant"}, env, io2(), io2()); code != 0 {
		t.Fatalf("reply code %d", code)
	}
	if body := asJSON(t, rec.calls[1].body); body["instant"] != true {
		t.Fatalf("reply body = %v", body)
	}
}

func io2() *bytes.Buffer { return &bytes.Buffer{} }

// hookFormats maps each Claude Code and Codex hook format to the JSON it
// prints for the text block, built here independently of the production code.
func hookFormats(text string) map[string]map[string]any {
	return map[string]map[string]any{
		"claude-post-tool-use": {"hookSpecificOutput": map[string]any{
			"hookEventName": "PostToolUse", "additionalContext": text}},
		"claude-post-tool-use-failure": {"hookSpecificOutput": map[string]any{
			"hookEventName": "PostToolUseFailure", "additionalContext": text}},
		"claude-stop": {"decision": "block", "reason": text},
		"codex-post-tool-use": {"hookSpecificOutput": map[string]any{
			"hookEventName": "PostToolUse", "additionalContext": text}},
		"codex-stop": {"decision": "block", "reason": text},
	}
}

func TestInstantHookFormats(t *testing.T) {
	const text = "# Instant messages\n\"quoted\" <b> & \\ hello\n"
	raw, _ := json.Marshal(map[string]any{"ids": []string{"m-1"}, "text": text})
	for format, want := range hookFormats(text) {
		useRecorder(t, string(raw))
		code, out, errOut := runInstantCLI(t, instantEnv(t, true), "--format", format)
		if code != 0 || errOut != "" {
			t.Fatalf("%s: code %d err %q", format, code, errOut)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%s: output %q is not JSON: %v", format, out, err)
		}
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Fatalf("%s: got %s, want %s", format, gotJSON, wantJSON)
		}
	}
}

func TestInstantHookFormatsAreSilentWithoutLetters(t *testing.T) {
	for format := range hookFormats("") {
		useRecorder(t, `{"ids":[],"text":""}`)
		if code, out, errOut := runInstantCLI(t, instantEnv(t, true), "--format", format); code != 0 || out != "" || errOut != "" {
			t.Fatalf("%s empty: code %d out %q err %q", format, code, out, errOut)
		}
		if code, out, errOut := runInstantCLI(t, instantEnv(t, false), "--format", format); code != 0 || out != "" || errOut != "" {
			t.Fatalf("%s no flag: code %d out %q err %q", format, code, out, errOut)
		}
		if code, out, errOut := runInstantCLI(t, mapEnv("TARIBOY_INSTANT_MAIL_FLAG", "/x"), "--format", format); code != 0 || out != "" || errOut != "" {
			t.Fatalf("%s no socket: code %d out %q err %q", format, code, out, errOut)
		}
		rec, _ := useRecorder(t, "")
		rec.err = errors.New("connection refused")
		if code, out, errOut := runInstantCLI(t, instantEnv(t, true), "--format", format); code != 0 || out != "" || errOut != "" {
			t.Fatalf("%s socket error: code %d out %q err %q", format, code, out, errOut)
		}
	}
}

func TestInstantHookFormatTimeoutIsSilent(t *testing.T) {
	old := newCaller
	t.Cleanup(func() { newCaller = old })
	newCaller = func(string) Caller { return slowCaller{} }
	for _, format := range []string{"claude-stop", "codex-post-tool-use"} {
		start := time.Now()
		code, out, errOut := runInstantCLI(t, instantEnv(t, true), "--format", format)
		if code != 0 || out != "" || errOut != "" {
			t.Fatalf("%s: code %d out %q err %q", format, code, out, errOut)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("%s: instant took %s; want the 1.5 s timeout", format, elapsed)
		}
	}
}

func TestInstantOpenCodeToolAfterFormat(t *testing.T) {
	const text = "# Instant messages\n\"quoted\" <b> & \\ hello\n"
	raw, _ := json.Marshal(map[string]any{"ids": []string{"m-1"}, "text": text})
	useRecorder(t, string(raw))
	code, out, errOut := runInstantCLI(t, instantEnv(t, true), "--format", "opencode-tool-after")
	if code != 0 || errOut != "" || out != "\n\n"+text {
		t.Fatalf("code %d out %q err %q; want the text after a blank line", code, out, errOut)
	}
	useRecorder(t, `{"ids":[],"text":""}`)
	if code, out, errOut := runInstantCLI(t, instantEnv(t, true), "--format", "opencode-tool-after"); code != 0 || out != "" || errOut != "" {
		t.Fatalf("empty: code %d out %q err %q", code, out, errOut)
	}
	if code, out, errOut := runInstantCLI(t, instantEnv(t, false), "--format", "opencode-tool-after"); code != 0 || out != "" || errOut != "" {
		t.Fatalf("no flag: code %d out %q err %q", code, out, errOut)
	}
}
