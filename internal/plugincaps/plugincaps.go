// Package plugincaps declares built-in capability metadata.
package plugincaps

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/imagecontract"
)

var (
	CORE = []string{"whoami", "loop", "messages"}
	// OPTIONAL contains built-in capabilities selectable in an image. External
	// capabilities are accepted only through a resolver for installed manifests.
	OPTIONAL = []string{"context", "status", "schedule", "scripts", "goal", "image-creator", "llm-as-judge", "tasks"}
	// INSTRUCTION_ONLY contains schema-v2 built-ins that contribute no route,
	// command, shim, or legacy schema-v1 prompt fragment.
	INSTRUCTION_ONLY = []string{"workdir"}
)

// ResolvedPlugin is the capability information schema-v1 image construction
// needs from an installed external plugin. Schema v2 uses Installed only.
type ResolvedPlugin = imagecontract.ResolvedPlugin
type ExternalResolver = imagecontract.ExternalResolver

type Fragment struct {
	Plugin  string
	Name    string
	Order   int
	Path    string
	Body    string
	Teaches []string
	Tail    bool
}

// fragments contains legacy schema-v1 ordering and capability metadata only.
var fragments = []Fragment{
	{Plugin: "whoami", Name: "system:whoami", Order: 10, Path: "skills/whoami/SKILL.md", Teaches: []string{"scripts/whoami.sh"}},
	{Plugin: "messages", Name: "system:messages", Order: 20, Path: "skills/messages/SKILL.md", Teaches: []string{"scripts/messages.sh message", "scripts/messages.sh request", "scripts/messages.sh channel", "scripts/messages.sh sources"}},
	{Plugin: "context", Name: "system:context", Order: 30, Path: "skills/context/SKILL.md", Teaches: []string{"scripts/context.sh"}},
	{Plugin: "status", Name: "system:status", Order: 40, Path: "skills/status/SKILL.md", Teaches: []string{"scripts/status.sh"}},
	{Plugin: "schedule", Name: "system:schedule", Order: 50, Path: "skills/schedule/SKILL.md", Teaches: []string{"scripts/schedule.sh"}},
	{Plugin: "scripts", Name: "system:scripts", Order: 60, Path: "skills/scripts/SKILL.md", Teaches: []string{"scripts/scripts.sh"}},
	{Plugin: "goal", Name: "system:goal", Order: 70, Path: "skills/goal/SKILL.md", Teaches: []string{"scripts/goal.sh"}},
	{Plugin: "llm-as-judge", Name: "system:llm-as-judge", Order: 80, Path: "skills/llm-as-judge/SKILL.md", Teaches: []string{"scripts/judge.sh"}},
	{Plugin: "image-creator", Name: "system:image-creator", Order: 90, Path: "skills/image-creator/SKILL.md", Teaches: []string{"scripts/image_creator.sh"}},
	{Plugin: "tasks", Name: "system:tasks", Order: 100, Path: "skills/tasks/SKILL.md", Teaches: []string{"scripts/tasks.sh"}},
	{Plugin: "loop", Name: "system:i-am-done", Order: 1000, Path: "prompts/iteration-finish.md", Teaches: []string{"i-am-done"}, Tail: true},
}

func known(name string) bool {
	for _, n := range CORE {
		if n == name {
			return true
		}
	}
	return IsOptional(name)
}

func knownExplicit(name string) bool {
	_, ok := imagecontract.Builtin(name)
	return ok
}

func IsOptional(name string) bool {
	for _, n := range OPTIONAL {
		if n == name {
			return true
		}
	}
	return false
}

// Resolve retains the historical schema-v1 core union.
func Resolve(requested []string) ([]string, error) {
	return ResolveWithExternal(requested, nil)
}

func ResolveWithExternal(requested []string, resolver ExternalResolver) ([]string, error) {
	out := make([]string, 0, len(CORE)+len(requested))
	seen := map[string]bool{}
	for _, n := range CORE {
		out = append(out, n)
		seen[n] = true
	}
	for _, n := range requested {
		if !known(n) {
			if resolver == nil {
				return nil, fmt.Errorf("unknown plugin %q", n)
			}
			external, err := resolver(n)
			if err != nil {
				return nil, err
			}
			if !external.Installed {
				return nil, fmt.Errorf("unknown plugin %q", n)
			}
		}
		if !seen[n] {
			out = append(out, n)
			seen[n] = true
		}
	}
	return out, nil
}

// ValidateExplicit validates schema-v2 plugins without adding or reordering.
func ValidateExplicit(requested []string, resolver ExternalResolver) ([]string, error) {
	out := make([]string, 0, len(requested))
	seen := map[string]bool{}
	for _, name := range requested {
		if seen[name] {
			return nil, fmt.Errorf("duplicate plugin %q", name)
		}
		seen[name] = true
		if !knownExplicit(name) {
			if resolver == nil {
				return nil, fmt.Errorf("unknown plugin %q", name)
			}
			external, err := resolver(name)
			if err != nil {
				return nil, err
			}
			if !external.Installed {
				return nil, fmt.Errorf("unknown plugin %q", name)
			}
		}
		out = append(out, name)
	}
	return out, nil
}

func inSet(plugins []string, name string) bool {
	for _, plugin := range plugins {
		if plugin == name {
			return true
		}
	}
	return false
}

func readFragment(root, name string) ([]byte, error) {
	if root == "" {
		return nil, fmt.Errorf("legacy prompt Store is unavailable")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	return body, err
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func selectFragments(plugins []string, tail bool, storeRoot string) ([]Fragment, error) {
	var out []Fragment
	for _, metadata := range fragments {
		if metadata.Tail != tail || !inSet(plugins, metadata.Plugin) {
			continue
		}
		body, err := readFragment(storeRoot, metadata.Path)
		if err != nil {
			return nil, fmt.Errorf("read prompt for plugin %s: %w", metadata.Plugin, err)
		}
		fragment := metadata
		fragment.Body = string(body)
		if !tail && len(fragment.Teaches) > 0 {
			launcher := strings.Fields(fragment.Teaches[0])[0]
			if storeRoot != "" {
				root, err := filepath.Abs(storeRoot)
				if err != nil {
					return nil, fmt.Errorf("resolve Store root: %w", err)
				}
				installed := filepath.Join(root, filepath.Dir(fragment.Path), filepath.FromSlash(launcher))
				quoted := shellQuote(installed)
				fragment.Body = strings.ReplaceAll(fragment.Body, launcher, quoted)
				fragment.Body += "\n\nSchema-v1 compatibility launcher: `" + strings.Replace(fragment.Teaches[0], launcher, quoted, 1) + "`."
			} else {
				fragment.Body += "\n\nSchema-v1 compatibility launcher: `" + fragment.Teaches[0] + "`."
			}
		}
		out = append(out, fragment)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}

func BodyFragmentsFromStore(plugins []string, storeRoot string) ([]Fragment, error) {
	return selectFragments(plugins, false, storeRoot)
}

func TailFragmentsFromStore(plugins []string, storeRoot string) ([]Fragment, error) {
	return selectFragments(plugins, true, storeRoot)
}
