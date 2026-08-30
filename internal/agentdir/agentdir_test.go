package agentdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func buildImage(t *testing.T, st *image.Store, name string) {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "task.md"), []byte("BE A TEST AGENT"), 0o600); err != nil {
		t.Fatal(err)
	}
	im := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "context"}},
		Prompts:       []imagefile.Prompt{{Filepath: filepath.Join(src, "task.md")}},
		Dir:           src,
	}
	if _, err := image.Build(im, image.Ref{Name: name, Tag: "latest"}, st,
		func() (t2 time.Time) { return }); err != nil {
		t.Fatal(err)
	}
}

func TestProvisionAndLayout(t *testing.T) {
	base := t.TempDir()
	imgStore := &image.Store{Dir: filepath.Join(base, "images")}
	buildImage(t, imgStore, "basic")

	l := New(filepath.Join(base, "agents"), "smoke")
	skills := skillScriptsFor(t)
	a := agent.Agent{Name: "smoke", ImageRef: "basic:latest", Cwd: "", Plugins: []string{"whoami", "loop", "messages", "context"}}
	if err := Provision(l, a, imgStore, image.Ref{Name: "basic", Tag: "latest"}, skills); err != nil {
		t.Fatal(err)
	}
	// image unpacked
	if _, err := os.Stat(filepath.Join(l.ImageDir(), "PROMPT.md")); err != nil {
		t.Fatalf("image not unpacked: %v", err)
	}
	// config.json is dead (never read at runtime) — Provision must NOT write it
	if _, err := os.Stat(filepath.Join(l.Root, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config.json should not exist after Provision, stat err=%v", err)
	}
	// bin shims exist and are executable and reference the skill script
	tools, err := os.ReadFile(filepath.Join(l.BinDir(), "tools"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tools), filepath.Join(skills, "agent-tools/scripts/tools.py")) {
		t.Fatalf("tools shim does not exec the tools skill script: %s", tools)
	}
	info, _ := os.Stat(filepath.Join(l.BinDir(), "i-am-done"))
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("i-am-done not executable: %v", info.Mode())
	}
	// workdir default is the agent workdir
	if _, err := os.Stat(l.Workdir()); err != nil {
		t.Fatalf("workdir missing: %v", err)
	}
}

func TestLayoutPathsAndLive(t *testing.T) {
	agents := t.TempDir()
	l := New(agents, "smoke")
	id := "smoke-20260706100000-1"
	if err := l.EnsureIteration(id); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(l.PromptPath(id)) != "PROMPT.md" {
		t.Fatalf("prompt path: %s", l.PromptPath(id))
	}
	if _, err := os.Stat(l.LogsDir(id)); err != nil {
		t.Fatalf("logs dir not created: %v", err)
	}
	// no shim.sock yet -> not live
	live, err := ListLive(agents, "")
	if err != nil || len(live) != 0 {
		t.Fatalf("unexpected live: %+v err=%v", live, err)
	}
	// drop the per-agent shim.sock placeholder -> becomes live, mapped to the
	// newest iteration.
	if err := os.WriteFile(l.ShimSock(), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	live, err = ListLive(agents, "")
	if err != nil || len(live) != 1 || live[0].Agent != "smoke" || live[0].ID != id {
		t.Fatalf("live = %+v err=%v", live, err)
	}
	if live[0].ShimSock != l.ShimSock() {
		t.Fatalf("live sock = %q, want %q", live[0].ShimSock, l.ShimSock())
	}
}

func TestListLiveMapsNewestIteration(t *testing.T) {
	agents := t.TempDir()
	l := New(agents, "smoke")
	older := "smoke-20260706100000-1"
	newer := "smoke-20260706110000-1"
	for _, id := range []string{older, newer} {
		if err := l.EnsureIteration(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(l.ShimSock(), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	live, err := ListLive(agents, "")
	if err != nil || len(live) != 1 {
		t.Fatalf("live = %+v err=%v", live, err)
	}
	if live[0].ID != newer {
		t.Fatalf("mapped to %q, want newest %q", live[0].ID, newer)
	}
}

func TestLayoutAuditLog(t *testing.T) {
	l := New("/agents", "manager")
	if got := l.AuditLog(); got != "/agents/manager/audit.jsonl" {
		t.Fatalf("AuditLog = %q", got)
	}
}

func TestProvisionReconcilesConditionalTasksShim(t *testing.T) {
	base := t.TempDir()
	imgStore := &image.Store{Dir: filepath.Join(base, "images")}
	buildImage(t, imgStore, "basic")
	layout := New(filepath.Join(base, "agents"), "worker")
	skills := skillScriptsFor(t)
	ref := image.Ref{Name: "basic", Tag: "latest"}

	enabled := agent.Agent{
		Name: "worker", ImageRef: ref.String(),
		Plugins: []string{"whoami", "loop", "messages", "tasks"},
	}
	if err := Provision(layout, enabled, imgStore, ref, skills); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(layout.BinDir(), "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), filepath.Join(skills, "tasks/scripts/tasks.py")) ||
		!strings.Contains(string(raw), `"$@"`) {
		t.Fatalf("tasks shim = %q", raw)
	}
	info, err := os.Stat(filepath.Join(layout.BinDir(), "tasks"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("tasks shim is not executable: info=%v err=%v", info, err)
	}

	disabled := enabled
	disabled.Plugins = []string{"whoami", "loop", "messages"}
	if err := Provision(layout, disabled, imgStore, ref, skills); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.BinDir(), "tasks")); !os.IsNotExist(err) {
		t.Fatalf("stale tasks shim survived disabled reprovision: %v", err)
	}
}
