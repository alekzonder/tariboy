package taskcli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shellText breaks a shell argument: an apostrophe, backticks, a command
// substitution and a backslash escape must reach the daemon unchanged.
const shellText = "It's `fine`: $(date) and tr -d '\\n'\n\n- second line"

func writeTextFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func notTerminal(t *testing.T) {
	t.Helper()
	old := isTerminal
	t.Cleanup(func() { isTerminal = old })
	isTerminal = func(io.Reader) bool { return false }
}

func TestCommentAndAskReadTextFromFileOrStdin(t *testing.T) {
	notTerminal(t)
	path := writeTextFile(t, shellText+"\n")
	tests := []struct {
		name  string
		argv  []string
		stdin string
		op    bool
		route string
		body  map[string]any
	}{
		{"agent comment file", []string{"comment", "DEV-1", "--file", path}, "", false, "/tools/tasks/comment", map[string]any{"key": "DEV-1", "body": shellText}},
		{"agent comment stdin", []string{"comment", "DEV-1"}, shellText + "\n", false, "/tools/tasks/comment", map[string]any{"key": "DEV-1", "body": shellText}},
		{"agent ask file", []string{"ask", "DEV-1", "user:login", "--file", path}, "", false, "/tools/tasks/ask", map[string]any{"key": "DEV-1", "principal": "user:login", "body": shellText}},
		{"agent ask stdin", []string{"ask", "DEV-1", "user:login"}, shellText, false, "/tools/tasks/ask", map[string]any{"key": "DEV-1", "principal": "user:login", "body": shellText}},
		{"operator comment file", []string{"comment", "DEV-1", "--file", path}, "", true, "/api/tasks/DEV-1/comments", map[string]any{"body": shellText}},
		{"operator ask stdin", []string{"ask", "DEV-1", "user:login"}, shellText, true, "/api/tasks/DEV-1/comments", map[string]any{"body": "@user:login\n\n" + shellText, "idempotency_key": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{result: json.RawMessage(`{}`)}
			withCaller(t, r)
			withStdin(t, tt.stdin)
			env := agentEnv()
			if tt.op {
				env = operatorEnv(t)
			}
			var errOut strings.Builder
			if code := Run(context.Background(), tt.argv, env, io.Discard, &errOut); code != 0 {
				t.Fatalf("code %d: %s", code, &errOut)
			}
			if len(r.calls) != 1 || r.calls[0].route != tt.route || !sameJSON(r.calls[0].body, tt.body) {
				t.Fatalf("calls = %#v; want %s %#v", r.calls, tt.route, tt.body)
			}
		})
	}
}

func TestCommentAndAskRefuseBadTextInput(t *testing.T) {
	notTerminal(t)
	path := writeTextFile(t, "text")
	binary := writeTextFile(t, "ok \xff\xfe")
	large := writeTextFile(t, strings.Repeat("x", 64<<10+1))
	tests := []struct {
		name     string
		argv     []string
		stdin    string
		terminal bool
		want     string
	}{
		{"text and file", []string{"comment", "DEV-1", "words", "--file", path}, "", false, "pass TEXT, --body, or --file, not more than one"},
		{"body and file", []string{"comment", "DEV-1", "--body", "b", "--file", path}, "", false, "pass TEXT, --body, or --file, not more than one"},
		{"question and file", []string{"ask", "DEV-1", "worker", "why", "--file", path}, "", false, "pass the question or --file, not both"},
		{"comment terminal", []string{"comment", "DEV-1"}, "never read", true, "pass TEXT, --file PATH, or pipe the text on stdin"},
		{"ask terminal", []string{"ask", "DEV-1", "worker"}, "never read", true, "pass the question, --file PATH, or pipe the text on stdin"},
		{"missing file", []string{"comment", "DEV-1", "--file", filepath.Join(t.TempDir(), "none.md")}, "", false, "no such file"},
		{"binary file", []string{"comment", "DEV-1", "--file", binary}, "", false, binary + " is not valid UTF-8"},
		{"large file", []string{"ask", "DEV-1", "worker", "--file", large}, "", false, "larger than 64 KiB"},
		{"empty stdin comment", []string{"comment", "DEV-1"}, " \n", false, "tasks comment: comment text is required"},
		{"empty stdin ask", []string{"ask", "DEV-1", "worker"}, "", false, "tasks ask: principal and question are required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{result: json.RawMessage(`{}`)}
			withCaller(t, r)
			withStdin(t, tt.stdin)
			isTerminal = func(io.Reader) bool { return tt.terminal }
			var errOut strings.Builder
			if code := Run(context.Background(), tt.argv, agentEnv(), io.Discard, &errOut); code != 2 ||
				!strings.Contains(errOut.String(), tt.want) || len(r.calls) != 0 {
				t.Fatalf("code %d, stderr %q, calls %d; want 2, %q, none", code, &errOut, len(r.calls), tt.want)
			}
		})
	}
}
