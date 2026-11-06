// Package imagefile parses and validates Tariboyfile.yaml (spec §8).
package imagefile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/imagecontract"
	"gopkg.in/yaml.v3"
)

// DefaultFilename is looked up when a parser is given a directory.
const DefaultFilename = "Tariboyfile.yaml"

const SchemaV1MigrationMessage = "schema 1 image sources are no longer supported; migrate to schema 2"

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
	ImageVersion  string        `yaml:"image_version,omitempty" json:"image_version,omitempty"`
	SchemaVersion int           `yaml:"schema_version" json:"schema_version"`
	Extends       []string      `yaml:"extends,omitempty" json:"extends,omitempty"`
	Plugins       []V2Plugin    `yaml:"plugins" json:"plugins"`
	Skills        []SkillEntry  `yaml:"skills" json:"skills"`
	Prompts       []PromptEntry `yaml:"prompts" json:"prompts"`
	Dir           string        `yaml:"-" json:"-"`
}

func sourcePath(path string) (string, error) {
	return sourcePathFor(path, DefaultFilename)
}

func sourcePathFor(path, filename string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		path = filepath.Join(path, filename)
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
	var header struct {
		SchemaVersion int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(data, &header); err == nil && header.SchemaVersion == 1 {
		return nil, errors.New(SchemaV1MigrationMessage)
	}
	out, err := decodeV2(path, data)
	if err != nil {
		return nil, err
	}
	if err := validateVersionField(data); err != nil {
		return nil, err
	}
	if out.SchemaVersion != 2 {
		return nil, fmt.Errorf("imagefile schema_version must be 2, got %d", out.SchemaVersion)
	}
	if err := validateV2Extends(out.Extends); err != nil {
		return nil, err
	}
	if err := validateV2Plugins(out.Plugins); err != nil {
		return nil, err
	}
	if err := validateV2Skills(out.Skills); err != nil {
		return nil, err
	}
	if err := validateV2Prompts(out.Prompts); err != nil {
		return nil, err
	}
	return out, nil
}

// decodeV2 decodes the single YAML document of the source at path, refusing
// unknown fields.
func decodeV2(path string, data []byte) (*V2, error) {
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
	return &out, nil
}

func validateV2Extends(extends []string) error {
	for i, parent := range extends {
		if !strings.HasPrefix(parent, "./") && !strings.HasPrefix(parent, "../") && !filepath.IsAbs(parent) {
			return fmt.Errorf("extends[%d]: path %q must use ./, ../, or an absolute path", i, parent)
		}
	}
	return nil
}

func validateV2Plugins(plugins []V2Plugin) error {
	seenPlugins := map[string]bool{}
	for i, plugin := range plugins {
		if plugin.Name == "" {
			return fmt.Errorf("plugins[%d]: name is required", i)
		}
		if seenPlugins[plugin.Name] {
			return fmt.Errorf("plugins[%d]: duplicate plugin %q", i, plugin.Name)
		}
		seenPlugins[plugin.Name] = true
	}
	return nil
}

func validateV2Skills(skills []SkillEntry) error {
	if len(skills) > maxV2SkillEntries {
		return fmt.Errorf("skills has %d entries, maximum is %d", len(skills), maxV2SkillEntries)
	}
	for i, skill := range skills {
		if skill.Dir == "" {
			return fmt.Errorf("skills[%d]: dir is required", i)
		}
	}
	return nil
}

func validateV2Prompts(prompts []PromptEntry) error {
	seenRuntime := map[string]bool{}
	if len(prompts) > maxV2PromptEntries {
		return fmt.Errorf("prompts has %d entries, maximum is %d", len(prompts), maxV2PromptEntries)
	}
	for i, prompt := range prompts {
		if (prompt.File == "") == (prompt.Runtime == "") {
			return fmt.Errorf("prompts[%d]: exactly one of file or runtime is required", i)
		}
		if prompt.Runtime != "" {
			if _, ok := imagecontract.Runtime(prompt.Runtime); !ok {
				return fmt.Errorf("prompts[%d]: unknown runtime placeholder %q", i, prompt.Runtime)
			}
			if seenRuntime[prompt.Runtime] {
				return fmt.Errorf("prompts[%d]: duplicate runtime placeholder %q", i, prompt.Runtime)
			}
			seenRuntime[prompt.Runtime] = true
		}
	}
	return nil
}
