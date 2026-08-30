package agentdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
)

func binDirFor(t *testing.T) Layout {
	t.Helper()
	l := New(t.TempDir(), "worker")
	if err := os.MkdirAll(l.BinDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	return l
}

func skillScriptsFor(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "skills")
	for _, name := range []string{
		"agent-tools/scripts/tools.py",
		"loop/scripts/loop.py",
		"tasks/scripts/tasks.py",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestWriteShimsDispatchesToOwningSkillScripts(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	a := agent.Agent{Name: "worker", Plugins: []string{"loop", "tasks"}}
	if err := WriteShims(l, a, skills); err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{
		"tools":     filepath.Join(skills, "agent-tools/scripts/tools.py"),
		"i-am-done": filepath.Join(skills, "loop/scripts/loop.py"),
		"tasks":     filepath.Join(skills, "tasks/scripts/tasks.py"),
	}
	for name, want := range wants {
		raw, err := os.ReadFile(filepath.Join(l.BinDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Fatalf("%s shim = %q; want skill script %q", name, raw, want)
		}
		if !strings.Contains(string(raw), "exec python3 -B ") {
			t.Fatalf("%s shim can write bytecode into packaged skills: %q", name, raw)
		}
		if name == "i-am-done" && !strings.Contains(string(raw), ` done "$@"`) {
			t.Fatalf("i-am-done lost its arguments: %q", raw)
		}
	}
}

// A shim frozen at an old release path must be repointed at the live daemon's
// tools binary — this is the whole point of SUPER-224.
func TestWriteShimsRepointsStaleToolsPath(t *testing.T) {
	l := binDirFor(t)
	a := agent.Agent{Name: "worker", Plugins: []string{"loop", "tasks"}}
	stale := "/home/u/.tariboy/store/versions/0.21.6/skills"
	for _, name := range []string{"tools", "i-am-done", "tasks"} {
		body := "#!/usr/bin/env bash\nexec \"" + stale + "\" \"$@\"\n"
		if err := os.WriteFile(filepath.Join(l.BinDir(), name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	live := skillScriptsFor(t)
	if err := WriteShims(l, a, live); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"tools", "i-am-done", "tasks"} {
		raw, err := os.ReadFile(filepath.Join(l.BinDir(), name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(string(raw), stale) {
			t.Fatalf("%s still points at the stale client: %s", name, raw)
		}
		if !strings.Contains(string(raw), live) {
			t.Fatalf("%s does not point at the live client: %s", name, raw)
		}
		if !strings.HasSuffix(string(raw), "\"$@\"\n") {
			t.Fatalf("%s no longer forwards its arguments: %s", name, raw)
		}
		info, err := os.Stat(filepath.Join(l.BinDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %v, want 0700", name, info.Mode().Perm())
		}
	}
	done, err := os.ReadFile(filepath.Join(l.BinDir(), "i-am-done"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(done), "loop.py\" done") {
		t.Fatalf("i-am-done lost its subcommand: %s", done)
	}
}

// The tasks shim is conditional on the capability, in both directions.
func TestWriteShimsReconcilesTasksCapability(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	tasksPath := filepath.Join(l.BinDir(), "tasks")

	with := agent.Agent{Name: "worker", Plugins: []string{"loop", "tasks"}}
	if err := WriteShims(l, with, skills); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `tasks.py" "$@"`) {
		t.Fatalf("tasks shim = %q", raw)
	}

	without := agent.Agent{Name: "worker", Plugins: []string{"loop"}}
	if err := WriteShims(l, without, skills); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tasksPath); !os.IsNotExist(err) {
		t.Fatalf("tasks shim survived a capability-less agent: %v", err)
	}
	// And a second removal pass is not an error.
	if err := WriteShims(l, without, skills); err != nil {
		t.Fatalf("removal is not idempotent: %v", err)
	}
}

func TestWriteShimsReconcilesLoopCapability(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	donePath := filepath.Join(l.BinDir(), "i-am-done")

	with := agent.Agent{Name: "worker", Plugins: []string{"loop"}}
	if err := WriteShims(l, with, skills); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(donePath); err != nil {
		t.Fatalf("loop shim missing while enabled: %v", err)
	}

	without := agent.Agent{Name: "worker", Plugins: nil}
	if err := WriteShims(l, without, skills); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(donePath); !os.IsNotExist(err) {
		t.Fatalf("i-am-done survived a loop-less image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(l.BinDir(), "tools")); err != nil {
		t.Fatalf("generic tools shim must remain available: %v", err)
	}
	if err := WriteShims(l, without, skills); err != nil {
		t.Fatalf("loop shim removal is not idempotent: %v", err)
	}
}

// Re-running the daemon on the same version must leave the files untouched —
// same bytes, same mode, and not even a rewrite (mtime is preserved).
func TestWriteShimsIsIdempotent(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	a := agent.Agent{Name: "worker", Plugins: []string{"loop", "tasks"}}
	if err := WriteShims(l, a, skills); err != nil {
		t.Fatal(err)
	}
	type snap struct {
		body  []byte
		mode  os.FileMode
		mtime time.Time
	}
	// Backdate the files so a rewrite is unmistakable even on a coarse clock.
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	before := map[string]snap{}
	for _, name := range []string{"tools", "i-am-done", "tasks"} {
		p := filepath.Join(l.BinDir(), name)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = snap{body: body, mode: info.Mode().Perm(), mtime: info.ModTime()}
	}

	if err := WriteShims(l, a, skills); err != nil {
		t.Fatal(err)
	}
	for name, was := range before {
		p := filepath.Join(l.BinDir(), name)
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != string(was.body) {
			t.Fatalf("%s content changed: %q -> %q", name, was.body, body)
		}
		if info.Mode().Perm() != was.mode {
			t.Fatalf("%s mode changed: %v -> %v", name, was.mode, info.Mode().Perm())
		}
		if !info.ModTime().Equal(was.mtime) {
			t.Fatalf("%s was rewritten though nothing changed (mtime %s -> %s)",
				name, was.mtime, info.ModTime())
		}
	}
}

// WriteShims writes shims and nothing else: no image unpack, no tree creation.
func TestWriteShimsWritesOnlyShims(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	a := agent.Agent{Name: "worker", Plugins: []string{"loop"}}
	if err := WriteShims(l, a, skills); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "bin" {
		t.Fatalf("WriteShims created more than the bin shims: %v", entries)
	}
	names, err := os.ReadDir(l.BinDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("bin dir = %v, want tools and i-am-done only", names)
	}
}

// A missing bin dir is an error the caller can log, not a directory WriteShims
// silently conjures up.
func TestWriteShimsFailsWithoutBinDir(t *testing.T) {
	l := New(t.TempDir(), "ghost")
	skills := skillScriptsFor(t)
	a := agent.Agent{Name: "ghost", Plugins: []string{"loop"}}
	if err := WriteShims(l, a, skills); err == nil {
		t.Fatal("WriteShims succeeded for an unprovisioned agent dir")
	}
}
