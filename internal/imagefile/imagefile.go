// Package imagefile parses and validates Tariboyfile.yaml (spec §8).
package imagefile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultFilename is looked up when Parse is given a directory.
const DefaultFilename = "Tariboyfile.yaml"

type Plugin struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type Harness struct {
	Type           string `yaml:"type"`
	Model          string `yaml:"model"`
	Effort         string `yaml:"effort"`
	Interactive    bool   `yaml:"interactive"`
	InteractiveSet bool   `yaml:"-"`
}

type Policy struct {
	ToolsAllow []string `yaml:"tools_allow"`
	ToolsDeny  []string `yaml:"tools_deny"`
}

type Eval struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Prompt string `yaml:"prompt"`
}

// Prompt is either a plain body-prompt filepath (Name == "") or a
// {name, filepath} entry. A Name of "system:<plugin>" overrides that
// plugin's SYSTEM fragment.
type Prompt struct {
	Name     string
	Filepath string
}

func (p *Prompt) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		p.Filepath = value.Value
		return nil
	case yaml.MappingNode:
		allowed := map[string]bool{"name": true, "filepath": true}
		for i := 0; i+1 < len(value.Content); i += 2 {
			if k := value.Content[i].Value; !allowed[k] {
				return fmt.Errorf("prompt entry: unknown field %q", k)
			}
		}
		var m struct {
			Name     string `yaml:"name"`
			Filepath string `yaml:"filepath"`
		}
		if err := value.Decode(&m); err != nil {
			return err
		}
		if m.Filepath == "" {
			return fmt.Errorf("prompt entry %q: filepath is required", m.Name)
		}
		p.Name, p.Filepath = m.Name, m.Filepath
		return nil
	default:
		return fmt.Errorf("prompt entry must be a string or a {name, filepath} map")
	}
}

type Imagefile struct {
	SchemaVersion   int
	From            string
	Plugins         []Plugin
	RequiresSecrets []string
	Harness         Harness
	Env             map[string]string
	Policy          Policy
	Prompts         []Prompt
	Skills          []string
	Evals           []Eval
	Dir             string // directory holding the Tariboyfile
}

type rawImagefile struct {
	SchemaVersion   int               `yaml:"schema_version"`
	From            string            `yaml:"from"`
	Plugins         []Plugin          `yaml:"plugins"`
	RequiresSecrets []string          `yaml:"requires_secrets"`
	Harness         *rawHarness       `yaml:"harness"`
	Env             map[string]string `yaml:"env"`
	Policy          Policy            `yaml:"policy"`
	Prompts         []Prompt          `yaml:"prompts"`
	Skills          []string          `yaml:"skills"`
	Evals           []Eval            `yaml:"evals"`
}

type rawHarness struct {
	Type        string `yaml:"type"`
	Model       string `yaml:"model"`
	Effort      string `yaml:"effort"`
	Interactive *bool  `yaml:"interactive"`
}

func Parse(path string) (*Imagefile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		path = filepath.Join(path, DefaultFilename)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var raw rawImagefile
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	im := &Imagefile{
		SchemaVersion:   raw.SchemaVersion,
		From:            raw.From,
		Plugins:         raw.Plugins,
		RequiresSecrets: raw.RequiresSecrets,
		Env:             raw.Env,
		Policy:          raw.Policy,
		Prompts:         raw.Prompts,
		Skills:          raw.Skills,
		Evals:           raw.Evals,
		Dir:             filepath.Dir(path),
	}
	if raw.Harness != nil {
		im.Harness.Type = raw.Harness.Type
		im.Harness.Model = raw.Harness.Model
		im.Harness.Effort = raw.Harness.Effort
		if raw.Harness.Interactive != nil {
			im.Harness.Interactive = *raw.Harness.Interactive
			im.Harness.InteractiveSet = true
		}
	}
	if err := im.validate(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return im, nil
}

func (im *Imagefile) validate() error {
	if im.SchemaVersion > 1 {
		return fmt.Errorf("imagefile schema_version %d is newer than supported 1", im.SchemaVersion)
	}
	if im.SchemaVersion < 1 {
		return fmt.Errorf("imagefile schema_version must be 1, got %d", im.SchemaVersion)
	}
	switch im.Harness.Type {
	case "", "claude", "codex", "opencode", "stub":
	default:
		return fmt.Errorf("unsupported harness type %q (want claude, codex, opencode, or stub)", im.Harness.Type)
	}
	for i, pl := range im.Plugins {
		if pl.Name == "" {
			return fmt.Errorf("plugins[%d]: name is required", i)
		}
	}
	for i := range im.Prompts {
		abs, err := im.resolveExisting(im.Prompts[i].Filepath, false)
		if err != nil {
			return fmt.Errorf("prompt %q: %w", im.Prompts[i].Filepath, err)
		}
		im.Prompts[i].Filepath = abs
	}
	for i := range im.Skills {
		abs, err := im.resolveExisting(im.Skills[i], true)
		if err != nil {
			return fmt.Errorf("skill %q: %w", im.Skills[i], err)
		}
		im.Skills[i] = abs
	}
	for i := range im.Evals {
		if im.Evals[i].Prompt == "" {
			continue
		}
		abs, err := im.resolveExisting(im.Evals[i].Prompt, false)
		if err != nil {
			return fmt.Errorf("eval %q prompt %q: %w", im.Evals[i].Name, im.Evals[i].Prompt, err)
		}
		im.Evals[i].Prompt = abs
	}
	return nil
}

// resolveExisting makes p absolute (relative to the Tariboyfile dir) and
// checks it exists; wantDir requires it to be a directory.
func (im *Imagefile) resolveExisting(p string, wantDir bool) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(im.Dir, p)
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if wantDir && !st.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return p, nil
}
