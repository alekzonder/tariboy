package agentdir

import (
	"os"
	"os/exec"
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
		"loop/scripts/loop.sh",
		"tasks/scripts/tasks.sh",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDirectShimsForwardArgsToSkillLaunchers(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	argsPath := filepath.Join(t.TempDir(), "args")
	for _, script := range []string{
		filepath.Join(skills, "loop", "scripts", "loop.sh"),
		filepath.Join(skills, "tasks", "scripts", "tasks.sh"),
	} {
		if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SHIM_ARGS\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SHIM_ARGS", argsPath)
	if err := WriteShims(l, agent.Agent{Name: "worker", Plugins: []string{"loop", "tasks"}}, skills); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"i-am-done": "done\n--idle\n",
		"tasks":     "show\nTARI-41\n",
	} {
		args := []string{"--idle"}
		if name == "tasks" {
			args = []string{"show", "TARI-41"}
		}
		if output, err := exec.Command(filepath.Join(l.BinDir(), name), args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v: %s", name, err, output)
		}
		got, err := os.ReadFile(argsPath)
		if err != nil || string(got) != want {
			t.Fatalf("%s args = %q, want %q (err=%v)", name, got, want, err)
		}
	}
}

func TestWriteShimsDispatchesDirectlyToOwningSkillScripts(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	a := agent.Agent{Name: "worker", Plugins: []string{"loop", "tasks"}}
	if err := WriteShims(l, a, skills); err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{
		"i-am-done": filepath.Join(skills, "loop/scripts/loop.sh"),
		"tasks":     filepath.Join(skills, "tasks/scripts/tasks.sh"),
	}
	for name, want := range wants {
		raw, err := os.ReadFile(filepath.Join(l.BinDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Fatalf("%s shim = %q; want skill script %q", name, raw, want)
		}
		if !strings.Contains(string(raw), "exec ") {
			t.Fatalf("%s shim does not exec its owning skill: %q", name, raw)
		}
		if name == "i-am-done" && !strings.Contains(string(raw), ` done "$@"`) {
			t.Fatalf("i-am-done lost its arguments: %q", raw)
		}
	}
	if _, err := os.Stat(filepath.Join(l.BinDir(), "tools")); !os.IsNotExist(err) {
		t.Fatalf("central tools shim exists: %v", err)
	}
}

func TestWriteShimsRequiresPython3BeforeWriting(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	t.Setenv("PATH", t.TempDir())

	err := WriteShims(l, agent.Agent{Name: "worker"}, skills)
	if err == nil || !strings.Contains(err.Error(), "python3") {
		t.Fatalf("WriteShims error = %v, want missing python3", err)
	}
	if _, statErr := os.Stat(filepath.Join(l.BinDir(), "i-am-done")); !os.IsNotExist(statErr) {
		t.Fatalf("i-am-done shim was written without python3: %v", statErr)
	}
}

func TestWriteShimsRequiresExecutableDirectScriptsBeforeWriting(t *testing.T) {
	l := binDirFor(t)
	skills := skillScriptsFor(t)
	loop := filepath.Join(skills, "loop", "scripts", "loop.sh")
	if err := os.Chmod(loop, 0o600); err != nil {
		t.Fatal(err)
	}
	err := WriteShims(l, agent.Agent{Name: "worker", Plugins: []string{"loop"}}, skills)
	if err == nil || !strings.Contains(err.Error(), "skill script unavailable") {
		t.Fatalf("WriteShims error = %v, want unavailable direct script", err)
	}
	if _, statErr := os.Stat(filepath.Join(l.BinDir(), "i-am-done")); !os.IsNotExist(statErr) {
		t.Fatalf("i-am-done shim was written for a non-executable launcher: %v", statErr)
	}
}

func TestWriteShimsRepointsDirectSkillShimsAndRemovesManagedTools(t *testing.T) {
	l := binDirFor(t)
	a := agent.Agent{Name: "worker", Plugins: []string{"loop", "tasks"}}
	stale := "/home/u/.tariboy/store/versions/0.21.6/skills"
	for name, body := range map[string]string{
		"tools":     "#!/usr/bin/env bash\nexec python3 -B \"" + stale + "/agent-tools/scripts/tools.py\" \"$@\"\n",
		"i-am-done": "#!/usr/bin/env bash\nexec \"" + stale + "/loop/scripts/loop.sh\" done \"$@\"\n",
		"tasks":     "#!/usr/bin/env bash\nexec \"" + stale + "/tasks/scripts/tasks.sh\" \"$@\"\n",
	} {
		if err := os.WriteFile(filepath.Join(l.BinDir(), name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	live := skillScriptsFor(t)
	if err := WriteShims(l, a, live); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"i-am-done", "tasks"} {
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
	if !strings.Contains(string(done), "loop.sh\" done") {
		t.Fatalf("i-am-done lost its subcommand: %s", done)
	}
	if _, err := os.Stat(filepath.Join(l.BinDir(), "tools")); !os.IsNotExist(err) {
		t.Fatalf("managed legacy tools shim survived: %v", err)
	}
}

func TestWriteShimsKeepsUserOwnedToolsShim(t *testing.T) {
	l := binDirFor(t)
	tools := filepath.Join(l.BinDir(), "tools")
	body := "#!/bin/sh\necho user tool\n"
	if err := os.WriteFile(tools, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteShims(l, agent.Agent{Name: "worker"}, skillScriptsFor(t)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(tools)
	if err != nil || string(got) != body {
		t.Fatalf("user-owned tools shim changed: %q err=%v", got, err)
	}
}

func TestWriteShimsKeepsCustomWrapperThatMentionsLegacyDispatcher(t *testing.T) {
	l := binDirFor(t)
	tools := filepath.Join(l.BinDir(), "tools")
	body := "#!/usr/bin/env bash\n# exec python3 agent-tools/scripts/tools.py through a custom wrapper\nexec python3 -B /custom/tools.py \"$@\"\n"
	if err := os.WriteFile(tools, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteShims(l, agent.Agent{Name: "worker"}, skillScriptsFor(t)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(tools)
	if err != nil || string(got) != body {
		t.Fatalf("custom tools wrapper changed: %q err=%v", got, err)
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
	if !strings.Contains(string(raw), `tasks.sh" "$@"`) {
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
	if _, err := os.Stat(filepath.Join(l.BinDir(), "tools")); !os.IsNotExist(err) {
		t.Fatalf("central tools shim exists: %v", err)
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
	for _, name := range []string{"i-am-done", "tasks"} {
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
	if len(names) != 1 || names[0].Name() != "i-am-done" {
		t.Fatalf("bin dir = %v, want i-am-done only", names)
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
