package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/harness"
)

// hookAgentBin is an agent PATH with a claude and a codex that print version,
// each in its own form, and the tools a launch needs. Without tmail it has no
// tmail.
func hookAgentBin(t *testing.T, version string, tmail bool) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"python3", "bash"} {
		writeHarnessExecutable(t, filepath.Join(bin, name))
	}
	if tmail {
		writeHarnessExecutable(t, filepath.Join(bin, "tmail"))
	}
	for name, script := range map[string]string{
		"claude": "#!/bin/sh\necho '" + version + " (Claude Code)'\n",
		"codex":  "#!/bin/sh\necho 'codex-cli " + version + "'\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

type hookLaunch struct {
	argv   []string
	env    []string
	prompt string
	l      agentdir.Layout
	bin    string
	log    string
	files  bool
}

// launchWithHooks prepares one iteration of a hook-eligible agent after edit
// changes it, and returns the shim argv.
func launchWithHooks(t *testing.T, version string, tmail bool, skillArgs []string, edit func(*agent.Agent)) hookLaunch {
	t.Helper()
	return launchHooks(t, version, tmail, harness.SkillLaunchConfig{Args: skillArgs}, "", edit)
}

// launchHooks is launchWithHooks with the image skills and the daemon codex
// approval mode given in full.
func launchHooks(t *testing.T, version string, tmail bool, skills harness.SkillLaunchConfig, codexApproval string, edit func(*agent.Agent)) hookLaunch {
	t.Helper()
	bin := hookAgentBin(t, version, tmail)
	t.Setenv("PATH", t.TempDir())
	r, ag, l, _ := newHarnessPreflightRunner(t, false, map[string]string{"PATH": bin})
	var logs bytes.Buffer
	r.cfg.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	r.cfg.CodexApproval = func() string { return codexApproval }
	ag.Plugins = []string{"mail"}
	if edit != nil {
		edit(&ag)
	}
	ctx := context.WithValue(context.Background(), activatedImageContextKey{}, activatedImage{Skills: skills})
	prep, err := r.prepare(ctx, otel.Tracer("test"), ag, l, "path-agent-1", "")
	if err != nil {
		t.Fatal(err)
	}
	prompt, _ := os.ReadFile(l.PromptPath("path-agent-1"))
	files := false
	for _, typ := range []string{"claude", "codex"} {
		if _, err := os.Stat(l.InstantHooksDir(typ)); err == nil {
			files = true
		}
	}
	return hookLaunch{argv: prep.shimArgv, env: prep.env, prompt: string(prompt), l: l, bin: bin, log: logs.String(), files: files}
}

// codexHookArgs returns the argv items from --dangerously-bypass-hook-trust
// on: the codex instant mail hooks.
func codexHookArgs(argv []string) []string {
	i := slices.Index(argv, "--dangerously-bypass-hook-trust")
	if i < 0 {
		return nil
	}
	return argv[i:min(i+5, len(argv))]
}

func codexLaunch(t *testing.T, version, approval string, skills harness.SkillLaunchConfig, edit func(*agent.Agent)) hookLaunch {
	t.Helper()
	return launchHooks(t, version, true, skills, approval, func(a *agent.Agent) {
		a.HarnessType = "codex"
		if edit != nil {
			edit(a)
		}
	})
}

func TestCodexLaunchLoadsInstantHooksWithImageSkills(t *testing.T) {
	got := codexLaunch(t, "0.166.0", "", harness.SkillLaunchConfig{PromptPrefix: "IMAGE SKILL CATALOG\n"}, nil)
	hooks := codexHookArgs(got.argv)
	tmail := "'" + filepath.Join(got.bin, "tmail") + "' instant --format "
	if len(hooks) != 5 || hooks[1] != "-c" || hooks[3] != "-c" ||
		!strings.HasPrefix(hooks[2], "hooks.PostToolUse=") || !strings.Contains(hooks[2], tmail+"codex-post-tool-use") ||
		!strings.HasPrefix(hooks[4], "hooks.Stop=") || !strings.Contains(hooks[4], tmail+"codex-stop") {
		t.Fatalf("codex hook args = %q; want the trust bypass, PostToolUse and Stop running %s", hooks, tmail)
	}
	if !strings.Contains(got.prompt, "IMAGE SKILL CATALOG") {
		t.Fatalf("prompt lost the image skill catalog:\n%s", got.prompt)
	}
	if got.files {
		t.Fatal("codex hooks wrote a harness-hooks directory; want argv only")
	}
}

func TestCodexLaunchLoadsInstantHooksWithoutImageSkills(t *testing.T) {
	got := codexLaunch(t, "0.141.0", harness.CodexApprovalYolo, harness.SkillLaunchConfig{}, nil)
	if len(codexHookArgs(got.argv)) != 5 {
		t.Fatalf("argv = %q; want the codex hooks", got.argv)
	}
}

// Both codex modes get the same hooks, and the hooks change no config
// directory: no CODEX_HOME or XDG_CONFIG_HOME, the agent's own HOME, and no
// new file in the agent cwd or HOME.
func TestCodexExecAndTUILoadTheSameHooksWithoutConfigFiles(t *testing.T) {
	// The launch inherits the daemon environment, which may already carry
	// these keys (CI runners set XDG_CONFIG_HOME). Pin them so the check is
	// "passed through unchanged", whatever the host has.
	inherited := map[string]string{"CODEX_HOME": "", "XDG_CONFIG_HOME": "/inherited/xdg"}
	for key, value := range inherited {
		t.Setenv(key, value)
	}
	var launches []hookLaunch
	var dirs []string
	for _, interactive := range []bool{false, true} {
		cwd, home := t.TempDir(), t.TempDir()
		dirs = append(dirs, cwd, home)
		launches = append(launches, codexLaunch(t, "0.166.0", "", harness.SkillLaunchConfig{}, func(a *agent.Agent) {
			a.Interactive = interactive
			a.Cwd = cwd
			a.Env["HOME"] = home
		}))
		if got := environmentValue(launches[len(launches)-1].env, "HOME"); got != home {
			t.Fatalf("HOME = %q; want the agent's %q", got, home)
		}
	}
	exec, tui := launches[0], launches[1]
	if !slices.Contains(exec.argv, "exec") || slices.Contains(tui.argv, "exec") {
		t.Fatalf("modes not distinct: exec %q, tui %q", exec.argv, tui.argv)
	}
	// Each launch has its own bin directory; the hooks must match apart from it.
	e := strings.ReplaceAll(strings.Join(codexHookArgs(exec.argv), "\n"), exec.bin, "BIN")
	i := strings.ReplaceAll(strings.Join(codexHookArgs(tui.argv), "\n"), tui.bin, "BIN")
	if !strings.Contains(e, "'BIN/tmail' instant") || e != i {
		t.Fatalf("hooks: exec %q, tui %q", e, i)
	}
	for _, l := range launches {
		for key, want := range inherited {
			if v := environmentValue(l.env, key); v != want {
				t.Fatalf("%s = %q; want the inherited %q", key, v, want)
			}
		}
	}
	for _, dir := range dirs {
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("%s has %d new entries; want none", dir, len(entries))
		}
	}
}

func TestCodexInstantHooksAreNotLoaded(t *testing.T) {
	for name, tc := range map[string]struct {
		version, approval string
		edit              func(*agent.Agent)
		warns             bool
	}{
		"old codex":      {"0.140.0", "", nil, true},
		"unknown codex":  {"dev build", "", nil, true},
		"approve-for-me": {"0.166.0", harness.CodexApprovalApproveForMe, nil, true},
		"no mail":        {"0.166.0", "", func(a *agent.Agent) { a.Plugins = nil }, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := codexLaunch(t, tc.version, tc.approval, harness.SkillLaunchConfig{}, tc.edit)
			if hooks := codexHookArgs(got.argv); hooks != nil || got.files {
				t.Fatalf("hooks = %q, files = %v; want none", hooks, got.files)
			}
			if warned := strings.Contains(got.log, "instant mail hooks"); warned != tc.warns {
				t.Fatalf("warning = %v; want %v; log:\n%s", warned, tc.warns, got.log)
			}
		})
	}
}

func TestBareCodexLaunchHasNoInstantHooks(t *testing.T) {
	bin := hookAgentBin(t, "0.166.0", true)
	t.Setenv("PATH", t.TempDir())
	r, ag, l, _ := newHarnessPreflightRunner(t, true, map[string]string{"PATH": bin})
	ag.Plugins = []string{"mail"}
	ag.HarnessType = "codex"
	prep, err := r.prepare(context.Background(), otel.Tracer("test"), ag, l, "path-agent-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if hooks := codexHookArgs(prep.shimArgv); hooks != nil {
		t.Fatalf("bare codex hooks = %q", hooks)
	}
}

// OpenCode has no InstantMailHooker at all; see the harness test.
func TestClaudeLaunchHasNoCodexHooks(t *testing.T) {
	claude := launchWithHooks(t, "2.1.316", true, nil, nil)
	if hooks := codexHookArgs(claude.argv); hooks != nil {
		t.Fatalf("claude argv has codex hooks %q", hooks)
	}
	if _, err := os.Stat(claude.l.InstantHooksDir("codex")); err == nil {
		t.Fatal("claude launch wrote a codex hooks directory")
	}
}

// pluginDirs lists the --plugin-dir values of argv in order.
func pluginDirs(argv []string) []string {
	var dirs []string
	for i, a := range argv {
		if a == "--plugin-dir" && i+1 < len(argv) {
			dirs = append(dirs, argv[i+1])
		}
	}
	return dirs
}

func TestClaudeLaunchLoadsInstantHooksWithImageSkills(t *testing.T) {
	got := launchWithHooks(t, "2.1.316", true, []string{"--plugin-dir", "/bridges/claude"}, nil)
	hooksDir := got.l.InstantHooksDir("claude")
	if dirs := pluginDirs(got.argv); !slices.Equal(dirs, []string{"/bridges/claude", hooksDir}) {
		t.Fatalf("plugin dirs = %v; want the skill bridge, then %s", dirs, hooksDir)
	}
	body, err := os.ReadFile(filepath.Join(hooksDir, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	want := "'" + filepath.Join(got.bin, "tmail") + "' instant --format claude-stop"
	if c := doc.Hooks["Stop"][0].Hooks[0].Command; c != want {
		t.Fatalf("Stop command = %q; want %q", c, want)
	}
	if !strings.Contains(string(body), "claude-post-tool-use") {
		t.Fatalf("hooks.json lacks PostToolUse: %s", body)
	}
}

func TestClaudeLaunchLoadsInstantHooksWithoutImageSkills(t *testing.T) {
	got := launchWithHooks(t, "2.1.316", true, nil, nil)
	if dirs := pluginDirs(got.argv); !slices.Equal(dirs, []string{got.l.InstantHooksDir("claude")}) {
		t.Fatalf("plugin dirs = %v", dirs)
	}
}

func TestInteractiveAndPrintLaunchLoadTheSameHooks(t *testing.T) {
	printMode := launchWithHooks(t, "2.1.316", true, []string{"--plugin-dir", "/bridges/claude"}, nil)
	interactive := launchWithHooks(t, "2.1.316", true, []string{"--plugin-dir", "/bridges/claude"},
		func(a *agent.Agent) { a.Interactive = true })
	if slices.Contains(interactive.argv, "-p") || !slices.Contains(printMode.argv, "-p") {
		t.Fatalf("modes not distinct: print %v, interactive %v", printMode.argv, interactive.argv)
	}
	p, i := pluginDirs(printMode.argv), pluginDirs(interactive.argv)
	if len(p) != 2 || len(i) != 2 || p[0] != i[0] ||
		p[1] != printMode.l.InstantHooksDir("claude") || i[1] != interactive.l.InstantHooksDir("claude") {
		t.Fatalf("plugin dirs: print %v, interactive %v", p, i)
	}
}

func TestInstantHooksAreNotLoaded(t *testing.T) {
	for name, tc := range map[string]struct {
		version string
		tmail   bool
		edit    func(*agent.Agent)
		warns   bool
	}{
		"old claude":  {"2.1.226", true, nil, true},
		"no tmail":    {"2.1.316", false, nil, true},
		"no mail":     {"2.1.316", true, func(a *agent.Agent) { a.Plugins = []string{"context"} }, false},
		"legacy name": {"2.1.316", true, func(a *agent.Agent) { a.Plugins = []string{"messages"} }, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := launchWithHooks(t, tc.version, tc.tmail, nil, tc.edit)
			dirs := pluginDirs(got.argv)
			if name == "legacy name" {
				if len(dirs) != 1 {
					t.Fatalf("messages capability: plugin dirs = %v; want the hooks", dirs)
				}
				return
			}
			if len(dirs) != 0 || got.files {
				t.Fatalf("plugin dirs = %v, hooks written = %v; want none", dirs, got.files)
			}
			if warned := strings.Contains(got.log, "instant mail hooks"); warned != tc.warns {
				t.Fatalf("warning = %v; want %v; log:\n%s", warned, tc.warns, got.log)
			}
		})
	}
}

func TestBareClaudeLaunchHasNoInstantHooks(t *testing.T) {
	bin := hookAgentBin(t, "2.1.316", true)
	t.Setenv("PATH", t.TempDir())
	r, ag, l, _ := newHarnessPreflightRunner(t, true, map[string]string{"PATH": bin})
	ag.Plugins = []string{"mail"}
	prep, err := r.prepare(context.Background(), otel.Tracer("test"), ag, l, "path-agent-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if dirs := pluginDirs(prep.shimArgv); len(dirs) != 0 {
		t.Fatalf("bare plugin dirs = %v", dirs)
	}
}
