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

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/image"
)

type openCodePluginLaunch struct {
	prep      preparedRun
	l         agentdir.Layout
	bin, log  string
	home, cwd string
	plugins   []any
	files     bool
}

// launchOpenCodeWithPlugin prepares one OpenCode iteration of an agent with
// the mail capability after edit changes it. The fake opencode prints version
// for --version; tmail decides whether the agent PATH has a tmail.
func launchOpenCodeWithPlugin(t *testing.T, version string, tmail bool, skillEnv []string, edit func(*agent.Agent)) openCodePluginLaunch {
	t.Helper()
	binder := &openCodeBinder{fakeBinder: fakeBinder{base: "http://127.0.0.1:1234"}}
	r, ag, l, _ := newRunnerForProxyTest(t, binder)
	var logs bytes.Buffer
	r.cfg.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	root := filepath.Dir(r.cfg.AgentsDir)
	cwd := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	for _, dir := range []string{cwd, filepath.Join(home, ".config", "opencode")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(root, "bin")
	installOpenCodeProbe(t, bin)
	if tmail {
		writeHarnessExecutable(t, filepath.Join(bin, "tmail"))
	}
	ag.HarnessType = "opencode"
	ag.Model = "kimi-code-plan-global/k3"
	ag.Cwd = cwd
	ag.Plugins = []string{"mail"}
	ag.Env = map[string]string{
		"PATH": bin + ":/usr/bin:/bin", "HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"TEST_OPENCODE_VERSION": version, "TEST_OPENCODE_CONFIG": `{"model":"kimi-code-plan-global/k3"}`,
	}
	if edit != nil {
		edit(&ag)
	}
	if err := l.EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), activatedImageContextKey{}, activatedImage{Skills: harness.SkillLaunchConfig{Env: skillEnv}})
	prep, err := r.prepareSteps(ctx, ag, l, "alice-1", "")
	if err != nil {
		t.Fatal(err)
	}
	var overlay map[string]any
	if err := json.Unmarshal([]byte(environmentValue(prep.env, "OPENCODE_CONFIG_CONTENT")), &overlay); err != nil {
		t.Fatal(err)
	}
	plugins, _ := overlay["plugin"].([]any)
	_, statErr := os.Stat(l.InstantHooksDir("opencode"))
	return openCodePluginLaunch{prep: prep, l: l, bin: bin, log: logs.String(), home: home, cwd: cwd, plugins: plugins, files: statErr == nil}
}

func (got openCodePluginLaunch) wantPlugin(t *testing.T) {
	t.Helper()
	path := filepath.Join(got.l.InstantHooksDir("opencode"), "tariboy-instant-mail.js")
	if len(got.plugins) != 1 || got.plugins[0] != "file://"+path {
		t.Fatalf("overlay plugins = %v; want only file://%s", got.plugins, path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(filepath.Join(got.bin, "tmail"))
	if !strings.Contains(string(body), string(quoted)) || !strings.Contains(string(body), `"opencode-tool-after"`) {
		t.Fatalf("plugin does not run %s instant --format opencode-tool-after:\n%s", quoted, body)
	}
	if info, err := os.Stat(got.l.InstantHooksDir("opencode")); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("plugin dir = %v, %v; want 0700", info, err)
	}
}

func TestOpenCodeLaunchLoadsInstantPluginWithImageSkills(t *testing.T) {
	got := launchOpenCodeWithPlugin(t, "1.19.5", true, []string{"OPENCODE_CONFIG_DIR=/image-bridge"}, nil)
	got.wantPlugin(t)
	if environmentValue(got.prep.env, "OPENCODE_CONFIG_DIR") != "/image-bridge" {
		t.Fatal("image skill bridge lost")
	}
}

func TestOpenCodeLaunchLoadsInstantPluginWithoutImageSkills(t *testing.T) {
	got := launchOpenCodeWithPlugin(t, "1.20.0", true, nil, nil)
	got.wantPlugin(t)
	if environmentValue(got.prep.env, "OPENCODE_CONFIG_DIR") != "" {
		t.Fatal("OPENCODE_CONFIG_DIR set without image skills")
	}
}

func TestOpenCodeInteractiveAndRunLaunchLoadTheSamePlugin(t *testing.T) {
	run := launchOpenCodeWithPlugin(t, "1.19.5", true, nil, nil)
	interactive := launchOpenCodeWithPlugin(t, "1.19.5", true, nil, func(a *agent.Agent) { a.Interactive = true })
	run.wantPlugin(t)
	interactive.wantPlugin(t)
	command := func(got openCodePluginLaunch) []string {
		argv := got.prep.shimArgv[slices.Index(got.prep.shimArgv, "--")+1:]
		return strings.Split(strings.ReplaceAll(strings.Join(argv, "\x00"), got.l.Root, "AGENT"), "\x00")
	}
	if !slices.Equal(command(run), command(interactive)) {
		t.Fatalf("modes run different commands:\n%q\n%q", command(run), command(interactive))
	}
}

// The plugin changes neither HOME nor the user's OpenCode configuration and
// writes nothing to the agent's working directory.
func TestOpenCodeInstantPluginLeavesUserConfigAndCwdAlone(t *testing.T) {
	got := launchOpenCodeWithPlugin(t, "1.19.5", true, nil, nil)
	got.wantPlugin(t)
	if environmentValue(got.prep.env, "HOME") != got.home || environmentValue(got.prep.env, "XDG_CONFIG_HOME") != filepath.Join(got.home, ".config") {
		t.Fatal("HOME or XDG_CONFIG_HOME changed")
	}
	for _, dir := range []string{got.cwd, filepath.Join(got.home, ".config", "opencode")} {
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Fatalf("%s holds %v, %v; want nothing", dir, entries, err)
		}
	}
}

func TestOpenCodeInstantPluginIsNotLoaded(t *testing.T) {
	for name, tc := range map[string]struct {
		version string
		tmail   bool
		edit    func(*agent.Agent)
		warns   bool
	}{
		"old opencode":        {"1.19.4", true, nil, true},
		"unreadable version":  {"unknown", true, nil, true},
		"no tmail":            {"1.19.5", false, nil, true},
		"no mail":             {"1.19.5", true, func(a *agent.Agent) { a.Plugins = []string{"context"} }, false},
		"bare image":          {"1.19.5", true, func(a *agent.Agent) { a.ImageRef = image.BareRef.String() }, false},
		"no mail, old binary": {"1.0.0", true, func(a *agent.Agent) { a.Plugins = nil }, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := launchOpenCodeWithPlugin(t, tc.version, tc.tmail, nil, tc.edit)
			if len(got.plugins) != 0 || got.files {
				t.Fatalf("plugins = %v, files written = %v; want none", got.plugins, got.files)
			}
			if warned := strings.Contains(got.log, "instant mail hooks not loaded"); warned != tc.warns {
				t.Fatalf("warning = %v; want %v; log:\n%s", warned, tc.warns, got.log)
			}
		})
	}
}

func TestClaudeAndCodexLaunchesWriteNoOpenCodePlugin(t *testing.T) {
	for _, typ := range []string{"claude", "codex"} {
		got := launchWithHooks(t, "2.1.316", true, nil, func(a *agent.Agent) { a.HarnessType = typ })
		if _, err := os.Stat(got.l.InstantHooksDir("opencode")); !os.IsNotExist(err) {
			t.Fatalf("%s: opencode plugin dir stat = %v; want none", typ, err)
		}
		if typ == "claude" && len(pluginDirs(got.argv)) != 1 {
			t.Fatalf("claude plugin dirs = %v; want the hooks", pluginDirs(got.argv))
		}
	}
}
