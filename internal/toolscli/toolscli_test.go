package toolscli

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentapi"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/script"
)

func TestHelpDescribesCurrentTaskAsNative(t *testing.T) {
	if !strings.Contains(helpText, "native task") {
		t.Fatalf("help does not describe native current-task attribution:\n%s", helpText)
	}
}

func startAgentAPI(t *testing.T, plugins []string, ctxPath string) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "agent.sock")
	srv := agentapi.NewServer(agentapi.Deps{
		Agent: "smoke", Cwd: "/w", ContextPath: ctxPath, Plugins: plugins,
		CurrentIteration: func() string { return "iter-1" },
		SetDone:          func(string, bool) error { return nil },
		Status:           func() (map[string]any, error) { return map[string]any{"state": "running"}, nil },
	})
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, srv.Handler())
	t.Cleanup(func() { ln.Close() })
	return sock
}

func TestToolsWhoamiAndDone(t *testing.T) {
	sock := startAgentAPI(t, []string{"whoami", "loop", "messages"}, "")
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"whoami"}, &out, &errOut); code != 0 {
		t.Fatalf("whoami exit=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "smoke") {
		t.Fatalf("whoami out=%q", out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"loop", "done"}, &out, &errOut); code != 0 {
		t.Fatalf("loop done exit=%d err=%q", code, errOut.String())
	}
}

func TestToolsScriptCommands(t *testing.T) {
	var created script.CreateOnce
	var scheduled script.CreateSchedule
	var cancelled, removed string
	sock := filepath.Join(t.TempDir(), "agent.sock")
	srv := agentapi.NewServer(agentapi.Deps{
		Agent: "smoke", Plugins: []string{"scripts"},
		RunScriptOnce: func(in script.CreateOnce) (script.Definition, script.Run, error) {
			created = in
			return script.Definition{ID: "scr-1", Name: in.Name, Mode: script.ModeOnce, State: script.StateActive}, script.Run{ID: "srun-1", ScriptID: "scr-1", Status: script.RunPending}, nil
		},
		ScheduleScript: func(in script.CreateSchedule) (script.Definition, script.Run, error) {
			scheduled = in
			return script.Definition{ID: "scr-2", Name: in.Name, Mode: script.ModeEvery, State: script.StateActive}, script.Run{ID: "srun-2", ScriptID: "scr-2", Status: script.RunPending}, nil
		},
		CancelScriptTarget: func(id string) error { cancelled = id; return nil },
		RemoveScript:       func(id string) error { removed = id; return nil },
	})
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, srv.Handler())
	t.Cleanup(func() { ln.Close() })
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"script", "schedule", "watch", "--description", "watch CI", "--every", "60", "--quiet-exit", "2", "--", "check-ci"}, &out, &errOut); code != 0 {
		t.Fatalf("schedule exit=%d err=%q", code, errOut.String())
	}
	if scheduled.IntervalSeconds != 60 || scheduled.QuietExit == nil || *scheduled.QuietExit != 2 || scheduled.Command != "check-ci" || scheduled.Description != "watch CI" {
		t.Fatalf("schedule=%#v", scheduled)
	}
	if code := Run(sock, []string{"script", "run", "check", "--description", "one", "--", "make", "check"}, &out, &errOut); code != 0 {
		t.Fatalf("run exit=%d err=%q", code, errOut.String())
	}
	if created.Command != "make check" || created.Description != "one" {
		t.Fatalf("run=%#v", created)
	}
	if code := Run(sock, []string{"script", "cancel", "scr-1"}, &out, &errOut); code != 0 || cancelled != "scr-1" {
		t.Fatalf("cancel exit=%d id=%q", code, cancelled)
	}
	if code := Run(sock, []string{"script", "rm", "scr-1"}, &out, &errOut); code != 0 || removed != "scr-1" {
		t.Fatalf("rm exit=%d id=%q", code, removed)
	}
	if code := Run(sock, []string{"script", "schedule", "bad", "--every", "0", "--", "true"}, &out, &errOut); code != 2 {
		t.Fatalf("invalid every exit=%d", code)
	}
	if code := Run(sock, []string{"script", "add", "gone", "--", "true"}, &out, &errOut); code != 2 {
		t.Fatalf("removed add exit=%d", code)
	}
}

// TestToolsTaskCurrent proves the CLI maps `task current <id>` and
// `task current --clear` to the daemon SetTask hook with the right body.
func TestToolsTaskCurrent(t *testing.T) {
	var gotID string
	var gotClear bool
	sock := filepath.Join(t.TempDir(), "agent.sock")
	srv := agentapi.NewServer(agentapi.Deps{
		Agent:            "smoke",
		Plugins:          []string{"whoami", "loop", "messages", "current-task"},
		CurrentIteration: func() string { return "iter-1" },
		SetTask: func(id string, clear bool) (map[string]any, error) {
			gotID, gotClear = id, clear
			return map[string]any{"task_id": id, "epic_id": "e-1"}, nil
		},
	})
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, srv.Handler())
	t.Cleanup(func() { ln.Close() })

	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"task", "current", "dev-t-3e1.2"}, &out, &errOut); code != 0 {
		t.Fatalf("task current exit=%d err=%q", code, errOut.String())
	}
	if gotID != "dev-t-3e1.2" || gotClear {
		t.Fatalf("hook got id=%q clear=%v", gotID, gotClear)
	}

	gotID = ""
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"task", "current", "--clear"}, &out, &errOut); code != 0 {
		t.Fatalf("task current --clear exit=%d err=%q", code, errOut.String())
	}
	if !gotClear || gotID != "" {
		t.Fatalf("clear not mapped: id=%q clear=%v", gotID, gotClear)
	}

	// Missing id without --clear is a client-side usage error (exit 2).
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"task", "current"}, &out, &errOut); code != 2 {
		t.Fatalf("bare task current exit=%d, want 2 (err=%q)", code, errOut.String())
	}
}

func TestNativeTasksCommandsMapToTypedAgentActions(t *testing.T) {
	var actions []string
	var bodies []map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks"},
		TaskAction: func(action string, body map[string]any) (any, error) {
			actions = append(actions, action)
			bodies = append(bodies, body)
			return map[string]any{"action": action}, nil
		},
	})
	cases := [][]string{
		{"tasks", "mine"},
		{"tasks", "ready", "--claim"},
		{"tasks", "show", "TEST-1"},
		{"tasks", "create", "--queue", "TEST", "--title", "new task", "--priority", "P0"},
		{"tasks", "create", "--parent", "TEST-1", "--title", "child"},
		{"tasks", "update", "TEST-1", "--status", "in_progress", "--priority", "P1"},
		{"tasks", "assign", "TEST-1", "worker"},
		{"tasks", "comment", "TEST-1", "progress update"},
		{"tasks", "ask", "TEST-1", "user:customer", "Which option?"},
		{"tasks", "move", "TEST-2", "--parent", "TEST-1"},
		{"tasks", "block", "TEST-2", "--by", "TEST-3"},
		{"tasks", "relate", "TEST-1", "TEST-4"},
		{"tasks", "done", "TEST-1", "--complete-anyway"},
	}
	for _, args := range cases {
		var out, errOut bytes.Buffer
		if code := Run(sock, args, &out, &errOut); code != 0 {
			t.Fatalf("%v exit=%d err=%q", args, code, errOut.String())
		}
	}
	wantActions := []string{
		"mine", "ready", "show", "create", "create", "update", "assign",
		"comment", "ask", "move", "block", "relate", "done",
	}
	if strings.Join(actions, ",") != strings.Join(wantActions, ",") {
		t.Fatalf("actions = %v; want %v", actions, wantActions)
	}
	if bodies[1]["claim"] != true || bodies[3]["queue"] != "TEST" ||
		bodies[3]["priority"] != "P0" ||
		bodies[4]["parent_key"] != "TEST-1" ||
		bodies[5]["priority"] != "P1" ||
		bodies[8]["principal"] != "user:customer" ||
		bodies[12]["complete_anyway"] != true {
		t.Fatalf("mapped bodies = %#v", bodies)
	}
}

func TestNativeTasksUpdateForwardsExplicitEmptyManualBlockReason(t *testing.T) {
	var body map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks"},
		TaskAction: func(action string, got map[string]any) (any, error) {
			if action != "update" {
				t.Fatalf("action = %q; want update", action)
			}
			body = got
			return map[string]any{"action": action}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"tasks", "update", "TEST-1", "--manual-block-reason", ""}, &out, &errOut); code != 0 {
		t.Fatalf("update exit=%d err=%q", code, errOut.String())
	}
	value, ok := body["manual_block_reason"]
	if !ok || value != "" {
		t.Fatalf("manual_block_reason = %#v, present=%v; want explicit empty string", value, ok)
	}
}

func TestNativeTasksMoveDetachesOnlyWithExplicitToRoot(t *testing.T) {
	var body map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks"},
		TaskAction: func(action string, got map[string]any) (any, error) {
			body = got
			return map[string]any{"action": action}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"tasks", "move", "TEST-2", "--to-root"}, &out, &errOut); code != 0 {
		t.Fatalf("move --to-root exit=%d err=%q", code, errOut.String())
	}
	value, ok := body["parent_key"]
	if !ok || value != "" {
		t.Fatalf("parent_key = %#v, present=%v; want explicit empty string", value, ok)
	}

	// A move with no target used to detach silently, which makes a dropped --parent
	// indistinguishable from "take it out of its tree".
	body = nil
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"tasks", "move", "TEST-2"}, &out, &errOut); code == 0 {
		t.Fatalf("bare move exit=0, body=%#v; want a refusal", body)
	}
	if body != nil {
		t.Fatalf("bare move reached the daemon with %#v; want no request", body)
	}
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"tasks", "move", "TEST-2", "--to-root", "--parent", "TEST-1"}, &out, &errOut); code == 0 {
		t.Fatal("move --to-root --parent exit=0; want a refusal")
	}
}

func TestNativeTasksCreateExplainsFiledReport(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks"},
		TaskAction: func(action string, body map[string]any) (any, error) {
			return map[string]any{"key": "OWN-7", "queue": "OWN", "filed": true}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"tasks", "create", "--queue", "OWN", "--title", "lint red on main"}, &out, &errOut); code != 0 {
		t.Fatalf("create exit=%d err=%q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"OWN-7", "no longer visible to you", "do not file it again"} {
		if !strings.Contains(text, want) {
			t.Fatalf("create output %q; want it to contain %q", text, want)
		}
	}
}

func TestNativeTasksUpdateOmitsUntypedManualBlockReason(t *testing.T) {
	var body map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks"},
		TaskAction: func(action string, got map[string]any) (any, error) {
			if action != "update" {
				t.Fatalf("action = %q; want update", action)
			}
			body = got
			return map[string]any{"action": action}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"tasks", "update", "TEST-1", "--status", "in_progress"}, &out, &errOut); code != 0 {
		t.Fatalf("update exit=%d err=%q", code, errOut.String())
	}
	if value, ok := body["manual_block_reason"]; ok {
		t.Fatalf("manual_block_reason = %#v, present=%v; want absent (payload=%#v)", value, ok, body)
	}
	if len(body) != 2 || body["key"] != "TEST-1" || body["status"] != "in_progress" {
		t.Fatalf("payload = %#v; want exactly key and status", body)
	}
}

func TestNativeTasksRequiredValuesRejectEmptyOrMissing(t *testing.T) {
	for _, args := range [][]string{
		{"create", "--queue", "TEST", "--title", ""},
		{"create", "--title", "task"},
		{"assign", "TEST-1"},
	} {
		if _, _, _, err := nativeTasksCommand(newArgScan(args)); err == nil {
			t.Fatalf("nativeTasksCommand(%v) succeeded; want required-value error", args)
		}
	}
}

func TestNativeTasksShowPrintsPriority(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks"},
		TaskAction: func(action string, _ map[string]any) (any, error) {
			return map[string]any{"key": "TEST-1", "priority": "P0", "title": "urgent"}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"tasks", "show", "TEST-1"}, &out, &errOut); code != 0 {
		t.Fatalf("show exit=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "priority: P0") {
		t.Fatalf("show output = %q, want priority", out.String())
	}
	out.Reset()
	if code := Run(sock, []string{"tasks", "show", "TEST-1", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("show --json exit=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"priority":"P0"`) {
		t.Fatalf("show --json output = %q, want priority", out.String())
	}
}

// TestToolsLoopDoneIdle proves the --idle flag threads through the CLI and
// handler as productive = !idle: a plain `loop done` reports productive, and
// `loop done --idle` reports non-productive, into SetDone.
func TestToolsLoopDoneIdle(t *testing.T) {
	var gotProductive bool
	sock := filepath.Join(t.TempDir(), "agent.sock")
	srv := agentapi.NewServer(agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "iter-1" },
		SetDone:          func(_ string, productive bool) error { gotProductive = productive; return nil },
		Status:           func() (map[string]any, error) { return map[string]any{"state": "running"}, nil },
	})
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, srv.Handler())
	t.Cleanup(func() { ln.Close() })

	var out, errOut bytes.Buffer
	// plain done => productive true
	if code := Run(sock, []string{"loop", "done"}, &out, &errOut); code != 0 {
		t.Fatalf("loop done exit=%d err=%q", code, errOut.String())
	}
	if !gotProductive {
		t.Fatalf("plain loop done: productive=%v, want true", gotProductive)
	}
	// idle done => productive false
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"loop", "done", "--idle"}, &out, &errOut); code != 0 {
		t.Fatalf("loop done --idle exit=%d err=%q", code, errOut.String())
	}
	if gotProductive {
		t.Fatalf("loop done --idle: productive=%v, want false", gotProductive)
	}
}

// TestToolsLoopDoneIdleValue proves --idle honors its parsed value, not just
// presence: --idle=false / --idle 0 stay productive; --idle=true / --idle 1
// are idle. Regression for lpq.9 (toolscli.go ignored the parsed value).
func TestToolsLoopDoneIdleValue(t *testing.T) {
	var gotProductive bool
	sock := filepath.Join(t.TempDir(), "agent.sock")
	srv := agentapi.NewServer(agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "iter-1" },
		SetDone:          func(_ string, productive bool) error { gotProductive = productive; return nil },
		Status:           func() (map[string]any, error) { return map[string]any{"state": "running"}, nil },
	})
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, srv.Handler())
	t.Cleanup(func() { ln.Close() })

	cases := []struct {
		args           []string
		wantProductive bool
	}{
		{[]string{"loop", "done", "--idle=false"}, true},
		{[]string{"loop", "done", "--idle", "0"}, true},
		{[]string{"loop", "done", "--idle=true"}, false},
		{[]string{"loop", "done", "--idle", "1"}, false},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		if code := Run(sock, tc.args, &out, &errOut); code != 0 {
			t.Fatalf("%v exit=%d err=%q", tc.args, code, errOut.String())
		}
		if gotProductive != tc.wantProductive {
			t.Fatalf("%v: productive=%v, want %v", tc.args, gotProductive, tc.wantProductive)
		}
	}
}

func TestToolsStatusSet(t *testing.T) {
	var gotMsg string
	sock := filepath.Join(t.TempDir(), "agent.sock")
	srv := agentapi.NewServer(agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages", "status"},
		CurrentIteration: func() string { return "iter-1" },
		SetDone:          func(string, bool) error { return nil },
		Status:           func() (map[string]any, error) { return map[string]any{"state": "running"}, nil },
		SetStatus: func(msg string) (map[string]any, error) {
			gotMsg = msg
			return map[string]any{"message": msg, "updated": "2026-07-09T10:00:00Z"}, nil
		},
	})
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, srv.Handler())
	t.Cleanup(func() { ln.Close() })

	var out, errOut bytes.Buffer
	// A multi-word message is joined into one status line.
	if code := Run(sock, []string{"status", "set", "reviewing", "the", "diff"}, &out, &errOut); code != 0 {
		t.Fatalf("status set exit=%d err=%q", code, errOut.String())
	}
	if gotMsg != "reviewing the diff" {
		t.Fatalf("server received %q", gotMsg)
	}
	if !strings.Contains(out.String(), "reviewing the diff") {
		t.Fatalf("out=%q", out.String())
	}
	// A missing message is a usage error (exit 2), no server call.
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"status", "set"}, &out, &errOut); code != 2 {
		t.Fatalf("status set with no message: exit=%d, want 2", code)
	}
}

func TestToolsContextRoundTrip(t *testing.T) {
	ctxPath := filepath.Join(t.TempDir(), "CONTEXT.md")
	sock := startAgentAPI(t, []string{"whoami", "loop", "messages", "context"}, ctxPath)
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"context", "set", "hello world"}, &out, &errOut); code != 0 {
		t.Fatalf("set exit=%d err=%q", code, errOut.String())
	}
	if data, _ := os.ReadFile(ctxPath); string(data) != "hello world" {
		t.Fatalf("CONTEXT.md = %q", data)
	}
	out.Reset()
	if code := Run(sock, []string{"context", "get"}, &out, &errOut); code != 0 {
		t.Fatalf("get exit=%d", code)
	}
	if !strings.Contains(out.String(), "hello world") {
		t.Fatalf("get out=%q", out.String())
	}
}

func TestToolsPluginDisabled(t *testing.T) {
	sock := startAgentAPI(t, []string{"whoami", "loop", "messages"}, "")
	var out, errOut bytes.Buffer
	code := Run(sock, []string{"context", "get"}, &out, &errOut)
	if code == 0 || !strings.Contains(errOut.String(), "plugin 'context' not enabled") {
		t.Fatalf("exit=%d err=%q", code, errOut.String())
	}
}

func TestToolsHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run("", []string{"help"}, &out, &errOut); code != 0 {
		t.Fatalf("help exit=%d", code)
	}
	for _, want := range []string{"loop done", "whoami", "context get", "context set", "status"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q: %s", want, out.String())
		}
	}
}

func TestToolsMissingSocket(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run("", []string{"whoami"}, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d, want 2 (no socket)", code)
	}
}

func startAgentAPIFull(t *testing.T, d agentapi.Deps) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "agent.sock")
	srv := agentapi.NewServer(d)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, srv.Handler())
	t.Cleanup(func() { ln.Close() })
	return sock
}

func TestToolsMessageSend(t *testing.T) {
	var got bus.Message
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		Publish: func(m bus.Message) (bus.Message, error) { m.ID = "m1"; got = m; return m, nil },
	})
	var out, errOut bytes.Buffer
	code := Run(sock, []string{"message", "send", "--channel", "chat:room", "--type", "note",
		"--subject", "env=prod", "--text", "hello there"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d err=%q", code, errOut.String())
	}
	if got.Channel != "chat:room" || got.Type != "note" || got.Text != "hello there" ||
		got.Subject["env"] != "prod" {
		t.Fatalf("published wrong message: %+v", got)
	}
}

func TestToolsChannelSubscribeAndSources(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		Subscribe: func(ch string, mt bus.Matcher, tf []string) (bus.Subscription, error) {
			return bus.Subscription{ID: "sub-1", Channel: ch}, nil
		},
		Channels: func() ([]bus.Channel, error) { return []bus.Channel{{Name: "chat:room", Kind: "chat"}}, nil },
		ProvidedChannels: func() ([]agentapi.ProvidedChannel, error) {
			return []agentapi.ProvidedChannel{
				{Channel: "issue-provider:query", Provider: "issue-provider", Params: []string{"query"}, Help: "run a query"},
			}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"channel", "subscribe", "chat:room", "--type", "note.*"}, &out, &errOut); code != 0 {
		t.Fatalf("subscribe exit=%d err=%q", code, errOut.String())
	}
	out.Reset()
	if code := Run(sock, []string{"sources"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "chat:room") {
		t.Fatalf("sources exit=%d out=%q", code, out.String())
	}
	// Provider channel is listed even without a channel row, and its annotation
	// (provider=, params: {...}, first help line) is rendered on one line.
	line := ""
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, "issue-provider:query") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("sources missing issue-provider:query row: %q", out.String())
	}
	for _, want := range []string{"provider=issue-provider", "params: {query}", "run a query"} {
		if !strings.Contains(line, want) {
			t.Fatalf("issue-provider:query row %q missing %q", line, want)
		}
	}
}

// TestToolsSourcesZeroChannels verifies renderSources prints a non-blank,
// informative line when the response has no channels (review finding #4): it
// used to suppress the generic renderer and print nothing at all.
func TestToolsSourcesZeroChannels(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		Channels: func() ([]bus.Channel, error) { return nil, nil },
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"sources"}, &out, &errOut); code != 0 {
		t.Fatalf("sources exit=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "no channels") {
		t.Fatalf("sources with zero channels should print an informative line, got %q", out.String())
	}
}

func TestToolsHelpListsNewCommands(t *testing.T) {
	var out, errOut bytes.Buffer
	Run("", []string{"help"}, &out, &errOut)
	for _, want := range []string{"message send", "message ls", "message processed", "message reply",
		"message dlq", "request --channel", "channel subscribe", "sources", "schedule add", "script run", "image build"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q", want)
		}
	}
}

func TestToolsGroupCommands(t *testing.T) {
	var got bus.Message
	var groupLoopMember, groupLoopAction, selfLoopAction string
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "manager", Plugins: []string{"whoami", "loop", "messages", "status"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		GroupInfo: func() (map[string]any, error) {
			return map[string]any{"group": "dev-team", "lead": "manager", "members": []string{"manager", "worker"}}, nil
		},
		GroupStatus: func(member string) (map[string]any, error) {
			if member == "worker" {
				return map[string]any{"member": map[string]any{"name": "worker", "state": "idle"}}, nil
			}
			return map[string]any{"members": []map[string]any{{"name": "manager"}, {"name": "worker"}}, "count": 2}, nil
		},
		GroupSend: func(member, typ, text, deadline string) (map[string]any, error) {
			got = bus.Message{Channel: bus.InboxChannel(member), Type: typ, Text: text, Data: map[string]any{"deadline": deadline}}
			return map[string]any{"sent": true, "target": member}, nil
		},
		GroupObserve: func(member string, tail int) (map[string]any, error) {
			return map[string]any{"member": member, "tail": tail, "events": []map[string]any{{"kind": "status"}}, "count": 1}, nil
		},
		GroupLoop: func(member, action string) (map[string]any, error) {
			groupLoopMember, groupLoopAction = member, action
			return map[string]any{"member": member, "action": action}, nil
		},
		LoopControl: func(action string) (map[string]any, error) {
			selfLoopAction = action
			return map[string]any{"action": action}, nil
		},
	})
	var out, errOut bytes.Buffer

	if code := Run(sock, []string{"group", "info"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "dev-team") {
		t.Fatalf("group info exit=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := Run(sock, []string{"group", "status", "worker"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "worker") {
		t.Fatalf("group status exit=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := Run(sock, []string{"group", "request", "worker", "--deadline", "5m", "--text", "what is blocking you?"}, &out, &errOut); code != 0 {
		t.Fatalf("group request exit=%d err=%q", code, errOut.String())
	}
	if got.Channel != "agent:worker:inbox" || got.Type != "group.request" || got.Text != "what is blocking you?" ||
		got.Data["deadline"] != "5m" {
		t.Fatalf("group request published wrong message: %+v", got)
	}
	out.Reset()
	if code := Run(sock, []string{"group", "observe", "worker", "--tail", "7"}, &out, &errOut); code != 0 ||
		!strings.Contains(out.String(), "tail: 7") {
		t.Fatalf("group observe exit=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := Run(sock, []string{"group", "loop", "start", "worker"}, &out, &errOut); code != 0 ||
		groupLoopMember != "worker" || groupLoopAction != "start" {
		t.Fatalf("group loop start exit=%d member=%q action=%q err=%q", code, groupLoopMember, groupLoopAction, errOut.String())
	}
	out.Reset()
	if code := Run(sock, []string{"loop", "stop"}, &out, &errOut); code != 0 || selfLoopAction != "stop" {
		t.Fatalf("loop stop exit=%d action=%q err=%q", code, selfLoopAction, errOut.String())
	}
}

func TestToolsGroupUsageErrors(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "manager", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
	})
	cases := [][]string{
		{"group", "send"},
		{"group", "send", "worker"},
		{"group", "request", "worker"},
		{"group", "observe"},
		{"group", "loop", "start"},
	}
	for _, args := range cases {
		var out, errOut bytes.Buffer
		if code := Run(sock, args, &out, &errOut); code != 2 {
			t.Fatalf("%v exit=%d, want 2 (err=%q)", args, code, errOut.String())
		}
	}
}

func TestImageBuildVerb(t *testing.T) {
	var gotTag, gotPath string
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "creator", Plugins: []string{"whoami", "loop", "messages", "image-creator"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		BuildImage: func(name, tag, path string) (map[string]any, error) {
			gotTag, gotPath = name+":"+tag, path
			return map[string]any{"name": "authored", "tag": "latest", "digest": "abc", "layers": 2}, nil
		},
	})
	var out, errOut bytes.Buffer
	code := Run(sock, []string{"image", "build", "--name", "authored", "--path", "authored"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, err = %s", code, errOut.String())
	}
	if gotTag != "authored:latest" || gotPath != "authored" {
		t.Fatalf("BuildImage called with (%q,%q)", gotTag, gotPath)
	}
}

func TestImageBuildVerbMissingFlags(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "creator", Plugins: []string{"whoami", "loop", "messages", "image-creator"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		BuildImage: func(name, tag, path string) (map[string]any, error) {
			t.Fatal("must not be called")
			return nil, nil
		},
	})
	var out, errOut bytes.Buffer
	code := Run(sock, []string{"image", "build", "--name", "authored"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "--name and --path are required") {
		t.Fatalf("errOut = %q", errOut.String())
	}
}

// TestToolsPhasePRouting proves the Phase P subcommands map to the right routes
// and bodies: processed (id + result from trailing args), reply (id + data),
// request (channel/text/deadline), ls --all, dlq requeue, and subscribe --params.
func TestToolsPhasePRouting(t *testing.T) {
	var gotProcessed struct{ id, result string }
	var gotReply struct {
		id, text string
		data     map[string]any
	}
	var gotReq struct{ channel, text, deadline string }
	var gotRequeue string
	var gotParams map[string]any
	var lsStatus string
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		Inbox: func(status string, _ int, _ string) ([]bus.InboxItem, error) {
			lsStatus = status
			return nil, nil
		},
		MarkProcessed: func(id, result string) (bus.InboxItem, error) {
			gotProcessed.id, gotProcessed.result = id, result
			return bus.InboxItem{Message: bus.Message{ID: id}, Result: result}, nil
		},
		Reply: func(id, text string, data map[string]any, _ string) (bus.Message, error) {
			gotReply.id, gotReply.text, gotReply.data = id, text, data
			return bus.Message{ID: "r1", InReplyTo: id}, nil
		},
		Request: func(channel, text, deadline string) (bus.Message, error) {
			gotReq.channel, gotReq.text, gotReq.deadline = channel, text, deadline
			return bus.Message{ID: "q1", CorrelationID: "q1", Channel: channel}, nil
		},
		Requeue: func(id string) error { gotRequeue = id; return nil },
		SubscribeParams: func(ch string, _ bus.Matcher, _ []string, params map[string]any) (bus.Subscription, error) {
			gotParams = params
			return bus.Subscription{ID: "sub-1", Channel: ch, Watch: "wf"}, nil
		},
	})
	run := func(args ...string) {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := Run(sock, args, &out, &errOut); code != 0 {
			t.Fatalf("%v exit=%d err=%q", args, code, errOut.String())
		}
	}

	run("message", "processed", "m1", "read", "and", "acted")
	if gotProcessed.id != "m1" || gotProcessed.result != "read and acted" {
		t.Fatalf("processed = %+v", gotProcessed)
	}

	run("message", "reply", "m2", "--text", "answer", "--data", `{"k":"v"}`)
	if gotReply.id != "m2" || gotReply.text != "answer" || gotReply.data["k"] != "v" {
		t.Fatalf("reply = %+v", gotReply)
	}

	run("request", "--channel", "svc:q", "--text", "do X", "--deadline", "5m")
	if gotReq.channel != "svc:q" || gotReq.text != "do X" || gotReq.deadline != "5m" {
		t.Fatalf("request = %+v", gotReq)
	}

	run("message", "dlq", "requeue", "m3")
	if gotRequeue != "m3" {
		t.Fatalf("requeue = %q", gotRequeue)
	}

	run("channel", "subscribe", "issue-provider:query", "--params", `{"query":"Open"}`)
	if gotParams["query"] != "Open" {
		t.Fatalf("params = %v", gotParams)
	}

	run("message", "ls", "--all")
	if lsStatus != "all" {
		t.Fatalf("ls --all status = %q, want all", lsStatus)
	}
	run("message", "ls")
	if lsStatus != "pending" {
		t.Fatalf("ls status = %q, want pending", lsStatus)
	}
}

// TestToolsProcessedRequiresResult: the CLI rejects an empty result before it
// ever reaches the socket.
func TestToolsProcessedRequiresResult(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		MarkProcessed: func(string, string) (bus.InboxItem, error) {
			t.Fatal("MarkProcessed must not be called on empty result")
			return bus.InboxItem{}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"message", "processed", "m1"}, &out, &errOut); code != 2 {
		t.Fatalf("expected exit 2 for missing result, got %d", code)
	}
	if !strings.Contains(errOut.String(), "result is required") {
		t.Fatalf("err=%q", errOut.String())
	}
}

func TestToolsChatCommandIsUnknown(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"chat", "ls"}, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("err=%q", errOut.String())
	}
}

func TestToolsJudgeCommandsMapBodiesAndReadJSONFiles(t *testing.T) {
	var actions []string
	var bodies []map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "smoke", Plugins: []string{"llm-as-judge"}, CurrentIteration: func() string { return "i" },
		JudgeAction: func(action string, body map[string]any) (map[string]any, error) {
			actions, bodies = append(actions, action), append(bodies, body)
			return map[string]any{"ok": true}, nil
		},
	})
	dir := t.TempDir()
	requestFile := filepath.Join(dir, "criteria.txt")
	resultFile := filepath.Join(dir, "result.json")
	if err := os.WriteFile(requestFile, []byte("judge this exactly"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultFile, []byte(`{"verdict":"pass"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"judge", "run", "create", "--request-file", requestFile, "--selector", `{"iteration_ids":["i-1"]}`, "--judges", "j1,j2", "--summary-agent", "lead", "--judge-group", "judges", "--judges-per-iteration", "2"}, &out, &errOut); code != 0 {
		t.Fatalf("create exit=%d err=%q", code, errOut.String())
	}
	if actions[0] != "run.create" || bodies[0]["original_request"] != "judge this exactly" || bodies[0]["agent"] != nil || bodies[0]["iteration"] != nil {
		t.Fatalf("create body=%v", bodies[0])
	}
	if code := Run(sock, []string{"judge", "analysis", "submit", "--assignment", "a1", "--file", resultFile}, &out, &errOut); code != 0 {
		t.Fatalf("submit exit=%d err=%q", code, errOut.String())
	}
	if actions[1] != "analysis.submit" || bodies[1]["assignment_id"] != "a1" || bodies[1]["raw_submission"] != `{"verdict":"pass"}` {
		t.Fatalf("submit body=%v", bodies[1])
	}
	if code := Run(sock, []string{"judge", "improvement", "submit", "run-1", "--file", resultFile}, &out, &errOut); code != 0 {
		t.Fatalf("improvement submit exit=%d err=%q", code, errOut.String())
	}
	if actions[2] != "improvement.submit" || bodies[2]["run_id"] != "run-1" || bodies[2]["raw_submission"] != `{"verdict":"pass"}` {
		t.Fatalf("improvement body=%v", bodies[2])
	}
	if code := Run(sock, []string{"judge", "run", "create", "--request-file", requestFile, "--selector", `{bad`, "--judges", "j1", "--summary-agent", "lead"}, &out, &errOut); code != 2 {
		t.Fatalf("invalid selector exit=%d err=%q", code, errOut.String())
	}
}

func TestToolsJudgeAutomationBeginIsAThinAuthenticatedAction(t *testing.T) {
	var action string
	var body map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "lead", Plugins: []string{"llm-as-judge"}, CurrentIteration: func() string { return "iteration-1" },
		JudgeAction: func(gotAction string, gotBody map[string]any) (map[string]any, error) {
			action, body = gotAction, gotBody
			return map[string]any{"ok": true}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"judge", "automation", "begin", "--revision", "7", "--delivery", "delivery-1", "--limit", "3"}, &out, &errOut); code != 0 {
		t.Fatalf("begin exit=%d err=%q", code, errOut.String())
	}
	if action != "automation.begin" || body["config_revision"] != float64(7) || body["delivery_id"] != "delivery-1" || body["limit"] != float64(3) {
		t.Fatalf("action=%q body=%v", action, body)
	}
}

func TestToolsJudgeIterationsSearchSeparatesJudgeAndTargetGroups(t *testing.T) {
	var actions []string
	var bodies []map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "smoke", Plugins: []string{"llm-as-judge"}, CurrentIteration: func() string { return "i" },
		JudgeAction: func(action string, body map[string]any) (map[string]any, error) {
			actions, bodies = append(actions, action), append(bodies, body)
			return map[string]any{"ok": true}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"judge", "iterations", "search", "--agent", "target", "--judge-group", "judges", "--group", "targets"}, &out, &errOut); code != 0 {
		t.Fatalf("search exit=%d err=%q", code, errOut.String())
	}
	if len(actions) != 1 || actions[0] != "iterations.search" {
		t.Fatalf("actions=%v", actions)
	}
	if bodies[0]["judge_group"] != "judges" || bodies[0]["group"] != nil {
		t.Fatalf("search body=%v", bodies[0])
	}
	selector := bodies[0]["selector"].(map[string]any)
	if selector["group"] != "targets" || selector["judge_group"] != nil {
		t.Fatalf("search selector=%v", selector)
	}

	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"judge", "iterations", "search", "--agent", "target"}, &out, &errOut); code != 2 {
		t.Fatalf("missing judge group exit=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "--judge-group") {
		t.Fatalf("missing judge group error=%q", errOut.String())
	}
	if len(actions) != 1 {
		t.Fatalf("missing judge group sent request: actions=%v", actions)
	}
}

func TestToolsJudgeWorkClaimPrintsCriteria(t *testing.T) {
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "judge", Plugins: []string{"llm-as-judge"}, CurrentIteration: func() string { return "i" },
		JudgeAction: func(action string, body map[string]any) (map[string]any, error) {
			if action != "work.claim" {
				t.Fatalf("action=%q", action)
			}
			return map[string]any{"claimed": true, "criteria": "evaluate the preserved request"}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"judge", "work", "claim"}, &out, &errOut); code != 0 {
		t.Fatalf("claim exit=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "criteria: evaluate the preserved request") {
		t.Fatalf("claim output=%q", out.String())
	}
}

// startCountingSock serves an agent socket that answers everything with an
// empty object and counts requests. Tests that must prove "no request reached
// the daemon" assert the counter stayed at zero — an exit code alone cannot
// tell a client-side rejection from a failed round trip.
func startCountingSock(t *testing.T, hits *int32) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{}")
	}))
	t.Cleanup(func() { ln.Close() })
	return sock
}

// TestToolsUnknownFlagIsRejected pins the SUPER-224 root cause: a flag this
// build of the client does not know must be a loud client-side error, never a
// silent no-op. It covers both spellings (--flag value and --flag=value), a
// command that reads no flags at all, and the two subcommand builders.
func TestToolsUnknownFlagIsRejected(t *testing.T) {
	var hits int32
	sock := startCountingSock(t, &hits)
	cases := []struct {
		args []string
		flag string
	}{
		{[]string{"status", "set", "x", "--bogus"}, "--bogus"},
		{[]string{"whoami", "--bogus"}, "--bogus"},
		{[]string{"tasks", "update", "SUPER-1", "--prioriti", "P1"}, "--prioriti"},
		{[]string{"tasks", "update", "SUPER-1", "--prioriti=P1"}, "--prioriti"},
		{[]string{"message", "send", "--channel", "c", "--text", "t", "--bogus=1"}, "--bogus"},
		{[]string{"judge", "work", "claim", "--bogus"}, "--bogus"},
		{[]string{"context", "set", "hello", "--bogus"}, "--bogus"},
		{[]string{"script", "run", "n", "--description", "d", "--bogus", "--", "true"}, "--bogus"},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := Run(sock, tc.args, &out, &errOut)
		if code != 2 {
			t.Fatalf("%v exit=%d, want 2 (err=%q)", tc.args, code, errOut.String())
		}
		if !strings.Contains(errOut.String(), "unknown flag "+tc.flag) {
			t.Fatalf("%v errOut=%q, want it to name %s", tc.args, errOut.String(), tc.flag)
		}
		// The message must say which command rejected the flag, so a version
		// drift is diagnosable from the error alone.
		if !strings.Contains(errOut.String(), tc.args[0]) {
			t.Fatalf("%v errOut=%q does not name the command", tc.args, errOut.String())
		}
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("client made %d request(s) to the daemon; want 0", n)
	}
}

// TestNativeTasksUpdatePriorityReachesPayload is the regression for the symptom
// that opened SUPER-224: `tasks update KEY --priority P1` must be understood by
// this build and must carry the priority into the request body.
func TestNativeTasksUpdatePriorityReachesPayload(t *testing.T) {
	var body map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks"},
		TaskAction: func(action string, got map[string]any) (any, error) {
			body = got
			return map[string]any{"action": action}, nil
		},
	})
	var out, errOut bytes.Buffer
	if code := Run(sock, []string{"tasks", "update", "SUPER-1", "--priority", "P1"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d err=%q", code, errOut.String())
	}
	if body["priority"] != "P1" || body["key"] != "SUPER-1" {
		t.Fatalf("payload=%#v", body)
	}
}

func TestWorkflowTaskCommandsTranslateWithoutIdentity(t *testing.T) {
	var action string
	var body map[string]any
	sock := startAgentAPIFull(t, agentapi.Deps{Agent: "alice", Plugins: []string{"tasks"}, TaskAction: func(gotAction string, got map[string]any) (any, error) {
		action, body = gotAction, got
		return map[string]any{"ok": true}, nil
	}})
	var out, errOut bytes.Buffer
	args := []string{"tasks", "work", "complete", "42", "--task-revision", "7", "--assignment-revision", "3", "--outcome", "approved", "--idempotency-key", "finish-42"}
	if code := Run(sock, args, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d err=%q", code, errOut.String())
	}
	if action != "work_complete" || body["assignment_id"] != "42" || body["outcome"] != "approved" {
		t.Fatalf("action/body=%q %#v", action, body)
	}
	for _, forbidden := range []string{"agent", "actor", "author", "principal"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("identity leaked in payload: %#v", body)
		}
	}
}

// TestToolsKnownFlagsAndPositionalsSurviveTheCheck guards the other side of the
// rejection: values that merely look like flags, positionals, and the script
// command tail after `--` must still parse exactly as before.
func TestToolsKnownFlagsAndPositionalsSurviveTheCheck(t *testing.T) {
	var body map[string]any
	var created script.CreateOnce
	sock := startAgentAPIFull(t, agentapi.Deps{
		Agent: "alice", Plugins: []string{"tasks", "scripts"},
		TaskAction: func(action string, got map[string]any) (any, error) {
			body = got
			return map[string]any{"action": action}, nil
		},
		RunScriptOnce: func(in script.CreateOnce) (script.Definition, script.Run, error) {
			created = in
			return script.Definition{ID: "scr-1", Name: in.Name, Mode: script.ModeOnce, State: script.StateActive}, script.Run{ID: "srun-1", ScriptID: "scr-1", Status: script.RunPending}, nil
		},
	})
	var out, errOut bytes.Buffer
	// --body=<value starting with dashes> keeps its value; the key stays positional.
	if code := Run(sock, []string{"tasks", "comment", "SUPER-1", "--body=--dashes in the value"}, &out, &errOut); code != 0 {
		t.Fatalf("comment exit=%d err=%q", code, errOut.String())
	}
	if body["key"] != "SUPER-1" || body["body"] != "--dashes in the value" {
		t.Fatalf("comment payload=%#v", body)
	}
	// Everything after `--` is the script's command line, flags included.
	out.Reset()
	errOut.Reset()
	if code := Run(sock, []string{"script", "run", "watch", "--description", "d", "--", "ci", "--verbose"}, &out, &errOut); code != 0 {
		t.Fatalf("script run exit=%d err=%q", code, errOut.String())
	}
	if created.Command != "ci --verbose" {
		t.Fatalf("script command=%q", created.Command)
	}
}
