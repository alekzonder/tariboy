package tfile

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// The marshal structs fix the key order of the written file; empty fields
// and empty lists are left out.
type marshalSkill struct {
	Dir  string `yaml:"dir,omitempty"`
	Git  string `yaml:"git,omitempty"`
	URL  string `yaml:"url,omitempty"`
	Path string `yaml:"path,omitempty"`
	Ref  string `yaml:"ref,omitempty"`
	Only string `yaml:"only,omitempty"`
}

type marshalPrompt struct {
	File    string `yaml:"file,omitempty"`
	Git     string `yaml:"git,omitempty"`
	URL     string `yaml:"url,omitempty"`
	Runtime string `yaml:"runtime,omitempty"`
	Path    string `yaml:"path,omitempty"`
	Ref     string `yaml:"ref,omitempty"`
	Only    string `yaml:"only,omitempty"`
}

type marshalFile struct {
	SchemaVersion int             `yaml:"schema_version"`
	ImageVersion  string          `yaml:"image_version,omitempty"`
	Extends       []string        `yaml:"extends,omitempty"`
	Plugins       []Plugin        `yaml:"plugins,omitempty"`
	Skills        []marshalSkill  `yaml:"skills,omitempty"`
	Prompts       []marshalPrompt `yaml:"prompts,omitempty"`
}

// Marshal writes f as a Tariboyfile. Keys come in the order schema_version,
// image_version, extends, plugins, skills, prompts; an entry writes only its
// non-empty fields. Parse of the output equals f.
func Marshal(f *File) ([]byte, error) {
	m := marshalFile{
		SchemaVersion: f.SchemaVersion,
		ImageVersion:  f.ImageVersion,
		Extends:       f.Extends,
		Plugins:       f.Plugins,
	}
	for _, s := range f.Skills {
		m.Skills = append(m.Skills, marshalSkill{Dir: s.Dir, Git: s.Git, URL: s.URL, Path: s.Path, Ref: s.Ref, Only: s.Only})
	}
	for _, p := range f.Prompts {
		m.Prompts = append(m.Prompts, marshalPrompt{File: p.File, Git: p.Git, URL: p.URL, Runtime: p.Runtime, Path: p.Path, Ref: p.Ref, Only: p.Only})
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
