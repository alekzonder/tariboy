package loop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/harness"
)

// OpenCode discovers providers only after scripts have exported their final
// settings. Capture cwd and umask too, without sourcing those scripts twice.
func openCodeShellSnapshot(ctx context.Context, baseDir, agentScript, cwd string, env []string) ([]string, string, string, error) {
	bash, err := harness.FindExecutable("bash", env, cwd)
	if err != nil {
		return nil, "", "", errors.New("bash is required for OpenCode preparation")
	}
	prelude := strings.Replace(agentShellPrelude, `source "$script"`, `source "$script" >/dev/null 2>&1`, 1)
	const snapshot = `printf '%s\0' "$(pwd -P)" "$(umask)"; while IFS= read -r n; do printf '%s=%s\0' "$n" "${!n}"; done < <(compgen -e)`
	prelude = strings.Replace(prelude, `exec "${tariboy_harness_argv[@]}"`, snapshot, 1)
	ctx, cancel := context.WithTimeout(ctx, shellEnvTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bash, "-c", prelude, "tariboy-agent-shell", filepath.Join(baseDir, "global-agent-shell.sh"), agentScript)
	cmd.Env, cmd.Dir = env, cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	output := &openCodeShellOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil || output.overflow {
		return nil, "", "", errors.New("OpenCode shell scripts failed or exceeded their preparation limit")
	}
	parts := strings.Split(strings.TrimSuffix(output.buf.String(), "\x00"), "\x00")
	if len(parts) < 3 || !filepath.IsAbs(parts[0]) {
		return nil, "", "", errors.New("OpenCode shell scripts returned invalid environment")
	}
	mask, err := strconv.ParseUint(parts[1], 8, 12)
	if err != nil || mask > 0777 {
		return nil, "", "", errors.New("OpenCode shell scripts returned invalid umask")
	}
	return parts[2:], parts[0], parts[1], nil
}

type openCodeShellOutput struct {
	buf      bytes.Buffer
	overflow bool
}

func (b *openCodeShellOutput) Write(p []byte) (int, error) {
	if len(p) > 4<<20-b.buf.Len() {
		b.overflow = true
		return 0, errors.New("output limit")
	}
	return b.buf.Write(p)
}

func (r *ShimRunner) prepareOpenCode(ctx context.Context, l agentdir.Layout, cwd string, env, skillEnv []string, model, proxyURL, proxyToken string) ([]string, string, string, error) {
	binder, ok := r.cfg.Proxy.(interface {
		RegisterProviderRoutes(string, map[string]aiproxy.ProviderRoute) error
	})
	if !ok || proxyURL == "" || proxyToken == "" {
		return nil, "", "", errors.New("OpenCode requires provider routing through the iteration proxy")
	}
	env, cwd, mask, err := openCodeShellSnapshot(ctx, filepath.Dir(r.cfg.AgentsDir), l.ShellScriptPath(), cwd, env)
	if err != nil {
		return nil, "", "", err
	}
	env, err = mergeSkillLaunchEnv(env, skillEnv)
	if err != nil {
		return nil, "", "", err
	}
	executable, err := harness.FindExecutable("opencode", env, cwd)
	if err != nil {
		return nil, "", "", errors.New("OpenCode executable not found after shell preparation")
	}
	env, routes, err := harness.PrepareOpenCodeProxy(ctx, executable, cwd, env, model, proxyURL)
	if err != nil {
		return nil, "", "", err
	}
	if err := binder.RegisterProviderRoutes(proxyToken, routes); err != nil {
		return nil, "", "", errors.New("OpenCode provider routes could not be registered")
	}
	return env, cwd, mask, nil
}
