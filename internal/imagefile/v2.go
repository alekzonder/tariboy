package imagefile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var runtimePromptNames = map[string]bool{
	"identity": true, "goal": true, "context": true, "messages": true,
	"awaiting-replies": true, "user-prompt": true, "one-shot": true,
	"workdir": true,
}

const (
	maxV2PromptEntries = 1024
	maxV2SkillEntries  = 128
)

type V2Plugin struct {
	Name string `yaml:"name" json:"name"`
}

type PromptEntry struct {
	File    string `yaml:"file,omitempty" json:"file,omitempty"`
	Runtime string `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}

type SkillEntry struct {
	Dir string `yaml:"dir" json:"dir"`
}

type V2 struct {
	SchemaVersion int           `yaml:"schema_version" json:"schema_version"`
	Plugins       []V2Plugin    `yaml:"plugins" json:"plugins"`
	Skills        []SkillEntry  `yaml:"skills" json:"skills"`
	Prompts       []PromptEntry `yaml:"prompts" json:"prompts"`
	Dir           string        `yaml:"-" json:"-"`
}

type Parsed struct {
	Version int
	V1      *Imagefile
	V2      *V2
}

func sourcePath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		path = filepath.Join(path, DefaultFilename)
	}
	return path, nil
}

func ParseV2(path string) (*V2, error) {
	path, err := sourcePath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var out V2
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple YAML documents are not allowed")
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out.Dir = filepath.Dir(path)
	if out.SchemaVersion != 2 {
		return nil, fmt.Errorf("imagefile schema_version must be 2, got %d", out.SchemaVersion)
	}
	seenPlugins := map[string]bool{}
	for i, plugin := range out.Plugins {
		if plugin.Name == "" {
			return nil, fmt.Errorf("plugins[%d]: name is required", i)
		}
		if seenPlugins[plugin.Name] {
			return nil, fmt.Errorf("plugins[%d]: duplicate plugin %q", i, plugin.Name)
		}
		seenPlugins[plugin.Name] = true
	}
	if len(out.Skills) > maxV2SkillEntries {
		return nil, fmt.Errorf("skills has %d entries, maximum is %d", len(out.Skills), maxV2SkillEntries)
	}
	for i, skill := range out.Skills {
		if skill.Dir == "" {
			return nil, fmt.Errorf("skills[%d]: dir is required", i)
		}
	}
	seenRuntime := map[string]bool{}
	if len(out.Prompts) > maxV2PromptEntries {
		return nil, fmt.Errorf("prompts has %d entries, maximum is %d", len(out.Prompts), maxV2PromptEntries)
	}
	for i, prompt := range out.Prompts {
		if (prompt.File == "") == (prompt.Runtime == "") {
			return nil, fmt.Errorf("prompts[%d]: exactly one of file or runtime is required", i)
		}
		if prompt.Runtime != "" {
			if !runtimePromptNames[prompt.Runtime] {
				return nil, fmt.Errorf("prompts[%d]: unknown runtime placeholder %q", i, prompt.Runtime)
			}
			if seenRuntime[prompt.Runtime] {
				return nil, fmt.Errorf("prompts[%d]: duplicate runtime placeholder %q", i, prompt.Runtime)
			}
			seenRuntime[prompt.Runtime] = true
		}
	}
	return &out, nil
}

func ParseAny(path string) (*Parsed, error) {
	resolved, err := sourcePath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	var header struct {
		SchemaVersion int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("parse %s: %w", resolved, err)
	}
	switch header.SchemaVersion {
	case 1:
		v1, err := Parse(resolved)
		if err != nil {
			return nil, err
		}
		return &Parsed{Version: 1, V1: v1}, nil
	case 2:
		v2, err := ParseV2(resolved)
		if err != nil {
			return nil, err
		}
		return &Parsed{Version: 2, V2: v2}, nil
	default:
		return nil, fmt.Errorf("imagefile schema_version must be 1 or 2, got %d", header.SchemaVersion)
	}
}
