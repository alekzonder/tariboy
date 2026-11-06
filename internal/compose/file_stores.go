package compose

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
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
// or the entry source (default: the entry name) of a declared Store. Tag is
// the tag the build publishes and a queue follows (default latest); Version,
// when set, is the version the source must give.
type SourceSpec struct {
	Path    string `yaml:"path,omitempty"`
	Store   string `yaml:"store,omitempty"`
	Source  string `yaml:"source,omitempty"`
	Tag     string `yaml:"tag,omitempty"`
	Version string `yaml:"version,omitempty"`
}

func (s SourceSpec) declared() bool {
	return s.Path != "" || s.Store != "" || s.Source != "" || s.Tag != "" || s.Version != ""
}

// tagOrLatest is the tag the entry publishes.
func (s SourceSpec) tagOrLatest() string {
	if s.Tag == "" {
		return "latest"
	}
	return s.Tag
}

// workflowVersion is the version a workflow source must give: version, or a
// version tag, since a workflow build publishes only latest and its version.
func (s SourceSpec) workflowVersion() string {
	if s.Version == "" && s.Tag != "latest" {
		return s.Tag
	}
	return s.Version
}

// sourceName is the name of the entry in its Store: source, or the entry name.
func (s SourceSpec) sourceName(entry string) string {
	if s.Source == "" {
		return entry
	}
	return s.Source
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
		if s.Git != "" && !httpsGitURL(s.Git) {
			out = append(out, fmt.Errorf("store %q git must be an https URL without credentials, query or fragment", name))
		}
	}
	return out
}

// httpsGitURL reports whether s is https://HOST/PATH without user info, query
// or fragment, the only git source compose registers.
func httpsGitURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil &&
		strings.Trim(u.Path, "/") != "" && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// sourceProblems checks the source of the workflow or image entry name.
func (f File) sourceProblems(kind, name string, s SourceSpec) []error {
	var out []error
	if (s.Path == "") == (s.Store == "") {
		out = append(out, fmt.Errorf("%s %q needs exactly one source: path or store", kind, name))
	}
	if s.Version != "" {
		if _, err := version.ParseSemVer(s.Version); err != nil {
			out = append(out, fmt.Errorf("%s %q version: %w", kind, name, err))
		}
	}
	if err := tagProblem(kind, name, s.Tag); err != nil {
		out = append(out, err)
	}
	if s.Store == "" {
		if s.Source != "" {
			out = append(out, fmt.Errorf("%s %q source needs store", kind, name))
		}
		return out
	}
	if _, ok := f.Stores[s.Store]; !ok {
		out = append(out, fmt.Errorf("%s %q references undeclared store %q", kind, name, s.Store))
	}
	return out
}

// tagProblem checks an entry tag. A workflow build publishes only latest and
// its version, so a workflow tag is one of those; an image tag is any image
// tag but "." and "..".
func tagProblem(kind, name, tag string) error {
	if tag == "" {
		return nil
	}
	if kind == "workflow" {
		if _, err := version.ParseSemVer(tag); err != nil && tag != "latest" {
			return fmt.Errorf("workflow %q tag %q: want latest or a version", name, tag)
		}
		return nil
	}
	if ref, err := image.ParseRef("x:" + tag); err != nil || ref.Tag != tag || tag == "." || tag == ".." {
		return fmt.Errorf("%s %q invalid tag %q", kind, name, tag)
	}
	return nil
}
