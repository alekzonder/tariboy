package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/harness"
)

func TestOpenCodeRefusesLaunchWithoutProxy(t *testing.T) {
	t.Parallel()
	r, ag, _, _ := newRunnerForProxyTest(t, nil)
	ag.HarnessType = "opencode"
	_, err := r.Run(context.Background(), ag, "manual", "alice-1", "")
	if err == nil || !strings.Contains(err.Error(), "proxy") {
		t.Fatalf("expected proxy preparation failure, got %v", err)
	}
	if len(r.cfg.Spawner.(*fakeProxySpawner).argv) != 0 {
		t.Fatal("unproxied harness started")
	}
}

func TestOpenCodeLaunchConfigIsEntirelyMasked(t *testing.T) {
	t.Parallel()
	_, env := maskLaunch(nil, []string{`OPENCODE_CONFIG_CONTENT={"provider":{"custom":{"options":{"apiKey":"private-provider-key"}}}}`}, nil)
	if env[0] != "OPENCODE_CONFIG_CONTENT=***" {
		t.Fatal("inline OpenCode configuration leaked into launch evidence")
	}
}

func TestOpenCodeShellSnapshotPreservesExportsDirectoryAndUmask(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	cwd := t.TempDir()
	script := filepath.Join(base, "agent.sh")
	counter := filepath.Join(base, "count")
	body := fmt.Sprintf("echo run >> %q\ncd %q\numask 077\nexport ROUTE_SETTING='value with spaces'\nprintf 'startup noise'\n", counter, cwd)
	if err := os.WriteFile(script, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	env, dir, mask, err := openCodeShellSnapshot(context.Background(), base, script, base, []string{"PATH=/usr/bin:/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if dir != cwd || mask != "0077" || !slices.Contains(env, "ROUTE_SETTING=value with spaces") {
		t.Fatalf("snapshot directory=%q mask=%q", dir, mask)
	}
	got, _ := os.ReadFile(counter)
	if string(got) != "run\n" {
		t.Fatalf("scripts ran more than once: %q", got)
	}
}

func TestOpenCodeShellSnapshotRedactsFailures(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	script := filepath.Join(base, "agent.sh")
	if err := os.WriteFile(script, []byte("echo private-password >&2\nexit 8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := openCodeShellSnapshot(context.Background(), base, script, base, []string{"PATH=/usr/bin:/bin"})
	if err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("unsafe shell failure: %v", err)
	}
}

type openCodeBinder struct {
	fakeBinder
	routes map[string]aiproxy.ProviderRoute
	reject bool
}

func (b *openCodeBinder) RegisterProviderRoutes(_ string, routes map[string]aiproxy.ProviderRoute) error {
	if b.reject {
		return errors.New("private-registration-error")
	}
	b.routes = routes
	return nil
}
func installOpenCodeProbe(t *testing.T, dir string) {
	t.Helper()
	body, err := os.ReadFile("../harness/testdata/opencode-probe.py")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "opencode"), body, 0700); err != nil {
		t.Fatal(err)
	}
}
func TestOpenCodeLaunchUsesFinalShellEnvironmentOnce(t *testing.T) {
	t.Parallel()
	for _, interactive := range []bool{false, true} {
		t.Run(fmt.Sprint(interactive), func(t *testing.T) {
			binder := &openCodeBinder{fakeBinder: fakeBinder{base: "http://127.0.0.1:1234"}}
			r, ag, l, _ := newRunnerForProxyTest(t, binder)
			ag.HarnessType = "opencode"
			ag.Interactive = interactive
			ag.Model = "kimi-code-plan-global/k3"
			r.cfg.HasTmuxSession = func(string) bool { return false }
			root := filepath.Dir(r.cfg.AgentsDir)
			ag.Cwd = root
			bin := filepath.Join(root, "bin")
			installOpenCodeProbe(t, bin)
			cwd := filepath.Join(root, "after-cd")
			if err := os.MkdirAll(cwd, 0700); err != nil {
				t.Fatal(err)
			}
			ag.Env = map[string]string{"PATH": bin + ":" + os.Getenv("PATH"), "OPENCODE_CONFIG_CONTENT": `{"model":"other/small"}`, "OPENAI_BASE_URL": "https://original-openai.example/v1"}
			cfg := `{"model":"kimi-code-plan-global/k3","provider":{"kimi-code-plan-global":{"options":{"baseURL":"https://script-upstream.example/v1","apiKey":"script-private-key"}}}}`
			count := filepath.Join(root, "count")
			script := fmt.Sprintf("printf 'global\\n' >> %q\nexport OPENCODE_CONFIG_CONTENT='%s'\n", count, cfg)
			if err := os.WriteFile(filepath.Join(root, "global-agent-shell.sh"), []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			script = fmt.Sprintf("printf 'agent\\n' >> %q\ncd %q\numask 077\nexport OPENCODE_CONFIG_DIR=/shell-override\n", count, cwd)
			if err := os.WriteFile(l.ShellScriptPath(), []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if err := l.EnsureIteration("alice-1"); err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), activatedImageContextKey{}, activatedImage{Skills: harness.SkillLaunchConfig{Env: []string{"OPENCODE_CONFIG_DIR=/image-bridge"}}})
			prep, err := r.prepareSteps(ctx, ag, l, "alice-1", "")
			if err != nil {
				t.Fatal(err)
			}
			if prep.cwd != cwd || binder.routes["kimi-code-plan-global"].BaseURL != "https://script-upstream.example/v1" {
				t.Fatal("final shell settings were not used for preparation")
			}
			if !slices.Contains(prep.env, "OPENCODE_CONFIG_DIR=/image-bridge") || !slices.Contains(prep.env, "OPENAI_BASE_URL=https://original-openai.example/v1") {
				t.Fatal("skill or original provider environment lost")
			}
			if strings.Contains(strings.Join(prep.maskedEnv, "\n"), "script-private-key") {
				t.Fatal("inline credential leaked")
			}
			argv := prep.shimArgv[slices.Index(prep.shimArgv, "--")+1:]
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Env, cmd.Dir = prep.env, prep.cwd
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("launch failed: %v %s", err, out)
			}
			got, _ := os.ReadFile(count)
			if string(got) != "global\nagent\n" {
				t.Fatalf("scripts reran at launch: %q", got)
			}
			info, err := os.Stat(filepath.Join(cwd, "created"))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("cwd/umask not preserved: %v %v", info, err)
			}
		})
	}
}
func TestOpenCodePreparationFailureRevokesLease(t *testing.T) {
	t.Parallel()
	binder := &openCodeBinder{fakeBinder: fakeBinder{base: "http://127.0.0.1:1234"}, reject: true}
	r, ag, _, _ := newRunnerForProxyTest(t, binder)
	ag.Cwd = t.TempDir()
	ag.HarnessType = "opencode"
	ag.Model = "kimi-code-plan-global/k3"
	bin := filepath.Join(t.TempDir(), "bin")
	installOpenCodeProbe(t, bin)
	ag.Env = map[string]string{"PATH": bin + ":" + os.Getenv("PATH")}
	_, err := r.Run(context.Background(), ag, "manual", "alice-1", "")
	if err == nil || !strings.Contains(err.Error(), "routes could not be registered") || strings.Contains(err.Error(), "private-registration-error") {
		t.Fatalf("wrong error: %v", err)
	}
	if len(binder.minted) != 1 || len(binder.revoked) != 1 || len(r.cfg.Spawner.(*fakeProxySpawner).argv) != 0 {
		t.Fatal("failed preparation leaked lease or started harness")
	}
}

func TestOpenCodeShellStartupCannotReplacePreparedRouting(t *testing.T) {
	t.Parallel()
	binder := &openCodeBinder{fakeBinder: fakeBinder{base: "http://127.0.0.1:1234"}}
	r, ag, l, _ := newRunnerForProxyTest(t, binder)
	ag.HarnessType = "opencode"
	ag.Model = "kimi-code-plan-global/k3"
	root := filepath.Dir(r.cfg.AgentsDir)
	ag.Cwd = root
	bin := filepath.Join(root, "bin")
	installOpenCodeProbe(t, bin)
	startup := filepath.Join(root, "bashenv")
	body := `export OPENCODE_CONFIG_CONTENT='{"model":"kimi-code-plan-global/k3","provider":{"kimi-code-plan-global":{"options":{"baseURL":"https://original.example/v1"}}}}'`
	if err := os.WriteFile(startup, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ag.Env = map[string]string{"PATH": bin + ":" + os.Getenv("PATH"), "BASH_ENV": startup}
	if err := l.EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}
	prep, err := r.prepareSteps(context.Background(), ag, l, "alice-1", "")
	if err != nil {
		t.Fatal(err)
	}
	argv := prep.shimArgv[slices.Index(prep.shimArgv, "--")+1:]
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env, cmd.Dir = prep.env, prep.cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "/_opencode/kimi-code-plan-global") {
		t.Fatal("BASH_ENV replaced prepared routing at launch")
	}
}
