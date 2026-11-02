package harness

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyClaudeHasInstantMailHooks(t *testing.T) {
	t.Setenv("TARIBOY_STUB_HARNESS", "/bin/true")
	for _, typ := range []string{"claude", "codex", "opencode", "cursor", "stub"} {
		adapter, err := Get(typ)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := adapter.(InstantMailHooker)
		if ok != (typ == "claude") {
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
	hooks, err := claude{}.InstantMailHooks(tmail, dir)
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
	if _, err := (claude{}).InstantMailHooks("tmail", "/x/claude"); err == nil {
		t.Fatal("relative tmail accepted")
	}
	if _, err := (claude{}).InstantMailHooks("/bin/tmail", "rel/claude"); err == nil {
		t.Fatal("relative plugin dir accepted")
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
