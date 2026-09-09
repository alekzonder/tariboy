// Package imagecontract owns the daemon capabilities an image may declare.
package imagecontract

import "fmt"

type ResolvedPlugin struct {
	Installed bool
	Prompt    string
	HasPrompt bool
}

type ExternalResolver func(name string) (ResolvedPlugin, error)

type Capability struct {
	Plugin   string
	Skill    string
	Launcher string
}

type Skill struct {
	Name        string
	Executables map[string]bool
}

type Input struct {
	Plugins  []string
	Runtimes []string
	Skills   []Skill
}

var builtins = map[string]Capability{
	"whoami":        {Plugin: "whoami", Skill: "whoami"},
	"loop":          {Plugin: "loop", Skill: "loop", Launcher: "scripts/loop.sh"},
	"messages":      {Plugin: "messages", Skill: "messages"},
	"context":       {Plugin: "context", Skill: "context"},
	"status":        {Plugin: "status", Skill: "status"},
	"schedule":      {Plugin: "schedule", Skill: "schedule"},
	"scripts":       {Plugin: "scripts", Skill: "scripts"},
	"goal":          {Plugin: "goal", Skill: "goal"},
	"image-creator": {Plugin: "image-creator", Skill: "image-creator"},
	"llm-as-judge":  {Plugin: "llm-as-judge", Skill: "llm-as-judge"},
	"tasks":         {Plugin: "tasks", Skill: "tasks", Launcher: "scripts/tasks.sh"},
	"workdir":       {Plugin: "workdir", Skill: "workdir"},
}

var runtimes = map[string]Capability{
	"identity":         {Plugin: "whoami", Skill: "whoami"},
	"goal":             {Plugin: "goal", Skill: "goal"},
	"workdir":          {Plugin: "workdir", Skill: "workdir"},
	"context":          {Plugin: "context", Skill: "context"},
	"messages":         {Plugin: "messages", Skill: "messages"},
	"awaiting-replies": {Plugin: "messages", Skill: "messages"},
	"user-prompt":      {},
	"one-shot":         {},
}

func Builtin(name string) (Capability, bool) {
	capability, ok := builtins[name]
	return capability, ok
}

func Runtime(name string) (Capability, bool) {
	capability, ok := runtimes[name]
	return capability, ok
}

func Validate(input Input, resolver ExternalResolver) error {
	plugins := make(map[string]bool, len(input.Plugins))
	for _, name := range input.Plugins {
		if plugins[name] {
			return fmt.Errorf("duplicate plugin %q", name)
		}
		plugins[name] = true
		if _, ok := Builtin(name); ok {
			continue
		}
		if resolver == nil {
			return fmt.Errorf("unknown plugin %q", name)
		}
		resolved, err := resolver(name)
		if err != nil {
			return err
		}
		if !resolved.Installed {
			return fmt.Errorf("unknown plugin %q", name)
		}
	}
	for _, name := range input.Runtimes {
		capability, ok := Runtime(name)
		if !ok {
			return fmt.Errorf("unknown runtime placeholder %q", name)
		}
		if capability.Plugin != "" && !plugins[capability.Plugin] {
			return fmt.Errorf("runtime placeholder %q requires plugin %q", name, capability.Plugin)
		}
	}
	skills := make(map[string]Skill, len(input.Skills))
	for _, skill := range input.Skills {
		skills[skill.Name] = skill
	}
	for _, name := range input.Plugins {
		capability, builtin := Builtin(name)
		if !builtin || capability.Skill == "" {
			continue
		}
		skill, ok := skills[capability.Skill]
		if !ok {
			return fmt.Errorf("plugin %q requires packaged skill %q", name, capability.Skill)
		}
		if capability.Launcher != "" && !skill.Executables[capability.Launcher] {
			return fmt.Errorf("plugin %q requires executable launcher %q", name, capability.Launcher)
		}
	}
	return nil
}
