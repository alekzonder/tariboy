package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
)

// minimumClaudeInstantHooksVersion is the oldest claude that gets the instant
// mail hooks: PostToolUse and PostToolUseFailure with
// hookSpecificOutput.additionalContext, Stop with decision block, plugin hooks
// and a repeatable --plugin-dir. It matches the skill bridge minimum; every
// changelog fix the hooks need is older.
const minimumClaudeInstantHooksVersion = "2.1.227"

// instantHookTimeoutSeconds bounds one hook run in Claude Code. tmail instant
// gives up after 1.5 s on its own.
const instantHookTimeoutSeconds = 5

// InstantMailHooks is the harness plugin that delivers instant letters into a
// running iteration. The daemon writes Files under the plugin directory and
// appends Args to the harness argv. ConfigPlugin is the plugin URL that
// OpenCode loads from its configuration overlay. Commands are the shell
// commands the hooks run; the stall probe does not count a hook shell as an
// agent command.
type InstantMailHooks struct {
	Files          []agentdir.BridgeFile
	Args           []string
	ConfigPlugin   string
	MinimumVersion string
	Commands       []string
}

// instantHookCommand is the shell command of one instant mail hook.
func instantHookCommand(tmail, format string) string {
	return shellQuote(tmail) + " instant --format " + format
}

// InstantMailHooker is an optional Adapter interface for a harness whose hooks
// run tmail instant. The claude, codex and opencode adapters implement it.
// cfg is the launch config the adapter's Command gets.
type InstantMailHooker interface {
	InstantMailHooks(tmail, pluginDir string, cfg Config) (InstantMailHooks, error)
}

// minimumCodexInstantHooksVersion is the oldest codex that runs PostToolUse
// and Stop hooks given in -c overrides with --dangerously-bypass-hook-trust:
// 0.131.0 to 0.140.0 accept the flag but do not run hooks from -c.
const minimumCodexInstantHooksVersion = "0.141.0"

// InstantMailHooks for codex passes the hooks as -c config overrides, so no
// file lands in CODEX_HOME, the user's ~/.codex or the repository. Codex runs
// only trusted hooks and keeps trust in the user's config.toml, so the hooks
// need --dangerously-bypass-hook-trust. That flag also runs every unreviewed
// user or repository hook, which in approve-for-me would run outside the
// sandbox: there the hooks are refused.
func (codex) InstantMailHooks(tmail, _ string, cfg Config) (InstantMailHooks, error) {
	if !filepath.IsAbs(tmail) {
		return InstantMailHooks{}, errors.New("instant mail hooks need an absolute tmail path")
	}
	if cfg.CodexApproval == CodexApprovalApproveForMe {
		return InstantMailHooks{}, errors.New("codex approval mode approve-for-me: the hook trust bypass would let unreviewed hooks run outside the sandbox")
	}
	var commands []string
	hook := func(event, matcher, format string) string {
		commands = append(commands, instantHookCommand(tmail, format))
		// A JSON string is a valid TOML basic string.
		command, _ := json.Marshal(commands[len(commands)-1])
		return fmt.Sprintf(`hooks.%s=[{%shooks=[{type="command",command=%s,timeout=%d}]}]`,
			event, matcher, command, instantHookTimeoutSeconds)
	}
	args := []string{
		"--dangerously-bypass-hook-trust",
		"-c", hook("PostToolUse", `matcher="*",`, "codex-post-tool-use"),
		"-c", hook("Stop", "", "codex-stop"),
	}
	return InstantMailHooks{Args: args, MinimumVersion: minimumCodexInstantHooksVersion, Commands: commands}, nil
}

func (claude) InstantMailHooks(tmail, pluginDir string, _ Config) (InstantMailHooks, error) {
	if !filepath.IsAbs(tmail) || !filepath.IsAbs(pluginDir) {
		return InstantMailHooks{}, errors.New("instant mail hooks need absolute tmail and plugin paths")
	}
	manifest, err := generatedJSON(".claude-plugin/plugin.json", map[string]string{
		"name": "tariboy-instant-mail", "version": "0.0.0",
		"description": "Delivers Tariboy instant letters into the running iteration",
	})
	if err != nil {
		return InstantMailHooks{}, err
	}
	var commands []string
	command := func(format string) []map[string]any {
		commands = append(commands, instantHookCommand(tmail, format))
		return []map[string]any{{
			"type": "command", "timeout": instantHookTimeoutSeconds,
			"command": commands[len(commands)-1],
		}}
	}
	hooks, err := generatedJSON("hooks/hooks.json", map[string]any{"hooks": map[string]any{
		"PostToolUse":        []map[string]any{{"matcher": "*", "hooks": command("claude-post-tool-use")}},
		"PostToolUseFailure": []map[string]any{{"matcher": "*", "hooks": command("claude-post-tool-use-failure")}},
		"Stop":               []map[string]any{{"hooks": command("claude-stop")}},
	}})
	if err != nil {
		return InstantMailHooks{}, err
	}
	return InstantMailHooks{
		Files:          []agentdir.BridgeFile{manifest, hooks},
		Args:           []string{"--plugin-dir", pluginDir},
		MinimumVersion: minimumClaudeInstantHooksVersion,
		Commands:       commands,
	}, nil
}

// shellQuote quotes s as one POSIX shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// CheckMinimumVersion runs executable --version with env and fails when the
// version it prints is older than minimum or cannot be read.
func CheckMinimumVersion(executable string, env []string, minimum string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "--version")
	cmd.Env = append(os.Environ(), env...)
	output := &boundedOutput{remaining: 64 << 10}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("version probe failed: %w", err)
	}
	installed, text, err := parseVersion(output.buf.String())
	if err != nil {
		return errors.New("installed version is unrecognized")
	}
	want, _, err := parseVersion(minimum)
	if err != nil {
		return fmt.Errorf("invalid minimum version %q", minimum)
	}
	if !versionAtLeast(installed, want) {
		return fmt.Errorf("version %s or newer is required; installed version is %s", minimum, text)
	}
	return nil
}
