// Package tfile parses Tariboyfile.yaml (schema v2 and v3), flattens extends
// and resolves local entries. It does not fetch remote sources.
package tfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"gopkg.in/yaml.v3"
)

// Filename is looked up when a parser is given a directory.
const Filename = "Tariboyfile.yaml"

const (
	maxSkillEntries  = 128
	maxPromptEntries = 1024
	maxFileSize      = 4 << 20
	onlyTariboy      = "tariboy"
	pluginsPrefix    = "$PLUGINS/"
)

// runtimeNames are the runtime placeholders of internal/imagecontract.
var runtimeNames = []string{
	"identity", "goal", "workdir", "context", "messages",
	"awaiting-replies", "user-prompt", "one-shot",
}

// IsRuntime reports whether name is a runtime placeholder.
func IsRuntime(name string) bool {
	return slices.Contains(runtimeNames, name)
}

// Plugin is a plugin declaration. tbox keeps it and never runs it.
type Plugin struct {
	Name string `yaml:"name"`
}

// SkillEntry is one skills[] item. BaseDir is the directory of the
// Tariboyfile that declared it; it is not read from YAML.
type SkillEntry struct {
	Dir     string `yaml:"dir,omitempty"`
	Git     string `yaml:"git,omitempty"`
	Path    string `yaml:"path,omitempty"`
	Ref     string `yaml:"ref,omitempty"`
	URL     string `yaml:"url,omitempty"`
	Only    string `yaml:"only,omitempty"`
	BaseDir string `yaml:"-"`
}

// PromptEntry is one prompts[] item. BaseDir is the directory of the
// Tariboyfile that declared it; it is not read from YAML.
type PromptEntry struct {
	File    string `yaml:"file,omitempty"`
	Git     string `yaml:"git,omitempty"`
	Path    string `yaml:"path,omitempty"`
	Ref     string `yaml:"ref,omitempty"`
	URL     string `yaml:"url,omitempty"`
	Runtime string `yaml:"runtime,omitempty"`
	Only    string `yaml:"only,omitempty"`
	BaseDir string `yaml:"-"`
}

// File is a parsed Tariboyfile.
type File struct {
	Dir           string        `yaml:"-"`
	SchemaVersion int           `yaml:"schema_version"`
	ImageVersion  string        `yaml:"image_version,omitempty"`
	Extends       []string      `yaml:"extends,omitempty"`
	Plugins       []Plugin      `yaml:"plugins"`
	Skills        []SkillEntry  `yaml:"skills"`
	Prompts       []PromptEntry `yaml:"prompts"`
}

// Parse reads and validates the Tariboyfile at path, or in path when it is a
// directory.
func Parse(path string) (*File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		path = filepath.Join(path, Filename)
	}
	data, err := fsafe.ReadFileNoFollow(path, maxFileSize)
	if err != nil {
		return nil, err
	}
	out, err := decode(path, data)
	if err != nil {
		return nil, err
	}
	if err := validateVersionField(data); err != nil {
		return nil, err
	}
	if err := out.validate(); err != nil {
		return nil, err
	}
	for i := range out.Skills {
		out.Skills[i].BaseDir = out.Dir
	}
	for i := range out.Prompts {
		out.Prompts[i].BaseDir = out.Dir
	}
	return out, nil
}

func decode(path string, data []byte) (*File, error) {
	var header struct {
		SchemaVersion int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(data, &header); err == nil && header.SchemaVersion == 1 {
		return nil, errors.New("schema 1 image sources are not supported; migrate to schema 2 or 3")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var out File
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple YAML documents are not allowed")
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out.Dir = filepath.Dir(path)
	return &out, nil
}

func (f *File) validate() error {
	if f.SchemaVersion != 2 && f.SchemaVersion != 3 {
		return fmt.Errorf("schema_version must be 2 or 3, got %d", f.SchemaVersion)
	}
	if err := validateExtends(f.Extends); err != nil {
		return err
	}
	if err := validatePlugins(f.Plugins); err != nil {
		return err
	}
	if len(f.Skills) > maxSkillEntries {
		return fmt.Errorf("skills has %d entries, maximum is %d", len(f.Skills), maxSkillEntries)
	}
	if len(f.Prompts) > maxPromptEntries {
		return fmt.Errorf("prompts has %d entries, maximum is %d", len(f.Prompts), maxPromptEntries)
	}
	for i, s := range f.Skills {
		if err := validateSkill(f.SchemaVersion, s); err != nil {
			return fmt.Errorf("skills[%d]: %w", i, err)
		}
	}
	seenRuntime := map[string]bool{}
	for i, p := range f.Prompts {
		if err := validatePrompt(f.SchemaVersion, p, seenRuntime); err != nil {
			return fmt.Errorf("prompts[%d]: %w", i, err)
		}
	}
	return nil
}

func validateExtends(extends []string) error {
	for i, parent := range extends {
		if !strings.HasPrefix(parent, "./") && !strings.HasPrefix(parent, "../") && !filepath.IsAbs(parent) {
			return fmt.Errorf("extends[%d]: path %q must use ./, ../, or an absolute path", i, parent)
		}
	}
	return nil
}

func validatePlugins(plugins []Plugin) error {
	seen := map[string]bool{}
	for i, p := range plugins {
		if p.Name == "" {
			return fmt.Errorf("plugins[%d]: name is required", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("plugins[%d]: duplicate plugin %q", i, p.Name)
		}
		seen[p.Name] = true
	}
	return nil
}

func count(values ...string) int {
	n := 0
	for _, v := range values {
		if v != "" {
			n++
		}
	}
	return n
}

// validateRemote checks the git/path/ref/url/only fields shared by skills and
// prompts. Schema v2 allows none of them.
func validateRemote(schema int, git, path, ref, url, only string) error {
	if schema == 2 {
		if count(git, path, ref, url, only) > 0 {
			return errors.New("git, path, ref, url and only need schema_version 3")
		}
		return nil
	}
	if only != "" && only != onlyTariboy {
		return fmt.Errorf("only must be %q, got %q", onlyTariboy, only)
	}
	if git != "" && path == "" {
		return errors.New("path is required with git")
	}
	if git == "" && (path != "" || ref != "") {
		return errors.New("path and ref are allowed only with git")
	}
	return nil
}

func validateSkill(schema int, s SkillEntry) error {
	if err := validateRemote(schema, s.Git, s.Path, s.Ref, s.URL, s.Only); err != nil {
		return err
	}
	if count(s.Dir, s.Git, s.URL) != 1 {
		return errors.New("exactly one of dir, git or url is required")
	}
	if schema == 3 && strings.HasPrefix(s.Dir, pluginsPrefix) && s.Only != onlyTariboy {
		return fmt.Errorf("dir %q needs only: %s", s.Dir, onlyTariboy)
	}
	return nil
}

func validatePrompt(schema int, p PromptEntry, seenRuntime map[string]bool) error {
	if err := validateRemote(schema, p.Git, p.Path, p.Ref, p.URL, p.Only); err != nil {
		return err
	}
	if count(p.File, p.Git, p.URL, p.Runtime) != 1 {
		return errors.New("exactly one of file, git, url or runtime is required")
	}
	if schema == 3 && strings.HasPrefix(p.File, pluginsPrefix) && p.Only != onlyTariboy {
		return fmt.Errorf("file %q needs only: %s", p.File, onlyTariboy)
	}
	if p.Runtime == "" {
		return nil
	}
	if !IsRuntime(p.Runtime) {
		return fmt.Errorf("unknown runtime placeholder %q", p.Runtime)
	}
	if seenRuntime[p.Runtime] {
		return fmt.Errorf("duplicate runtime placeholder %q", p.Runtime)
	}
	seenRuntime[p.Runtime] = true
	return nil
}

// ForTbox returns a copy without entries marked only: tariboy and without
// runtime prompts. Plugins are emptied.
func (f *File) ForTbox() *File {
	out := *f
	out.Plugins = []Plugin{}
	out.Skills = []SkillEntry{}
	out.Prompts = []PromptEntry{}
	out.Extends = slices.Clone(f.Extends)
	for _, s := range f.Skills {
		if s.Only != onlyTariboy {
			out.Skills = append(out.Skills, s)
		}
	}
	for _, p := range f.Prompts {
		if p.Only != onlyTariboy && p.Runtime == "" {
			out.Prompts = append(out.Prompts, p)
		}
	}
	return &out
}

// Role is the manifest field "role" (same JSON as internal/rolefile).
type Role struct {
	Text string `json:"text"`
}

const (
	roleFileName   = "ROLE.md"
	roleLimitChars = 500
)

// RoleRead reads dir/ROLE.md without following a symlink. A missing file or
// one of white space only returns nil, nil.
func RoleRead(dir string) (*Role, error) {
	body, err := fsafe.ReadFileNoFollow(filepath.Join(dir, roleFileName), maxFileSize)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", roleFileName, err)
	}
	return roleFromBytes(body)
}

func roleFromBytes(body []byte) (*Role, error) {
	text := string(body)
	if !utf8.Valid(body) {
		return nil, errors.New("ROLE.md is not valid UTF-8")
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	if n := len([]rune(text)); n > roleLimitChars {
		return nil, fmt.Errorf("%s is %d characters, more than the limit of %d characters", roleFileName, n, roleLimitChars)
	}
	return &Role{Text: text}, nil
}
