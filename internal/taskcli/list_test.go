package taskcli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestParseListFlags(t *testing.T) {
	argv := []string{"list", "--queue", "DEV", "--status", "plan,review", "--category", "in_progress", "--priority", "P0,P1",
		"--assignee", "worker", "--author", "user:me", "--customer", "user:me", "--group", "core", "--subtree", "DEV-1",
		"--waiting-for", "me", "--blocked", "false", "--text", "search", "--view", "all", "--sort", "updated", "--limit", "10", "--after", "DEV-9"}
	want := map[string]any{"after": "DEV-9", "assignee": "worker", "author": "user:me", "blocked": "false", "category": "in_progress",
		"customer": "user:me", "group": "core", "limit": "10", "priority": "P0,P1", "queue": "DEV", "sort": "updated", "status": "plan,review",
		"status_view": "all", "subtree": "DEV-1", "text": "search", "waiting_for": "me"}
	for _, command := range []string{"list", "mine"} {
		argv[0] = command
		got, err := parse(argv)
		if err != nil {
			t.Fatal(err)
		}
		if got.action != command || !sameJSON(got.payload, want) {
			t.Fatalf("parse(%q) = %#v, want payload %#v", argv, got, want)
		}
	}
	got, err := parse([]string{"list"})
	if err != nil || !sameJSON(got.payload, map[string]any{"limit": "50"}) {
		t.Fatalf("parse(list) = %#v, %v; want the default limit 50", got, err)
	}
}

func TestParseListRejectsBadValues(t *testing.T) {
	for _, argv := range [][]string{
		{"list", "--parent", "DEV-1", "--subtree", "DEV-1"},
		{"list", "--sort", "title"},
		{"list", "--view", "open"},
		{"list", "--blocked", "yes"},
		{"list", "--limit", "0"},
		{"list", "--limit", "501"},
		{"list", "--priority", "P0,urgent"},
		{"list", "--category", "plan"},
		{"list", "stray"},
	} {
		if _, err := parse(argv); err == nil {
			t.Fatalf("parse(%q) succeeded, want usage error", argv)
		}
	}
}

func TestListRoutesInBothModes(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	recorded := &recorder{result: json.RawMessage(`{"tasks":[],"sequence":1}`)}
	newCaller = func(string) Caller { return recorded }
	argv := []string{"list", "--author", "user:me", "--customer", "user:you", "--view", "closed"}
	if code := Run(context.Background(), argv, operatorEnv(t), io.Discard, io.Discard); code != 0 {
		t.Fatalf("operator code = %d", code)
	}
	if len(recorded.calls) != 1 || recorded.calls[0].method != "GET" || recorded.calls[0].route != "/api/tasks" ||
		!sameJSON(recorded.calls[0].body, map[string]string{"author": "user:me", "customer": "user:you", "status_view": "closed", "limit": "50"}) {
		t.Fatalf("operator calls = %#v", recorded.calls)
	}
	recorded.calls = nil
	if code := Run(context.Background(), argv, mapEnv("TARIBOY_TOOLS_SOCKET", "/agent.sock"), io.Discard, io.Discard); code != 0 {
		t.Fatalf("agent code = %d", code)
	}
	if len(recorded.calls) != 1 || recorded.calls[0].route != "/tools/tasks/list" ||
		!sameJSON(recorded.calls[0].body, map[string]any{"by_author": "user:me", "by_customer": "user:you", "limit": "50", "status_view": "closed"}) {
		t.Fatalf("agent calls = %#v", recorded.calls)
	}
}

func TestListPrintsTableAndNextCommand(t *testing.T) {
	old := newCaller
	defer func() { newCaller = old }()
	recorded := &recorder{result: json.RawMessage(`{"tasks":[
		{"key":"DEV-1","priority":"P1","status":"plan","category":"in_progress","assignee":"worker","updated_at":"2026-10-14T15:47:03.5Z","title":"Design\nthe command","blocked":false},
		{"key":"DEV-2","priority":"P3","status":"open","category":"open","assignee":"","updated_at":"2026-10-13T10:53:00Z","title":"` + strings.Repeat("я", 70) + `","blocked":true}
	],"next_cursor":"DEV-2","sequence":4}`)}
	newCaller = func(string) Caller { return recorded }
	var out strings.Builder
	if code := Run(context.Background(), []string{"list", "--text", "two words", "--limit", "2"}, operatorEnv(t), &out, io.Discard); code != 0 {
		t.Fatalf("code = %d", code)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("output = %q, want header, two rows and next", out.String())
	}
	for _, field := range []string{"KEY", "PRI", "STATUS", "ASSIGNEE", "UPDATED", "TITLE"} {
		if !strings.Contains(lines[0], field) {
			t.Fatalf("header %q lacks %s", lines[0], field)
		}
	}
	for _, field := range []string{"DEV-1", "P1", "plan (in_progress)", "worker", "2026-10-14 15:47", "Design the command"} {
		if !strings.Contains(lines[1], field) {
			t.Fatalf("row %q lacks %q", lines[1], field)
		}
	}
	if !strings.Contains(lines[2], "B DEV-2") || strings.Contains(lines[2], "open (open)") || !strings.Contains(lines[2], strings.Repeat("я", 59)+"…") || strings.Contains(lines[2], strings.Repeat("я", 61)) {
		t.Fatalf("blocked row = %q", lines[2])
	}
	if lines[3] != "next: ttasks list --text 'two words' --limit 2 --after DEV-2" {
		t.Fatalf("next line = %q", lines[3])
	}

	recorded.result = json.RawMessage(`{"tasks":[],"sequence":4}`)
	out.Reset()
	if code := Run(context.Background(), []string{"mine", "--after=DEV-2"}, operatorEnv(t), &out, io.Discard); code != 0 || out.String() != "no tasks match\n" {
		t.Fatalf("empty = %d %q", code, out.String())
	}
}
