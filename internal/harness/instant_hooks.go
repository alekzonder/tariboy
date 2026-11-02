package harness

import (
	"context"
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
// appends Args to the harness argv.
type InstantMailHooks struct {
	Files          []agentdir.BridgeFile
	Args           []string
	MinimumVersion string
}

// InstantMailHooker is an optional Adapter interface for a harness whose hooks
// run tmail instant. Only the claude adapter implements it.
type InstantMailHooker interface {
	InstantMailHooks(tmail, pluginDir string) (InstantMailHooks, error)
}

func (claude) InstantMailHooks(tmail, pluginDir string) (InstantMailHooks, error) {
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
	command := func(format string) []map[string]any {
		return []map[string]any{{
			"type": "command", "timeout": instantHookTimeoutSeconds,
			"command": shellQuote(tmail) + " instant --format " + format,
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
