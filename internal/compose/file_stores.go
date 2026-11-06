package compose

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/stores"
	"github.com/alekzonder/tariboy/internal/version"
)

// StoreSpec is a stores entry of a version 2 file: a git repository at an
// optional ref, or a local path.
type StoreSpec struct {
	Git  string `yaml:"git,omitempty"`
	Ref  string `yaml:"ref,omitempty"`
	Path string `yaml:"path,omitempty"`
}

// SourceSpec is where a version 2 workflow or image comes from: a local path,
// or the entry source (default: the entry name) of a declared Store at an
// optional version.
type SourceSpec struct {
	Path    string `yaml:"path,omitempty"`
	Store   string `yaml:"store,omitempty"`
	Source  string `yaml:"source,omitempty"`
	Version string `yaml:"version,omitempty"`
}

func (s SourceSpec) declared() bool {
	return s.Path != "" || s.Store != "" || s.Source != "" || s.Version != ""
}

func (f File) storeProblems() []error {
	if len(f.Stores) == 0 {
		return nil
	}
	if f.Version == 1 {
		return []error{fmt.Errorf("the stores section needs version: 2")}
	}
	var out []error
	for _, name := range sortedKeys(f.Stores) {
		s := f.Stores[name]
		if !stores.ValidName(name) {
			out = append(out, fmt.Errorf("invalid store name %q", name))
			continue
		}
		if (s.Git == "") == (s.Path == "") {
			out = append(out, fmt.Errorf("store %q needs exactly one source: git or path", name))
		}
		if s.Ref != "" && s.Git == "" {
			out = append(out, fmt.Errorf("store %q ref needs git", name))
		}
	}
	return out
}

// sourceProblems checks the source of the workflow or image entry name.
func (f File) sourceProblems(kind, name string, s SourceSpec) []error {
	var out []error
	if (s.Path == "") == (s.Store == "") {
		out = append(out, fmt.Errorf("%s %q needs exactly one source: path or store", kind, name))
	}
	if s.Store == "" {
		if s.Source != "" {
			out = append(out, fmt.Errorf("%s %q source needs store", kind, name))
		}
		if s.Version != "" {
			out = append(out, fmt.Errorf("%s %q version needs store", kind, name))
		}
		return out
	}
	if s.Version != "" {
		if _, err := version.ParseSemVer(s.Version); err != nil {
			out = append(out, fmt.Errorf("%s %q version: %w", kind, name, err))
		}
	}
	if _, ok := f.Stores[s.Store]; !ok {
		out = append(out, fmt.Errorf("%s %q references undeclared store %q", kind, name, s.Store))
	}
	return out
}
