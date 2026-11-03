package harness

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOnlyClaudeCodexAndOpenCodeHaveInstantMailHooks(t *testing.T) {
	t.Setenv("TARIBOY_STUB_HARNESS", "/bin/true")
	for _, typ := range []string{"claude", "codex", "opencode", "cursor", "stub"} {
		adapter, err := Get(typ)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := adapter.(InstantMailHooker)
		if ok != (typ == "claude" || typ == "codex" || typ == "opencode") {
			t.Errorf("%s implements InstantMailHooker = %v", typ, ok)
		}
	}
}

type hookEntry struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	} `json:"hooks"`
}

func claudeHooks(t *testing.T, tmail, dir string) (InstantMailHooks, map[string][]hookEntry) {
	t.Helper()
	// Claude ignores the codex approval mode.
	hooks, err := claude{}.InstantMailHooks(tmail, dir, Config{CodexApproval: CodexApprovalApproveForMe})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range hooks.Files {
		if f.Mode != 0o600 {
			t.Errorf("%s mode = %o; want 0600", f.Path, f.Mode)
		}
		files[f.Path] = f.Body
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(files[".claude-plugin/plugin.json"], &manifest); err != nil || manifest.Name != "tariboy-instant-mail" {
		t.Fatalf("plugin.json = %s, %v", files[".claude-plugin/plugin.json"], err)
	}
	var doc struct {
		Hooks map[string][]hookEntry `json:"hooks"`
	}
	if err := json.Unmarshal(files["hooks/hooks.json"], &doc); err != nil {
		t.Fatalf("hooks.json = %s, %v", files["hooks/hooks.json"], err)
	}
	return hooks, doc.Hooks
}

func TestClaudeInstantMailHooksRunTmailInstant(t *testing.T) {
	hooks, events := claudeHooks(t, "/opt/tariboy/bin/tmail", "/agents/a/harness-hooks/claude")
	want := map[string]string{
		"PostToolUse":        "'/opt/tariboy/bin/tmail' instant --format claude-post-tool-use",
		"PostToolUseFailure": "'/opt/tariboy/bin/tmail' instant --format claude-post-tool-use-failure",
		"Stop":               "'/opt/tariboy/bin/tmail' instant --format claude-stop",
	}
	if len(events) != len(want) {
		t.Fatalf("events = %v; want %v", events, want)
	}
	for event, command := range want {
		entries := events[event]
		if len(entries) != 1 || len(entries[0].Hooks) != 1 {
			t.Fatalf("%s = %+v", event, entries)
		}
		h := entries[0].Hooks[0]
		if h.Type != "command" || h.Command != command || h.Timeout != 5 {
			t.Errorf("%s hook = %+v; want command %q, timeout 5", event, h, command)
		}
		if event != "Stop" && entries[0].Matcher != "*" {
			t.Errorf("%s matcher = %q; want *", event, entries[0].Matcher)
		}
	}
	if strings.Join(hooks.Args, " ") != "--plugin-dir /agents/a/harness-hooks/claude" {
		t.Fatalf("args = %q", hooks.Args)
	}
	if hooks.MinimumVersion != minimumClaudeInstantHooksVersion {
		t.Fatalf("minimum version = %q", hooks.MinimumVersion)
	}
}

// The quoted path survives a POSIX shell, which is how Claude Code runs a
// command hook, even with a space and an apostrophe in it.
func TestClaudeInstantMailHooksQuoteTheTmailPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "it's a dir")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tmail := filepath.Join(dir, "tmail")
	if err := os.WriteFile(tmail, []byte("#!/bin/sh\nprintf '%s|' \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, events := claudeHooks(t, tmail, "/x/claude")
	out, err := exec.Command("/bin/sh", "-c", events["Stop"][0].Hooks[0].Command).Output()
	if err != nil || string(out) != "instant|--format|claude-stop|" {
		t.Fatalf("shell run = %q, %v", out, err)
	}
}

func TestClaudeInstantMailHooksNeedAnAbsoluteTmail(t *testing.T) {
	if _, err := (claude{}).InstantMailHooks("tmail", "/x/claude", Config{}); err == nil {
		t.Fatal("relative tmail accepted")
	}
	if _, err := (claude{}).InstantMailHooks("/bin/tmail", "rel/claude", Config{}); err == nil {
		t.Fatal("relative plugin dir accepted")
	}
}

// codexHookCommand returns the command of the one hook in a
// -c hooks.EVENT=[...] value. The value is TOML; its command is a TOML basic
// string, which json.Unmarshal reads because tariboy writes it as JSON.
func codexHookCommand(t *testing.T, arg, event, matcher string) string {
	t.Helper()
	prefix := "hooks." + event + "=[{" + matcher + `hooks=[{type="command",command=`
	suffix := ",timeout=5}]}]"
	if !strings.HasPrefix(arg, prefix) || !strings.HasSuffix(arg, suffix) {
		t.Fatalf("%s arg = %q; want %q...%q", event, arg, prefix, suffix)
	}
	var command string
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(arg, prefix), suffix)), &command); err != nil {
		t.Fatalf("%s command in %q: %v", event, arg, err)
	}
	return command
}

func TestCodexInstantMailHooksRunTmailInstant(t *testing.T) {
	hooks, err := codex{}.InstantMailHooks("/opt/tariboy/bin/tmail", "/agents/a/harness-hooks/codex", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks.Files) != 0 {
		t.Fatalf("files = %+v; want none: the hooks live in argv", hooks.Files)
	}
	if len(hooks.Args) != 5 || hooks.Args[0] != "--dangerously-bypass-hook-trust" || hooks.Args[1] != "-c" || hooks.Args[3] != "-c" {
		t.Fatalf("args = %q", hooks.Args)
	}
	if c := codexHookCommand(t, hooks.Args[2], "PostToolUse", `matcher="*",`); c != "'/opt/tariboy/bin/tmail' instant --format codex-post-tool-use" {
		t.Errorf("PostToolUse command = %q", c)
	}
	if c := codexHookCommand(t, hooks.Args[4], "Stop", ""); c != "'/opt/tariboy/bin/tmail' instant --format codex-stop" {
		t.Errorf("Stop command = %q", c)
	}
	if hooks.MinimumVersion != "0.141.0" {
		t.Fatalf("minimum version = %q", hooks.MinimumVersion)
	}
	yolo, err := codex{}.InstantMailHooks("/opt/tariboy/bin/tmail", "/x", Config{CodexApproval: CodexApprovalYolo})
	if err != nil || strings.Join(yolo.Args, "\x00") != strings.Join(hooks.Args, "\x00") {
		t.Fatalf("yolo hooks = %q, %v; want the default", yolo.Args, err)
	}
}

// Codex runs a command hook through a shell; the quoted path survives it, and
// the TOML string survives a path with quotes and backslashes.
func TestCodexInstantMailHooksQuoteTheTmailPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), `it's a "dir" \x`)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tmail := filepath.Join(dir, "tmail")
	if err := os.WriteFile(tmail, []byte("#!/bin/sh\nprintf '%s|' \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	hooks, err := codex{}.InstantMailHooks(tmail, "/x/codex", Config{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/bin/sh", "-c", codexHookCommand(t, hooks.Args[4], "Stop", "")).Output()
	if err != nil || string(out) != "instant|--format|codex-stop|" {
		t.Fatalf("shell run = %q, %v", out, err)
	}
}

func TestCodexInstantMailHooksAreRefused(t *testing.T) {
	if _, err := (codex{}).InstantMailHooks("tmail", "/x/codex", Config{}); err == nil {
		t.Fatal("relative tmail accepted")
	}
	// The trust bypass would let an unreviewed repository hook run outside the
	// approve-for-me sandbox.
	if _, err := (codex{}).InstantMailHooks("/bin/tmail", "/x/codex", Config{CodexApproval: CodexApprovalApproveForMe}); err == nil {
		t.Fatal("approve-for-me accepted")
	}
}

func writeVersionScript(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho '"+output+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckMinimumVersion(t *testing.T) {
	for output, ok := range map[string]bool{
		"2.1.227 (Claude Code)": true,
		"2.1.316 (Claude Code)": true,
		"2.1.226 (Claude Code)": false,
		"unknown":               false,
	} {
		err := CheckMinimumVersion(writeVersionScript(t, output), nil, "2.1.227")
		if (err == nil) != ok {
			t.Errorf("%q: err = %v; want ok %v", output, err, ok)
		}
	}
}

// Commands is what the stall probe compares a hook shell against, so it must
// hold exactly the commands hooks.json runs.
func TestClaudeInstantMailHooksCommandsMatchHooksJSON(t *testing.T) {
	hooks, events := claudeHooks(t, "/opt/tariboy/bin/tmail", "/agents/a/harness-hooks/claude")
	var written []string
	for _, entries := range events {
		for _, e := range entries {
			for _, h := range e.Hooks {
				written = append(written, h.Command)
			}
		}
	}
	assertSameCommands(t, hooks.Commands, written)
}

func TestCodexInstantMailHooksCommandsMatchArgs(t *testing.T) {
	hooks, err := codex{}.InstantMailHooks("/opt/tariboy/bin/tmail", "/x", Config{})
	if err != nil {
		t.Fatal(err)
	}
	written := []string{
		codexHookCommand(t, hooks.Args[2], "PostToolUse", `matcher="*",`),
		codexHookCommand(t, hooks.Args[4], "Stop", ""),
	}
	assertSameCommands(t, hooks.Commands, written)
}

func assertSameCommands(t *testing.T, got, want []string) {
	t.Helper()
	got, want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("Commands = %q; want %q", got, want)
	}
}
