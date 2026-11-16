package harness

// Probe of Claude Code 2.1.325 on 2026-11-16, read-only:
//
//   - `claude --version` prints "2.1.325 (Claude Code)".
//   - `claude --help` lists `--plugin-dir <path>` ("Load a plugin from a
//     directory or .zip for this session only") and `--append-system-prompt
//     <prompt>` as interactive-capable options. `--append-system-prompt-file`
//     has no entry of its own; it appears only in the `--bare` description
//     ("Explicitly provide context via: --system-prompt[-file],
//     --append-system-prompt[-file], ..."), which is not a print-mode option.
//     The help marks many options "only works with --print" and does not mark
//     either append flag that way.
//
// Decision: use `--append-system-prompt-file`. Verified live on 2.1.325: in an
// interactive session and with `-p`, the model quoted a marker from the prompt
// file and named an image skill as `<image>:<skill>`.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

const claudeMinimum = "2.1.227"

type claude struct{}

func (claude) Name() string { return "claude" }

// Clean has nothing to do: the Claude bridge lives in the run directory.
func (claude) Clean(Host, string) error { return nil }

func (claude) Prepare(ctx context.Context, host Host, run rundir.Run, extra []string) (Launch, error) {
	path, err := lookPath(host, "claude")
	if err != nil {
		return Launch{}, err
	}
	if err := checkClaudeVersion(ctx, host, path); err != nil {
		return Launch{}, err
	}
	argv := []string{"claude"}
	if len(run.Skills) > 0 {
		plugin, err := writeClaudePlugin(run)
		if err != nil {
			return Launch{}, err
		}
		argv = append(argv, "--plugin-dir", plugin)
	}
	empty, err := promptEmpty(run)
	if err != nil {
		return Launch{}, err
	}
	if !empty {
		argv = append(argv, "--append-system-prompt-file", run.PromptPath)
	}
	return Launch{Path: path, Argv: append(argv, extra...), Env: imageEnv(run)}, nil
}

func checkClaudeVersion(ctx context.Context, host Host, path string) error {
	out, err := host.Output(ctx, nil, path, "--version")
	if err != nil {
		return fmt.Errorf("probe claude version: %w", err)
	}
	if atLeast(string(out), claudeMinimum) {
		return nil
	}
	found := "unknown"
	if _, v, ok := parseVersion(string(out)); ok {
		found = v
	}
	return ErrUnsupportedVersion{Harness: "claude", Found: found, Minimum: claudeMinimum}
}

// writeClaudePlugin builds <run.Dir>/plugin with .claude-plugin/plugin.json
// and a copy of the image skills. The plugin is staged and swapped into place
// under the run directory lock, so two simultaneous first launches build it
// once; a plugin directory with the expected manifest is complete and is
// reused untouched.
func writeClaudePlugin(run rundir.Run) (string, error) {
	plugin := filepath.Join(run.Dir, "plugin")
	manifestPath := filepath.Join(plugin, ".claude-plugin", "plugin.json")
	manifest, err := claudeManifest(run)
	if err != nil {
		return "", err
	}
	if sameFile(manifestPath, manifest) {
		return plugin, nil
	}
	unlock, err := rundir.LockDir(run.Dir)
	if err != nil {
		return "", err
	}
	defer unlock()
	if sameFile(manifestPath, manifest) {
		return plugin, nil
	}
	sweepStaging(run.Dir)
	stage, err := os.MkdirTemp(run.Dir, stagePrefix)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := copyTree(run.SkillsDir, filepath.Join(stage, "skills")); err != nil {
		return "", fmt.Errorf("copy skills into the claude plugin: %w", err)
	}
	if err := writeIfChanged(filepath.Join(stage, ".claude-plugin", "plugin.json"), manifest); err != nil {
		return "", err
	}
	if err := swapDir(stage, plugin); err != nil {
		return "", err
	}
	return plugin, nil
}

func claudeManifest(run rundir.Run) ([]byte, error) {
	version := strings.TrimSpace(run.ImageVersion)
	if version == "" {
		version = "0.0.0"
	}
	body, err := json.MarshalIndent(struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{pluginName(run.Name), version}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
