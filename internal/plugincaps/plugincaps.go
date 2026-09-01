// Package plugincaps declares built-in capability metadata. Schema-v1 images
// retain compatibility by rendering the same skill instructions schema v2 packages.
package plugincaps

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	storeassets "github.com/alekzonder/tariboy/store"
)

var (
	CORE = []string{"whoami", "loop", "messages"}
	// OPTIONAL contains built-in capabilities selectable in an image. External
	// capabilities are accepted only through a resolver for installed manifests.
	OPTIONAL = []string{"context", "status", "schedule", "scripts", "image-creator", "current-task", "llm-as-judge", "tasks"}
	// INSTRUCTION_ONLY contains schema-v2 built-ins that contribute no route,
	// command, shim, or legacy schema-v1 prompt fragment.
	INSTRUCTION_ONLY = []string{"workdir"}
)

// ResolvedPlugin is the capability information schema-v1 image construction
// needs from an installed external plugin. Schema v2 uses Installed only.
type ResolvedPlugin struct {
	Installed bool
	Prompt    string
	HasPrompt bool
}

type ExternalResolver func(name string) (ResolvedPlugin, error)

type Fragment struct {
	Plugin  string
	Name    string
	Order   int
	Path    string
	Body    string
	Teaches []string
	Tail    bool
}

// fragments contains ordering and capability metadata only. The canonical text
// is maintained under store/skills and installed into the current version Store.
var fragments = []Fragment{
	{Plugin: "whoami", Name: "system:whoami", Order: 10, Path: "skills/whoami/SKILL.md", Teaches: []string{"scripts/whoami.sh"}},
	{Plugin: "messages", Name: "system:messages", Order: 20, Path: "skills/messages/SKILL.md", Teaches: []string{"scripts/messages.sh message", "scripts/messages.sh request", "scripts/messages.sh channel", "scripts/messages.sh sources"}},
	{Plugin: "context", Name: "system:context", Order: 30, Path: "skills/context/SKILL.md", Teaches: []string{"scripts/context.sh"}},
	{Plugin: "status", Name: "system:status", Order: 40, Path: "skills/status/SKILL.md", Teaches: []string{"scripts/status.sh"}},
	{Plugin: "schedule", Name: "system:schedule", Order: 50, Path: "skills/schedule/SKILL.md", Teaches: []string{"scripts/schedule.sh"}},
	{Plugin: "scripts", Name: "system:scripts", Order: 60, Path: "skills/scripts/SKILL.md", Teaches: []string{"scripts/scripts.sh"}},
	{Plugin: "current-task", Name: "system:current-task", Order: 70, Path: "skills/current-task/SKILL.md", Teaches: []string{"scripts/current_task.sh"}},
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
	if known(name) {
		return true
	}
	for _, instruction := range INSTRUCTION_ONLY {
		if instruction == name {
			return true
		}
	}
	return false
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
		return storeassets.ReadBundled(name)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if os.IsNotExist(err) {
		// Direct library/command tests can construct a temporary base without
		// starting the daemon installer. Production startup verifies this root.
		return storeassets.ReadBundled(name)
	}
	return body, err
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
			fragment.Body += "\n\nSchema-v1 compatibility launchers: `" + fragment.Teaches[0] + "`."
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

// BodyFragments and TailFragments are legacy convenience APIs for callers that
// run before Store installation. Production image builds pass an installed root.
func BodyFragments(plugins []string) []Fragment {
	result, err := BodyFragmentsFromStore(plugins, "")
	if err != nil {
		panic(err)
	}
	return result
}

func TailFragments(plugins []string) []Fragment {
	result, err := TailFragmentsFromStore(plugins, "")
	if err != nil {
		panic(err)
	}
	return result
}
