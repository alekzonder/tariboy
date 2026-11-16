package harness

// Probe of OpenCode 1.19.5 on 2026-11-16, read-only, every config directory
// a throwaway one under a temp dir:
//
//   - `opencode debug paths` prints the same data, config, cache and state
//     paths with and without OPENCODE_CONFIG_DIR (config stays
//     ~/.config/opencode, data stays ~/.local/share/opencode). The variable
//     adds a config directory; it does not move the global config or the data
//     directory where OpenCode keeps its credentials.
//   - With XDG_CONFIG_HOME pointing at a throwaway "global" config that sets
//     `model` and `provider`, plus OPENCODE_CONFIG_DIR holding
//     {"skills":{"paths":[<abs>]},"instructions":[<abs PROMPT.md>]},
//     `opencode debug config` shows the global `model` and `provider` and the
//     overlay's `skills.paths` and `instructions` together: the configs merge.
//   - `opencode debug skill` lists the skill from `skills.paths`.
//   - `opencode debug config` keeps the absolute `instructions` path as given
//     and reports no error.
//
// Not verified: that credentials resolve in a real session (no credential
// file was read and no session started; only the unchanged data path was
// observed), and that a session loads the absolute `instructions` file into
// the system prompt (`opencode debug agent build` does not show the prompt).

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

const (
	openCodeConfigEnv = "OPENCODE_CONFIG_DIR"
	// openCodeMinimum is the version whose contract was verified; an older
	// OpenCode is reported against it when the probe fails.
	openCodeMinimum = "1.19.5"
)

type openCode struct{}

func (openCode) Name() string { return "opencode" }

// Clean has nothing to do: the OpenCode config lives in the run directory.
func (openCode) Clean(Host, string) error { return nil }

func (openCode) Prepare(ctx context.Context, host Host, run rundir.Run, extra []string) (Launch, error) {
	path, err := lookPath(host, "opencode")
	if err != nil {
		return Launch{}, err
	}
	dir := filepath.Join(run.Dir, "opencode")
	if err := lockedOpenCodeConfig(run, dir); err != nil {
		return Launch{}, err
	}
	if err := probeOpenCode(ctx, host, path, dir, run.SkillsDir); err != nil {
		return Launch{}, err
	}
	launch := Launch{
		Path: path,
		Argv: append([]string{"opencode"}, extra...),
		Env:  append([]string{openCodeConfigEnv + "=" + dir}, imageEnv(run)...),
	}
	if old := host.Getenv(openCodeConfigEnv); old != "" && old != dir {
		launch.Notes = append(launch.Notes, fmt.Sprintf("tbox: %s=%s is replaced for this session; config from that directory is not loaded", openCodeConfigEnv, old))
	}
	return launch, nil
}

// lockedOpenCodeConfig writes the config under the run directory lock, so
// simultaneous first launches do not race on the config directory.
func lockedOpenCodeConfig(run rundir.Run, dir string) error {
	unlock, err := rundir.LockDir(run.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	return writeOpenCodeConfig(run, dir)
}

func writeOpenCodeConfig(run rundir.Run, dir string) error {
	var cfg struct {
		Skills struct {
			Paths []string `json:"paths"`
		} `json:"skills"`
		Instructions []string `json:"instructions,omitempty"`
	}
	cfg.Skills.Paths = []string{run.SkillsDir}
	empty, err := promptEmpty(run)
	if err != nil {
		return err
	}
	if !empty {
		cfg.Instructions = []string{run.PromptPath}
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeIfChanged(filepath.Join(dir, "opencode.json"), append(body, '\n'))
}

// probeOpenCode runs `opencode debug config` with the generated config
// directory and requires the skills path in the resolved config.
func probeOpenCode(ctx context.Context, host Host, path, dir, skills string) error {
	out, err := host.Output(ctx, []string{openCodeConfigEnv + "=" + dir}, path, "debug", "config")
	if err != nil {
		return fmt.Errorf("probe opencode config: %w", err)
	}
	if strings.Contains(string(out), skills) {
		return nil
	}
	found := "unknown"
	if v, err := host.Output(ctx, nil, path, "--version"); err == nil {
		if _, text, ok := parseVersion(string(v)); ok {
			found = text
		}
	}
	return ErrUnsupportedVersion{Harness: "opencode", Found: found, Minimum: openCodeMinimum}
}
