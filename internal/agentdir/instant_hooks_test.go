package agentdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstantHooksDirIsUnderTheAgentRoot(t *testing.T) {
	l := New("/base/agents", "bob")
	if got := l.InstantHooksDir("claude"); got != "/base/agents/bob/harness-hooks/claude" {
		t.Fatalf("InstantHooksDir = %q", got)
	}
}

func TestWritePluginFilesReplacesTheContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "harness-hooks", "claude")
	write := func(body string) {
		t.Helper()
		if err := WritePluginFiles(dir, []BridgeFile{
			{Path: ".claude-plugin/plugin.json", Body: []byte("{}"), Mode: 0o600},
			{Path: "hooks/hooks.json", Body: []byte(body), Mode: 0o600},
		}); err != nil {
			t.Fatal(err)
		}
	}
	write("old tmail")
	write("new tmail")
	got, err := os.ReadFile(filepath.Join(dir, "hooks", "hooks.json"))
	if err != nil || string(got) != "new tmail" {
		t.Fatalf("hooks.json = %q, %v", got, err)
	}
	for _, p := range []string{dir, filepath.Join(dir, "hooks"), filepath.Join(dir, ".claude-plugin")} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("%s = %v, %v; want a 0700 directory", p, info, err)
		}
	}
	if info, _ := os.Stat(filepath.Join(dir, "hooks", "hooks.json")); info.Mode().Perm() != 0o600 {
		t.Errorf("hooks.json mode = %o", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "hooks"))
	if len(entries) != 1 {
		t.Fatalf("hooks dir = %v; want no temporary files", entries)
	}
}

func TestWritePluginFilesRejectsEscapingPaths(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"../x", "/abs", ""} {
		if err := WritePluginFiles(dir, []BridgeFile{{Path: p, Body: nil, Mode: 0o600}}); err == nil {
			t.Errorf("path %q accepted", p)
		}
	}
}
