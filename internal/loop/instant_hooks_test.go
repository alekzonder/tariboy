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

// hookAgentBin is an agent PATH with a claude that prints version, and the
// tools a launch needs. Without tmail it has no tmail.
func hookAgentBin(t *testing.T, version string, tmail bool) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"python3", "bash", "codex"} {
		writeHarnessExecutable(t, filepath.Join(bin, name))
	}
	if tmail {
		writeHarnessExecutable(t, filepath.Join(bin, "tmail"))
	}
	script := "#!/bin/sh\necho '" + version + " (Claude Code)'\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

type hookLaunch struct {
	argv  []string
	l     agentdir.Layout
	bin   string
	log   string
	files bool
}

// launchWithHooks prepares one iteration of a hook-eligible agent after edit
// changes it, and returns the shim argv.
func launchWithHooks(t *testing.T, version string, tmail bool, skillArgs []string, edit func(*agent.Agent)) hookLaunch {
	t.Helper()
	bin := hookAgentBin(t, version, tmail)
	t.Setenv("PATH", t.TempDir())
	r, ag, l, _ := newHarnessPreflightRunner(t, false, map[string]string{"PATH": bin})
	var logs bytes.Buffer
	r.cfg.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	ag.Plugins = []string{"mail"}
	if edit != nil {
		edit(&ag)
	}
	ctx := context.WithValue(context.Background(), activatedImageContextKey{},
		activatedImage{Skills: harness.SkillLaunchConfig{Args: skillArgs}})
	prep, err := r.prepare(ctx, otel.Tracer("test"), ag, l, "path-agent-1", "")
	if err != nil {
		t.Fatal(err)
	}
	_, statErr := os.Stat(l.InstantHooksDir("claude"))
	return hookLaunch{argv: prep.shimArgv, l: l, bin: bin, log: logs.String(), files: statErr == nil}
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
		"old claude":    {"2.1.226", true, nil, true},
		"no tmail":      {"2.1.316", false, nil, true},
		"no mail":       {"2.1.316", true, func(a *agent.Agent) { a.Plugins = []string{"context"} }, false},
		"legacy name":   {"2.1.316", true, func(a *agent.Agent) { a.Plugins = []string{"messages"} }, false},
		"codex harness": {"2.1.316", true, func(a *agent.Agent) { a.HarnessType = "codex" }, false},
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
